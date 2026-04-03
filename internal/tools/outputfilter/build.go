package outputfilter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ── Go ──────────────────────────────────────────────────────────────────

func filterGo(sub, output string) (string, bool) {
	switch sub {
	case "test":
		return filterGoTest(output), true
	case "build":
		return filterGoBuild(output), true
	case "vet":
		return filterGoVet(output), true
	case "install":
		return filterGoBuild(output), true
	default:
		return "", false
	}
}

// goTestEvent mirrors the JSON output of `go test -json`.
type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func filterGoTest(output string) string {
	// Try JSON format first (go test -json)
	if strings.Contains(output, `"Action"`) {
		if result := filterGoTestJSON(output); result != "" {
			return result
		}
	}
	// Fall back to plain text filtering
	return filterGoTestPlain(output)
}

func filterGoTestJSON(output string) string {
	type pkgResult struct {
		pass        int
		fail        int
		skip        int
		failedTests []struct {
			name    string
			outputs []string
		}
	}

	packages := make(map[string]*pkgResult)
	testOutputs := make(map[string][]string) // "pkg/test" -> output lines

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev goTestEvent
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		pkg := ev.Package
		if pkg == "" {
			continue
		}
		if packages[pkg] == nil {
			packages[pkg] = &pkgResult{}
		}
		pr := packages[pkg]

		switch ev.Action {
		case "pass":
			if ev.Test != "" {
				pr.pass++
			}
		case "fail":
			if ev.Test != "" {
				pr.fail++
				key := pkg + "/" + ev.Test
				pr.failedTests = append(pr.failedTests, struct {
					name    string
					outputs []string
				}{ev.Test, testOutputs[key]})
			}
		case "skip":
			if ev.Test != "" {
				pr.skip++
			}
		case "output":
			if ev.Test != "" {
				key := pkg + "/" + ev.Test
				text := strings.TrimRight(ev.Output, "\n")
				if text != "" {
					testOutputs[key] = append(testOutputs[key], text)
				}
			}
		}
	}

	totalPass, totalFail, totalSkip := 0, 0, 0
	for _, pr := range packages {
		totalPass += pr.pass
		totalFail += pr.fail
		totalSkip += pr.skip
	}

	if totalPass == 0 && totalFail == 0 {
		return "Go test: No tests found"
	}

	if totalFail == 0 {
		return fmt.Sprintf("Go test: %d passed in %d packages", totalPass, len(packages))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Go test: %d passed, %d failed", totalPass, totalFail)
	if totalSkip > 0 {
		fmt.Fprintf(&b, ", %d skipped", totalSkip)
	}
	fmt.Fprintf(&b, " in %d packages\n", len(packages))

	for pkg, pr := range packages {
		if pr.fail == 0 {
			continue
		}
		shortPkg := compactPackageName(pkg)
		fmt.Fprintf(&b, "\n%s (%d passed, %d failed)\n", shortPkg, pr.pass, pr.fail)
		for _, ft := range pr.failedTests {
			fmt.Fprintf(&b, "  [FAIL] %s\n", ft.name)
			shown := 0
			for _, line := range ft.outputs {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "error") || strings.Contains(lower, "expected") ||
					strings.Contains(lower, "got") || strings.Contains(lower, "panic") {
					if shown < 5 {
						fmt.Fprintf(&b, "     %s\n", truncate(line, 120))
						shown++
					}
				}
			}
		}
	}

	return strings.TrimSpace(b.String())
}

func filterGoTestPlain(output string) string {
	lines := strings.Split(output, "\n")
	var failures []string
	var summary []string
	inFail := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FAIL:") {
			inFail = true
			failures = append(failures, trimmed)
			continue
		}
		if inFail && trimmed != "" && !strings.HasPrefix(trimmed, "---") && !strings.HasPrefix(trimmed, "===") {
			failures = append(failures, "    "+trimmed)
			if len(failures) > 50 {
				inFail = false
			}
			continue
		}
		inFail = false
		if strings.HasPrefix(trimmed, "FAIL") || strings.HasPrefix(trimmed, "ok") {
			summary = append(summary, trimmed)
		}
	}

	if len(failures) == 0 && len(summary) > 0 {
		return strings.Join(summary, "\n")
	}
	if len(failures) > 0 {
		all := append(failures, "")
		all = append(all, summary...)
		return strings.Join(all, "\n")
	}
	return Generic(output)
}

