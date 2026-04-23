package mcp

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Abraxas-365/claudio/internal/config"
)

// ---------------------------------------------------------------------------
// FilterTools
// ---------------------------------------------------------------------------

func TestFilterTools_EmptyAllowed_ReturnsAll(t *testing.T) {
	all := []string{"caido-search", "caido-replay", "edit", "read"}
	got := FilterTools(nil, all)
	if !reflect.DeepEqual(got, all) {
		t.Errorf("expected all tools, got %v", got)
	}
}

func TestFilterTools_EmptyAllowedSlice_ReturnsAll(t *testing.T) {
	all := []string{"tool-a", "tool-b"}
	got := FilterTools([]string{}, all)
	if !reflect.DeepEqual(got, all) {
		t.Errorf("expected all tools, got %v", got)
	}
}

func TestFilterTools_ExactMatch_ReturnsMatching(t *testing.T) {
	all := []string{"read", "edit", "write", "bash"}
	got := FilterTools([]string{"read", "write"}, all)
	want := []string{"read", "write"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestFilterTools_GlobPattern_MatchesPrefix(t *testing.T) {
	all := []string{"caido-search", "caido-replay", "read", "edit"}
	got := FilterTools([]string{"caido-*"}, all)
	want := []string{"caido-search", "caido-replay"}
	sortStrings(got)
	sortStrings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestFilterTools_NoMatch_ReturnsEmpty(t *testing.T) {
	all := []string{"read", "edit", "write"}
	got := FilterTools([]string{"nonexistent"}, all)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterTools_MixedExactAndGlob(t *testing.T) {
	all := []string{"caido-search", "caido-replay", "read", "edit", "bash"}
	got := FilterTools([]string{"caido-*", "read"}, all)
	want := []string{"caido-search", "caido-replay", "read"}
	sortStrings(got)
	sortStrings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestFilterTools_AllToolsEmpty_ReturnsEmpty(t *testing.T) {
	got := FilterTools([]string{"caido-*"}, []string{})
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterTools_WildcardPattern_ReturnsAll(t *testing.T) {
	all := []string{"tool-a", "tool-b", "other"}
	got := FilterTools([]string{"*"}, all)
	sortStrings(got)
	sortStrings(all)
	if !reflect.DeepEqual(got, all) {
		t.Errorf("want %v, got %v", all, got)
	}
}

func TestFilterTools_NoDuplicates_WhenMultiplePatternMatch(t *testing.T) {
	// "read" matches both "read" and "r*" — should appear only once
	all := []string{"read", "write"}
	got := FilterTools([]string{"read", "r*"}, all)
	count := 0
	for _, v := range got {
		if v == "read" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'read' exactly once, got %d times in %v", count, got)
	}
}

// ---------------------------------------------------------------------------
// MCPToolNames — unit test without real servers
// ---------------------------------------------------------------------------

func TestMCPToolNames_NoRunningServers_ReturnsEmpty(t *testing.T) {
	m := &Manager{
		servers: map[string]*ServerState{},
		configs: map[string]config.MCPServerConfig{},
	}
	// No running servers → empty slice
	names := m.MCPToolNames()
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestMCPToolNames_StoppedServer_Excluded(t *testing.T) {
	m := &Manager{
		servers: map[string]*ServerState{
			"mcp1": {
				Name:   "mcp1",
				Status: "stopped",
				Client: nil,
			},
		},
		configs: map[string]config.MCPServerConfig{},
	}
	names := m.MCPToolNames()
	if len(names) != 0 {
		t.Errorf("stopped server tools should not appear, got %v", names)
	}
}

func TestMCPToolNames_ErrorServer_Excluded(t *testing.T) {
	m := &Manager{
		servers: map[string]*ServerState{
			"mcp1": {
				Name:   "mcp1",
				Status: "error",
				Client: nil,
			},
		},
		configs: map[string]config.MCPServerConfig{},
	}
	names := m.MCPToolNames()
	if len(names) != 0 {
		t.Errorf("error server tools should not appear, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sortStrings(s []string) {
	sort.Strings(s)
}
