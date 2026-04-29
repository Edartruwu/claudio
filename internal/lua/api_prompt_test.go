package lua

// api_prompt_test.go — unit tests for claudio.prompt.* Lua API and RunPromptHooks.
//
// Coverage:
//   - RunPromptHooks empty list → no-op
//   - Hook returns nil → pass-through
//   - Hook returns bool true → pass-through
//   - Hook returns bool false → cancelled
//   - Hook returns string → text replaced
//   - Hook chain: transform then cancel
//   - Hook panics → recovered, chain continues
//   - set_placeholder before SetPrompt → applied on wire-up
//   - set_mode invalid arg → error, not stored

import (
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tui/prompt"
)

// ── RunPromptHooks ────────────────────────────────────────────────────────────

// TestRunPromptHooks_EmptyList shows empty hook list is a no-op.
func TestRunPromptHooks_EmptyList(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	text, cancelled := rt.RunPromptHooks("hello")
	if cancelled {
		t.Error("empty hook list: expected not cancelled")
	}
	if text != "hello" {
		t.Errorf("empty hook list: text changed: got %q", text)
	}
}

// TestRunPromptHooks_NilReturn passes text through when hook returns nothing.
func TestRunPromptHooks_NilReturn(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "nil-hook", `
claudio.prompt.on_submit(function(text)
  -- return nothing (nil)
end)
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	text, cancelled := rt.RunPromptHooks("hello")
	if cancelled {
		t.Error("nil return: expected not cancelled")
	}
	if text != "hello" {
		t.Errorf("nil return: expected %q, got %q", "hello", text)
	}
}

// TestRunPromptHooks_TrueReturn passes text through when hook returns true.
func TestRunPromptHooks_TrueReturn(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "true-hook", `
claudio.prompt.on_submit(function(text)
  return true
end)
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	text, cancelled := rt.RunPromptHooks("hello")
	if cancelled {
		t.Error("true return: expected not cancelled")
	}
	if text != "hello" {
		t.Errorf("true return: expected %q, got %q", "hello", text)
	}
}

// TestRunPromptHooks_FalseReturn cancels submission when hook returns false.
func TestRunPromptHooks_FalseReturn(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "cancel-hook", `
claudio.prompt.on_submit(function(text)
  return false
end)
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	_, cancelled := rt.RunPromptHooks("hello")
	if !cancelled {
		t.Error("false return: expected cancelled")
	}
}

// TestRunPromptHooks_StringReturn replaces text when hook returns a string.
func TestRunPromptHooks_StringReturn(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "transform-hook", `
claudio.prompt.on_submit(function(text)
  return "[" .. text .. "]"
end)
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	text, cancelled := rt.RunPromptHooks("hello")
	if cancelled {
		t.Error("string return: expected not cancelled")
	}
	if text != "[hello]" {
		t.Errorf("string return: expected %q, got %q", "[hello]", text)
	}
}

// TestRunPromptHooks_ChainTransformThenCancel verifies hook order:
// first hook transforms text, second hook cancels.
func TestRunPromptHooks_ChainTransformThenCancel(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Hook 1: transform
	dir1 := writePlugin(t, "chain-transform", `
claudio.prompt.on_submit(function(text)
  return "transformed"
end)
`)
	if err := rt.LoadPlugin(dir1); err != nil {
		t.Fatalf("LoadPlugin 1: %v", err)
	}

	// Hook 2: cancel (but should see transformed text)
	dir2 := writePlugin(t, "chain-cancel", `
claudio.prompt.on_submit(function(text)
  if text == "transformed" then
    return false
  end
  return text
end)
`)
	if err := rt.LoadPlugin(dir2); err != nil {
		t.Fatalf("LoadPlugin 2: %v", err)
	}

	_, cancelled := rt.RunPromptHooks("original")
	if !cancelled {
		t.Error("chain: expected cancelled after transform+cancel")
	}
}

