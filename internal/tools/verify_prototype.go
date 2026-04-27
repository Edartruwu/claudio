package tools

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed verify_prototype_runner.py
var verifyPrototypeRunnerPy string

// VerifyPrototypeTool wraps a Playwright-based Python script that click-tests
// interactive HTML prototypes — clicking buttons, filling forms, navigating screens.
type VerifyPrototypeTool struct{}

type verifyPrototypeInput struct {
	HTMLPath             string   `json:"html_path"`
	TestScenario         string   `json:"test_scenario"`
	ClickSequence        []string `json:"click_sequence"`
	ScreenshotOnEachStep bool     `json:"screenshot_on_each_step"`
	TimeoutMS            int      `json:"timeout_ms"`
}

type verifyPrototypeOutput struct {
	Passed         bool     `json:"passed"`
	StepsCompleted int      `json:"steps_completed"`
	StepsTotal     int      `json:"steps_total"`
	ConsoleErrors  []string `json:"console_errors"`
	Screenshots    []string `json:"screenshots"`
	FailureReason  string   `json:"failure_reason"`
	DurationMS     int      `json:"duration_ms"`
}

func (t *VerifyPrototypeTool) Name() string { return "VerifyPrototype" }

func (t *VerifyPrototypeTool) Description() string {
	return "Click-test an interactive HTML prototype using Playwright. Auto-clicks buttons, fills forms, and navigates screens to verify interactions work correctly."
}

func (t *VerifyPrototypeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"html_path": {
				"type": "string",
				"description": "Path to the HTML prototype file to test."
			},
			"test_scenario": {
				"type": "string",
				"description": "Natural language description of what to test (e.g. 'click the login button, fill the form, submit'). Reserved for future AI-driven testing."
			},
			"click_sequence": {
				"type": "array",
				"items": {"type": "string"},
				"description": "CSS selectors to click in order (e.g. ['#login-btn', '.submit', 'button[type=submit]'])."
			},
			"screenshot_on_each_step": {
				"type": "boolean",
				"description": "Capture a screenshot after each interaction step. Default: false."
			},
			"timeout_ms": {
				"type": "integer",
				"description": "Per-step timeout in milliseconds. Default: 30000."
			}
		},
		"required": ["html_path"]
	}`)
}

func (t *VerifyPrototypeTool) IsReadOnly() bool                        { return true }
func (t *VerifyPrototypeTool) RequiresApproval(_ json.RawMessage) bool { return false }

func (t *VerifyPrototypeTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	// 1. Parse input.
	var in verifyPrototypeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &Result{Content: fmt.Sprintf("Invalid input: %v", err), IsError: true}, nil
	}
	if in.HTMLPath == "" {
		return &Result{Content: "html_path is required", IsError: true}, nil
	}
	if in.TimeoutMS <= 0 {
		in.TimeoutMS = 30000
	}

	// Resolve absolute HTML path.
	htmlAbs, err := filepath.Abs(in.HTMLPath)
	if err != nil {
		return &Result{Content: fmt.Sprintf("cannot resolve html_path: %v", err), IsError: true}, nil
	}
	if _, err := os.Stat(htmlAbs); err != nil {
		return &Result{Content: fmt.Sprintf("html_path not found: %s", htmlAbs), IsError: true}, nil
	}

	// 2. Check prerequisites.
	if err := checkPython3Available(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}
	if err := checkPlaywrightAvailable(); err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	// 3. Write embedded runner script to temp file.
	tmpScript, err := os.CreateTemp("", "verify-prototype-*.py")
	if err != nil {
		return &Result{Content: fmt.Sprintf("cannot create temp script: %v", err), IsError: true}, nil
	}
	defer os.Remove(tmpScript.Name())

	if _, err := tmpScript.WriteString(verifyPrototypeRunnerPy); err != nil {
		tmpScript.Close()
		return &Result{Content: fmt.Sprintf("cannot write temp script: %v", err), IsError: true}, nil
	}
	tmpScript.Close()

	// 4. Build JSON input for the runner.
	runnerInput := map[string]interface{}{
		"html_path":              htmlAbs,
		"test_scenario":          in.TestScenario,
		"click_sequence":         in.ClickSequence,
		"screenshot_on_each_step": in.ScreenshotOnEachStep,
		"timeout_ms":             in.TimeoutMS,
	}
	inputJSON, _ := json.Marshal(runnerInput)

	// 5. Execute runner with 60s timeout.
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	//nolint:gosec // tmpScript.Name() is a temp file we wrote ourselves
	cmd := exec.CommandContext(cmdCtx, "python3", tmpScript.Name())
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil {
		errMsg := fmt.Sprintf("verify_prototype_runner.py failed.\nstdout: %s\nstderr: %s\nexit: %v",
			stdout.String(), stderr.String(), runErr)
		return &Result{Content: errMsg, IsError: true}, nil
	}

	// 6. Parse runner output.
	var out verifyPrototypeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return &Result{Content: fmt.Sprintf(
			"Failed to parse runner output: %v\nraw stdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String()), IsError: true}, nil
	}

	// 7. Build result.
	resultJSON, _ := json.MarshalIndent(out, "", "  ")

	var sb strings.Builder
	sb.WriteString(string(resultJSON))

	if s := strings.TrimSpace(stderr.String()); s != "" {
		sb.WriteString("\n\n--- runner warnings ---\n")
		sb.WriteString(s)
	}

	return &Result{Content: sb.String()}, nil
}
