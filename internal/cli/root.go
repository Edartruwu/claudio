package cli

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Abraxas-365/claudio/internal/agents"
	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/app"
	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/auth/refresh"
	"github.com/Abraxas-365/claudio/internal/bus"
	"github.com/Abraxas-365/claudio/internal/cli/attachclient"
	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/plugins"
	"github.com/Abraxas-365/claudio/internal/prompts"
	"github.com/Abraxas-365/claudio/internal/query"
	"github.com/Abraxas-365/claudio/internal/rules"
	"github.com/Abraxas-365/claudio/internal/session"
	"github.com/Abraxas-365/claudio/internal/services/skills"
	"github.com/Abraxas-365/claudio/internal/snippets"
	"github.com/Abraxas-365/claudio/internal/teams"
	"github.com/Abraxas-365/claudio/internal/tools"
	"github.com/Abraxas-365/claudio/internal/tui"
	"github.com/Abraxas-365/claudio/internal/tui/agentselector"
	"github.com/Abraxas-365/claudio/internal/tui/prompt"
	"github.com/Abraxas-365/claudio/internal/tui/teamselector"
	"github.com/Abraxas-365/claudio/internal/utils"
)

var (
	flagModel               string
	flagAgent               string
	flagTeam                string
	flagVerbose             bool
	flagHeadless            bool
	flagContext             string
	flagBudget              float64
	flagResume              string
	flagDangerouslySkipPerm bool
	flagPrint               bool
	flagAttach              string
	flagName                string
	flagMaster              bool
)

// appInstance is initialized before command execution.
var appInstance *app.App

// attachClient holds the ComandCenter attach client (set by --attach flag).
var attachClient *attachclient.Client

var rootCmd = &cobra.Command{
	Use:   "claudio [prompt]",
	Short: "Claudio your AI pair programmer in the terminal",
	Long: `Claudio is an open-source AI coding assistant that lives in your terminal.

It understands your codebase, executes tools, manages sessions, and helps you
ship faster — all without leaving the command line. Built in pure Go for speed,
security, and hackability.`,
	Args: cobra.ArbitraryArgs,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip app init for commands that don't need it
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}

		cwd, _ := os.Getwd()

		// Find git root for consistent project identity
		projectRoot := config.FindGitRoot(cwd)

		// Trust check: if project has config, verify it's trusted
		if config.HasProjectConfig(projectRoot) {
			ts := config.NewTrustStore()
			if !ts.IsTrusted(projectRoot) {
				info := config.ScanProjectConfig(projectRoot)
				// Only prompt if there are security-relevant configs (hooks, MCP)
				if info.HasHooks || info.HasMCP {
					fmt.Fprint(os.Stderr, config.FormatTrustPrompt(projectRoot, info))
					var answer string
					fmt.Scanln(&answer)
					if strings.ToLower(strings.TrimSpace(answer)) != "y" {
						return fmt.Errorf("project not trusted — skipping project configuration")
					}
				}
				// Mark as trusted
				ts.Trust(projectRoot, info.HasHooks, info.HasMCP, info.HasRules)
			}
		}

		settings, err := config.Load(projectRoot)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[PERM-DEBUG] projectRoot=%q permissionRules=%d\n", projectRoot, len(settings.PermissionRules))
		for i, r := range settings.PermissionRules {
			fmt.Fprintf(os.Stderr, "[PERM-DEBUG]   rule[%d] tool=%q pattern=%q behavior=%q\n", i, r.Tool, r.Pattern, r.Behavior)
		}

		// Apply CLI flag overrides
		if flagModel != "" {
			settings.Model = flagModel
		}
		if flagBudget > 0 {
			settings.MaxBudget = flagBudget
		}
		if flagDangerouslySkipPerm {
			settings.PermissionMode = "dangerously-skip-permissions"
			fmt.Fprintln(os.Stderr, "\033[33m⚠ WARNING: All permission checks are disabled. Tools will execute without approval.\033[0m")
		}

		a, err := app.New(settings, projectRoot)
		if err != nil {
			return fmt.Errorf("failed to initialize: %w", err)
		}
		appInstance = a

		// Force headless mode when --attach is set
		if flagAttach != "" {
			flagHeadless = true
		}

		// Setup attach client if --attach flag set
		if flagAttach != "" {
			password := os.Getenv("COMANDCENTER_PASSWORD")
			if password == "" {
				log.Printf("Warning: COMANDCENTER_PASSWORD env var not set, attaching without auth\n")
			}

			client := attachclient.New(flagAttach, password, flagName, flagMaster, flagAgent, flagTeam)
			if err := client.Connect(context.Background()); err != nil {
				log.Printf("Warning: failed to attach to ComandCenter: %v\n", err)
			} else {
				attachClient = client
				
				// Inject attach client into session coordination tools
				appInstance.InjectAttachClient(attachClient, flagAttach)

				// Subscribe to bus events and forward task/agent events to ComandCenter
				appInstance.Bus.SubscribeAll(func(event bus.Event) {
					switch event.Type {
					case attach.EventTaskCreated:
						var p attach.TaskCreatedPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventTaskCreated, p)
						}
					case attach.EventTaskUpdated:
						var p attach.TaskUpdatedPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventTaskUpdated, p)
						}
					case attach.EventAgentStatus:
						var p attach.AgentStatusPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventAgentStatus, p)
						}
					case attach.EventClearHistory:
						var p attach.ClearHistoryPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventClearHistory, p)
						}
					case attach.EventConfigChanged:
						var p attach.ConfigChangedPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventConfigChanged, p)
						}
					case attach.EventAgentChanged:
						var p attach.AgentChangedPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventAgentChanged, p)
						}
					case attach.EventTeamChanged:
						var p attach.TeamChangedPayload
						if err := json.Unmarshal(event.Payload, &p); err == nil {
							_ = attachClient.SendEvent(attach.EventTeamChanged, p)
						}
					}
				})
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Headless+attach mode: run engine in loop, inject messages from PWA
		if flagHeadless && flagAttach != "" && attachClient != nil {
			return runHeadlessAttach(args)
		}

		// Detect pipe mode: if stdout is not a terminal, use print mode
		isPiped := !isTerminal()
		if isPiped || flagPrint {
			if len(args) == 0 {
				// Read prompt from stdin when piped
				scanner := bufio.NewScanner(os.Stdin)
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				if len(lines) == 0 {
					return fmt.Errorf("no prompt provided")
				}
				return runSinglePrompt(strings.Join(lines, "\n"))
			}
			return runSinglePrompt(strings.Join(args, " "))
		}

		if len(args) > 0 {
			return runSinglePrompt(strings.Join(args, " "))
		}
		return runInteractive()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "", "AI model to use (e.g., claude-opus-4-6)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagHeadless, "headless", false, "Run in headless server mode (no TUI)")
	rootCmd.PersistentFlags().StringVar(&flagContext, "context", "", "Load context profile (dev, review, research, or path)")
	rootCmd.PersistentFlags().Float64Var(&flagBudget, "budget", 0, "Session budget limit in USD (0 = unlimited)")
	rootCmd.PersistentFlags().StringVarP(&flagResume, "resume", "r", "", "Resume a previous session (no value = most recent)")
	rootCmd.PersistentFlags().Lookup("resume").NoOptDefVal = "last"
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "dangerously-skip-permissions", false, "Skip all permission prompts (use with caution)")
	rootCmd.PersistentFlags().BoolVar(&flagDangerouslySkipPerm, "yolo", false, "Alias for --dangerously-skip-permissions")
	rootCmd.PersistentFlags().BoolVar(&flagPrint, "print", false, "Print-only mode (no TUI, clean stdout for piping)")
	rootCmd.PersistentFlags().StringVar(&flagAgent, "agent", "", "Run as a specific agent persona (e.g., prab, backend-senior)")
	rootCmd.PersistentFlags().StringVar(&flagTeam, "team", "", "Pre-load a team template at startup (e.g., backend-team)")
	rootCmd.PersistentFlags().StringVar(&flagAttach, "attach", "", "ComandCenter server URL (e.g. http://localhost:8080)")
	rootCmd.PersistentFlags().StringVar(&flagName, "name", "", "Session display name in ComandCenter")
	rootCmd.PersistentFlags().BoolVar(&flagMaster, "master", false, "Mark this session as the master session")
}

