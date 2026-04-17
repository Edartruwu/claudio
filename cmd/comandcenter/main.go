package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Abraxas-365/claudio/internal/comandcenter"
)

func main() {
	password := flag.String("password", "", "shared secret for bearer auth (required)")
	port := flag.Int("port", 8080, "HTTP port to listen on")
	dbPath := flag.String("db", "", "path to SQLite database (default: ~/.claudio/comandcenter.db)")
	dataDir := flag.String("data-dir", "", "path to uploads directory (default: ~/.claudio/uploads/)")
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
	srv := comandcenter.NewServer(*password, storage, hub, *dataDir)

	addr := fmt.Sprintf(":%d", *port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ComandCenter listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down")
	if err := httpSrv.Close(); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
