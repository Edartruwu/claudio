package outputfilter

import (
	"fmt"
	"strings"
)

// filterTerraform filters `terraform` command output.
func filterTerraform(sub, output string) (string, bool) {
	switch sub {
	case "plan":
		return filterTerraformPlan(output), true
	case "apply":
		return filterTerraformApply(output), true
	case "destroy":
		return filterTerraformApply(output), true // destroy uses same filter as apply
	case "init":
		return filterTerraformInit(output), true
	case "validate":
		return filterTerraformValidate(output), true
	case "state":
		// For state subcommands, we need to infer the sub-sub-command from output content
		// state list and state output are generally clean, state show needs filtering
		if strings.Contains(output, "id") && strings.Contains(output, "=") {
			// Likely state show output
			return filterTerraformStateShow(output), true
		}
		// state list and state output are clean — passthrough
		return output, true
	case "output":
		// output subcommand is clean — passthrough
		return output, true
	default:
		return Generic(output), true
	}
}

// filterTerraformPlan filters terraform plan output, keeping only:
// - The Plan summary line (Plan: X to add, Y to change, Z to destroy)
// - Resource lines with +/−/~ prefix
// - Error and warning lines
func filterTerraformPlan(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	var errors []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and common noise
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip refreshing state and reading lines
		if strings.HasPrefix(lower, "refreshing state") || strings.HasPrefix(lower, "reading ") {
			continue
		}

		// Skip data source refresh lines (data.foo. or data.bar.)
		if strings.HasPrefix(trimmed, "data.") && strings.Contains(trimmed, ": Reading") {
			continue
		}

		// Skip provider version lines
		if strings.Contains(lower, "provider version") || strings.Contains(lower, "terraform version") {
			continue
		}

		// Keep Plan: summary line
		if strings.Contains(trimmed, "Plan:") {
			result = append(result, trimmed)
			continue
		}

		// Keep resource change lines (start with +, -, ~, <-)
		if len(trimmed) > 0 && (trimmed[0] == '+' || trimmed[0] == '-' || trimmed[0] == '~') {
			result = append(result, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "<=") {
			result = append(result, trimmed)
			continue
		}

		// Keep Error and Warning lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "warning:") {
			errors = append(errors, trimmed)
			continue
		}
		// Also catch Error and Warning without colon (terraform format)
		if strings.HasPrefix(trimmed, "Error ") || strings.HasPrefix(trimmed, "WARNING ") || strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "Warning:") {
			errors = append(errors, trimmed)
			continue
		}
	}

	// Build result with summary first, then resources, then errors
	if len(result) == 0 && len(errors) == 0 {
		return "terraform plan: ok (no changes)"
	}

	var b strings.Builder
	for _, r := range result {
		fmt.Fprintln(&b, r)
	}
	if len(errors) > 0 {
		fmt.Fprintln(&b, "")
		for _, e := range errors {
			fmt.Fprintln(&b, e)
		}
	}
	return strings.TrimSpace(b.String())
}

// filterTerraformApply filters terraform apply/destroy output, keeping only:
// - Resource action lines (aws_instance.foo: Creating..., aws_instance.foo: Creation complete)
// - Final summary lines (Apply complete!, Destroy complete!)
// - Error lines
func filterTerraformApply(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	var errors []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip progress dots and spinner characters
		if strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "⠋") ||
			strings.HasPrefix(trimmed, "⠙") || strings.HasPrefix(trimmed, "⠹") {
			continue
		}

		// Skip elapsed time lines
		if strings.Contains(lower, "elapsed time") {
			continue
		}

		// Keep resource action lines (e.g., "aws_instance.foo: Creating...")
		if strings.Contains(trimmed, ": Creating") || strings.Contains(trimmed, ": Modifying") ||
			strings.Contains(trimmed, ": Destroying") || strings.Contains(trimmed, ": Reading") ||
			strings.Contains(trimmed, "Creation complete") || strings.Contains(trimmed, "Modifications complete") ||
			strings.Contains(trimmed, "Destruction complete") {
			result = append(result, trimmed)
			continue
		}

		// Keep summary lines
		if strings.Contains(lower, "apply complete") || strings.Contains(lower, "destroy complete") {
			result = append(result, trimmed)
			continue
		}

		// Keep Error and Warning lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "warning:") {
			errors = append(errors, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "Error ") || strings.HasPrefix(trimmed, "WARNING ") ||
			strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "Warning:") {
			errors = append(errors, trimmed)
			continue
		}
	}

	if len(result) == 0 && len(errors) == 0 {
		return "terraform apply: ok"
	}

	var b strings.Builder
	for _, r := range result {
		fmt.Fprintln(&b, r)
	}
	if len(errors) > 0 {
		fmt.Fprintln(&b, "")
		for _, e := range errors {
			fmt.Fprintln(&b, e)
		}
	}
	return strings.TrimSpace(b.String())
}

// filterTerraformInit filters terraform init output, keeping only:
// - The success line (Terraform has been successfully initialized)
// - Error and warning lines
// Strips all provider download/install/version finding lines
func filterTerraformInit(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	var errors []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Skip provider/version finding/downloading/installing lines
		if strings.Contains(lower, "downloading provider") || strings.Contains(lower, "installing provider") ||
			strings.Contains(lower, "finding") || strings.Contains(lower, "versions") ||
			strings.Contains(lower, "version constraint") || strings.Contains(lower, "terraform init") {
			continue
		}

		// Skip progress lines
		if strings.HasPrefix(trimmed, "│") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "└") {
			continue
		}

		// Keep the success line
		if strings.Contains(lower, "successfully initialized") {
			result = append(result, trimmed)
			continue
		}

		// Keep Error and Warning lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "warning:") {
			errors = append(errors, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "Error ") || strings.HasPrefix(trimmed, "WARNING ") ||
			strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "Warning:") {
			errors = append(errors, trimmed)
			continue
		}
	}

	if len(result) == 0 && len(errors) == 0 {
		return "terraform init: ok"
	}

	var b strings.Builder
	for _, r := range result {
		fmt.Fprintln(&b, r)
	}
	if len(errors) > 0 {
		fmt.Fprintln(&b, "")
		for _, e := range errors {
			fmt.Fprintln(&b, e)
		}
	}
	return strings.TrimSpace(b.String())
}

// filterTerraformValidate filters terraform validate output, keeping only:
// - Result line (Success! or error/warning messages)
func filterTerraformValidate(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)

		// Keep success line
		if strings.Contains(lower, "success") {
			result = append(result, trimmed)
			continue
		}

		// Keep Error and Warning lines
		if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "warning:") {
			result = append(result, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "Error ") || strings.HasPrefix(trimmed, "WARNING ") ||
			strings.HasPrefix(trimmed, "Error:") || strings.HasPrefix(trimmed, "Warning:") {
			result = append(result, trimmed)
			continue
		}
	}

	if len(result) == 0 {
		return "terraform validate: ok"
	}

	return strings.Join(result, "\n")
}

// filterTerraformStateShow filters terraform state show output,
// removing internal metadata fields like id and timeouts
func filterTerraformStateShow(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip lines that are just metadata keys
		if strings.HasPrefix(trimmed, "id") && strings.Contains(trimmed, "=") {
			continue
		}
		if strings.HasPrefix(trimmed, "timeouts") && strings.Contains(trimmed, "=") {
			continue
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}
