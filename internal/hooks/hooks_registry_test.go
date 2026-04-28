package hooks

import (
	"testing"
)

func newTestManager() *Manager {
	return NewManager(HooksConfig{})
}

func TestGetRegisteredEventTypes_IncludesBuiltins(t *testing.T) {
	m := newTestManager()
	types := m.GetRegisteredEventTypes()

	if len(types) < 19 {
		t.Fatalf("want at least 19 built-in types, got %d", len(types))
	}

	byName := make(map[string]*EventTypeInfo, len(types))
	for _, et := range types {
		byName[et.Name] = et
	}

	builtins := []string{
		"PreToolUse", "PostToolUse", "PostToolUseFailure",
		"PreCompact", "PostCompact", "SessionStart", "SessionEnd",
		"Stop", "SubagentStart", "SubagentStop", "UserPromptSubmit",
		"TaskCreated", "TaskCompleted", "WorktreeCreate", "WorktreeRemove",
		"ConfigChange", "CwdChanged", "FileChanged", "Notification",
	}
	for _, name := range builtins {
		et, ok := byName[name]
		if !ok {
			t.Errorf("built-in event type %q missing from registry", name)
			continue
		}
		if !et.BuiltIn {
			t.Errorf("event type %q should have BuiltIn=true", name)
		}
		if et.Description == "" {
			t.Errorf("event type %q has empty description", name)
		}
	}
}

func TestRegisterEventType_CustomType(t *testing.T) {
	m := newTestManager()
	m.RegisterEventType("plugin.database.query_start", "Fires before a DB query")

	types := m.GetRegisteredEventTypes()
	byName := make(map[string]*EventTypeInfo, len(types))
	for _, et := range types {
		byName[et.Name] = et
	}

	et, ok := byName["plugin.database.query_start"]
	if !ok {
		t.Fatal("custom event type not found in registry")
	}
	if et.BuiltIn {
		t.Error("custom event type should have BuiltIn=false")
	}
	if et.Description != "Fires before a DB query" {
		t.Errorf("unexpected description: %q", et.Description)
	}
}

func TestGetRegisteredEventTypes_IncludesCustom(t *testing.T) {
	m := newTestManager()
	before := len(m.GetRegisteredEventTypes())

	m.RegisterEventType("plugin.custom.event", "Custom plugin event")
	m.RegisterEventType("plugin.custom.event2", "Another custom event")

	after := m.GetRegisteredEventTypes()
	if len(after) != before+2 {
		t.Errorf("want %d types after registration, got %d", before+2, len(after))
	}
}

func TestRegisterEventType_OverwriteDoesNotDuplicate(t *testing.T) {
	m := newTestManager()
	m.RegisterEventType("plugin.dupe", "First description")
	m.RegisterEventType("plugin.dupe", "Second description")

	count := 0
	for _, et := range m.GetRegisteredEventTypes() {
		if et.Name == "plugin.dupe" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate registration should overwrite, but found %d entries", count)
	}
}

func TestEventRegistry_IsKnown(t *testing.T) {
	r := newEventRegistry()
	r.register("foo.bar", "test", false)

	if !r.isKnown("foo.bar") {
		t.Error("registered name should be known")
	}
	if r.isKnown("foo.baz") {
		t.Error("unregistered name should not be known")
	}
}
