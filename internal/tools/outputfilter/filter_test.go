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

// ── GitHub CLI ───────────────────────────────────────────────────────────

func TestFilterGhPr_List(t *testing.T) {
	input := `
#123  Fix login bug         feature/login  OPEN
#122  Update dependencies   chore/deps     OPEN
#121  Add dark mode         feature/dark   MERGED
`
	got := Filter("gh pr list", input)
	if !strings.Contains(got, "#123") {
		t.Errorf("expected PR list, got:\n%s", got)
	}
	if !strings.Contains(got, "OPEN") {
		t.Errorf("expected status in output, got:\n%s", got)
	}
}

func TestFilterGhPr_Empty(t *testing.T) {
	got := Filter("gh pr list", "")
	if got != "" {
		t.Errorf("expected empty for empty input, got: %q", got)
	}
}

func TestFilterGhIssue_List(t *testing.T) {
	input := `#45  Bug: crash on startup   bug      OPEN
#44  Feature: dark mode       feature  OPEN
#43  Docs: update README      docs     CLOSED`
	got := Filter("gh issue list", input)
	if !strings.Contains(got, "#45") {
		t.Errorf("expected issue list, got:\n%s", got)
	}
}

func TestFilterGhIssue_Empty(t *testing.T) {
	got := Filter("gh issue list", "")
	if got != "" {
		t.Errorf("expected empty for empty input, got: %q", got)
	}
}

func TestFilterGhRun_List(t *testing.T) {
	input := `STATUS      NAME            WORKFLOW  BRANCH  EVENT  ID
completed   CI              ci.yml    main    push   123456
in_progress Build           build.yml main    push   123457`
	got := Filter("gh run list", input)
	if !strings.Contains(got, "completed") && !strings.Contains(got, "STATUS") {
		t.Errorf("expected run status, got:\n%s", got)
	}
}

// ── AWS CLI ──────────────────────────────────────────────────────────────

func TestFilterAwsSts_GetCallerIdentity(t *testing.T) {
	input := `{
    "UserId": "AIDAXXXXXXXXXXXXXXXXX",
    "Account": "123456789012",
    "Arn": "arn:aws:iam::123456789012:user/alice",
    "ResponseMetadata": {
        "RequestId": "abc-123",
        "HTTPStatusCode": 200,
        "HTTPHeaders": {}
    }
}`
	got := Filter("aws sts get-caller-identity", input)
	if strings.Contains(got, "ResponseMetadata") {
		t.Errorf("expected ResponseMetadata stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "Account") {
		t.Errorf("expected Account preserved, got:\n%s", got)
	}
}

func TestFilterAwsSts_Empty(t *testing.T) {
	// Filter() short-circuits on empty input — same as all other commands
	got := Filter("aws sts get-caller-identity", "")
	if got != "" {
		t.Errorf("expected empty for empty input, got: %q", got)
	}
}

func TestFilterAwsS3_Ls(t *testing.T) {
	input := `2024-01-01 10:00:00       1234 file1.txt
2024-01-02 11:00:00       5678 file2.txt
2024-01-03 12:00:00      91234 archive.zip`
	got := Filter("aws s3 ls", input)
	if !strings.Contains(got, "file1.txt") {
		t.Errorf("expected file listing, got:\n%s", got)
	}
}

func TestFilterAwsLogs_FilterLogEvents(t *testing.T) {
	input := `{
    "events": [
        {"timestamp": 1234567890000, "message": "ERROR: connection refused", "ingestionTime": 111},
        {"timestamp": 1234567890001, "message": "INFO: request completed", "ingestionTime": 112}
    ],
    "ResponseMetadata": {"RequestId": "xyz"}
}`
	got := Filter("aws logs filter-log-events", input)
	if !strings.Contains(got, "connection refused") {
		t.Errorf("expected log message, got:\n%s", got)
	}
	if strings.Contains(got, "ResponseMetadata") {
		t.Errorf("expected ResponseMetadata stripped, got:\n%s", got)
	}
}

// ── kubectl ──────────────────────────────────────────────────────────────

func TestFilterKubectlGet_Pods(t *testing.T) {
	input := `NAME                          READY   STATUS    RESTARTS   AGE
nginx-7848d4b86f-xk9mn        1/1     Running   0          2d
postgres-5d9f4b7c8-qr2st      1/1     Running   0          5d
redis-6c8b4f9d7-mn3uv         0/1     Pending   0          1m`
	got := Filter("kubectl get pods", input)
	if !strings.Contains(got, "NAME") {
		t.Errorf("expected header row, got:\n%s", got)
	}
	if !strings.Contains(got, "Running") {
		t.Errorf("expected pod status, got:\n%s", got)
	}
}

func TestFilterKubectlGet_Empty(t *testing.T) {
	got := Filter("kubectl get pods", "No resources found.")
	if got == "" {
		t.Error("expected non-empty output")
	}
}

func TestFilterKubectlLogs_Tail(t *testing.T) {
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "2024-01-01T00:00:00Z INFO log line "+itoa(i))
	}
	input := strings.Join(lines, "\n")
	got := Filter("kubectl logs my-pod", input)
	if !strings.Contains(got, "last 50") {
		t.Errorf("expected tail indicator, got:\n%s", got)
	}
	// Should not contain the first line
	if strings.Contains(got, "log line 0\n") {
		t.Errorf("expected early lines truncated, got first line in output")
	}
}