func Execute() error {
	return rootCmd.Execute()
}

func runSinglePrompt(prompt string) error {
	return runSinglePromptWithCtx(context.Background(), prompt)
}

func runSinglePromptWithCtx(parent context.Context, prompt string) error {
	if !appInstance.Auth.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run: claudio auth login")
	}

	// Proactive token refresh
	if _, err := refresh.CheckAndRefreshIfNeeded(appInstance.Storage, false); err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", err)
		}
	}

	ctx, cancel := signal.NotifyContext(parent, os.Interrupt)
	defer cancel()
	defer appInstance.Close()
	// attachClient.Close() is intentionally NOT deferred here.
	// In headless+attach mode runSinglePrompt is called in a loop — the WS
	// connection must stay open across turns. runHeadlessAttach owns the close.

	reg, modelOverride, extraPluginInfos := applyAgentOverrides(appInstance.Tools)
	if modelOverride != "" {
		appInstance.Config.Model = modelOverride
		appInstance.API.SetModel(modelOverride)
	}

	// Inject advisor tool for the principal agent when configured.
	// principalEngine is captured by the closure so GetMessages works once
	// the engine is assigned below.
	var principalEngine *query.Engine
	if appInstance.Config.Advisor != nil {
		advisorSystemPrompt, advisorModel := buildAdvisorConfig(appInstance.Config.Advisor)
		count := 0
		advisorTool := tools.NewAdvisorTool(tools.AdvisorToolConfig{
			Definition: agents.AgentDefinition{SystemPrompt: advisorSystemPrompt},
			Model:      advisorModel,
			MaxUses:    appInstance.Config.Advisor.MaxUses,
			UsedCount:  &count,
			GetMessages: func() []api.Message {
				if principalEngine == nil {
					return nil
				}
				return principalEngine.Messages()
			},
			Client: appInstance.API,
		})
		reg.Register(advisorTool)
	}

	var handler query.EventHandler = &query.StdoutHandler{Verbose: flagVerbose}
	if attachClient != nil {
		handler = attachclient.NewEventProxy(handler, attachClient)
	}
	singleTurnCfg := query.EngineConfig{
		Hooks:           appInstance.Hooks,
		Analytics:       appInstance.Analytics,
		TaskRuntime:     appInstance.TaskRuntime,
		Model:           appInstance.Config.Model,
		PermissionMode:  appInstance.Config.PermissionMode,
		PermissionRules: appInstance.Config.PermissionRules,
		OnTurnEnd:       appInstance.MemoryExtractor(),
	}
	if appInstance.Config.CavemanEnabled() {
		if c := skills.BundledSkillContent("caveman"); c != "" {
			singleTurnCfg.CavemanMsg = "**CAVEMAN ULTRA MODE ACTIVE — respond in caveman ultra for the entire session. Active for all agents and sub-agents. Only the human user can disable with \"stop caveman\" or \"normal mode\".**\n\n" + c + "\n\nLevel: ultra.\n\n**EXCEPTION — structured protocol output:** Always use exact format for `### Done` completion reports (exact header, all required bullet fields). Caveman style inside the fields is fine. Never skip or rename the header."
		}
	}
	engine := query.NewEngineWithConfig(appInstance.API, reg, handler, singleTurnCfg)
	principalEngine = engine // allow GetMessages closure to resolve

	sys := buildFullSystemPrompt()
	if section := prompts.PluginsSection(extraPluginInfos); section != "" {
		sys += "\n\n" + section
	}
	engine.SetSystem(sys)
	engine.SetUserContext(prompts.FormatUserContextMessage(buildUserContext(), ""))
	engine.SetSystemContext(buildSystemContext())

	err := engine.Run(ctx, prompt)

	// Print cost summary to stderr
	printCostSummary()

	return err
}

