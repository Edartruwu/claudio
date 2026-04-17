package tasks

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CronRunner polls CronStore.Due() and fires entries in goroutines.
// Uses callback functions to avoid importing comandcenter/engine packages.
type CronRunner struct {
	store *CronStore

	// InjectFn is called for inline fires — injects prompt into session as user msg.
	InjectFn func(sessionID, prompt string)

	// StoreFn is called for background fires — stores result in session as assistant msg.
	StoreFn func(sessionID, agentName, content string)

	// ResolveModelFn resolves an agent name to (modelID, systemPrompt).
	// For shorthands like "haiku"/"sonnet"/"opus" it returns the model ID + empty system prompt.
	// For named agents it loads the agent definition and returns model + system prompt.
	ResolveModelFn func(agentName string) (modelID, systemPrompt string)

	// RunBackgroundFn runs a single-turn engine with the given model, system prompt, and user prompt.
	// Returns the assistant response text.
	RunBackgroundFn func(ctx context.Context, modelID, systemPrompt, prompt string) (string, error)

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCronRunner creates a runner backed by the given store.
func NewCronRunner(store *CronStore) *CronRunner {
	return &CronRunner{store: store}
}

// Start begins the poll loop. Ticks every 60s, fires due entries.
func (r *CronRunner) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.wg.Add(1)
	go r.loop(ctx)
}

// Stop cancels the poll loop and waits for in-flight goroutines.
func (r *CronRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *CronRunner) loop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Fire immediately on start for any already-due entries.
	r.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *CronRunner) tick(ctx context.Context) {
	due := r.store.Due()
	for _, entry := range due {
		// Mark run first to advance NextRun (prevent re-fire on next tick).
		if err := r.store.MarkRun(entry.ID); err != nil {
			log.Printf("[cron] mark run %s: %v", entry.ID, err)
			continue
		}

		e := entry // capture for goroutine
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.fire(ctx, e)
		}()
	}
}

func (r *CronRunner) fire(ctx context.Context, entry CronEntry) {
	entryType := entry.Type
	if entryType == "" {
		entryType = "inline"
	}

	switch entryType {
	case "inline":
		r.fireInline(entry)
	case "background":
		r.fireBackground(ctx, entry)
	default:
		log.Printf("[cron] %s: unknown type %q, skipping", entry.ID, entryType)
	}
}

func (r *CronRunner) fireInline(entry CronEntry) {
	if r.InjectFn == nil {
		log.Printf("[cron] %s: InjectFn not set, skipping inline fire", entry.ID)
		return
	}
	if entry.SessionID == "" {
		log.Printf("[cron] %s: no session ID, skipping inline fire", entry.ID)
		return
	}
	log.Printf("[cron] %s: inline fire → session %s", entry.ID, entry.SessionID)
	r.InjectFn(entry.SessionID, entry.Prompt)
}

func (r *CronRunner) fireBackground(ctx context.Context, entry CronEntry) {
	if r.RunBackgroundFn == nil {
		log.Printf("[cron] %s: RunBackgroundFn not set, skipping background fire", entry.ID)
		return
	}
	if r.StoreFn == nil {
		log.Printf("[cron] %s: StoreFn not set, skipping background fire", entry.ID)
		return
	}

	agent := entry.Agent
	if agent == "" {
		agent = "sonnet" // default model for background
	}

	var modelID, systemPrompt string
	if r.ResolveModelFn != nil {
		modelID, systemPrompt = r.ResolveModelFn(agent)
	} else {
		modelID = resolveModelShorthand(agent)
	}

	if modelID == "" {
		modelID = "claude-sonnet-4-6"
	}

	log.Printf("[cron] %s: background fire (model=%s, agent=%s)", entry.ID, modelID, agent)

	response, err := r.RunBackgroundFn(ctx, modelID, systemPrompt, entry.Prompt)
	if err != nil {
		log.Printf("[cron] %s: background engine error: %v", entry.ID, err)
		response = fmt.Sprintf("[cron error: %v]", err)
	}

	agentName := fmt.Sprintf("⏰ %s/%s", agent, entry.ID)
	sessionID := entry.SessionID
	if sessionID == "" {
		log.Printf("[cron] %s: no session ID for background result, dropping", entry.ID)
		return
	}

	r.StoreFn(sessionID, agentName, response)
}

// resolveModelShorthand maps common aliases to full model IDs.
func resolveModelShorthand(name string) string {
	switch name {
	case "haiku":
		return "claude-haiku-4-5-20251001"
	case "sonnet":
		return "claude-sonnet-4-6"
	case "opus":
		return "claude-opus-4-6"
	default:
		return ""
	}
}