// TestRunPromptHooks_PanicRecovered verifies a panicking hook is recovered
// and the chain continues with original text.
func TestRunPromptHooks_PanicRecovered(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Hook 1: causes a Lua error (simulated via calling a nil value)
	dir1 := writePlugin(t, "panic-hook", `
claudio.prompt.on_submit(function(text)
  -- trigger a Lua runtime error
  local x = nil
  return x.field  -- index nil → error
end)
`)
	if err := rt.LoadPlugin(dir1); err != nil {
		t.Fatalf("LoadPlugin panic: %v", err)
	}

	// Hook 2: normal passthrough — should still run after panic in hook 1
	dir2 := writePlugin(t, "after-panic", `
claudio.prompt.on_submit(function(text)
  return "after"
end)
`)
	if err := rt.LoadPlugin(dir2); err != nil {
		t.Fatalf("LoadPlugin after: %v", err)
	}

	text, cancelled := rt.RunPromptHooks("hello")
	if cancelled {
		t.Error("panic hook: chain should not be cancelled")
	}
	// After panic, result is nil (pass-through); hook 2 runs and transforms.
	if text != "after" {
		t.Errorf("panic hook: expected %q after recovery, got %q", "after", text)
	}
}

// ── set_placeholder ───────────────────────────────────────────────────────────

// TestSetPlaceholder_DeferredApply verifies placeholder set before SetPrompt
// is applied when SetPrompt is called.
func TestSetPlaceholder_DeferredApply(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "placeholder-plugin", `
claudio.prompt.set_placeholder("Ask me anything...")
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Wire prompt after plugin loaded (simulating root.go startup order)
	p := prompt.New()
	rt.SetPrompt(&p)

	if p.Placeholder() != "Ask me anything..." {
		t.Errorf("deferred placeholder: got %q, want %q",
			p.Placeholder(), "Ask me anything...")
	}
}

// TestSetPlaceholder_ImmediateApply verifies placeholder applied immediately
// if prompt already wired.
func TestSetPlaceholder_ImmediateApply(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	// Wire first
	p := prompt.New()
	rt.SetPrompt(&p)

	dir := writePlugin(t, "placeholder-immediate", `
claudio.prompt.set_placeholder("Immediate!")
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	if p.Placeholder() != "Immediate!" {
		t.Errorf("immediate placeholder: got %q, want %q", p.Placeholder(), "Immediate!")
	}
}

// ── set_mode ─────────────────────────────────────────────────────────────────

// TestSetMode_InvalidMode verifies invalid mode is rejected.
func TestSetMode_InvalidMode(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "bad-mode", `
-- this should return an error without crashing
local ok, err = pcall(function()
  claudio.prompt.set_mode("invalid")
end)
-- errors are non-fatal from Lua side; just checking no panic
`)
	// LoadPlugin should not panic even with invalid mode call
	_ = rt.LoadPlugin(dir)
	// If we get here, no crash. Check promptDesiredMode not set to bad value.
	rt.promptMu.RLock()
	mode := rt.promptDesiredMode
	rt.promptMu.RUnlock()
	if mode == "invalid" {
		t.Error("invalid mode was stored; expected it to be rejected")
	}
}

// TestSetMode_VimMode verifies "vim" mode is stored and applied on SetPrompt.
func TestSetMode_VimMode(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "vim-mode", `
claudio.prompt.set_mode("vim")
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	p := prompt.New()
	rt.SetPrompt(&p)

	if !p.IsVimEnabled() {
		t.Error("vim mode: expected vim enabled after SetPrompt")
	}
}

// TestSetMode_SimpleMode verifies "simple" mode disables vim if active.
func TestSetMode_SimpleMode(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "simple-mode", `
claudio.prompt.set_mode("simple")
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Create prompt with vim already on
	p := prompt.New()
	p.ToggleVim()
	if !p.IsVimEnabled() {
		t.Fatal("precondition: vim should be enabled before SetPrompt")
	}

	rt.SetPrompt(&p)

	if p.IsVimEnabled() {
		t.Error("simple mode: expected vim disabled after SetPrompt")
	}
}

// ── hook chain: empty text ────────────────────────────────────────────────────

// TestRunPromptHooks_EmptyText verifies empty string passes through correctly.
func TestRunPromptHooks_EmptyText(t *testing.T) {
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "empty-text", `
claudio.prompt.on_submit(function(text)
  return text .. "suffix"
end)
`)
	if err := rt.LoadPlugin(dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	text, cancelled := rt.RunPromptHooks("")
	if cancelled {
		t.Error("empty text: unexpected cancellation")
	}
	if !strings.HasSuffix(text, "suffix") {
		t.Errorf("empty text: expected suffix appended, got %q", text)
	}
}
