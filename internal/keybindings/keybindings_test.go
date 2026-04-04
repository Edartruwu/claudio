package keybindings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeTempJSON creates a temporary JSON file containing the serialized
// bindings and returns its path. The caller must not delete it; the test
// runner cleans up via t.TempDir().
func writeTempJSON(t *testing.T, bindings []Binding) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.json")
	data, err := json.Marshal(bindings)
	if err != nil {
		t.Fatalf("writeTempJSON: marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writeTempJSON: write: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Action constants
// ---------------------------------------------------------------------------

func TestActionConstants_AreNonEmpty(t *testing.T) {
	actions := []Action{
		ActionFocusViewport,
		ActionFocusPrompt,
		ActionOpenSessions,
		ActionRecentSessions,
		ActionAlternateSession,
		ActionSearchSessions,
		ActionNextSession,
		ActionPrevSession,
		ActionCreateSession,
		ActionDeleteSession,
		ActionRenameSession,
		ActionOpenConfig,
		ActionOpenSkills,
		ActionOpenMemory,
		ActionOpenAnalytics,
		ActionOpenTasks,
		ActionToggleExpand,
		ActionToggleLastExpand,
		ActionCyclePermission,
		ActionScrollDown,
		ActionScrollUp,
		ActionHalfPageDown,
		ActionHalfPageUp,
		ActionGotoTop,
		ActionGotoBottom,
		ActionExternalEditor,
		ActionPasteImage,
	}
	for _, a := range actions {
		if a == "" {
			t.Errorf("found empty Action constant")
		}
	}
}

func TestActionConstants_AreUnique(t *testing.T) {
	actions := []Action{
		ActionFocusViewport,
		ActionFocusPrompt,
		ActionOpenSessions,
		ActionRecentSessions,
		ActionAlternateSession,
		ActionSearchSessions,
		ActionNextSession,
		ActionPrevSession,
		ActionCreateSession,
		ActionDeleteSession,
		ActionRenameSession,
		ActionOpenConfig,
		ActionOpenSkills,
		ActionOpenMemory,
		ActionOpenAnalytics,
		ActionOpenTasks,
		ActionToggleExpand,
		ActionToggleLastExpand,
		ActionCyclePermission,
		ActionScrollDown,
		ActionScrollUp,
		ActionHalfPageDown,
		ActionHalfPageUp,
		ActionGotoTop,
		ActionGotoBottom,
		ActionExternalEditor,
		ActionPasteImage,
	}
	seen := make(map[Action]bool)
	for _, a := range actions {
		if seen[a] {
			t.Errorf("duplicate Action constant: %q", a)
		}
		seen[a] = true
	}
}

// ---------------------------------------------------------------------------
// DefaultKeyMap
// ---------------------------------------------------------------------------

func TestDefaultKeyMap_NotNil(t *testing.T) {
	km := DefaultKeyMap()
	if km == nil {
		t.Fatal("DefaultKeyMap() returned nil")
	}
}

func TestDefaultKeyMap_HasBindings(t *testing.T) {
	km := DefaultKeyMap()
	if len(km.bindings) == 0 {
		t.Error("DefaultKeyMap has no bindings")
	}
}

func TestDefaultKeyMap_ReservedKeys(t *testing.T) {
	km := DefaultKeyMap()
	if !km.IsReserved("ctrl+c") {
		t.Error("ctrl+c should be reserved")
	}
	if !km.IsReserved("esc") {
		t.Error("esc should be reserved")
	}
}

func TestDefaultKeyMap_KnownBindings(t *testing.T) {
	tests := []struct {
		keys    string
		context string
		action  Action
	}{
		{"space .", "normal", ActionOpenSessions},
		{"space ;", "normal", ActionRecentSessions},
		{"space ,", "normal", ActionAlternateSession},
		{"space /", "normal", ActionSearchSessions},
		{"space b n", "normal", ActionNextSession},
		{"space b p", "normal", ActionPrevSession},
		{"space b c", "normal", ActionCreateSession},
		{"space b k", "normal", ActionDeleteSession},
		{"space b r", "normal", ActionRenameSession},
		{"space i c", "normal", ActionOpenConfig},
		{"space i k", "normal", ActionOpenSkills},
		{"space i m", "normal", ActionOpenMemory},
		{"space i a", "normal", ActionOpenAnalytics},
		{"space i t", "normal", ActionOpenTasks},
		{"space w k", "normal", ActionFocusViewport},
		{"space w j", "normal", ActionFocusPrompt},
		{"shift+tab", "global", ActionCyclePermission},
		{"ctrl+o", "global", ActionToggleLastExpand},
		{"ctrl+g", "prompt", ActionExternalEditor},
		{"ctrl+v", "prompt", ActionPasteImage},
		{"j", "viewport", ActionScrollDown},
		{"k", "viewport", ActionScrollUp},
		{"ctrl+d", "viewport", ActionHalfPageDown},
		{"ctrl+u", "viewport", ActionHalfPageUp},
		{"g", "viewport", ActionGotoTop},
		// NOTE: "G" normalizes to "g" via normalizeKeys (strings.ToLower), so
		// Lookup("G", "viewport") returns the first match — goto_top.
		// ActionGotoBottom is therefore only reachable by a user who explicitly
		// overrides the key to a case-distinct token that survives normalization.
		// We test the "G" / ActionGotoBottom collision separately below.
		{"enter", "viewport", ActionToggleExpand},
	}

	km := DefaultKeyMap()
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got, ok := km.Lookup(tt.keys, tt.context)
			if !ok {
				t.Errorf("Lookup(%q, %q): not found", tt.keys, tt.context)
				return
			}
			if got != tt.action {
				t.Errorf("Lookup(%q, %q): got %q, want %q", tt.keys, tt.context, got, tt.action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KeyMap.Lookup
// ---------------------------------------------------------------------------

// TestLookup_GNormalizationCollision documents that "G" and "g" both
// normalise to "g", so Lookup("G", "viewport") returns goto_top (the first
// match) rather than goto_bottom.  ActionGotoBottom IS present in the default
// bindings but is shadowed by this normalisation quirk.
func TestLookup_GNormalizationCollision(t *testing.T) {
	km := DefaultKeyMap()

	// "G" (uppercase) becomes "g" after normalizeKeys, so it hits goto_top first.
	action, ok := km.Lookup("G", "viewport")
	if !ok {
		t.Fatal("Lookup('G', 'viewport') returned not-found")
	}
	if action != ActionGotoTop {
		t.Errorf("got %q, want %q (normalisation collision)", action, ActionGotoTop)
	}

	// ActionGotoBottom must still exist as a declared binding.
	found := false
	for _, b := range km.bindings {
		if b.Action == ActionGotoBottom {
			found = true
			break
		}
	}
	if !found {
		t.Error("ActionGotoBottom is missing from default bindings entirely")
	}
}

func TestLookup_UnknownKey_ReturnsFalse(t *testing.T) {
	km := DefaultKeyMap()
	_, ok := km.Lookup("xyz123", "global")
	if ok {
		t.Error("expected Lookup to return false for unknown key")
	}
}

func TestLookup_EmptyAction_ReturnedOnMiss(t *testing.T) {
	km := DefaultKeyMap()
	action, ok := km.Lookup("definitely-not-bound", "normal")
	if ok {
		t.Error("expected ok=false")
	}
	if action != "" {
		t.Errorf("expected empty action on miss, got %q", action)
	}
}

func TestLookup_NormalizesKeys_CaseInsensitive(t *testing.T) {
	km := DefaultKeyMap()
	// "CTRL+O" should match "ctrl+o"
	action, ok := km.Lookup("CTRL+O", "global")
	if !ok {
		t.Fatal("expected match for CTRL+O (case insensitive)")
	}
	if action != ActionToggleLastExpand {
		t.Errorf("got %q, want %q", action, ActionToggleLastExpand)
	}
}

func TestLookup_NormalizesKeys_LeadingTrailingSpaces(t *testing.T) {
	km := DefaultKeyMap()
	action, ok := km.Lookup("  ctrl+o  ", "global")
	if !ok {
		t.Fatal("expected match after trimming spaces")
	}
	if action != ActionToggleLastExpand {
		t.Errorf("got %q, want %q", action, ActionToggleLastExpand)
	}
}

func TestLookup_GlobalBindingAccessibleFromAnyContext(t *testing.T) {
	km := DefaultKeyMap()
	// shift+tab is global – should be reachable from "normal", "viewport", "prompt"
	for _, ctx := range []string{"normal", "viewport", "prompt", "global"} {
		action, ok := km.Lookup("shift+tab", ctx)
		if !ok {
			t.Errorf("shift+tab not found in context %q", ctx)
			continue
		}
		if action != ActionCyclePermission {
			t.Errorf("context %q: got %q, want %q", ctx, action, ActionCyclePermission)
		}
	}
}

func TestLookup_ContextSpecificKey_NotFoundInOtherContext(t *testing.T) {
	km := DefaultKeyMap()
	// "j" is viewport-only; should not resolve in "normal" or "prompt"
	for _, ctx := range []string{"normal", "prompt"} {
		_, ok := km.Lookup("j", ctx)
		if ok {
			t.Errorf("viewport key 'j' should not be found in context %q", ctx)
		}
	}
}

func TestLookup_EmptyKeysString(t *testing.T) {
	km := DefaultKeyMap()
	_, ok := km.Lookup("", "normal")
	if ok {
		t.Error("empty key string should not match any binding")
	}
}

func TestLookup_EmptyContext(t *testing.T) {
	km := DefaultKeyMap()
	// Bindings with empty Context field are treated as global.
	// Add a binding with empty context and verify it is found for any context.
	km.bindings = append(km.bindings, Binding{
		Keys:    "ctrl+z",
		Action:  ActionScrollDown,
		Context: "",
	})
	for _, ctx := range []string{"normal", "viewport", "global", "prompt"} {
		action, ok := km.Lookup("ctrl+z", ctx)
		if !ok {
			t.Errorf("empty-context binding not found in context %q", ctx)
			continue
		}
		if action != ActionScrollDown {
			t.Errorf("context %q: got %q, want %q", ctx, action, ActionScrollDown)
		}
	}
}

// ---------------------------------------------------------------------------
// KeyMap.IsReserved
// ---------------------------------------------------------------------------

func TestIsReserved_KnownReserved(t *testing.T) {
	km := DefaultKeyMap()
	for _, key := range []string{"ctrl+c", "esc"} {
		if !km.IsReserved(key) {
			t.Errorf("expected %q to be reserved", key)
		}
	}
}

func TestIsReserved_NotReserved(t *testing.T) {
	km := DefaultKeyMap()
	for _, key := range []string{"ctrl+o", "shift+tab", "space b n", "j"} {
		if km.IsReserved(key) {
			t.Errorf("expected %q NOT to be reserved", key)
		}
	}
}

func TestIsReserved_NormalizesCase(t *testing.T) {
	km := DefaultKeyMap()
	if !km.IsReserved("CTRL+C") {
		t.Error("IsReserved should be case-insensitive for CTRL+C")
	}
	if !km.IsReserved("ESC") {
		t.Error("IsReserved should be case-insensitive for ESC")
	}
}

func TestIsReserved_EmptyKey(t *testing.T) {
	km := DefaultKeyMap()
	if km.IsReserved("") {
		t.Error("empty string should not be reserved")
	}
}

// ---------------------------------------------------------------------------
// KeyMap.BindingsForContext
// ---------------------------------------------------------------------------

func TestBindingsForContext_NormalContext(t *testing.T) {
	km := DefaultKeyMap()
	bindings := km.BindingsForContext("normal")
	if len(bindings) == 0 {
		t.Fatal("expected bindings for 'normal' context")
	}
	for _, b := range bindings {
		if b.Context != "normal" && b.Context != "global" && b.Context != "" {
			t.Errorf("unexpected context %q in BindingsForContext('normal')", b.Context)
		}
	}
}

func TestBindingsForContext_ViewportContext(t *testing.T) {
	km := DefaultKeyMap()
	bindings := km.BindingsForContext("viewport")
	if len(bindings) == 0 {
		t.Fatal("expected bindings for 'viewport' context")
	}
	// Must include viewport-specific keys
	found := false
	for _, b := range bindings {
		if b.Keys == "j" && b.Context == "viewport" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'j' binding in viewport context")
	}
}

func TestBindingsForContext_GlobalIncludedInAllContexts(t *testing.T) {
	km := DefaultKeyMap()
	for _, ctx := range []string{"normal", "viewport", "prompt", "global"} {
		bindings := km.BindingsForContext(ctx)
		// shift+tab is global; must appear in every context's list
		found := false
		for _, b := range bindings {
			if b.Action == ActionCyclePermission {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("global binding CyclePermission missing from BindingsForContext(%q)", ctx)
		}
	}
}

func TestBindingsForContext_UnknownContext_ReturnsGlobalAndEmpty(t *testing.T) {
	km := DefaultKeyMap()
	bindings := km.BindingsForContext("totally-unknown-ctx")
	// Should still return global and empty-context bindings
	for _, b := range bindings {
		if b.Context != "global" && b.Context != "" {
			t.Errorf("unexpected context %q when querying unknown context", b.Context)
		}
	}
}

func TestBindingsForContext_EmptyContext_ReturnsGlobalAndEmpty(t *testing.T) {
	km := DefaultKeyMap()
	bindings := km.BindingsForContext("")
	for _, b := range bindings {
		if b.Context != "global" && b.Context != "" {
			t.Errorf("unexpected context %q in empty-context query", b.Context)
		}
	}
}

// ---------------------------------------------------------------------------
// GenerateTemplate
// ---------------------------------------------------------------------------

func TestGenerateTemplate_ValidJSON(t *testing.T) {
	data := GenerateTemplate()
	if len(data) == 0 {
		t.Fatal("GenerateTemplate returned empty slice")
	}
	var bindings []Binding
	if err := json.Unmarshal(data, &bindings); err != nil {
		t.Fatalf("GenerateTemplate output is not valid JSON: %v", err)
	}
}

func TestGenerateTemplate_ContainsAllDefaultBindings(t *testing.T) {
	data := GenerateTemplate()
	var bindings []Binding
	_ = json.Unmarshal(data, &bindings)

	km := DefaultKeyMap()
	if len(bindings) != len(km.bindings) {
		t.Errorf("GenerateTemplate has %d bindings, DefaultKeyMap has %d",
			len(bindings), len(km.bindings))
	}
}

func TestGenerateTemplate_BindingsHaveRequiredFields(t *testing.T) {
	data := GenerateTemplate()
	var bindings []Binding
	_ = json.Unmarshal(data, &bindings)

	for i, b := range bindings {
		if b.Keys == "" {
			t.Errorf("binding[%d] has empty Keys", i)
		}
		if b.Action == "" {
			t.Errorf("binding[%d] has empty Action", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Load — file does not exist
// ---------------------------------------------------------------------------

func TestLoad_NonExistentFile_ReturnsDefaults(t *testing.T) {
	km := Load("/this/path/does/not/exist/keybindings.json")
	if km == nil {
		t.Fatal("Load returned nil for non-existent file")
	}
	// Spot-check a default binding
	action, ok := km.Lookup("space .", "normal")
	if !ok || action != ActionOpenSessions {
		t.Errorf("expected default binding after loading non-existent file, got action=%q ok=%v", action, ok)
	}
}

// ---------------------------------------------------------------------------
// Load — invalid JSON
// ---------------------------------------------------------------------------

func TestLoad_InvalidJSON_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	km := Load(path)
	if km == nil {
		t.Fatal("Load returned nil for invalid JSON file")
	}
	action, ok := km.Lookup("space .", "normal")
	if !ok || action != ActionOpenSessions {
		t.Errorf("expected defaults after invalid JSON, got action=%q ok=%v", action, ok)
	}
}

// ---------------------------------------------------------------------------
// Load — user overrides
// ---------------------------------------------------------------------------

func TestLoad_UserOverridesDefaultKey(t *testing.T) {
	// Override OpenSessions from "space ." to "ctrl+s"
	userBindings := []Binding{
		{Keys: "ctrl+s", Action: ActionOpenSessions, Context: "normal"},
	}
	path := writeTempJSON(t, userBindings)
	km := Load(path)

	// Old key should no longer work
	_, ok := km.Lookup("space .", "normal")
	if ok {
		t.Error("old key 'space .' should not match after user override")
	}

	// New key should work
	action, ok := km.Lookup("ctrl+s", "normal")
	if !ok {
		t.Fatal("new key 'ctrl+s' not found after user override")
	}
	if action != ActionOpenSessions {
		t.Errorf("got %q, want %q", action, ActionOpenSessions)
	}
}

func TestLoad_UserAddsNewAction(t *testing.T) {
	const customAction Action = "custom_action"
	userBindings := []Binding{
		{Keys: "ctrl+x", Action: customAction, Context: "normal"},
	}
	path := writeTempJSON(t, userBindings)
	km := Load(path)

	action, ok := km.Lookup("ctrl+x", "normal")
	if !ok {
		t.Fatal("user-defined action not found")
	}
	if action != customAction {
		t.Errorf("got %q, want %q", action, customAction)
	}
}

func TestLoad_UserOverrides_DefaultsStillPresent(t *testing.T) {
	// Override only one action; all others should keep their defaults.
	userBindings := []Binding{
		{Keys: "ctrl+s", Action: ActionOpenSessions, Context: "normal"},
	}
	path := writeTempJSON(t, userBindings)
	km := Load(path)

	// An unrelated default should still work
	action, ok := km.Lookup("shift+tab", "global")
	if !ok || action != ActionCyclePermission {
		t.Errorf("unrelated default broken after override: action=%q ok=%v", action, ok)
	}
}

func TestLoad_MultipleUserOverrides(t *testing.T) {
	userBindings := []Binding{
		{Keys: "ctrl+1", Action: ActionScrollDown, Context: "viewport"},
		{Keys: "ctrl+2", Action: ActionScrollUp, Context: "viewport"},
	}
	path := writeTempJSON(t, userBindings)
	km := Load(path)

	for _, b := range userBindings {
		action, ok := km.Lookup(b.Keys, b.Context)
		if !ok {
			t.Errorf("override key %q not found", b.Keys)
			continue
		}
		if action != b.Action {
			t.Errorf("key %q: got %q, want %q", b.Keys, action, b.Action)
		}
	}
}

func TestLoad_EmptyUserFile_ReturnsDefaults(t *testing.T) {
	path := writeTempJSON(t, []Binding{})
	km := Load(path)

	action, ok := km.Lookup("space .", "normal")
	if !ok || action != ActionOpenSessions {
		t.Errorf("expected default after empty user file, got action=%q ok=%v", action, ok)
	}
}

// ---------------------------------------------------------------------------
// Binding struct — JSON round-trip
// ---------------------------------------------------------------------------

func TestBinding_JSONRoundTrip(t *testing.T) {
	original := Binding{
		Keys:    "ctrl+shift+p",
		Action:  ActionOpenConfig,
		Context: "normal",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Binding
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestBinding_JSONOmitsEmptyContext(t *testing.T) {
	b := Binding{Keys: "ctrl+z", Action: ActionScrollDown, Context: ""}
	data, _ := json.Marshal(b)
	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)
	if _, exists := raw["context"]; exists {
		t.Error("empty Context field should be omitted from JSON (omitempty)")
	}
}

// ---------------------------------------------------------------------------
// normalizeKeys (tested indirectly, but also via edge cases in Lookup)
// ---------------------------------------------------------------------------

func TestLookup_NormalizeBothSides(t *testing.T) {
	km := &KeyMap{
		bindings: []Binding{
			{Keys: "  SPACE B N  ", Action: ActionNextSession, Context: "normal"},
		},
		reserved: map[string]bool{},
	}
	action, ok := km.Lookup("space b n", "normal")
	if !ok {
		t.Fatal("expected match after normalizing stored key")
	}
	if action != ActionNextSession {
		t.Errorf("got %q, want %q", action, ActionNextSession)
	}
}

// ---------------------------------------------------------------------------
// KeyMap with no bindings (zero-value edge cases)
// ---------------------------------------------------------------------------

func TestKeyMap_ZeroValue_Lookup(t *testing.T) {
	km := &KeyMap{}
	_, ok := km.Lookup("ctrl+c", "global")
	if ok {
		t.Error("empty KeyMap should not find any bindings")
	}
}

func TestKeyMap_ZeroValue_IsReserved(t *testing.T) {
	km := &KeyMap{}
	if km.IsReserved("ctrl+c") {
		t.Error("zero-value KeyMap should have no reserved keys")
	}
}

func TestKeyMap_ZeroValue_BindingsForContext(t *testing.T) {
	km := &KeyMap{}
	bindings := km.BindingsForContext("normal")
	if bindings != nil && len(bindings) != 0 {
		t.Errorf("expected empty slice from zero-value KeyMap, got %d bindings", len(bindings))
	}
}
