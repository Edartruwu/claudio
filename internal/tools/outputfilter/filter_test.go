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

// ── Terraform ────────────────────────────────────────────────────────────

func TestFilterTerraformPlan_WithChanges(t *testing.T) {
	input := `Terraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:
  + create
  - destroy
  ~ update in-place

Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami           = "ami-0c55b159cbfafe1f0"
      + instance_type = "t2.micro"
    }

  # aws_security_group.example will be created
  + resource "aws_security_group" "example" {
      + name = "example"
    }

  # aws_instance.old will be destroyed
  - resource "aws_instance" "old" {
      - id = "i-0123456789abcdef0"
    }

Plan: 2 to add, 0 to change, 1 to destroy.`
	got := Filter("terraform plan", input)
	// Should keep the Plan summary
	if !strings.Contains(got, "Plan: 2 to add") {
		t.Errorf("expected Plan summary, got:\n%s", got)
	}
	// Should keep resource lines with +/- prefix
	if !strings.Contains(got, "+ resource") {
		t.Errorf("expected resource creation line, got:\n%s", got)
	}
	if !strings.Contains(got, "- resource") {
		t.Errorf("expected resource destruction line, got:\n%s", got)
	}
	// Should strip execution plan header
	if strings.Contains(got, "execution plan") {
		t.Errorf("expected execution plan line stripped, got:\n%s", got)
	}
}

func TestFilterTerraformPlan_WithRefreshingState(t *testing.T) {
	input := `Refreshing state... [id=i-0123456789abcdef0]

Terraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:

  # aws_instance.example will be updated in-place
  ~ resource "aws_instance" "example" {
      ~ tags = {
          + "environment" = "prod"
        }
    }

Plan: 0 to add, 1 to change, 0 to destroy.`
	got := Filter("terraform plan", input)
	// Should strip refreshing state
	if strings.Contains(got, "Refreshing state") {
		t.Errorf("expected Refreshing state stripped, got:\n%s", got)
	}
	// Should keep Plan summary
	if !strings.Contains(got, "Plan: 0 to add, 1 to change") {
		t.Errorf("expected Plan summary, got:\n%s", got)
	}
}

func TestFilterTerraformApply_Success(t *testing.T) {
	input := `aws_instance.example: Creating...
aws_instance.example: Creation complete after 15s [id=i-0123456789abcdef0]
aws_security_group.example: Creating...
aws_security_group.example: Creation complete after 2s [id=sg-0123456789abcdef0]

Apply complete! Resources: 2 added, 0 changed, 0 destroyed.`
	got := Filter("terraform apply", input)
	// Should keep creation complete lines
	if !strings.Contains(got, "Creation complete") {
		t.Errorf("expected completion line, got:\n%s", got)
	}
	// Should keep apply summary
	if !strings.Contains(got, "Apply complete") {
		t.Errorf("expected apply summary, got:\n%s", got)
	}
}

func TestFilterTerraformInit_Success(t *testing.T) {
	input := `Initializing the backend...

Initializing provider plugins
- Finding hashicorp/aws versions matching ">= 5.0"...
- Installing hashicorp/aws v5.0.0...
- Installed hashicorp/aws v5.0.0 (signed by HashiCorp)

Terraform has been successfully initialized!

You may now begin working with Terraform. Try running "terraform plan" to see
any changes that would be made to your infrastructure. If you approve those
changes, you can apply them with "terraform apply".`
	got := Filter("terraform init", input)
	// Should only keep the success message
	if !strings.Contains(got, "successfully initialized") {
		t.Errorf("expected success message, got:\n%s", got)
	}
	// Should strip provider downloading/installation
	if strings.Contains(got, "Downloading") || strings.Contains(got, "Installing") {
		t.Errorf("expected provider lines stripped, got:\n%s", got)
	}
	// Should strip version finding
	if strings.Contains(got, "Finding hashicorp/aws") {
		t.Errorf("expected version finding stripped, got:\n%s", got)
	}
}

func TestFilterTerraformInit_WithError(t *testing.T) {
	input := `Initializing the backend...

Initializing provider plugins
- Finding hashicorp/aws versions matching ">= 5.0"...

Error: Failed to download provider

Could not download hashicorp/aws: Failed to fetch auth token`
	got := Filter("terraform init", input)
	// Should keep error message
	if !strings.Contains(got, "Error:") {
		t.Errorf("expected error message, got:\n%s", got)
	}
	// Should strip version finding
	if strings.Contains(got, "Finding hashicorp/aws") {
		t.Errorf("expected version finding stripped, got:\n%s", got)
	}
}

