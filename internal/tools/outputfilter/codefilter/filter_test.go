package codefilter_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Abraxas-365/claudio/internal/tools/outputfilter/codefilter"
)

// ── DetectLanguage ────────────────────────────────────────────────────────────

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		ext  string
		want codefilter.Language
	}{
		{"go", codefilter.LangGo},
		{"GO", codefilter.LangGo}, // case-insensitive
		{"rs", codefilter.LangRust},
		{"py", codefilter.LangPython},
		{"pyw", codefilter.LangPython},
		{"js", codefilter.LangJavaScript},
		{"mjs", codefilter.LangJavaScript},
		{"cjs", codefilter.LangJavaScript},
		{"ts", codefilter.LangTypeScript},
		{"tsx", codefilter.LangTypeScript},
		{"c", codefilter.LangC},
		{"h", codefilter.LangC},
		{"cpp", codefilter.LangCpp},
		{"cc", codefilter.LangCpp},
		{"cxx", codefilter.LangCpp},
		{"hpp", codefilter.LangCpp},
		{"java", codefilter.LangJava},
		{"rb", codefilter.LangRuby},
		{"sh", codefilter.LangShell},
		{"bash", codefilter.LangShell},
		{"zsh", codefilter.LangShell},
		// Data formats — must never be filtered
		{"json", codefilter.LangData},
		{"jsonc", codefilter.LangData},
		{"yaml", codefilter.LangData},
		{"yml", codefilter.LangData},
		{"toml", codefilter.LangData},
		{"xml", codefilter.LangData},
		{"csv", codefilter.LangData},
		{"tsv", codefilter.LangData},
		{"graphql", codefilter.LangData},
		{"sql", codefilter.LangData},
		{"md", codefilter.LangData},
		{"markdown", codefilter.LangData},
		{"txt", codefilter.LangData},
		{"env", codefilter.LangData},
		{"lock", codefilter.LangData},
		// Unknown
		{"", codefilter.LangUnknown},
		{"xyz", codefilter.LangUnknown},
		{"wasm", codefilter.LangUnknown},
	}
	for _, c := range cases {
		got := codefilter.DetectLanguage(c.ext)
		if got != c.want {
			t.Errorf("DetectLanguage(%q) = %v, want %v", c.ext, got, c.want)
		}
	}
}

// ── MinimalFilter — Data formats are never touched ────────────────────────────

// Regression test: JSON package-lock style files must NEVER be comment-stripped
// even though they may contain keys like "resolved" that look like code.
func TestMinimalFilter_JSONNotStripped(t *testing.T) {
	input := `{
  "packages": {
    "node_modules/example": {
      "version": "1.0.0",
      "resolved": "https://registry.npmjs.org/example/-/example-1.0.0.tgz",
      "integrity": "sha512-abc123"
    }
  },
  "dependencies": {
    "example": {
      "version": "1.0.0"
    }
  }
}`
	got := codefilter.MinimalFilter(input, codefilter.LangData)
	if got != input {
		t.Error("MinimalFilter must not modify LangData files")
	}
}

func TestMinimalFilter_YAMLNotStripped(t *testing.T) {
	input := `name: my-app
version: 1.0.0
dependencies:
  foo: "^1.2.3"
  bar: "~2.0"
`
	got := codefilter.MinimalFilter(input, codefilter.LangData)
	if got != input {
		t.Error("MinimalFilter must not modify YAML (LangData) files")
	}
}

func TestMinimalFilter_TOMLNotStripped(t *testing.T) {
	input := `[package]
name = "my-crate"
version = "0.1.0"

[dependencies]
serde = "1.0"
`
	got := codefilter.MinimalFilter(input, codefilter.LangData)
	if got != input {
		t.Error("MinimalFilter must not modify TOML (LangData) files")
	}
}

// ── MinimalFilter — Go ─────────────────────────────────────────────────────────

