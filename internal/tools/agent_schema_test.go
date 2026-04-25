package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Abraxas-365/claudio/internal/teams"
)

// makeRunner returns a TeammateRunner with the given active team name.
func makeRunner(activeTeam string) *teams.TeammateRunner {
	r := teams.NewTeammateRunner(nil, nil)
	if activeTeam != "" {
		r.SetActiveTeam(activeTeam)
	}
	return r
}

func schemaHasField(t *testing.T, schema json.RawMessage, field string) bool {
	t.Helper()
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}
	_, ok := s.Properties[field]
	return ok
}

// TestAgentTool_Schema_NoTeam — run_in_background absent when no team active.
func TestAgentTool_Schema_NoTeam(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	if schemaHasField(t, schema, "run_in_background") {
		t.Error("run_in_background should not be in schema when no team is active")
	}
}

// TestAgentTool_Schema_NoTeam_NilRunner — nil TeamRunner also hides the field.
func TestAgentTool_Schema_NoTeam_NilRunner(t *testing.T) {
	tool := &AgentTool{TeamRunner: nil}
	schema := tool.InputSchema()
	if schemaHasField(t, schema, "run_in_background") {
		t.Error("run_in_background should not be in schema with nil TeamRunner")
	}
}

// TestAgentTool_Schema_TeamActive — run_in_background present when team is active.
func TestAgentTool_Schema_TeamActive(t *testing.T) {
	tool := &AgentTool{TeamRunner: makeRunner("my-team")}
	schema := tool.InputSchema()
	if !schemaHasField(t, schema, "run_in_background") {
		t.Error("run_in_background should be in schema when a team is active")
	}
}

// TestAgentTool_Schema_AlwaysHasRequiredFields — core fields always present.
func TestAgentTool_Schema_AlwaysHasRequiredFields(t *testing.T) {
	for _, teamActive := range []bool{false, true} {
		var runner *teams.TeammateRunner
		if teamActive {
			runner = makeRunner("some-team")
		}
		tool := &AgentTool{TeamRunner: runner}
		schema := tool.InputSchema()
		for _, field := range []string{"prompt", "description", "subagent_type", "model", "max_turns", "task_ids", "isolation"} {
			if !schemaHasField(t, schema, field) {
				t.Errorf("teamActive=%v: required field %q missing from schema", teamActive, field)
			}
		}
	}
}

// TestAgentTool_Schema_CacheInvalidatesOnTeamChange — schema regenerates when
// team-active state changes between calls.
func TestAgentTool_Schema_CacheInvalidatesOnTeamChange(t *testing.T) {
	runner := makeRunner("")
	tool := &AgentTool{TeamRunner: runner}

	// No team — field absent
	schema1 := tool.InputSchema()
	if schemaHasField(t, schema1, "run_in_background") {
		t.Error("run_in_background should be absent before team activation")
	}

	// Activate team — field must appear
	runner.SetActiveTeam("new-team")
	schema2 := tool.InputSchema()
	if !schemaHasField(t, schema2, "run_in_background") {
		t.Error("run_in_background should appear after team activation")
	}

	// Deactivate team — field must disappear again
	runner.SetActiveTeam("")
	schema3 := tool.InputSchema()
	if schemaHasField(t, schema3, "run_in_background") {
		t.Error("run_in_background should disappear after team deactivation")
	}
}

// TestAgentTool_Execute_BackgroundRejectedWithoutTeam — model calling
// run_in_background without a team gets a clear error, not silent fallback.
func TestAgentTool_Execute_BackgroundRejectedWithoutTeam(t *testing.T) {
	tool := &AgentTool{
		RunAgent: func(_ context.Context, system, prompt string) (string, error) {
			return "result", nil
		},
	}

	input := json.RawMessage(`{"prompt":"test","description":"test","run_in_background":true}`)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when run_in_background without team")
	}
	if result.Content == "" {
		t.Error("expected non-empty error message")
	}
}
