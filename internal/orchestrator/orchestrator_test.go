package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// successAgent always returns "<agentType>:<prompt>" with no error.
func successAgent(_ context.Context, agentType, prompt string) (string, error) {
	return fmt.Sprintf("%s:%s", agentType, prompt), nil
}

// errorAgent always returns an error.
func errorAgent(_ context.Context, _, _ string) (string, error) {
	return "", errors.New("agent failed")
}

// slowAgent sleeps for d before returning successfully.
func slowAgent(d time.Duration) func(context.Context, string, string) (string, error) {
	return func(_ context.Context, agentType, prompt string) (string, error) {
		time.Sleep(d)
		return fmt.Sprintf("%s:%s", agentType, prompt), nil
	}
}

// ─── New ────────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	o := New(successAgent)
	if o == nil {
		t.Fatal("New returned nil")
	}
	if o.results == nil {
		t.Error("results map should be initialised")
	}
	if o.ExecuteAgent == nil {
		t.Error("ExecuteAgent callback should be set")
	}
}

func TestNew_NilCallback(t *testing.T) {
	// New should not panic when passed nil – it is the caller's responsibility
	// to supply a valid callback before calling Run/RunParallel.
	o := New(nil)
	if o == nil {
		t.Fatal("New returned nil")
	}
}

// ─── Run ────────────────────────────────────────────────────────────────────