// filterGoBuild keeps only error lines.
func filterGoBuild(output string) string {
	var errors []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(trimmed, ".go:") || strings.Contains(lower, "error") ||
			strings.Contains(lower, "undefined") || strings.Contains(lower, "cannot") {
			errors = append(errors, trimmed)
		}
	}
	if len(errors) == 0 {
		return "Go build: Success"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Go build: %d errors\n", len(errors))
	for i, e := range errors {
		if i >= 20 {
			fmt.Fprintf(&b, "\n... +%d more errors\n", len(errors)-20)
			break
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, truncate(e, 120))
	}
	return strings.TrimSpace(b.String())
}

// filterGoVet keeps only issue lines.
func filterGoVet(output string) string {
	var issues []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ".go:") {
			issues = append(issues, trimmed)
		}
	}
	if len(issues) == 0 {
		return "Go vet: No issues found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Go vet: %d issues\n", len(issues))
	for i, iss := range issues {
		if i >= 20 {
			fmt.Fprintf(&b, "\n... +%d more issues\n", len(issues)-20)
			break
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, truncate(iss, 120))
	}
	return strings.TrimSpace(b.String())
}

// ── Cargo ───────────────────────────────────────────────────────────────

func filterCargo(sub, output string) (string, bool) {
	switch sub {
	case "build":
		return filterCargoBuild(output), true
	case "test":
		return filterCargoTest(output), true
	case "clippy":
		return filterCargoClippy(output), true
	default:
		return "", false
	}
}

func filterCargoBuild(output string) string {
	var errors []string
	var warnings int
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "error") {
			errors = append(errors, trimmed)
		}
		if strings.HasPrefix(trimmed, "warning") {
			warnings++
		}
	}
	if len(errors) == 0 {
		if warnings > 0 {
			return fmt.Sprintf("Cargo build: Success (%d warnings)", warnings)
		}
		return "Cargo build: Success"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Cargo build: %d errors, %d warnings\n", len(errors), warnings)
	for _, e := range errors {
		fmt.Fprintf(&b, "  %s\n", truncate(e, 120))
	}
	return strings.TrimSpace(b.String())
}

func filterCargoTest(output string) string {
	lines := strings.Split(output, "\n")
	var failures []string
	var summaryLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "test result:") || strings.HasPrefix(trimmed, "failures:") {
			summaryLines = append(summaryLines, trimmed)
		}
		if strings.Contains(trimmed, "FAILED") || strings.HasPrefix(trimmed, "---- ") {
			failures = append(failures, trimmed)
		}
	}

	if len(failures) == 0 && len(summaryLines) > 0 {
		return strings.Join(summaryLines, "\n")
	}
	if len(failures) > 0 {
		all := append(failures, summaryLines...)
		return strings.Join(all, "\n")
	}
	return Generic(output)
}

func filterCargoClippy(output string) string {
	var warnings []string
	var errors []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "warning:") {
			warnings = append(warnings, trimmed)
		}
		if strings.HasPrefix(trimmed, "error") {
			errors = append(errors, trimmed)
		}
	}
	if len(errors) == 0 && len(warnings) == 0 {
		return "Clippy: No issues found"
	}
	var b strings.Builder
	if len(errors) > 0 {
		fmt.Fprintf(&b, "Clippy: %d errors, %d warnings\n", len(errors), len(warnings))
		for _, e := range errors {
			fmt.Fprintf(&b, "  %s\n", truncate(e, 120))
		}
	} else {
		fmt.Fprintf(&b, "Clippy: %d warnings\n", len(warnings))
	}
	for _, w := range warnings {
		if len(b.String()) > 2000 {
			fmt.Fprintf(&b, "  ... +%d more\n", len(warnings))
			break
		}
		fmt.Fprintf(&b, "  %s\n", truncate(w, 120))
	}
	return strings.TrimSpace(b.String())
}

