// Package orchestrator coordinates multi-agent workflows with sequential
// phases and parallel execution support.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Phase represents a named stage in a workflow.
type Phase struct {
	Name        string
	Description string
	AgentType   string // Agent type to use (e.g., "Explore", "Plan", "general-purpose")
	Prompt      string // The task prompt for this phase
	DependsOn   string // Previous phase name that must complete first
}

// PhaseResult holds the output of a completed phase.
type PhaseResult struct {
	Phase     string
	Output    string
	Duration  time.Duration
	Error     error
	Completed bool
}

// Workflow defines a multi-phase agent workflow.
type Workflow struct {
	Name   string
	Phases []Phase
}

// Orchestrator manages workflow execution.
type Orchestrator struct {
	mu      sync.Mutex
	results map[string]*PhaseResult
	// ExecuteAgent is the callback for actually running an agent.
	// It receives the agent type, prompt, and returns the output.
	ExecuteAgent func(ctx context.Context, agentType, prompt string) (string, error)
}

// New creates a new Orchestrator with the given agent execution callback.
func New(executeAgent func(ctx context.Context, agentType, prompt string) (string, error)) *Orchestrator {
	return &Orchestrator{
		results:      make(map[string]*PhaseResult),
		ExecuteAgent: executeAgent,
	}
}

// Run executes a workflow, respecting phase dependencies.
func (o *Orchestrator) Run(ctx context.Context, workflow *Workflow) ([]PhaseResult, error) {
	o.mu.Lock()
	o.results = make(map[string]*PhaseResult)
	o.mu.Unlock()

	var allResults []PhaseResult

	for _, phase := range workflow.Phases {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return allResults, ctx.Err()
		}

		// Check dependency
		if phase.DependsOn != "" {
			o.mu.Lock()
			dep, ok := o.results[phase.DependsOn]
			o.mu.Unlock()

			if !ok || !dep.Completed || dep.Error != nil {
				result := PhaseResult{
					Phase: phase.Name,
					Error: fmt.Errorf("dependency %q not satisfied", phase.DependsOn),
				}
				allResults = append(allResults, result)
				continue
			}
		}

		// Execute phase
		start := time.Now()
		output, err := o.ExecuteAgent(ctx, phase.AgentType, phase.Prompt)
		duration := time.Since(start)

		result := PhaseResult{
			Phase:     phase.Name,
			Output:    output,
			Duration:  duration,
			Error:     err,
			Completed: err == nil,
		}

		o.mu.Lock()
		o.results[phase.Name] = &result
		o.mu.Unlock()

		allResults = append(allResults, result)
	}

	return allResults, nil
}

// RunParallel executes independent phases in parallel.
func (o *Orchestrator) RunParallel(ctx context.Context, phases []Phase) []PhaseResult {
	results := make([]PhaseResult, len(phases))
	var wg sync.WaitGroup

	for i, phase := range phases {
		wg.Add(1)
		go func(idx int, p Phase) {
			defer wg.Done()

			start := time.Now()
			output, err := o.ExecuteAgent(ctx, p.AgentType, p.Prompt)

			results[idx] = PhaseResult{
				Phase:     p.Name,
				Output:    output,
				Duration:  time.Since(start),
				Error:     err,
				Completed: err == nil,
			}
		}(i, phase)
	}

	wg.Wait()
	return results
}

// StandardWorkflow creates the standard research→plan→implement→review→verify workflow.
func StandardWorkflow(taskDescription string) *Workflow {
	return &Workflow{
		Name: "standard",
		Phases: []Phase{
			{
				Name:        "research",
				Description: "Explore codebase and gather context",
				AgentType:   "Explore",
				Prompt:      fmt.Sprintf("Research and explore the codebase to understand what's needed for: %s\n\nBe very thorough. Search for related files, understand patterns, and identify all relevant code.", taskDescription),
			},
			{
				Name:        "plan",
				Description: "Design implementation approach",
				AgentType:   "Plan",
				Prompt:      fmt.Sprintf("Based on the codebase exploration, create a detailed implementation plan for: %s\n\nIdentify all files to change, the order of changes, and potential risks.", taskDescription),
				DependsOn:   "research",
			},
			{
				Name:        "implement",
				Description: "Execute the implementation",
				AgentType:   "general-purpose",
				Prompt:      fmt.Sprintf("Implement the following task: %s\n\nFollow the plan and make all necessary code changes.", taskDescription),
				DependsOn:   "plan",
			},
			{
				Name:        "verify",
				Description: "Verify the implementation",
				AgentType:   "verification",
				Prompt:      fmt.Sprintf("Verify that the implementation of '%s' is correct. Run tests, check for regressions, and validate the changes.", taskDescription),
				DependsOn:   "implement",
			},
		},
	}
}

// FormatResults creates a human-readable summary of workflow results.
func FormatResults(results []PhaseResult) string {
	var sb strings.Builder

	sb.WriteString("## Workflow Results\n\n")

	for _, r := range results {
		status := "PASS"
		if r.Error != nil {
			status = "FAIL"
		} else if !r.Completed {
			status = "SKIP"
		}

		sb.WriteString(fmt.Sprintf("### %s [%s] (%s)\n\n", r.Phase, status, r.Duration.Round(time.Second)))

		if r.Error != nil {
			sb.WriteString(fmt.Sprintf("Error: %v\n\n", r.Error))
		}

		if r.Output != "" {
			// Truncate long outputs
			output := r.Output
			if len(output) > 2000 {
				output = output[:2000] + "\n... (truncated)"
			}
			sb.WriteString(output)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}
