package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Abraxas-365/claudio/internal/attach"
	"github.com/Abraxas-365/claudio/internal/bus"
)

// AgentTaskInput defines parameters for spawning a background agent task.
type AgentTaskInput struct {
	Prompt      string
	Description string
	AgentType   string
	Model       string // model override
	System      string // system prompt for the agent
	SessionID   string   // owning session for access control
	EventBus    *bus.Bus // optional; used to publish agent status events

	// RunAgent is the callback that actually executes the agent.
	// It receives context, system prompt, user prompt, and an output writer.
	// Returns the final text output and error.
	RunAgent func(ctx context.Context, system, prompt string) (string, error)
}

// SpawnAgentTask starts an agent in the background and returns immediately.
func SpawnAgentTask(rt *Runtime, input AgentTaskInput) (*TaskState, error) {
	if input.RunAgent == nil {
		return nil, fmt.Errorf("RunAgent callback required")
	}

	id := rt.GenerateID(TypeAgent)

	output, err := NewTaskOutput(rt.outputDir, id)
	if err != nil {
		return nil, fmt.Errorf("create output: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	desc := input.Description
	if desc == "" {
		desc = truncateCmd(input.Prompt, 60)
	}

	state := &TaskState{
		ID:          id,
		Type:        TypeAgent,
		Status:      StatusRunning,
		Description: desc,
		SessionID:   input.SessionID,
		AgentType:   input.AgentType,
		Prompt:      input.Prompt,
		OutputFile:  output.Path(),
		StartTime:   time.Now(),
		cancel:      cancel,
	}

	rt.Register(state)

	go runAgentTask(ctx, rt, state, output, input)

	return state, nil
}

func runAgentTask(ctx context.Context, rt *Runtime, state *TaskState, output *TaskOutput, input AgentTaskInput) {
	defer output.Close()

	// Write header
	output.Write([]byte(fmt.Sprintf("[Agent: %s (%s)]\nTask: %s\nStarted: %s\n\n",
		state.Description, input.AgentType,
		input.Prompt, state.StartTime.Format("15:04:05"))))

	result, err := input.RunAgent(ctx, input.System, input.Prompt)

	if err != nil {
		if ctx.Err() == context.Canceled {
			rt.SetStatus(state.ID, StatusKilled, "killed by user")
			output.Write([]byte("\n[KILLED]\n"))
			// Publish event
			if input.EventBus != nil {
				payload, _ := json.Marshal(attach.AgentStatusPayload{
					Name:   state.Description,
					Status: "killed",
				})
				input.EventBus.Publish(bus.Event{
					Type:    attach.EventAgentStatus,
					Payload: payload,
				})
			}
			return
		}
		rt.SetStatus(state.ID, StatusFailed, err.Error())
		output.Write([]byte(fmt.Sprintf("\n[FAILED] %v\n", err)))
		// Publish event
		if input.EventBus != nil {
			payload, _ := json.Marshal(attach.AgentStatusPayload{
				Name:   state.Description,
				Status: "failed",
			})
			input.EventBus.Publish(bus.Event{
				Type:    attach.EventAgentStatus,
				Payload: payload,
			})
		}
		return
	}

	output.Write([]byte(fmt.Sprintf("\n[RESULT]\n%s\n", result)))
	rt.SetStatus(state.ID, StatusCompleted, "")
	// Publish event
	if input.EventBus != nil {
		payload, _ := json.Marshal(attach.AgentStatusPayload{
			Name:   state.Description,
			Status: "completed",
		})
		input.EventBus.Publish(bus.Event{
			Type:    attach.EventAgentStatus,
			Payload: payload,
		})
	}
}

// RunAgentFunc creates the RunAgent callback using a query engine factory.
// This bridges the task system with the query engine.
type AgentEngineFactory func(system, model string) AgentRunner

// AgentRunner executes a single prompt and returns the text output.
type AgentRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// SimpleAgentRunner captures agent output into a buffer.
type SimpleAgentRunner struct {
	output bytes.Buffer
}

// OnTextDelta accumulates text from streaming responses.
func (r *SimpleAgentRunner) OnTextDelta(text string) {
	r.output.WriteString(text)
}

// Result returns the accumulated output.
func (r *SimpleAgentRunner) Result() string {
	return strings.TrimSpace(r.output.String())
}