// ── npm / pnpm / yarn ───────────────────────────────────────────────────

func filterNpm(sub, output string) (string, bool) {
	switch sub {
	case "install", "i", "ci":
		return filterNpmInstall(output), true
	case "run", "test", "t":
		return Generic(output), true
	default:
		return "", false
	}
}

func filterNpmInstall(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Keep summary lines
		if strings.Contains(lower, "added") && strings.Contains(lower, "package") {
			result = append(result, trimmed)
		}
		if strings.Contains(lower, "up to date") {
			result = append(result, trimmed)
		}
		// Keep warnings and errors
		if strings.HasPrefix(lower, "warn") || strings.Contains(lower, " warn ") ||
			strings.HasPrefix(lower, "err") ||
			strings.Contains(lower, "vulnerability") || strings.Contains(lower, "vulnerabilities") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "npm install: ok"
	}
	return strings.Join(result, "\n")
}

// ── pip ─────────────────────────────────────────────────────────────────

func filterPip(sub, output string) (string, bool) {
	if sub != "install" {
		return "", false
	}
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Skip download/progress
		if strings.HasPrefix(lower, "downloading") || strings.HasPrefix(lower, "collecting") ||
			strings.Contains(lower, "already satisfied") {
			continue
		}
		if strings.HasPrefix(lower, "successfully") || strings.Contains(lower, "error") ||
			strings.Contains(lower, "warning") || strings.HasPrefix(lower, "installed") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "pip install: ok", true
	}
	return strings.Join(result, "\n"), true
}

// ── Docker ──────────────────────────────────────────────────────────────

func filterDocker(sub, output string) (string, bool) {
	switch sub {
	case "build":
		return filterDockerBuild(output), true
	case "pull", "push":
		return filterDockerPullPush(output), true
	default:
		return "", false
	}
}

func filterDockerBuild(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip layer progress
		if strings.Contains(trimmed, "Pulling") || strings.Contains(trimmed, "Waiting") ||
			strings.Contains(trimmed, "Downloading") || strings.Contains(trimmed, "Extracting") {
			continue
		}
		// Keep step headers, errors, and final lines
		if strings.HasPrefix(trimmed, "Step") || strings.HasPrefix(trimmed, "#") ||
			strings.Contains(trimmed, "error") || strings.Contains(trimmed, "ERROR") ||
			strings.HasPrefix(trimmed, "Successfully") || strings.HasPrefix(trimmed, "FINISHED") {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return "docker build: ok"
	}
	return strings.Join(result, "\n")
}

func filterDockerPullPush(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip layer progress
		if strings.Contains(trimmed, ": Pulling") || strings.Contains(trimmed, ": Waiting") ||
			strings.Contains(trimmed, ": Downloading") || strings.Contains(trimmed, ": Extracting") ||
			strings.Contains(trimmed, ": Pushed") || strings.Contains(trimmed, ": Preparing") {
			continue
		}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return "ok"
	}
	return strings.Join(result, "\n")
}

// ── Make ────────────────────────────────────────────────────────────────

func filterMake(output string) (string, bool) {
	lines := strings.Split(output, "\n")
	var errors []string
	totalLines := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		totalLines++
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
			errors = append(errors, trimmed)
		}
	}

	// If short output or has errors, don't filter aggressively
	if totalLines <= 30 || len(errors) > 0 {
		return "", false
	}

	// Long successful make output — apply generic filter
	return Generic(output), true
}

// ── Helpers ─────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func compactPackageName(pkg string) string {
	if idx := strings.LastIndex(pkg, "/"); idx >= 0 {
		return pkg[idx+1:]
	}
	return pkg
}
