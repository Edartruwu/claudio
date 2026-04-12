package tomlfilter

import (
	"os"
	"strings"
	"testing"
)

const testTOML = `
schema_version = 1

[filters.test_strip_ansi]
description = "test: strip ANSI"
match_command = "^ansitest\\b"
strip_ansi = true

[filters.test_replace]
description = "test: replace"
match_command = "^repltest\\b"
replace = [
  { pattern = "foo", replacement = "bar" },
  { pattern = "hello", replacement = "world" },
]

[filters.test_match_output]
description = "test: match_output short-circuit"
match_command = "^matchtest\\b"
match_output = [
  { pattern = "ERROR: fatal", message = "fatal error detected" },
]

[filters.test_match_output_unless]
description = "test: match_output with unless"
match_command = "^unlesstest\\b"
match_output = [
  { pattern = "ERROR", message = "error found", unless = "WARNING" },
]

[filters.test_strip_lines]
description = "test: strip lines matching"
match_command = "^striplines\\b"
strip_lines_matching = ["^DEBUG:", "^TRACE:"]

[filters.test_keep_lines]
description = "test: keep lines matching"
match_command = "^keeplines\\b"
keep_lines_matching = ["^IMPORTANT:", "^CRITICAL:"]

[filters.test_truncate]
description = "test: truncate lines at"
match_command = "^trunctest\\b"
truncate_lines_at = 10

[filters.test_head]
description = "test: head lines"
match_command = "^headtest\\b"
head_lines = 3

[filters.test_tail]
description = "test: tail lines"
match_command = "^tailtest\\b"
tail_lines = 2

[filters.test_max_lines]
description = "test: max lines"
match_command = "^maxtest\\b"
max_lines = 5

[filters.test_on_empty]
description = "test: on_empty fallback"
match_command = "^emptytest\\b"
strip_lines_matching = [".*"]
on_empty = "all lines stripped"

[filters.test_full_pipeline]
description = "test: full 8-stage pipeline"
match_command = "^fulltest\\b"
strip_ansi = true
replace = [
  { pattern = "secret", replacement = "REDACTED" },
]
strip_lines_matching = ["^#"]
truncate_lines_at = 20
head_lines = 10
tail_lines = 5
max_lines = 4
on_empty = "nothing left"
`

func mustRegistry(t *testing.T, tomlData string) *Registry {
	t.Helper()
	r, err := NewRegistryFromTOML(tomlData)
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}
	return r
}