func TestMinimalFilter_GoStripsLineComments(t *testing.T) {
	input := `package main

import "fmt"

// This standalone comment should be stripped.
func main() {
	// Another comment to strip.
	fmt.Println("hello") // inline comment stays
}
`
	got := codefilter.MinimalFilter(input, codefilter.LangGo)

	if strings.Contains(got, "This standalone comment should be stripped") {
		t.Error("expected standalone // comment to be stripped")
	}
	if strings.Contains(got, "Another comment to strip") {
		t.Error("expected indented // comment to be stripped")
	}
	// Code lines must survive.
	if !strings.Contains(got, `fmt.Println("hello")`) {
		t.Error("expected code line to be kept")
	}
	if !strings.Contains(got, "func main()") {
		t.Error("expected function signature to be kept")
	}
}

func TestMinimalFilter_GoStripsBlockComments(t *testing.T) {
	input := `package main

/* This block comment
   spans multiple lines. */
func main() {}
`
	got := codefilter.MinimalFilter(input, codefilter.LangGo)
	if strings.Contains(got, "This block comment") {
		t.Error("expected block comment content to be stripped")
	}
	if strings.Contains(got, "spans multiple lines") {
		t.Error("expected block comment content to be stripped")
	}
	if !strings.Contains(got, "func main()") {
		t.Error("expected function signature to be kept")
	}
}

func TestMinimalFilter_GoSingleLineBlockComment(t *testing.T) {
	input := `package main

/* single line block comment */
var x = 1
`
	got := codefilter.MinimalFilter(input, codefilter.LangGo)
	if strings.Contains(got, "single line block comment") {
		t.Error("expected single-line block comment to be stripped")
	}
	if !strings.Contains(got, "var x = 1") {
		t.Error("expected var declaration to be kept")
	}
}

func TestMinimalFilter_GoCollapsesBlankLines(t *testing.T) {
	input := "package main\n\n\n\n\nfunc main() {}\n"
	got := codefilter.MinimalFilter(input, codefilter.LangGo)
	lines := strings.Split(got, "\n")
	blanks := 0
	maxBlanks := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blanks++
			if blanks > maxBlanks {
				maxBlanks = blanks
			}
		} else {
			blanks = 0
		}
	}
	if maxBlanks > 2 {
		t.Errorf("expected at most 2 consecutive blank lines, got %d", maxBlanks)
	}
}

// ── MinimalFilter — Rust keeps doc comments ──────────────────────────────────

func TestMinimalFilter_RustKeepsDocComments(t *testing.T) {
	input := `// This plain comment should be stripped.
/// This doc comment should be kept.
fn foo() {}
`
	got := codefilter.MinimalFilter(input, codefilter.LangRust)

	if strings.Contains(got, "plain comment should be stripped") {
		t.Error("expected plain // comment to be stripped")
	}
	if !strings.Contains(got, "/// This doc comment should be kept") {
		t.Error("expected /// doc comment to be kept")
	}
}

func TestMinimalFilter_RustStripsBlockComment(t *testing.T) {
	input := `/* block comment */
fn bar() {}
`
	got := codefilter.MinimalFilter(input, codefilter.LangRust)
	if strings.Contains(got, "block comment") {
		t.Error("expected block comment to be stripped")
	}
	if !strings.Contains(got, "fn bar()") {
		t.Error("expected function to be kept")
	}
}

// ── MinimalFilter — Python ───────────────────────────────────────────────────

func TestMinimalFilter_PythonStripsHashComments(t *testing.T) {
	input := `# This comment should be stripped
def foo():
    # Another stripped comment
    return 42
`
	got := codefilter.MinimalFilter(input, codefilter.LangPython)
	if strings.Contains(got, "This comment should be stripped") {
		t.Error("expected # comment to be stripped")
	}
	if strings.Contains(got, "Another stripped comment") {
		t.Error("expected # comment to be stripped")
	}
	if !strings.Contains(got, "def foo():") {
		t.Error("expected function signature to be kept")
	}
}