func TestFilterKubectlDescribe_Pod(t *testing.T) {
	input := `Name:             nginx-7848d4b86f-xk9mn
Namespace:        default
Labels:           app=nginx
Annotations:      kubernetes.io/created-by: {"kind":"SerializedReference"}
                  another-annotation: value
                  third-annotation: value2
                  fourth-annotation: value3
Status:           Running
IP:               10.0.0.1
ManagedFields:
  manager: kubectl
  operation: Update
  apiVersion: v1
Containers:
  nginx:
    Image:    nginx:1.21
    Port:     80/TCP`
	got := Filter("kubectl describe pod nginx", input)
	if !strings.Contains(got, "Name:") {
		t.Errorf("expected Name field, got:\n%s", got)
	}
	if !strings.Contains(got, "Status:") {
		t.Errorf("expected Status field, got:\n%s", got)
	}
	// ManagedFields should be stripped
	if strings.Contains(got, "ManagedFields") {
		t.Errorf("expected ManagedFields stripped, got:\n%s", got)
	}
}

// ── Ruby ─────────────────────────────────────────────────────────────────

func TestFilterRspec_Failures(t *testing.T) {
	input := `..F.

Failures:

1) UserModel#validate fails when email is blank
   Failure/Error: expect(user.valid?).to eq(true)

     expected: true
          got: false

   # ./spec/models/user_spec.rb:15:in 'block (3 levels) in <top (required)>'

2 examples, 1 failure

Finished in 0.05432 seconds
`
	got := Filter("rspec spec/models/user_spec.rb", input)
	if !strings.Contains(got, "1)") {
		t.Errorf("expected failure detail, got:\n%s", got)
	}
	if !strings.Contains(got, "example") {
		t.Errorf("expected summary, got:\n%s", got)
	}
	// Passing dots should not dominate output
	if strings.Contains(got, "..F.") {
		t.Errorf("expected progress dots stripped, got:\n%s", got)
	}
}

func TestFilterRspec_AllPassing(t *testing.T) {
	input := `....

Finished in 0.1234 seconds
4 examples, 0 failures
`
	got := Filter("rspec", input)
	if !strings.Contains(got, "example") {
		t.Errorf("expected summary line, got:\n%s", got)
	}
	// Should not show passing dots
	if strings.Contains(got, "....") {
		t.Errorf("expected passing dots stripped, got:\n%s", got)
	}
}

