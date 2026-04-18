package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/auth"
	authstorage "github.com/Abraxas-365/claudio/internal/auth/storage"
	"github.com/Abraxas-365/claudio/internal/comandcenter"
	"github.com/Abraxas-365/claudio/internal/comandcenter/web"
	"github.com/Abraxas-365/claudio/internal/tasks"
)

func main() {
	password   := flag.String("password", "", "shared secret for bearer auth (required)")
	port       := flag.Int("port", 8080, "HTTP port to listen on")
	dbPath     := flag.String("db", "", "path to SQLite database (default: ~/.claudio/comandcenter.db)")
	dataDir    := flag.String("data-dir", "", "path to uploads directory (default: ~/.claudio/uploads/)")
	configPath := flag.String("config", "", "path to cc-config.json (default: ~/.claudio/cc-config.json)")
	flag.Parse()

	if *password == "" {
		fmt.Fprintln(os.Stderr, "error: --password is required")
		os.Exit(1)
	}

	// Resolve defaults relative to ~/.claudio/.
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("cannot determine home dir: %v", err)
	}
	claudioDir := filepath.Join(home, ".claudio")

	if *dbPath == "" {
		*dbPath = filepath.Join(claudioDir, "comandcenter.db")
	}
	if *dataDir == "" {
		*dataDir = filepath.Join(claudioDir, "uploads")
	}
	if *configPath == "" {
		*configPath = defaultCCConfigPath()
	}

	// Ensure directories exist.
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		log.Fatalf("create db dir: %v", err)
	}
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	storage, err := comandcenter.Open(*dbPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer storage.Close()

	hub := comandcenter.NewHub(storage)

	// Cron runner — fires inline crons by sending user messages to attached sessions via hub.
	cronPath := filepath.Join(claudioDir, "cron.json")
	cronStore := tasks.NewCronStore(cronPath)
	if err := cronStore.Load(); err != nil {
		log.Printf("warning: load cron store: %v", err)
	}
	cronRunner := tasks.NewCronRunner(cronStore)
	cronRunner.InjectFn = func(sessionID, prompt string) {
		payload := attach.UserMsgPayload{Content: prompt}
		env, err := attach.NewEnvelope(attach.EventMsgUser, payload)
		if err != nil {
			log.Printf("[cron] inject envelope build: %v", err)
			return
		}
		if err := hub.Send(sessionID, env); err != nil {
			log.Printf("[cron] inject send to session %s: %v", sessionID, err)
		}
	}
	cronRunner.StoreFn = func(sessionID, agentName, content string) {
		msg := comandcenter.Message{
			ID:        genID(),
			SessionID: sessionID,
			Role:      "assistant",
			Content:   content,
			AgentName: agentName,
			CreatedAt: time.Now(),
		}
		if err := storage.InsertMessage(msg); err != nil {
			log.Printf("[cron] store msg for session %s: %v", sessionID, err)
			return
		}
		env, err := attach.NewEnvelope(attach.EventMsgAssistant, attach.AssistantMsgPayload{
			Content:   content,
			AgentName: agentName,
		})
		if err != nil {
			log.Printf("[cron] broadcast envelope build: %v", err)
			return
		}
		hub.Broadcast(sessionID, env)
	}
	// Shared API client — used for /compact in web UI and background cron execution.
	sharedAPIClient := func() *api.Client {
		store := authstorage.NewDefaultStorage()
		resolver := auth.NewResolver(store)
		return api.NewClient(resolver, api.WithPromptCaching(false))
	}()

	// Wire background execution via auth resolver (keychain + OAuth + env).
	{
		apiClient := sharedAPIClient
		cronRunner.RunBackgroundFn = func(ctx context.Context, modelID, systemPrompt, prompt string) (string, error) {
			contentJSON, _ := json.Marshal([]map[string]string{{"type": "text", "text": prompt}})
			req := &api.MessagesRequest{
				Model:     modelID,
				MaxTokens: 8192,
				System:    systemPrompt,
				Messages: []api.Message{
					{Role: "user", Content: json.RawMessage(contentJSON)},
				},
			}
			resp, err := apiClient.SendMessage(ctx, req)
			if err != nil {
				return "", fmt.Errorf("background cron API call: %w", err)
			}
			var parts []string
			for _, block := range resp.Content {
				if block.Type == "text" && block.Text != "" {
					parts = append(parts, block.Text)
				}
			}
			return strings.Join(parts, "\n"), nil
		}
	}

	cronCtx, cronCancel := context.WithCancel(context.Background())
	cronRunner.Start(cronCtx)

	srv := comandcenter.NewServer(*password, storage, hub, *dataDir)

	// Mount browser UI (WhatsApp-style chat interface).
	webSrv := web.NewWebServer(storage, hub, *password, *dataDir)
	webSrv.SetCronStore(cronStore)
	webSrv.SetAPIClient(sharedAPIClient)
	webSrv.SetTeamTemplatesDir(filepath.Join(claudioDir, "team-templates"))
	if pk, _, err := storage.GetOrCreateVAPIDKeys(); err == nil && pk != "" {
		webSrv.SetVAPIDPublicKey(pk)
	}
	webSrv.RegisterRoutes(srv.Mux())

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	// Load session config and build launcher.
	ccCfg, err := LoadCCConfig(*configPath)
	if err != nil {
		log.Fatalf("load cc-config: %v", err)
	}
	serverURL := fmt.Sprintf("http://localhost:%d", *port)
	launcher := NewSessionLauncher(ccCfg, serverURL, *password)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ComandCenter listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Give the HTTP server a moment to bind before spawning attach sessions.
	if len(ccCfg.Sessions) > 0 {
		time.Sleep(500 * time.Millisecond)
		log.Printf("launching %d managed session(s)", len(ccCfg.Sessions))
		launcher.Start()
	}

	<-quit
	log.Println("shutting down")
	launcher.Stop()
	cronCancel()
	cronRunner.Stop()
	if err := httpSrv.Close(); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func genID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