func TestMinimalFilter_PythonKeepsDocstrings(t *testing.T) {
	input := `def foo():
    """This is a docstring and must be kept."""
    return 42

def bar():
    """
    Multi-line docstring.
    Also kept.
    """
    pass
`
	got := codefilter.MinimalFilter(input, codefilter.LangPython)
	if !strings.Contains(got, "This is a docstring") {
		t.Error("expected single-line docstring to be kept")
	}
	if !strings.Contains(got, "Multi-line docstring") {
		t.Error("expected multi-line docstring to be kept")
	}
}

// ── AggressiveFilter ──────────────────────────────────────────────────────────

func TestAggressiveFilter_DataUsesMinimal(t *testing.T) {
	input := `{"key": "value", "num": 42}`
	got := codefilter.AggressiveFilter(input, codefilter.LangData)
	if got != input {
		t.Error("AggressiveFilter on LangData must not modify content")
	}
}

func TestAggressiveFilter_GoKeepsSignaturesAndImports(t *testing.T) {
	input := `package main

import (
	"fmt"
	"os"
)

// StripMe is a comment.
func main() {
	fmt.Println("hello")
	x := 5
	_ = x
}

type Config struct {
	Name string
	Age  int
}

func helper(x int) int {
	return x + 1
}

const MaxRetries = 3
`
	got := codefilter.AggressiveFilter(input, codefilter.LangGo)

	// Imports must be kept.
	if !strings.Contains(got, "import") {
		t.Error("expected import block to be kept")
	}
	// Function signatures must be kept.
	if !strings.Contains(got, "func main()") {
		t.Error("expected func main() signature to be kept")
	}
	if !strings.Contains(got, "func helper(x int) int") {
		t.Error("expected func helper signature to be kept")
	}
	// Implementation bodies must be replaced.
	if !strings.Contains(got, "// ... implementation") {
		t.Error("expected implementation placeholder")
	}
	// Body content must be gone.
	if strings.Contains(got, `fmt.Println("hello")`) {
		t.Error("expected function body content to be removed")
	}
	if strings.Contains(got, "x := 5") {
		t.Error("expected function body content to be removed")
	}
	// Const declaration must be kept.
	if !strings.Contains(got, "MaxRetries") {
		t.Error("expected const declaration to be kept")
	}
}

func TestAggressiveFilter_GoStructFieldsKept(t *testing.T) {
	// Struct bodies are type declarations — fields should be preserved.
	input := `package main

type Point struct {
	X int
	Y int
}
`
	got := codefilter.AggressiveFilter(input, codefilter.LangGo)
	if !strings.Contains(got, "type Point struct") {
		t.Error("expected struct signature to be kept")
	}
	// Fields should survive because struct bodies are not function bodies.
	if !strings.Contains(got, "X int") {
		t.Error("expected struct fields to be kept")
	}
}

// ── SmartTruncate ─────────────────────────────────────────────────────────────

func TestSmartTruncate_ShortContentUnchanged(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5"
	got := codefilter.SmartTruncate(content, 10, codefilter.LangGo)
	if got != content {
		t.Error("SmartTruncate should return unchanged content when under maxLines")
	}
}

func TestSmartTruncate_ExactMaxLinesUnchanged(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	content := strings.Join(lines, "\n")
	got := codefilter.SmartTruncate(content, 10, codefilter.LangGo)
	if got != content {
		t.Error("SmartTruncate should return unchanged content when equal to maxLines")
	}
}

