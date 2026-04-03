package outputfilter

import (
	"strings"
	"testing"
)

func TestGeneric_CollapseBlanks(t *testing.T) {
	input := "line1\n\n\n\n\nline2"
	got := Generic(input)
	if strings.Count(got, "\n\n") > 1 {
		t.Errorf("expected collapsed blanks, got:\n%s", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Error("expected both lines to be present")
	}
}

func TestGeneric_DedupLines(t *testing.T) {
	input := "ok\nok\nok\nok\nok\nok\nok\nok\nok\nok\ndone"
	got := Generic(input)
	if strings.Count(got, "ok\n") > 3 {
		t.Errorf("expected dedup, got:\n%s", got)
	}
	if !strings.Contains(got, "repeated") {
		t.Errorf("expected 'repeated' marker, got:\n%s", got)
	}
}

func TestGeneric_StripANSI(t *testing.T) {
	input := "\x1b[31mERROR\x1b[0m: something failed"
	got := Generic(input)
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI stripped, got: %q", got)
	}
	if !strings.Contains(got, "ERROR: something failed") {
		t.Errorf("expected clean text, got: %q", got)
	}
}

func TestGeneric_TruncateLong(t *testing.T) {
	long := strings.Repeat("x", 600)
	got := Generic(long)
	if len(got) > 510 {
		t.Errorf("expected truncated line, got length %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected ... suffix")
	}
}

func TestFilterGitPush_UpToDate(t *testing.T) {
	input := "Everything up-to-date"
	got := Filter("git push", input)
	if got != "ok (up-to-date)" {
		t.Errorf("expected 'ok (up-to-date)', got: %q", got)
	}
}

func TestFilterGitPush_WithProgress(t *testing.T) {
	input := `Enumerating objects: 5, done.
Counting objects: 100% (5/5), done.
Compressing objects: 100% (3/3), done.
Writing objects: 100% (3/3), 1.23 KiB | 1.23 MiB/s, done.
Total 3 (delta 2), reused 0 (delta 0)
remote: Resolving deltas: 100% (2/2), completed with 2 local objects.
To github.com/user/repo.git
   abc1234..def5678  main -> main`
	got := Filter("git push origin main", input)
	if strings.Contains(got, "Enumerating") {
		t.Error("expected progress lines stripped")
	}
	if !strings.Contains(got, "main -> main") {
		t.Errorf("expected branch result, got:\n%s", got)
	}
}

func TestFilterGitPull_UpToDate(t *testing.T) {
	got := Filter("git pull", "Already up to date.\n")
	if got != "ok (up-to-date)" {
		t.Errorf("expected 'ok (up-to-date)', got: %q", got)
	}
}

func TestFilterGitFetch_NoUpdates(t *testing.T) {
	got := Filter("git fetch", "")
	if got != "" {
		t.Errorf("expected empty, got: %q", got)
	}
}

func TestFilterGoBuild_Success(t *testing.T) {
	got := Filter("go build ./...", "")
	if got != "" {
		t.Errorf("expected empty for success, got: %q", got)
	}
}

func TestFilterGoBuild_Errors(t *testing.T) {
	input := `# github.com/user/repo/pkg
pkg/foo.go:10:5: undefined: bar
pkg/foo.go:15:2: cannot use x (type int) as type string`
	got := Filter("go build ./...", input)
	if !strings.Contains(got, "2 errors") {
		t.Errorf("expected error count, got:\n%s", got)
	}
	if !strings.Contains(got, "undefined: bar") {
		t.Errorf("expected error detail, got:\n%s", got)
	}
}

func TestFilterGoTest_Plain(t *testing.T) {
	input := `--- FAIL: TestSomething (0.00s)
    foo_test.go:42: expected 1, got 2
FAIL
FAIL	github.com/user/repo/pkg	0.005s
ok	github.com/user/repo/other	0.003s`
	got := Filter("go test ./...", input)
	if !strings.Contains(got, "FAIL: TestSomething") {
		t.Errorf("expected failure, got:\n%s", got)
	}
	if !strings.Contains(got, "ok") {
		t.Errorf("expected ok summary, got:\n%s", got)
	}
}

func TestFilterNpmInstall(t *testing.T) {
	input := `npm warn deprecated package@1.0.0: use something else
npm warn deprecated other@2.0.0: old

added 150 packages in 5s

10 packages are looking for funding
  run ` + "`npm fund`" + ` for details`
	got := Filter("npm install", input)
	if !strings.Contains(got, "added 150 packages") {
		t.Errorf("expected summary, got:\n%s", got)
	}
	if !strings.Contains(got, "deprecated") {
		t.Errorf("expected warnings, got:\n%s", got)
	}
}

func TestFilterCargoBuild_Success(t *testing.T) {
	input := `   Compiling foo v0.1.0
   Compiling bar v0.2.0
    Finished dev [unoptimized + debuginfo] target(s) in 2.34s`
	got := Filter("cargo build", input)
	if got != "Cargo build: Success" {
		t.Errorf("expected success, got: %q", got)
	}
}

func TestFilterCargoBuild_WithErrors(t *testing.T) {
	input := `   Compiling foo v0.1.0
error[E0308]: mismatched types
warning: unused variable: x`
	got := Filter("cargo build", input)
	if !strings.Contains(got, "1 errors") {
		t.Errorf("expected error count, got:\n%s", got)
	}
}

func TestFilter_UnknownCommand(t *testing.T) {
	input := "some output\nsome output\nsome output\nsome output\nsome output\ndone"
	got := Filter("unknowncmd --flag", input)
	// Should apply generic filter
	if !strings.Contains(got, "done") {
		t.Errorf("expected generic filtered output, got:\n%s", got)
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"git push", "git push"},
		{"FOO=bar git push", "git push"},
		{"FOO=bar BAZ=qux go test ./...", "go test ./..."},
		{"  cargo build  ", "cargo build"},
	}
	for _, tt := range tests {
		got := normalizeCommand(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFilterDockerBuild(t *testing.T) {
	input := `Step 1/5 : FROM golang:1.21
 ---> abc123
Step 2/5 : COPY . .
 ---> Using cache
 ---> def456
Downloading [===========>                        ] 45%
Step 3/5 : RUN go build
Successfully built abc123def`
	got := Filter("docker build .", input)
	if strings.Contains(got, "Downloading") {
		t.Error("expected download progress stripped")
	}
	if !strings.Contains(got, "Step 1/5") {
		t.Errorf("expected step headers, got:\n%s", got)
	}
	if !strings.Contains(got, "Successfully built") {
		t.Errorf("expected final line, got:\n%s", got)
	}
}

func TestFilter_EmptyOutput(t *testing.T) {
	got := Filter("git status", "")
	if got != "" {
		t.Errorf("expected empty, got: %q", got)
	}
}