// buildAdvisorConfig resolves the system prompt and model for an AdvisorSettings block.
// Rule: always append AdvisorSystemPrompt(); prepend the subagent's own prompt when set.
// Model precedence: AdvisorSettings.Model > subagent.Model > appInstance.Config.Model.
// runHeadlessAttach runs the engine in headless mode, processing messages from the inject channel.
// Used when --attach is set: engine loop reads from app.InjectCh and runs each message as a turn.
// Wires attachClient.OnUserMessage to app.InjectMessage.
func runHeadlessAttach(args []string) error {
	if !appInstance.Auth.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run: claudio auth login")
	}

	// Proactive token refresh
	if _, err := refresh.CheckAndRefreshIfNeeded(appInstance.Storage, false); err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	defer appInstance.Close()
	defer attachClient.Close()

	// Wire PWA user messages → app.InjectMessage (not program.Send, which is TUI-only)
	attachClient.OnUserMessage(func(payload attach.UserMsgPayload) {
		appInstance.InjectPayload(payload)
	})

	// Wire interrupt signal from ComandCenter → app.Interrupt
	attachClient.OnInterrupt(func() {
		appInstance.Interrupt()
	})

	// --- Session setup: find existing session by name or create new one ---
	sess := session.New(appInstance.DB)
	projectDir, _ := os.Getwd()
	if flagName != "" {
		if existing, err := sess.FindByTitle(flagName, projectDir); err == nil && existing != nil {
			if _, err := sess.Resume(existing.ID); err != nil && flagVerbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to resume session %q: %v\n", flagName, err)
			}
		} else {
			if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
			}
			_ = sess.SetTitle(flagName)
		}
	} else {
		if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
		}
	}

	// DB-stored values win over CLI flags — preserves manual web UI changes.
	// CLI flags (from cc-config.json) only apply on first creation when DB has nothing.
	if cur := sess.Current(); cur != nil {
		if cur.AgentType != "" {
			flagAgent = cur.AgentType
		}
		if cur.TeamTemplate != "" {
			flagTeam = cur.TeamTemplate
		}
	}

	// --- Gap 2: Auto-instantiate team from --team flag in headless mode ---
	var headlessTeamTemplate *teams.TeamTemplate
	if flagTeam != "" {
		templatesDir := config.GetPaths().TeamTemplates
		if tmpl, err := teams.GetTemplate(templatesDir, flagTeam); err == nil {
			headlessTeamTemplate = tmpl
			sessionID := ""
			if cur := sess.Current(); cur != nil {
				sessionID = cur.ID
			}
			instantiateTeamDirect(tmpl, sessionID)
			_ = appInstance.DB.UpdateSessionTeamTemplate(sessionID, flagTeam)
		} else {
			fmt.Fprintf(os.Stderr, "warning: team template %q not found: %v\n", flagTeam, err)
		}
	}

	// --- Gap 1: Wire OnSetTeam for dynamic team selection in headless mode ---
	attachClient.OnSetTeam(func(payload attach.SetTeamPayload) {
		templatesDir := config.GetPaths().TeamTemplates
		tmpl, err := teams.GetTemplate(templatesDir, payload.TeamName)
		if err != nil {
			log.Printf("set_team: team template %q not found", payload.TeamName)
			return
		}
		sessionID := ""
		if cur := sess.Current(); cur != nil {
			sessionID = cur.ID
		}
		instantiateTeamDirect(tmpl, sessionID)
	})

	// --- Load history from DB into engine ---
	var initialMsgs []api.Message
	if storedMsgs, err := sess.GetMessages(); err == nil && len(storedMsgs) > 0 {
		initialMsgs = session.ReconstructEngineMessages(storedMsgs)
	}

	// --- Build ONE persistent engine for the session lifetime ---
	reg, modelOverride, extraPluginInfos := applyAgentOverrides(appInstance.Tools)

	// Wire capability-gated tools (e.g. design tools) for headless+attach mode.
	// In TUI mode this happens in applyAgentPersona; here we do it once at startup.
	// Always call RegisterCapabilityTools — ungated tools (e.g. ReviewDesignFidelity)
	// must be available to all agents regardless of capabilities.
	{
		var caps []string
		if flagAgent != "" {
			caps = agents.GetAgent(flagAgent).Capabilities
		}
		pusher := attachclient.NewAttachScreenshotPusher(attachClient)
		tools.RegisterCapabilityTools(reg, caps, appInstance.API, pusher, sess.Current().ID, appInstance.Config)
	}
	if modelOverride != "" {
		appInstance.Config.Model = modelOverride
		appInstance.API.SetModel(modelOverride)
	}

	var handler query.EventHandler = &query.StdoutHandler{Verbose: flagVerbose}
	if attachClient != nil {
		handler = attachclient.NewEventProxy(handler, attachClient)
	}

	// Capture session ID for bus filtering and runner stamping.
	currentSessionID := ""
	if cur := sess.Current(); cur != nil {
		currentSessionID = cur.ID
	}
	appInstance.TeamRunner.SetSessionID(currentSessionID)

	engineCfg := query.EngineConfig{
		Hooks:           appInstance.Hooks,
		Analytics:       appInstance.Analytics,
		TaskRuntime:     appInstance.TaskRuntime,
		SessionID:       currentSessionID,
		Model:           appInstance.Config.Model,
		PermissionMode:  appInstance.Config.PermissionMode,
		PermissionRules: appInstance.Config.PermissionRules,
		OnTurnEnd:       appInstance.MemoryExtractor(),
		OnAutoCompact: func(msgs []api.Message, summary string) {
			_ = sess.PersistCompacted(msgs)
			_ = sess.SaveSummary(summary)
		},
	}
	if appInstance.Config.CavemanEnabled() {
		if c := skills.BundledSkillContent("caveman"); c != "" {
			engineCfg.CavemanMsg = "**CAVEMAN ULTRA MODE ACTIVE — respond in caveman ultra for the entire session. Active for all agents and sub-agents. Only the human user can disable with \"stop caveman\" or \"normal mode\".**\n\n" + c + "\n\nLevel: ultra.\n\n**EXCEPTION — structured protocol output:** Always use exact format for `### Done` completion reports (exact header, all required bullet fields). Caveman style inside the fields is fine. Never skip or rename the header."
		}
	}

	engine := query.NewEngineWithConfig(appInstance.API, reg, handler, engineCfg)

	// Wire clear history from ComandCenter → wipe DB + engine in-memory messages.
	// Must be registered after engine is created so the closure can capture it.
	attachClient.OnClearHistory(func() {
		appInstance.ClearHistory(currentSessionID)
		engine.SetMessages(nil)
	})

	sys := buildFullSystemPrompt()
	if headlessTeamTemplate != nil {
		sys += "\n\n" + buildHeadlessTeamContextBlock(headlessTeamTemplate)
		sys += "\n\nWhen all team work is complete, call PurgeTeammates to clean up agent worktrees and remove completed/failed agents."
	}
	if section := prompts.PluginsSection(extraPluginInfos); section != "" {
		sys += "\n\n" + section
	}
	engine.SetSystem(sys)
	engine.SetUserContext(prompts.FormatUserContextMessage(buildUserContext(), ""))
	engine.SetSystemContext(buildSystemContext())
	if len(initialMsgs) > 0 {
		engine.SetMessages(initialMsgs)
	}

	// If initial prompt provided as arg, inject it first
	if len(args) > 0 {
		appInstance.InjectPayload(attach.UserMsgPayload{Content: strings.Join(args, " ")})
	}

	// Subscribe to teammate completion events and inject them into the engine's message loop.
	// This wakes the engine immediately when a teammate finishes — no human input needed.
	// A buffered relay goroutine ensures no notification is dropped if the engine is mid-turn.
	notifQueue := make(chan string, 32)
	go func() {
		for {
			select {
			case msg := <-notifQueue:
				select {
				case appInstance.InjectCh <- attach.UserMsgPayload{Content: msg}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	unsubTeammate := appInstance.Bus.Subscribe(attach.EventAgentStatus, func(e bus.Event) {
		if e.SessionID != currentSessionID {
			return // ignore events from other sessions
		}
		var p attach.AgentStatusPayload
		_ = json.Unmarshal(e.Payload, &p)
		if p.Status != "done" && p.Status != "failed" {
			return // only act on terminal states
		}
		if p.ParentAgentID != "" {
			return // grandchild — reports to its parent teammate, not main engine
		}
		msg := fmt.Sprintf("[System: teammate '%s' %s.\nResult: %s\nContinue — check TaskList for next steps.]", p.Name, p.Status, p.Result)
		select {
		case notifQueue <- msg:
		default:
			// notifQueue full (32 cap); drop — extremely unlikely under normal load
		}
	})
	defer unsubTeammate()

	// --- Message loop: one persistent engine across all turns ---
	for {
		select {
		case payload := <-appInstance.InjectCh:
			var prevModel string
			if payload.ModelOverride != "" {
				prevModel = appInstance.API.GetModel()
				appInstance.API.SetModel(payload.ModelOverride)
			}
			turnCtx, turnCancel := context.WithCancel(ctx)
			engineDone := make(chan struct{})
			// Monitor goroutine: cancel the turn context if an interrupt arrives.
			go func() {
				select {
				case <-appInstance.InterruptCh:
					turnCancel()
				case <-engineDone:
					// engine finished normally; nothing to do
				}
			}()
			var err error
			if len(payload.Attachments) > 0 {
				images := make([]api.UserContentBlock, 0, len(payload.Attachments))
				for _, att := range payload.Attachments {
					data, readErr := os.ReadFile(att.FilePath)
					if readErr != nil {
						continue
					}
					images = append(images, api.NewImageBlock(att.MimeType, base64.StdEncoding.EncodeToString(data)))
				}
				err = engine.RunWithImages(turnCtx, payload.Content, images)
			} else {
				err = engine.Run(turnCtx, payload.Content)
			}
			close(engineDone) // signal monitor goroutine to exit
			turnCancel()      // no-op if already cancelled; ensures context cleanup
			// Restore model if it was overridden for this turn.
			if prevModel != "" {
				appInstance.API.SetModel(prevModel)
			}
			// Drain any stale interrupt signal so the next turn starts clean.
			select {
			case <-appInstance.InterruptCh:
			default:
			}
			// Persist full conversation state after each turn.
			if msgs := engine.Messages(); len(msgs) > 0 {
				_ = sess.PersistCompacted(msgs)
			}
			if err != nil {
				log.Printf("attach: prompt error: %v\n", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// instantiateTeamDirect creates a team from a template and sets it as the active team.
// Guard: if a team is already active, this is a no-op (returns empty string).
// Returns the team name created, or "" if guard fired.
func instantiateTeamDirect(tmpl *teams.TeamTemplate, sessionID string) string {
	if appInstance.TeamRunner.ActiveTeamName() != "" {
		return "" // already active — skip double-create
	}
	sfx := sessionID
	if len(sfx) > 8 {
		sfx = sfx[:8]
	}
	teamName := tmpl.Name
	if sfx != "" {
		teamName = tmpl.Name + "-" + sfx
	}
	if _, err := appInstance.Teams.CreateTeam(teamName, tmpl.Description, sessionID, tmpl.Model); err != nil {
		_ = err // team may already exist; proceed anyway
	}
	for _, mem := range tmpl.Members {
		model := mem.Model
		if model == "" {
			model = tmpl.Model
		}
		_, _ = appInstance.Teams.AddMember(teamName, mem.Name, model, "", mem.SubagentType)
	}
	appInstance.TeamRunner.SetActiveTeam(teamName)
	log.Printf("team: instantiated %q from template %q", teamName, tmpl.Name)
	return teamName
}

// buildHeadlessTeamContextBlock builds the system prompt block for team context in headless mode.
// This mirrors the logic from TUI's ApplyTeamContextAtStartup but without BubbleTea dependencies.
func buildHeadlessTeamContextBlock(tmpl *teams.TeamTemplate) string {
	var memberLines []string
	for _, mem := range tmpl.Members {
		line := fmt.Sprintf("  - %s (%s)", mem.Name, mem.SubagentType)
		if mem.Model != "" {
			line += " model=" + mem.Model
		}
		memberLines = append(memberLines, line)
	}
	roster := strings.Join(memberLines, "\n")
	desc := ""
	if tmpl.Description != "" {
		desc = "\nDescription: " + tmpl.Description
	}
	return fmt.Sprintf(`## Active Team: %s%s
Members:
%s

The team is ready. Use SpawnTeammate to assign tasks to each member.`, tmpl.Name, desc, roster)
}

func buildAdvisorConfig(cfg *config.AdvisorSettings) (systemPrompt string, model string) {
	if cfg.SubagentType != "" {
		agentDef := agents.GetAgent(cfg.SubagentType)
		systemPrompt = agentDef.SystemPrompt + "\n\n" + prompts.AdvisorSystemPrompt()
		model = cfg.Model
		if model == "" {
			model = agentDef.Model
		}
	} else {
		systemPrompt = prompts.AdvisorSystemPrompt()
		model = cfg.Model
	}
	if model == "" {
		model = appInstance.Config.Model
	}
	return systemPrompt, model
}

func loadContextProfile(name string) (string, error) {
	paths := config.GetPaths()

	var path string
	switch name {
	case "dev", "review", "research":
		path = paths.Contexts + "/" + name + ".md"
	default:
		path = name
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not load context profile %q: %w", name, err)
	}
	return string(data), nil
}

// applyAgentOverrides clones the registry filtered by the --agent flag's DisallowedTools,
// and returns the model override string ("" if no agent or no model override) plus any
// extra plugin infos that should be appended to the system prompt.
func applyAgentOverrides(registry *tools.Registry) (*tools.Registry, string, []prompts.PluginInfo) {
	if flagAgent == "" {
		return registry, "", nil
	}
	agentDef := agents.GetAgent(flagAgent)
	filtered := registry.Clone()
	for _, name := range agentDef.DisallowedTools {
		filtered.Remove(name)
	}

	// Merge per-agent extra skills (additive — global skills remain available)
	if agentDef.ExtraSkillsDir != "" {
		if skillToolRaw, err := filtered.Get("Skill"); err == nil {
			if st, ok := skillToolRaw.(*tools.SkillTool); ok {
				// Clone the existing skills registry so we don't mutate the global one
				mergedReg := skills.NewRegistry()
				for _, s := range st.SkillsRegistry.All() {
					mergedReg.Register(s)
				}
				// Load extra skills from the agent's skills dir and merge in
				extraReg := skills.LoadAll("", agentDef.ExtraSkillsDir)
				for _, s := range extraReg.All() {
					mergedReg.Register(s)
				}
				// Replace the SkillTool with a fresh instance using the merged registry
				filtered.Remove("Skill")
				filtered.Register(&tools.SkillTool{
						SkillsRegistry: mergedReg,
						HooksManager:   st.HooksManager,
						ProjectRoot:    st.ProjectRoot,
						ExcludedNames:  st.ExcludedNames,
					})
			}
		}
	}

	// Register per-agent extra plugins (additive)
	var extraPluginInfos []prompts.PluginInfo
	if agentDef.ExtraPluginsDir != "" {
		extraPluginReg := plugins.NewRegistry()
		extraPluginReg.LoadDir(agentDef.ExtraPluginsDir)
		// Mirror OutputFilterEnabled from existing proxy tools in the registry
		outputFilterEnabled := false
		for _, t := range filtered.All() {
			if pt, ok := t.(*plugins.PluginProxyTool); ok {
				outputFilterEnabled = pt.OutputFilterEnabled
				break
			}
		}
		for _, p := range extraPluginReg.All() {
			pt := plugins.NewProxyTool(p)
			pt.OutputFilterEnabled = outputFilterEnabled
			filtered.Register(pt)
			extraPluginInfos = append(extraPluginInfos, prompts.PluginInfo{
				Name:         p.Name,
				Description:  p.Description,
				Instructions: p.Instructions,
			})
		}
	}

	// Re-wire ToolSearch so it sees the cloned registry (including any newly
	// registered agent-specific plugins), not the original pre-clone registry.
	if ts, err := filtered.Get("ToolSearch"); err == nil {
		if tst, ok := ts.(*tools.ToolSearchTool); ok {
			tst.SetRegistry(filtered)
		}
	}

	model := agentDef.Model
	if resolved, ok := appInstance.API.ResolveModelShortcut(model); ok {
		model = resolved
	}
	return filtered, model, extraPluginInfos
}

// buildFullSystemPrompt gathers all context (rules, context profiles, memory, output style)
// and builds the complete system prompt. CLAUDE.md is NOT included here — it moves to
// the user context message via buildUserContext() + engine.SetUserContext().
func buildFullSystemPrompt() string {
	paths := config.GetPaths()
	cwd, _ := os.Getwd()

	// Gather additional context sections
	var sections []string

	// 1. Load context profile if specified
	if flagContext != "" {
		if content, err := loadContextProfile(flagContext); err == nil {
			sections = append(sections, content)
		}
	}

	// 2. Load rules (excludes CLAUDE.md — that goes into the user context message via buildUserContext)
	rulesReg := rules.LoadAll(
		paths.Rules,
		cwd+"/.claudio/rules",
	)
	if rulesContent := rulesReg.ForSystemPrompt(); rulesContent != "" {
		sections = append(sections, rulesContent)
	}

	// 4. Learned instincts
	if appInstance.Learning != nil {
		if instinctContent := appInstance.Learning.ForSystemPrompt(cwd); instinctContent != "" {
			sections = append(sections, instinctContent)
		}
	}

	// 5. Output style
	if appInstance.Config.OutputStyle != "" {
		if styleContent := prompts.OutputStyleSection(prompts.OutputStyle(appInstance.Config.OutputStyle)); styleContent != "" {
			sections = append(sections, styleContent)
		}
	}

	// 6. Scratchpad directory
	scratchpadDir := filepath.Join(os.TempDir(), fmt.Sprintf("claudio-%d", os.Getpid()))
	if scratchpadSection := prompts.ScratchpadSection(scratchpadDir); scratchpadSection != "" {
		sections = append(sections, scratchpadSection)
	}

	// 7. Snippet documentation
	if snippetSection := snippets.ForSystemPrompt(appInstance.Config.Snippets); snippetSection != "" {
		sections = append(sections, snippetSection)
	}

	// 8. Installed plugins
	if appInstance.Plugins != nil && len(appInstance.Plugins.All()) > 0 {
		var pluginInfos []prompts.PluginInfo
		for _, p := range appInstance.Plugins.All() {
			pluginInfos = append(pluginInfos, prompts.PluginInfo{
				Name:         p.Name,
				Description:  p.Description,
				Instructions: p.Instructions,
			})
		}
		if pluginSection := prompts.PluginsSection(pluginInfos); pluginSection != "" {
			sections = append(sections, pluginSection)
		}
	}

	// Agent persona override (appended last so it has highest precedence over style/snippets)
	if flagAgent != "" {
		agentDef := agents.GetAgent(flagAgent)
		if agentDef.SystemPrompt != "" {
			sections = append(sections, agentDef.SystemPrompt)
		}
	}

	// Advisor protocol section — tells the principal agent when/how to call the advisor
	if appInstance.Config.Advisor != nil {
		sections = append(sections, prompts.AdvisorProtocolSection())
	}

	// Combine all additional context
	additionalCtx := strings.Join(sections, "\n\n")

	return prompts.BuildSystemPrompt(appInstance.Config.Model, additionalCtx)
}

// buildUserContext loads CLAUDE.md/CLAUDIO.md content (raw) for use in the user context message.
func buildUserContext() string {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return loadCLAUDEMD(cwd, home)
}

// buildSystemContext returns the formatted git status for appending to the system prompt.
func buildSystemContext() string {
	return prompts.FormatSystemContext(prompts.GetGitStatus())
}

// loadCLAUDEMD finds and loads CLAUDE.md or CLAUDIO.md from the project.
// Walks from git root to cwd, loading files at each level (closer = higher priority).
// Also resolves @path/to/file.md imports inline.
func loadCLAUDEMD(projectDir, homeDir string) string {
	cwd, _ := os.Getwd()

	// Collect directories from project root to cwd
	dirs := collectDirsRootToCwd(projectDir, cwd)

	var parts []string
	for _, dir := range dirs {
		for _, name := range []string{"CLAUDIO.md", "CLAUDE.md", ".claudio/CLAUDE.md"} {
			path := filepath.Join(dir, name)
			if content := utils.ReadFileIfExists(path); content != "" {
				content = resolveImports(content, dir, nil)
				parts = append(parts, content)
				break // only first match per directory
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// collectDirsRootToCwd returns directories from projectRoot down to cwd (inclusive).
// The result is ordered root-first so closer-to-cwd dirs appear later (higher priority).
func collectDirsRootToCwd(projectRoot, cwd string) []string {
	projectRoot = filepath.Clean(projectRoot)
	cwd = filepath.Clean(cwd)

	if projectRoot == cwd {
		return []string{projectRoot}
	}

	// Walk from cwd upward to projectRoot, collecting dirs
	var stack []string
	current := cwd
	for {
		stack = append(stack, current)
		if current == projectRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding projectRoot
			break
		}
		current = parent
	}

	// Reverse so project root comes first
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
}

// resolveImports replaces @path/to/file.md references in content with the file's contents.
// Prevents circular imports by tracking already-processed paths.
func resolveImports(content, baseDir string, seen map[string]bool) string {
	if seen == nil {
		seen = make(map[string]bool)
	}

	var result strings.Builder
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		// Check for @import pattern: line is just "@path/to/file.md"
		if strings.HasPrefix(trimmed, "@") && strings.HasSuffix(trimmed, ".md") && !strings.Contains(trimmed, " ") {
			importPath := trimmed[1:] // strip leading @

			// Resolve relative paths
			if !filepath.IsAbs(importPath) {
				if strings.HasPrefix(importPath, "~/") {
					home, _ := os.UserHomeDir()
					importPath = filepath.Join(home, importPath[2:])
				} else {
					importPath = filepath.Join(baseDir, importPath)
				}
			}

			importPath = filepath.Clean(importPath)

			if seen[importPath] {
				result.WriteString(line)
				result.WriteString("\n")
				continue
			}
			seen[importPath] = true

			if imported := utils.ReadFileIfExists(importPath); imported != "" {
				imported = resolveImports(imported, filepath.Dir(importPath), seen)
				result.WriteString(imported)
				result.WriteString("\n")
				continue
			}
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimRight(result.String(), "\n")
}

func runInteractive() error {
	if !appInstance.Auth.IsLoggedIn() {
		return fmt.Errorf("not logged in. Run: claudio auth login")
	}

	// Proactive token refresh
	if _, err := refresh.CheckAndRefreshIfNeeded(appInstance.Storage, false); err != nil {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: token refresh failed: %v\n", err)
		}
	}

	defer appInstance.Close()
	if attachClient != nil {
		defer attachClient.Close()
	}

	systemPrompt := buildFullSystemPrompt()
	if flagTeam != "" {
		systemPrompt += "\n\nWhen all team work is complete, call PurgeTeammates to clean up agent worktrees and remove completed/failed agents."
	}

	// Start or resume session
	sess := session.New(appInstance.DB)
	if flagResume != "" {
		if flagResume == "last" {
			// Resume most recent session in this project
			recent, err := sess.RecentForProject(1)
			if err != nil || len(recent) == 0 {
				// No previous session — start fresh
				if _, err := sess.Start(appInstance.Config.Model); err != nil && flagVerbose {
					fmt.Fprintf(os.Stderr, "Warning: failed to start session: %v\n", err)
				}
			} else {
				if _, err := sess.Resume(recent[0].ID); err != nil {
					return fmt.Errorf("failed to resume last session: %w", err)
				}
				if flagVerbose {
					fmt.Fprintf(os.Stderr, "Resumed session: %s (%s)\n", recent[0].Title, recent[0].ID[:8])
				}
			}
		} else {
			// Resume by ID prefix
			resumed, err := sess.Resume(flagResume)
			if err != nil {
				return fmt.Errorf("failed to resume session %q: %w", flagResume, err)
			}
			if flagVerbose {
				fmt.Fprintf(os.Stderr, "Resumed session: %s (%s)\n", resumed.Title, resumed.ID[:8])
			}
		}
	} else {
		// Don't create a session yet — it will be created lazily on first message.
		// This avoids polluting the session list with empty sessions.
	}

	reg, modelOverride, extraPluginInfos := applyAgentOverrides(appInstance.Tools)
	if modelOverride != "" {
		appInstance.Config.Model = modelOverride
		appInstance.API.SetModel(modelOverride)
	}
	if section := prompts.PluginsSection(extraPluginInfos); section != "" {
		systemPrompt += "\n\n" + section
	}

	// Inject advisor tool for the principal agent when configured.
	// currentEngine tracks the live engine so GetMessages can return the
	// current conversation. It is updated by WithEngineRef in the TUI.
	var currentEngine *query.Engine
	if appInstance.Config.Advisor != nil {
		advisorSystemPrompt, advisorModel := buildAdvisorConfig(appInstance.Config.Advisor)
		count := 0
		advisorTool := tools.NewAdvisorTool(tools.AdvisorToolConfig{
			Definition: agents.AgentDefinition{SystemPrompt: advisorSystemPrompt},
			Model:      advisorModel,
			MaxUses:    appInstance.Config.Advisor.MaxUses,
			UsedCount:  &count,
			GetMessages: func() []api.Message {
				if currentEngine == nil {
					return nil
				}
				return currentEngine.Messages()
			},
			Client: appInstance.API,
		})
		reg.Register(advisorTool)
	}

	engineCfg := &query.EngineConfig{
		Hooks:           appInstance.Hooks,
		Analytics:       appInstance.Analytics,
		TaskRuntime:     appInstance.TaskRuntime,
		Model:           appInstance.Config.Model,
		PermissionMode:  appInstance.Config.PermissionMode,
		PermissionRules: appInstance.Config.PermissionRules,
		OnTurnEnd:       appInstance.MemoryExtractor(),
		OnAutoCompact: func(messages []api.Message, summary string) {
			if sess != nil {
				_ = sess.PersistCompacted(messages)
				_ = sess.SaveSummary(summary)
			}
		},
	}
	if appInstance.Config.CavemanEnabled() {
		if c := skills.BundledSkillContent("caveman"); c != "" {
			engineCfg.CavemanMsg = "**CAVEMAN ULTRA MODE ACTIVE — respond in caveman ultra for the entire session. Active for all agents and sub-agents. Only the human user can disable with \"stop caveman\" or \"normal mode\".**\n\n" + c + "\n\nLevel: ultra.\n\n**EXCEPTION — structured protocol output:** Always use exact format for `### Done` completion reports (exact header, all required bullet fields). Caveman style inside the fields is fine. Never skip or rename the header."
		}
	}
	appCtx := &tui.AppContext{
		Bus:         appInstance.Bus,
		Session:     sess,
		Memory:      appInstance.Memory,
		Config:      appInstance.Config,
		Analytics:     appInstance.Analytics,
		FilterSavings: appInstance.FilterSavings,
		Learning:      appInstance.Learning,
		TaskRuntime: appInstance.TaskRuntime,
		DB:          appInstance.DB,
		Hooks:       appInstance.Hooks,
		Rules:       nil, // Rules are loaded separately in system prompt building
		Auditor:     appInstance.Auditor,
		TeamManager: appInstance.Teams,
		TeamRunner:  appInstance.TeamRunner,
	}
	tuiOpts := []tui.ModelOption{
		tui.WithSkills(appInstance.Skills),
		tui.WithEngineConfig(engineCfg),
		tui.WithAppContext(appCtx),
		tui.WithUserContext(prompts.FormatUserContextMessage(buildUserContext(), "")),
		tui.WithSystemContext(buildSystemContext()),
		tui.WithDB(appInstance.DB),
		tui.WithTeamTemplatesDir(config.GetPaths().TeamTemplates),
	}
	if appInstance.Config.Advisor != nil {
		tuiOpts = append(tuiOpts, tui.WithEngineRef(&currentEngine))
	}
	if attachClient != nil {
		tuiOpts = append(tuiOpts, tui.WithScreenshotPusher(attachclient.NewAttachScreenshotPusher(attachClient)))
	}
	model := tui.New(appInstance.API, reg, systemPrompt, sess, tuiOpts...)

	// Fall back to stored agent/team config when CLI flags are absent (matches headless resume).
	if cur := sess.Current(); cur != nil {
		if flagAgent == "" {
			flagAgent = cur.AgentType
		}
		if flagTeam == "" {
			flagTeam = cur.TeamTemplate
		}
	}

	// Apply --agent flag if specified
	if flagAgent != "" {
		agentDef := agents.GetAgent(flagAgent)
		msg := agentselector.AgentSelectedMsg{
			AgentType:    agentDef.Type,
			DisplayName:  agentDef.Type,
			SystemPrompt: agentDef.SystemPrompt,
			Model:        agentDef.Model,
			DisallowedTools: agentDef.DisallowedTools,
			Capabilities: agentDef.Capabilities,
		}
		model = model.ApplyAgentPersonaAtStartup(msg)
	}

	// Apply --team flag if specified
	if flagTeam != "" {
		teamTemplatesDir := config.GetPaths().TeamTemplates
		tmpl, err := teams.GetTemplate(teamTemplatesDir, flagTeam)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: team template %q not found: %v\n", flagTeam, err)
		} else {
			msg := teamselector.TeamSelectedMsg{
				TemplateName: tmpl.Name,
				Description:  tmpl.Description,
				Members:      tmpl.Members,
			}
			model = model.ApplyTeamContextAtStartup(msg, appCtx)
			// Instantiate team so SpawnTeammate works immediately
			sessionID := ""
			if cur := sess.Current(); cur != nil {
				sessionID = cur.ID
			}
			instantiateTeamDirect(tmpl, sessionID)
		}
	}

	p := tea.NewProgram(model, tea.WithAltScreen())

	// Wire ComandCenter user messages → TUI submit path.
	// program.Send dispatches through BubbleTea's Update loop, so the
	// message follows the exact same path as local keyboard input.
	if attachClient != nil && p != nil {
		attachClient.OnUserMessage(func(payload attach.UserMsgPayload) {
			var images []api.UserContentBlock
			for _, att := range payload.Attachments {
				data, err := os.ReadFile(att.FilePath)
				if err != nil {
					continue
				}
				images = append(images, api.NewImageBlock(att.MimeType, base64.StdEncoding.EncodeToString(data)))
			}
			p.Send(prompt.SubmitMsg{Text: payload.Content, Images: images})
		})

		// Wire agent/team changes from ComandCenter → TUI.
		attachClient.OnSetAgent(func(payload attach.SetAgentPayload) {
			customDirs := agents.GetCustomDirs()
			allAgents := agents.AllAgents(customDirs...)
			for _, def := range allAgents {
				if def.Type == payload.AgentType {
					p.Send(agentselector.AgentSelectedMsg{
						AgentType:       def.Type,
						DisplayName:     def.WhenToUse,
						SystemPrompt:    def.SystemPrompt,
						Model:           def.Model,
						DisallowedTools: def.DisallowedTools,
						Capabilities:    def.Capabilities,
					})
					return
				}
			}
			log.Printf("set_agent: agent type %q not found", payload.AgentType)
		})

		attachClient.OnSetTeam(func(payload attach.SetTeamPayload) {
			templatesDir := config.GetPaths().TeamTemplates
			tmpl, err := teams.GetTemplate(templatesDir, payload.TeamName)
			if err != nil {
				log.Printf("set_team: team template %q not found", payload.TeamName)
				return
			}
			// Directly instantiate the team so SpawnTeammate works immediately,
			// without relying solely on the BubbleTea event loop processing TeamSelectedMsg.
			// Eagerly start session for a stable team-name suffix.
			if sess.Current() == nil {
				_, _ = sess.Start(appInstance.Config.Model)
			}
			sessionID := ""
			if cur := sess.Current(); cur != nil {
				sessionID = cur.ID
			}
			instantiateTeamDirect(tmpl, sessionID)
			// Also forward to TUI so it can inject team tools and update the system prompt.
			if p != nil {
				p.Send(teamselector.TeamSelectedMsg{
					TemplateName: tmpl.Name,
					Description:  tmpl.Description,
					Members:      tmpl.Members,
				})
			}
		})
	}

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Print cost summary to stderr on exit
	printCostSummary()

	return nil
}

// isTerminal checks if stdout is connected to a terminal.
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// printCostSummary prints session analytics to stderr on exit.
func printCostSummary() {
	if appInstance == nil || appInstance.Analytics == nil {
		return
	}
	tokens := appInstance.Analytics.TotalTokens()
	if tokens == 0 {
		return
	}
	cost := appInstance.Analytics.Cost()
	fmt.Fprintf(os.Stderr, "\n\033[2m%s\033[0m\n", appInstance.Analytics.Report())
	// Save report
	if appInstance.Analytics != nil {
		sessID := "unknown"
		appInstance.Analytics.SaveReport(sessID)
	}
	_ = cost // used by Report() internally
}