// TestSmartTruncate_CountAccuracy verifies that N + originalKept == total
// where N is the number in the final "// ... N more lines (total: T)" marker.
func TestSmartTruncate_CountAccuracy(t *testing.T) {
	// Build a 200-line Go file with varying content so SmartTruncate has real
	// lines to classify as important vs not.
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	sb.WriteString("import \"fmt\"\n\n")
	for i := 0; i < 48; i++ {
		fmt.Fprintf(&sb, "var x%d = %d\n", i, i)
	}
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, "func helper%d(x int) int {\n\treturn x + %d\n}\n\n", i, i)
	}
	content := strings.TrimRight(sb.String(), "\n")

	totalLines := len(strings.Split(content, "\n"))
	if totalLines < 100 {
		t.Fatalf("test setup error: only %d lines generated", totalLines)
	}

	maxLines := 20
	result := codefilter.SmartTruncate(content, maxLines, codefilter.LangGo)
	resultLines := strings.Split(result, "\n")

	// Find the final overflow marker.
	var markerN, markerTotal int
	markerFound := false
	for _, l := range resultLines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "// ...") && strings.Contains(trimmed, "more lines (total:") {
			n, err := fmt.Sscanf(trimmed, "// ... %d more lines (total: %d)", &markerN, &markerTotal)
			if n == 2 && err == nil {
				markerFound = true
			}
		}
	}
	if !markerFound {
		t.Fatal("expected overflow marker '// ... N more lines (total: T)' not found in output")
	}
	if markerTotal != totalLines {
		t.Errorf("marker total = %d, want %d", markerTotal, totalLines)
	}

	// Count original lines kept (exclude virtual markers).
	originalKept := 0
	for _, l := range resultLines {
		trimmed := strings.TrimSpace(l)
		isMarker := (strings.HasPrefix(trimmed, "// ...") && strings.Contains(trimmed, "lines omitted")) ||
			(strings.HasPrefix(trimmed, "// ...") && strings.Contains(trimmed, "more lines (total:"))
		if !isMarker {
			originalKept++
		}
	}

	if markerN+originalKept != totalLines {
		t.Errorf("N(%d) + kept(%d) = %d, want %d (total)",
			markerN, originalKept, markerN+originalKept, totalLines)
	}
}

func TestSmartTruncate_MarkerAppearsWhenOverLimit(t *testing.T) {
	// 30 plain lines, maxLines = 10 → should have a final marker.
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("plain line %d", i+1)
	}
	content := strings.Join(lines, "\n")

	result := codefilter.SmartTruncate(content, 10, codefilter.LangGo)
	if !strings.Contains(result, "more lines (total:") {
		t.Error("expected overflow marker when content exceeds maxLines")
	}
}

// ── LangData is never filtered regardless of level ────────────────────────────

func TestDataLanguageNeverFiltered_Minimal(t *testing.T) {
	for _, ext := range []string{"json", "yaml", "toml", "xml", "csv", "lock", "md"} {
		lang := codefilter.DetectLanguage(ext)
		if lang != codefilter.LangData {
			t.Errorf("DetectLanguage(%q) should be LangData", ext)
			continue
		}
		input := fmt.Sprintf(`{"comment": "// not a code comment", "ext": "%s"}`, ext)
		got := codefilter.MinimalFilter(input, lang)
		if got != input {
			t.Errorf("MinimalFilter modified LangData file (ext=%s)", ext)
		}
	}
}

func TestDataLanguageNeverFiltered_Aggressive(t *testing.T) {
	input := `{
  "name": "test-package",
  "version": "1.0.0",
  "description": "// this looks like a comment but is a JSON value",
  "scripts": {
    "build": "tsc"
  }
}`
	got := codefilter.AggressiveFilter(input, codefilter.LangData)
	if got != input {
		t.Error("AggressiveFilter must not modify LangData files")
	}
}

// ── JavaScript / TypeScript ───────────────────────────────────────────────────

func TestMinimalFilter_JSStripsComments(t *testing.T) {
	input := `// top-level comment
import { foo } from './foo';

/* block comment */
function bar() {
  // body comment
  return foo();
}
`
	got := codefilter.MinimalFilter(input, codefilter.LangJavaScript)
	if strings.Contains(got, "top-level comment") {
		t.Error("expected top-level // comment stripped")
	}
	if strings.Contains(got, "block comment") {
		t.Error("expected /* block comment */ stripped")
	}
	if !strings.Contains(got, "import") {
		t.Error("expected import line kept")
	}
	if !strings.Contains(got, "function bar()") {
		t.Error("expected function signature kept")
	}
}