func TestStripAnsi(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "\x1b[31mred text\x1b[0m and \x1b[1mbold\x1b[0m"
	result, ok := r.Apply("ansitest", input)
	if !ok {
		t.Fatal("expected match")
	}
	if strings.Contains(result, "\x1b[") {
		t.Errorf("ANSI codes not stripped: %q", result)
	}
	if result != "red text and bold" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestReplace(t *testing.T) {
	r := mustRegistry(t, testTOML)
	result, ok := r.Apply("repltest", "foo and hello")
	if !ok {
		t.Fatal("expected match")
	}
	if result != "bar and world" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestMatchOutputShortCircuit(t *testing.T) {
	r := mustRegistry(t, testTOML)
	result, ok := r.Apply("matchtest", "some output\nERROR: fatal crash\nmore stuff")
	if !ok {
		t.Fatal("expected match")
	}
	if result != "fatal error detected" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestMatchOutputUnless(t *testing.T) {
	r := mustRegistry(t, testTOML)

	// Unless pattern matches too, so match_output rule should be skipped
	result, ok := r.Apply("unlesstest", "ERROR happened\nWARNING also present")
	if !ok {
		t.Fatal("expected match")
	}
	// The unless guard prevents the short-circuit, so we get the original text back
	if result != "ERROR happened\nWARNING also present" {
		t.Errorf("unless guard failed, got: %q", result)
	}

	// Without the unless pattern, it should short-circuit
	result, ok = r.Apply("unlesstest", "ERROR happened\nno warning here")
	if !ok {
		t.Fatal("expected match")
	}
	if result != "error found" {
		t.Errorf("expected short-circuit, got: %q", result)
	}
}

func TestStripLinesMatching(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "INFO: good\nDEBUG: noisy\nTRACE: verbose\nINFO: also good"
	result, ok := r.Apply("striplines", input)
	if !ok {
		t.Fatal("expected match")
	}
	if result != "INFO: good\nINFO: also good" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestKeepLinesMatching(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "noise\nIMPORTANT: keep this\nnoise2\nCRITICAL: and this"
	result, ok := r.Apply("keeplines", input)
	if !ok {
		t.Fatal("expected match")
	}
	if result != "IMPORTANT: keep this\nCRITICAL: and this" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestStripKeepMutualExclusion(t *testing.T) {
	badTOML := `
schema_version = 1

[filters.bad]
description = "bad: both set"
match_command = "^bad\\b"
strip_lines_matching = ["foo"]
keep_lines_matching = ["bar"]
`
	r, err := NewRegistryFromTOML(badTOML)
	if err != nil {
		t.Fatalf("parse should succeed, filter should be skipped: %v", err)
	}
	// The bad filter should have been skipped
	_, ok := r.Apply("bad command", "test")
	if ok {
		t.Error("filter with both strip and keep should have been rejected")
	}
}

func TestTruncateLinesAt(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "short\nthis is a very long line that exceeds the limit"
	result, ok := r.Apply("trunctest", input)
	if !ok {
		t.Fatal("expected match")
	}
	lines := strings.Split(result, "\n")
	if lines[0] != "short" {
		t.Errorf("short line should be unchanged, got: %q", lines[0])
	}
	if lines[1] != "this is a ..." {
		t.Errorf("long line should be truncated, got: %q", lines[1])
	}
}

func TestHeadLines(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "line1\nline2\nline3\nline4\nline5"
	result, ok := r.Apply("headtest", input)
	if !ok {
		t.Fatal("expected match")
	}
	if result != "line1\nline2\nline3" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestTailLines(t *testing.T) {
	r := mustRegistry(t, testTOML)
	input := "line1\nline2\nline3\nline4\nline5"
	result, ok := r.Apply("tailtest", input)
	if !ok {
		t.Fatal("expected match")
	}
	if result != "line4\nline5" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestMaxLines(t *testing.T) {
	r := mustRegistry(t, testTOML)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	input := strings.Join(lines, "\n")
	result, ok := r.Apply("maxtest", input)
	if !ok {
		t.Fatal("expected match")
	}
	got := strings.Count(result, "\n") + 1
	if got != 5 {
		t.Errorf("expected 5 lines, got %d", got)
	}
}

func TestOnEmpty(t *testing.T) {
	r := mustRegistry(t, testTOML)
	result, ok := r.Apply("emptytest", "anything\nwhatever\n")
	if !ok {
		t.Fatal("expected match")
	}
	if result != "all lines stripped" {
		t.Errorf("on_empty fallback not triggered, got: %q", result)
	}
}

func TestFullPipeline(t *testing.T) {
	r := mustRegistry(t, testTOML)
	// Build input that exercises all stages
	lines := []string{
		"# comment to strip",
		"\x1b[31msecret data line one\x1b[0m",
		"short",
		"this is a line with the word secret in it that is long enough",
		"# another comment",
		"line six",
		"line seven",
		"line eight",
		"line nine",
		"line ten",
		"line eleven",
		"line twelve",
	}
	input := strings.Join(lines, "\n")
	result, ok := r.Apply("fulltest", input)
	if !ok {
		t.Fatal("expected match")
	}

	// Verify ANSI stripped and secret replaced
	if strings.Contains(result, "\x1b[") {
		t.Error("ANSI codes should be stripped")
	}
	if strings.Contains(result, "secret") {
		t.Error("'secret' should be replaced with 'REDACTED'")
	}

	// Verify comments stripped
	if strings.Contains(result, "# comment") {
		t.Error("comment lines should be stripped")
	}

	// Verify max_lines cap
	resultLines := strings.Split(result, "\n")
	if len(resultLines) > 4 {
		t.Errorf("max_lines=4 but got %d lines: %v", len(resultLines), resultLines)
	}
}

func TestNoMatch(t *testing.T) {
	r := mustRegistry(t, testTOML)
	_, ok := r.Apply("unknowncommand", "output")
	if ok {
		t.Error("should not match unknown command")
	}
}

func TestSchemaVersionRejection(t *testing.T) {
	badTOML := `
schema_version = 99

[filters.test]
description = "test"
match_command = "^test\\b"
`
	_, err := NewRegistryFromTOML(badTOML)
	if err == nil {
		t.Error("expected error for unsupported schema_version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error should mention schema_version: %v", err)
	}
}

func TestNoFilterEnvVar(t *testing.T) {
	r := mustRegistry(t, testTOML)

	os.Setenv("CLAUDIO_NO_FILTER", "1")
	defer os.Unsetenv("CLAUDIO_NO_FILTER")

	_, ok := r.Apply("ansitest", "some output")
	if ok {
		t.Error("CLAUDIO_NO_FILTER=1 should bypass all TOML filters")
	}
}

func TestInvalidRegex(t *testing.T) {
	badTOML := `
schema_version = 1

[filters.bad_regex]
description = "bad regex"
match_command = "[invalid"
`
	r, err := NewRegistryFromTOML(badTOML)
	if err != nil {
		t.Fatalf("parse should succeed, bad filter should be skipped: %v", err)
	}
	// Bad filter should have been skipped
	if len(r.filters) != 0 {
		t.Errorf("expected 0 filters, got %d", len(r.filters))
	}
}

func TestHeadThenTail(t *testing.T) {
	// When both head_lines and tail_lines are set, head is applied first, then tail
	tomlData := `
schema_version = 1

[filters.both]
description = "head then tail"
match_command = "^both\\b"
head_lines = 5
tail_lines = 2
`
	r := mustRegistry(t, tomlData)
	input := "1\n2\n3\n4\n5\n6\n7\n8"
	result, ok := r.Apply("both", input)
	if !ok {
		t.Fatal("expected match")
	}
	// head_lines=5 → "1\n2\n3\n4\n5", then tail_lines=2 → "4\n5"
	if result != "4\n5" {
		t.Errorf("expected head then tail, got: %q", result)
	}
}

func TestReplaceChained(t *testing.T) {
	// Replacements are chained: output of first feeds into second
	tomlData := `
schema_version = 1

[filters.chain]
description = "chained replace"
match_command = "^chain\\b"
replace = [
  { pattern = "aaa", replacement = "bbb" },
  { pattern = "bbb", replacement = "ccc" },
]
`
	r := mustRegistry(t, tomlData)
	result, ok := r.Apply("chain", "aaa")
	if !ok {
		t.Fatal("expected match")
	}
	// aaa → bbb → ccc (chained on same line)
	if result != "ccc" {
		t.Errorf("expected chained replace, got: %q", result)
	}
}