func TestFilterRubocop_Offenses(t *testing.T) {
	input := `Inspecting 3 files
...

3 files inspected, 2 offenses detected
app/models/user.rb:10:5: C: Style/StringLiterals: Prefer single-quoted strings when you don't need string interpolation or special symbols.
app/controllers/home.rb:25:3: W: Lint/UnusedMethodArgument: Unused method argument - request.
`
	got := Filter("rubocop app/", input)
	if !strings.Contains(got, "user.rb") {
		t.Errorf("expected offense detail, got:\n%s", got)
	}
	if !strings.Contains(got, "inspected") {
		t.Errorf("expected summary, got:\n%s", got)
	}
	// Progress line should be stripped
	if strings.Contains(got, "Inspecting") {
		t.Errorf("expected Inspecting progress stripped, got:\n%s", got)
	}
}

func TestFilterRubocop_NoOffenses(t *testing.T) {
	input := `Inspecting 5 files
.....

5 files inspected, no offenses detected
`
	got := Filter("rubocop .", input)
	if !strings.Contains(got, "no offenses") && !strings.Contains(got, "No offenses") && !strings.Contains(got, "no offense") {
		t.Errorf("expected no-offenses message, got:\n%s", got)
	}
}

func TestFilterBundle_Install(t *testing.T) {
	input := `Fetching gem metadata from https://rubygems.org/..........
Fetching rails 7.1.0
Installing rails 7.1.0
Fetching activerecord 7.1.0
Installing activerecord 7.1.0
Bundle complete! 15 gems now installed.
Use ` + "`bundle info [gemname]`" + ` to see where a bundled gem is installed.`
	got := Filter("bundle install", input)
	if !strings.Contains(got, "Bundle complete") {
		t.Errorf("expected completion message, got:\n%s", got)
	}
	// Fetching/Installing lines should be stripped
	if strings.Contains(got, "Fetching rails") {
		t.Errorf("expected fetching progress stripped, got:\n%s", got)
	}
}

func TestFilterBundle_UnknownSubcommand(t *testing.T) {
	// bundle exec should fall through to generic
	input := "hello world"
	got := Filter("bundle exec rspec", input)
	if got == "" {
		t.Error("expected non-empty output")
	}
}

// ── Prisma ───────────────────────────────────────────────────────────────

func TestFilterPrismaGenerate_Success(t *testing.T) {
	input := `Environment variables loaded from .env
Prisma schema loaded from prisma/schema.prisma
✔ Generated Prisma Client (v5.7.0) to ./node_modules/@prisma/client in 234ms
`
	got := Filter("prisma generate", input)
	if strings.Contains(got, "Environment variables loaded") {
		t.Errorf("expected env load line stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "Generated") {
		t.Errorf("expected generated line, got:\n%s", got)
	}
}

func TestFilterPrismaGenerate_Empty(t *testing.T) {
	got := Filter("prisma generate", "")
	if got != "" {
		t.Errorf("expected empty for empty input, got: %q", got)
	}
}

func TestFilterPrismaMigrate_Dev(t *testing.T) {
	input := `Environment variables loaded from .env
Prisma schema loaded from prisma/schema.prisma

Drift detected: Your database schema is not in sync with your migration history.

The following migration(s) are applied to the database but missing from the migration history:
- 20240101000000_init

✔ Your database is now in sync with your schema.
`
	got := Filter("prisma migrate dev", input)
	if strings.Contains(got, "Environment variables loaded") {
		t.Errorf("expected env load line stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "migration") {
		t.Errorf("expected migration info, got:\n%s", got)
	}
}

func TestFilterPrismaDb_Push(t *testing.T) {
	input := `Environment variables loaded from .env
Prisma schema loaded from prisma/schema.prisma
The following changes will be applied to the database schema:
- Create "User" table
- Create "Post" table
✔ The database schema has been applied successfully.
`
	got := Filter("prisma db push", input)
	if !strings.Contains(got, "schema") {
		t.Errorf("expected schema reference, got:\n%s", got)
	}
	if strings.Contains(got, "Environment variables loaded") {
		t.Errorf("expected env line stripped, got:\n%s", got)
	}
}