func TestRun_EmptyWorkflow(t *testing.T) {
	o := New(successAgent)
	results, err := o.Run(context.Background(), &Workflow{Name: "empty"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRun_SinglePhase_Success(t *testing.T) {
	o := New(successAgent)
	wf := &Workflow{
		Name: "single",
		Phases: []Phase{
			{Name: "alpha", AgentType: "typeA", Prompt: "do something"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Phase != "alpha" {
		t.Errorf("phase name: got %q, want %q", r.Phase, "alpha")
	}
	if !r.Completed {
		t.Error("expected Completed=true")
	}
	if r.Error != nil {
		t.Errorf("unexpected error in result: %v", r.Error)
	}
	if r.Output != "typeA:do something" {
		t.Errorf("unexpected output: %q", r.Output)
	}
	if r.Duration < 0 {
		t.Error("Duration should be non-negative")
	}
}

func TestRun_SinglePhase_AgentError(t *testing.T) {
	o := New(errorAgent)
	wf := &Workflow{
		Name: "fail",
		Phases: []Phase{
			{Name: "p1", AgentType: "t", Prompt: "x"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Completed {
		t.Error("expected Completed=false when agent errors")
	}
	if r.Error == nil {
		t.Error("expected Error to be set")
	}
}

func TestRun_MultiplePhases_NoDependencies(t *testing.T) {
	o := New(successAgent)
	wf := &Workflow{
		Name: "multi",
		Phases: []Phase{
			{Name: "a", AgentType: "t1", Prompt: "p1"},
			{Name: "b", AgentType: "t2", Prompt: "p2"},
			{Name: "c", AgentType: "t3", Prompt: "p3"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	names := []string{"a", "b", "c"}
	for i, r := range results {
		if r.Phase != names[i] {
			t.Errorf("results[%d].Phase = %q, want %q", i, r.Phase, names[i])
		}
		if !r.Completed {
			t.Errorf("results[%d] should be completed", i)
		}
	}
}

func TestRun_DependencyChain_Success(t *testing.T) {
	var order []string
	agent := func(_ context.Context, agentType, _ string) (string, error) {
		order = append(order, agentType)
		return "ok", nil
	}

	o := New(agent)
	wf := &Workflow{
		Name: "chain",
		Phases: []Phase{
			{Name: "first", AgentType: "A"},
			{Name: "second", AgentType: "B", DependsOn: "first"},
			{Name: "third", AgentType: "C", DependsOn: "second"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Completed {
			t.Errorf("results[%d] should be completed", i)
		}
	}

	// Execution order must be preserved
	wantOrder := []string{"A", "B", "C"}
	for i, got := range order {
		if got != wantOrder[i] {
			t.Errorf("order[%d] = %q, want %q", i, got, wantOrder[i])
		}
	}
}

func TestRun_DependencyNotSatisfied_PhaseMissing(t *testing.T) {
	o := New(successAgent)
	// "second" depends on "ghost" which never runs
	wf := &Workflow{
		Name: "broken-dep",
		Phases: []Phase{
			{Name: "second", AgentType: "B", DependsOn: "ghost"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Error == nil {
		t.Error("expected dependency-not-satisfied error")
	}
	if !strings.Contains(r.Error.Error(), "ghost") {
		t.Errorf("error message should mention the missing dependency, got: %v", r.Error)
	}
	if r.Completed {
		t.Error("phase with unsatisfied dependency should not be Completed")
	}
}

func TestRun_DependencyNotSatisfied_UpstreamFailed(t *testing.T) {
	callCount := 0
	agent := func(_ context.Context, _, _ string) (string, error) {
		callCount++
		return "", errors.New("boom")
	}

	o := New(agent)
	wf := &Workflow{
		Name: "upstream-fail",
		Phases: []Phase{
			{Name: "first", AgentType: "A"},
			{Name: "second", AgentType: "B", DependsOn: "first"},
		},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Only the first phase should have called the agent
	if callCount != 1 {
		t.Errorf("agent should only be called once (for first phase), got %d calls", callCount)
	}

	// Second phase should report a dependency error, not the original agent error
	if results[1].Error == nil {
		t.Error("second phase should have an error")
	}
	if !strings.Contains(results[1].Error.Error(), "first") {
		t.Errorf("error should mention dependency %q, got: %v", "first", results[1].Error)
	}
}

func TestRun_ContextCancelled_BeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	o := New(successAgent)
	wf := &Workflow{
		Name: "cancelled",
		Phases: []Phase{
			{Name: "p1", AgentType: "t", Prompt: "x"},
		},
	}

	results, err := o.Run(ctx, wf)
	if err == nil {
		t.Error("expected context error, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for immediately-cancelled context, got %d", len(results))
	}
}

func TestRun_ContextCancelled_MidWorkflow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after the first phase completes
	calls := int32(0)
	agent := func(_ context.Context, agentType, prompt string) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			cancel()
		}
		return "ok", nil
	}

	o := New(agent)
	wf := &Workflow{
		Name: "mid-cancel",
		Phases: []Phase{
			{Name: "p1"},
			{Name: "p2"},
			{Name: "p3"},
		},
	}

	results, err := o.Run(ctx, wf)
	if err == nil {
		t.Error("expected context error after cancellation")
	}
	// Only p1 should have run
	if len(results) != 1 {
		t.Errorf("expected 1 result before cancellation, got %d", len(results))
	}
}

func TestRun_ReusableOrchestrator(t *testing.T) {
	o := New(successAgent)
	wf := &Workflow{
		Name: "reuse",
		Phases: []Phase{
			{Name: "x", AgentType: "t", Prompt: "p"},
		},
	}

	for i := 0; i < 3; i++ {
		results, err := o.Run(context.Background(), wf)
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
		if len(results) != 1 {
			t.Fatalf("run %d: expected 1 result, got %d", i, len(results))
		}
		if !results[0].Completed {
			t.Errorf("run %d: expected Completed=true", i)
		}
	}
}

// ─── RunParallel ─────────────────────────────────────────────────────────────

func TestRunParallel_EmptySlice(t *testing.T) {
	o := New(successAgent)
	results := o.RunParallel(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRunParallel_SinglePhase(t *testing.T) {
	o := New(successAgent)
	phases := []Phase{
		{Name: "solo", AgentType: "T", Prompt: "P"},
	}

	results := o.RunParallel(context.Background(), phases)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Phase != "solo" {
		t.Errorf("Phase = %q, want %q", r.Phase, "solo")
	}
	if !r.Completed {
		t.Error("expected Completed=true")
	}
	if r.Error != nil {
		t.Errorf("unexpected error: %v", r.Error)
	}
}

func TestRunParallel_MultiplePhases_AllSucceed(t *testing.T) {
	o := New(successAgent)
	phases := []Phase{
		{Name: "a", AgentType: "T1", Prompt: "P1"},
		{Name: "b", AgentType: "T2", Prompt: "P2"},
		{Name: "c", AgentType: "T3", Prompt: "P3"},
	}

	results := o.RunParallel(context.Background(), phases)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results slice is indexed by input order – check that.
	for i, p := range phases {
		if results[i].Phase != p.Name {
			t.Errorf("results[%d].Phase = %q, want %q", i, results[i].Phase, p.Name)
		}
		if !results[i].Completed {
			t.Errorf("results[%d] should be Completed", i)
		}
	}
}

func TestRunParallel_SomePhasesFail(t *testing.T) {
	agent := func(_ context.Context, agentType, _ string) (string, error) {
		if agentType == "bad" {
			return "", errors.New("bad agent")
		}
		return "ok", nil
	}

	o := New(agent)
	phases := []Phase{
		{Name: "good1", AgentType: "good"},
		{Name: "bad1", AgentType: "bad"},
		{Name: "good2", AgentType: "good"},
	}

	results := o.RunParallel(context.Background(), phases)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if !results[0].Completed || results[0].Error != nil {
		t.Errorf("results[0] (good1) should succeed")
	}
	if results[1].Completed || results[1].Error == nil {
		t.Errorf("results[1] (bad1) should fail")
	}
	if !results[2].Completed || results[2].Error != nil {
		t.Errorf("results[2] (good2) should succeed")
	}
}

func TestRunParallel_ActuallyParallel(t *testing.T) {
	// All phases sleep for 50 ms.  If they ran serially the total time would
	// exceed 3 × 50 ms = 150 ms; running in parallel it should be < 150 ms.
	const sleep = 50 * time.Millisecond
	o := New(slowAgent(sleep))

	phases := make([]Phase, 5)
	for i := range phases {
		phases[i] = Phase{Name: fmt.Sprintf("p%d", i), AgentType: "T", Prompt: "P"}
	}

	start := time.Now()
	results := o.RunParallel(context.Background(), phases)
	elapsed := time.Since(start)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	serialTime := sleep * time.Duration(len(phases))
	if elapsed >= serialTime {
		t.Errorf("RunParallel took %v which is >= serial time %v – phases may not be running in parallel", elapsed, serialTime)
	}
}

func TestRunParallel_Duration_Recorded(t *testing.T) {
	const sleep = 20 * time.Millisecond
	o := New(slowAgent(sleep))
	phases := []Phase{{Name: "p", AgentType: "T", Prompt: "P"}}

	results := o.RunParallel(context.Background(), phases)
	if results[0].Duration < sleep {
		t.Errorf("Duration %v is less than expected minimum %v", results[0].Duration, sleep)
	}
}

// ─── StandardWorkflow ────────────────────────────────────────────────────────

func TestStandardWorkflow_Structure(t *testing.T) {
	desc := "add user authentication"
	wf := StandardWorkflow(desc)

	if wf == nil {
		t.Fatal("StandardWorkflow returned nil")
	}
	if wf.Name != "standard" {
		t.Errorf("workflow Name = %q, want %q", wf.Name, "standard")
	}

	wantPhases := []struct {
		name      string
		agentType string
		dependsOn string
	}{
		{"research", "Explore", ""},
		{"plan", "Plan", "research"},
		{"implement", "general-purpose", "plan"},
		{"verify", "verification", "implement"},
	}

	if len(wf.Phases) != len(wantPhases) {
		t.Fatalf("expected %d phases, got %d", len(wantPhases), len(wf.Phases))
	}

	for i, want := range wantPhases {
		p := wf.Phases[i]
		if p.Name != want.name {
			t.Errorf("Phases[%d].Name = %q, want %q", i, p.Name, want.name)
		}
		if p.AgentType != want.agentType {
			t.Errorf("Phases[%d].AgentType = %q, want %q", i, p.AgentType, want.agentType)
		}
		if p.DependsOn != want.dependsOn {
			t.Errorf("Phases[%d].DependsOn = %q, want %q", i, p.DependsOn, want.dependsOn)
		}
		if !strings.Contains(p.Prompt, desc) {
			t.Errorf("Phases[%d].Prompt should contain task description", i)
		}
		if p.Description == "" {
			t.Errorf("Phases[%d].Description should not be empty", i)
		}
	}
}

func TestStandardWorkflow_EmptyDescription(t *testing.T) {
	wf := StandardWorkflow("")
	if wf == nil {
		t.Fatal("StandardWorkflow returned nil for empty description")
	}
	if len(wf.Phases) != 4 {
		t.Fatalf("expected 4 phases, got %d", len(wf.Phases))
	}
}

func TestStandardWorkflow_ExecutesEndToEnd(t *testing.T) {
	o := New(successAgent)
	wf := StandardWorkflow("build a feature")

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Completed {
			t.Errorf("results[%d] (%s) should be completed", i, r.Phase)
		}
	}
}

// ─── FormatResults ───────────────────────────────────────────────────────────

func TestFormatResults_EmptySlice(t *testing.T) {
	out := FormatResults(nil)
	if !strings.Contains(out, "## Workflow Results") {
		t.Errorf("output should contain header, got: %q", out)
	}
}

func TestFormatResults_SingleSuccess(t *testing.T) {
	results := []PhaseResult{
		{
			Phase:     "research",
			Output:    "found stuff",
			Duration:  2 * time.Second,
			Completed: true,
		},
	}

	out := FormatResults(results)

	if !strings.Contains(out, "research") {
		t.Error("output should contain phase name")
	}
	if !strings.Contains(out, "PASS") {
		t.Error("output should contain PASS for successful phase")
	}
	if !strings.Contains(out, "found stuff") {
		t.Error("output should contain the phase output")
	}
}

func TestFormatResults_ErrorPhase(t *testing.T) {
	results := []PhaseResult{
		{
			Phase: "plan",
			Error: errors.New("planning failed"),
		},
	}

	out := FormatResults(results)

	if !strings.Contains(out, "FAIL") {
		t.Error("output should contain FAIL for errored phase")
	}
	if !strings.Contains(out, "planning failed") {
		t.Error("output should contain the error message")
	}
}

func TestFormatResults_SkippedPhase(t *testing.T) {
	// Completed=false and Error=nil → SKIP
	results := []PhaseResult{
		{
			Phase:     "implement",
			Completed: false,
			Error:     nil,
		},
	}

	out := FormatResults(results)

	if !strings.Contains(out, "SKIP") {
		t.Error("output should contain SKIP for incomplete phase with no error")
	}
}

func TestFormatResults_OutputTruncation(t *testing.T) {
	longOutput := strings.Repeat("x", 3000)
	results := []PhaseResult{
		{
			Phase:     "big",
			Output:    longOutput,
			Completed: true,
		},
	}

	out := FormatResults(results)

	if !strings.Contains(out, "truncated") {
		t.Error("output longer than 2000 chars should be truncated")
	}
	// The truncated output should not contain more than 2000 'x' chars.
	xCount := strings.Count(out, "x")
	if xCount > 2000 {
		t.Errorf("found %d 'x' chars in output; expected at most 2000 after truncation", xCount)
	}
}

func TestFormatResults_OutputExactlyAtLimit(t *testing.T) {
	// Exactly 2000 chars should NOT be truncated.
	exactOutput := strings.Repeat("y", 2000)
	results := []PhaseResult{
		{
			Phase:     "exact",
			Output:    exactOutput,
			Completed: true,
		},
	}

	out := FormatResults(results)

	if strings.Contains(out, "truncated") {
		t.Error("output of exactly 2000 chars should not be truncated")
	}
}

func TestFormatResults_NoOutputSection_WhenEmpty(t *testing.T) {
	results := []PhaseResult{
		{
			Phase:     "silent",
			Completed: true,
			Output:    "",
		},
	}

	out := FormatResults(results)
	// The phase header should still appear
	if !strings.Contains(out, "silent") {
		t.Error("output should contain phase name even when Output is empty")
	}
}

func TestFormatResults_MultiplePhases(t *testing.T) {
	results := []PhaseResult{
		{Phase: "r1", Completed: true, Output: "out1"},
		{Phase: "r2", Error: errors.New("err2")},
		{Phase: "r3", Completed: false},
	}

	out := FormatResults(results)

	for _, want := range []string{"r1", "PASS", "r2", "FAIL", "r3", "SKIP"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q", want)
		}
	}
}

func TestFormatResults_Header(t *testing.T) {
	out := FormatResults([]PhaseResult{})
	if !strings.HasPrefix(out, "## Workflow Results") {
		t.Errorf("output should start with header, got: %q", out[:min(len(out), 40)])
	}
}

// ─── PhaseResult fields ──────────────────────────────────────────────────────

func TestPhaseResult_ZeroValue(t *testing.T) {
	var r PhaseResult
	if r.Phase != "" || r.Output != "" || r.Error != nil || r.Completed || r.Duration != 0 {
		t.Error("zero-value PhaseResult should have all zero fields")
	}
}

// ─── Phase fields ─────────────────────────────────────────────────────────────

func TestPhase_ZeroValue(t *testing.T) {
	var p Phase
	if p.Name != "" || p.Description != "" || p.AgentType != "" || p.Prompt != "" || p.DependsOn != "" {
		t.Error("zero-value Phase should have all empty fields")
	}
}

// ─── Workflow fields ──────────────────────────────────────────────────────────

func TestWorkflow_ZeroValue(t *testing.T) {
	var w Workflow
	if w.Name != "" || len(w.Phases) != 0 {
		t.Error("zero-value Workflow should have empty name and nil phases")
	}
}

// ─── Run duration is recorded ─────────────────────────────────────────────────

func TestRun_DurationRecorded(t *testing.T) {
	const sleep = 15 * time.Millisecond
	o := New(slowAgent(sleep))
	wf := &Workflow{
		Name:   "dur",
		Phases: []Phase{{Name: "p", AgentType: "T", Prompt: "P"}},
	}

	results, err := o.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Duration < sleep {
		t.Errorf("Duration %v is less than expected %v", results[0].Duration, sleep)
	}
}

// ─── Dependency error message ─────────────────────────────────────────────────

func TestRun_DependencyError_ContainsDependencyName(t *testing.T) {
	tests := []struct {
		name      string
		dependsOn string
	}{
		{"missing dep", "nonexistent"},
		{"special chars", "dep-with-dashes"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := New(successAgent)
			wf := &Workflow{
				Name: "dep-test",
				Phases: []Phase{
					{Name: "downstream", DependsOn: tc.dependsOn},
				},
			}
			results, _ := o.Run(context.Background(), wf)
			if len(results) == 0 {
				t.Fatal("expected at least one result")
			}
			if results[0].Error == nil {
				t.Fatal("expected error for unsatisfied dependency")
			}
			if !strings.Contains(results[0].Error.Error(), tc.dependsOn) {
				t.Errorf("error %q should contain dependency name %q", results[0].Error, tc.dependsOn)
			}
		})
	}
}

// ─── helpers (Go 1.21+) ───────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
