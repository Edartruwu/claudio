package lua

import (
	"strings"
	"sync"
	"testing"

	"github.com/Abraxas-365/claudio/internal/filters/luaregistry"
	"github.com/Abraxas-365/claudio/internal/tools/outputfilter"
)

// TestFilterAPI_Register verifies that a Lua-registered filter's declarative
// pipeline is applied when Filter() is called with a matching command.
func TestFilterAPI_Register(t *testing.T) {
	luaregistry.Reset()
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "filter-register", `
claudio.filter.register("test-strip", {
  match_command = "^mycommand",
  strip_ansi    = true,
  head_lines    = 3,
  on_empty      = "(nothing)",
})
`)
	if err := rt.LoadPlugin("filter-register", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Verify filter is listed
	names := luaregistry.List()
	found := false
	for _, n := range names {
		if n == "test-strip" {
			found = true
		}
	}
	if !found {
		t.Fatalf("filter 'test-strip' not in list: %v", names)
	}

	// Run filter: 5 lines → head_lines=3 → 3 lines
	input := "line1\nline2\nline3\nline4\nline5"
	result := outputfilter.Filter("mycommand", input)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), result)
	}
	if lines[0] != "line1" {
		t.Errorf("first line = %q, want %q", lines[0], "line1")
	}
}

// TestFilterAPI_Transform verifies the transform function receives pipeline
// output and its return value is used as final result.
func TestFilterAPI_Transform(t *testing.T) {
	luaregistry.Reset()
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "filter-transform", `
claudio.filter.register("test-xform", {
  match_command = "^xform-cmd",
  head_lines    = 2,
  transform     = function(output)
    return "TRANSFORMED:" .. output
  end,
})
`)
	if err := rt.LoadPlugin("filter-transform", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	input := "aaa\nbbb\nccc"
	result := outputfilter.Filter("xform-cmd arg1", input)

	// head_lines=2 → "aaa\nbbb", then transform prepends "TRANSFORMED:"
	expected := "TRANSFORMED:aaa\nbbb"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

// TestFilterAPI_NoMatch verifies that a filter with non-matching match_command
// does not alter the output (passthrough to generic).
func TestFilterAPI_NoMatch(t *testing.T) {
	luaregistry.Reset()
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "filter-nomatch", `
claudio.filter.register("no-match-filter", {
  match_command = "^thiswillnevermatchwhatwepass$",
  head_lines    = 1,
})
`)
	if err := rt.LoadPlugin("filter-nomatch", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	input := "line1\nline2\nline3"
	result := outputfilter.Filter("some-other-command", input)

	// Lua filter should NOT match → falls through to generic/other filters.
	// The output should NOT have head_lines=1 applied.
	if result == "line1" {
		t.Error("filter was applied despite non-matching match_command")
	}
}

// TestFilterAPI_ConcurrentSafe verifies concurrent filter calls don't race.
// Run with -race to detect data races.
func TestFilterAPI_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	luaregistry.Reset()
	rt := testRuntime(t)
	defer rt.Close()

	dir := writePlugin(t, "filter-concurrent", `
claudio.filter.register("concurrent-filter", {
  match_command = "^concurrent-cmd",
  head_lines    = 2,
  transform     = function(output)
    return "OK:" .. output
  end,
})
`)
	if err := rt.LoadPlugin("filter-concurrent", dir); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	input := "a\nb\nc\nd"
	expected := "OK:a\nb"

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := outputfilter.Filter("concurrent-cmd", input)
			if result != expected {
				t.Errorf("result = %q, want %q", result, expected)
			}
		}()
	}
	wg.Wait()
}