func TestFilterTerraformValidate_Success(t *testing.T) {
	input := `Success! The configuration is valid.`
	got := Filter("terraform validate", input)
	if !strings.Contains(got, "Success") {
		t.Errorf("expected success message, got:\n%s", got)
	}
}

func TestFilterTerraformDestroy_Success(t *testing.T) {
	input := `aws_instance.example: Destroying... [id=i-0123456789abcdef0]
aws_instance.example: Destruction complete after 30s

Destroy complete! Resources: 1 destroyed.`
	got := Filter("terraform destroy", input)
	// Should keep destruction lines
	if !strings.Contains(got, "Destruction complete") {
		t.Errorf("expected destruction line, got:\n%s", got)
	}
	// Should keep destroy summary
	if !strings.Contains(got, "Destroy complete") {
		t.Errorf("expected destroy summary, got:\n%s", got)
	}
}

// ── Ansible ─────────────────────────────────────────────────────────────

func TestFilterAnsible_SuccessfulPlaybook(t *testing.T) {
	input := `PLAY [webservers] ************************************************************

TASK [Gathering Facts] ********************************************************
ok: [web1]
ok: [web2]

TASK [Install nginx] **********************************************************
ok: [web1]
ok: [web2]

TASK [Start nginx] ************************************************************
ok: [web1]
ok: [web2]

PLAY RECAP ********************************************************************
web1                       : ok=3    changed=0    unreachable=0    failed=0
web2                       : ok=3    changed=0    unreachable=0    failed=0`

	got := Filter("ansible-playbook site.yml", input)
	if !strings.Contains(got, "PLAY [webservers]") {
		t.Errorf("expected PLAY header, got:\n%s", got)
	}
	if !strings.Contains(got, "PLAY RECAP") {
		t.Errorf("expected PLAY RECAP, got:\n%s", got)
	}
	if strings.Contains(got, "ok: [web1]") {
		t.Errorf("expected ok: lines stripped, got:\n%s", got)
	}
	if strings.Contains(got, "Gathering Facts") {
		t.Errorf("expected Gathering Facts stripped, got:\n%s", got)
	}
	if strings.Contains(got, "TASK [Install nginx]") {
		t.Errorf("expected ok-only TASK headers stripped, got:\n%s", got)
	}
}

func TestFilterAnsible_WithFailures(t *testing.T) {
	input := `PLAY [webservers] ************************************************************

TASK [Gathering Facts] ********************************************************
ok: [web1]

TASK [Deploy app] *************************************************************
changed: [web1] => {"changed": true, "dest": "/opt/app"}

TASK [Restart service] ********************************************************
fatal: [web1]: FAILED! => {"changed": false, "msg": "Service not found"}

PLAY RECAP ********************************************************************
web1                       : ok=1    changed=1    unreachable=0    failed=1`

	got := Filter("ansible-playbook deploy.yml", input)
	if !strings.Contains(got, "fatal:") {
		t.Errorf("expected fatal line kept, got:\n%s", got)
	}
	if !strings.Contains(got, "TASK [Restart service]") {
		t.Errorf("expected failed task header kept, got:\n%s", got)
	}
	if !strings.Contains(got, "TASK [Deploy app]") {
		t.Errorf("expected changed task header kept, got:\n%s", got)
	}
	if !strings.Contains(got, "changed: [web1]") {
		t.Errorf("expected changed line kept, got:\n%s", got)
	}
	// JSON blob should be stripped from changed line
	if strings.Contains(got, `"dest"`) {
		t.Errorf("expected JSON blob stripped from changed line, got:\n%s", got)
	}
	if !strings.Contains(got, "PLAY RECAP") {
		t.Errorf("expected recap kept, got:\n%s", got)
	}
}

// ── ESLint ──────────────────────────────────────────────────────────────

func TestFilterEslint_WithErrors(t *testing.T) {
	input := `/home/user/project/src/App.js
  3:10  error  'foo' is defined but never used  no-unused-vars
  7:5   warning  Unexpected console statement     no-console

/home/user/project/src/utils.js
  12:1  error  'bar' is not defined  no-undef

✖ 3 problems (2 errors, 1 warning)`

	got := Filter("eslint src/", input)
	if !strings.Contains(got, "no-unused-vars") {
		t.Errorf("expected error detail, got:\n%s", got)
	}
	if !strings.Contains(got, "no-undef") {
		t.Errorf("expected error detail, got:\n%s", got)
	}
	if !strings.Contains(got, "3 problems") {
		t.Errorf("expected summary, got:\n%s", got)
	}
}

func TestFilterEslint_NoIssues(t *testing.T) {
	got := Filter("eslint src/", "")
	if got != "" {
		t.Errorf("expected empty for empty input, got: %q", got)
	}
}

// ── TSC ─────────────────────────────────────────────────────────────────

func TestFilterTsc_WithErrors(t *testing.T) {
	input := `src/App.tsx(10,5): error TS2304: Cannot find name 'foo'.
src/utils.ts(25,10): error TS7006: Parameter 'x' implicitly has an 'any' type.

Found 2 errors in 2 files.`

	got := Filter("tsc --noEmit", input)
	if !strings.Contains(got, "error TS2304") {
		t.Errorf("expected TS error, got:\n%s", got)
	}
	if !strings.Contains(got, "error TS7006") {
		t.Errorf("expected TS error, got:\n%s", got)
	}
	if !strings.Contains(got, "Found 2 errors") {
		t.Errorf("expected error count, got:\n%s", got)
	}
}

func TestFilterTsc_WatchMode(t *testing.T) {
	input := `Starting compilation in watch mode...

src/index.ts(5,3): error TS2322: Type 'string' is not assignable to type 'number'.

Watching for file changes.

Found 1 error.`

	got := Filter("tsc --watch", input)
	if strings.Contains(got, "Starting compilation") {
		t.Errorf("expected watch mode line stripped, got:\n%s", got)
	}
	if strings.Contains(got, "Watching for file changes") {
		t.Errorf("expected watch status stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "error TS2322") {
		t.Errorf("expected error kept, got:\n%s", got)
	}
	if !strings.Contains(got, "Found 1 error") {
		t.Errorf("expected error count, got:\n%s", got)
	}
}

// ── Curl ────────────────────────────────────────────────────────────────

func TestFilterCurl_Verbose(t *testing.T) {
	input := `* Connected to example.com (93.184.216.34) port 443
* TLS 1.3 connection established
> GET /api/data HTTP/1.1
> Host: example.com
> User-Agent: curl/8.1.2
> Accept: */*
> 
< HTTP/1.1 200 OK
< Date: Mon, 01 Jan 2024 00:00:00 GMT
< Content-Type: application/json
< Server: nginx
< X-Request-Id: abc123
< 
{"status":"ok","data":[1,2,3]}`

	got := Filter("curl -v https://example.com/api/data", input)
	if !strings.Contains(got, "HTTP/1.1 200 OK") {
		t.Errorf("expected HTTP status line, got:\n%s", got)
	}
	if !strings.Contains(got, "Content-Type: application/json") {
		t.Errorf("expected Content-Type, got:\n%s", got)
	}
	if !strings.Contains(got, `{"status":"ok"`) {
		t.Errorf("expected response body, got:\n%s", got)
	}
	if strings.Contains(got, "Connected to") {
		t.Errorf("expected connection info stripped, got:\n%s", got)
	}
	if strings.Contains(got, "User-Agent") {
		t.Errorf("expected request headers stripped, got:\n%s", got)
	}
	if strings.Contains(got, "X-Request-Id") {
		t.Errorf("expected noise headers stripped, got:\n%s", got)
	}
}

func TestFilterCurl_NonVerbose(t *testing.T) {
	input := `{"status":"ok","data":"hello world"}`
	got := Filter("curl https://example.com/api/data", input)
	if got != input {
		t.Errorf("expected passthrough for non-verbose, got:\n%s", got)
	}
}

func TestFilterCurl_LongBodyTruncated(t *testing.T) {
	var lines []string
	for i := 0; i < 150; i++ {
		lines = append(lines, `{"line": `+itoa(i)+`}`)
	}
	input := strings.Join(lines, "\n")
	got := Filter("curl https://example.com/data", input)
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected truncation notice, got:\n%s", got)
	}
	// Should not contain line 120
	if strings.Contains(got, `"line": 120`) {
		t.Errorf("expected lines beyond 100 truncated, got late line in output")
	}
}
