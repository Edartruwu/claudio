package outputfilter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// filterAws dispatches AWS CLI output filtering by sub-command (service name).
func filterAws(sub, output string) (string, bool) {
	switch sub {
	case "sts":
		return filterAwsJSON(output, awsNoiseKeys), true
	case "ec2":
		return filterAwsJSON(output, awsNoiseKeys), true
	case "lambda":
		return filterAwsJSON(output, awsLambdaNoiseKeys), true
	case "logs":
		return filterAwsLogs(output), true
	case "cloudformation", "cf":
		return filterAwsJSON(output, awsNoiseKeys), true
	case "dynamodb":
		return filterAwsJSON(output, awsNoiseKeys), true
	case "iam":
		return filterAwsJSON(output, awsNoiseKeys), true
	case "s3":
		return filterAwsS3(output), true
	default:
		return "", false
	}
}

// awsNoiseKeys are JSON keys that add noise without meaningful content.
var awsNoiseKeys = map[string]bool{
	"ResponseMetadata": true,
	"RequestId":        true,
	"HTTPStatusCode":   true,
	"HTTPHeaders":      true,
	"RetryAttempts":    true,
}

// awsLambdaNoiseKeys adds lambda-specific noise keys.
var awsLambdaNoiseKeys = map[string]bool{
	"ResponseMetadata":     true,
	"RequestId":            true,
	"HTTPStatusCode":       true,
	"HTTPHeaders":          true,
	"RetryAttempts":        true,
	"CodeSha256":           true,
	"CodeSize":             true,
	"RevisionId":           true,
	"LastModified":         true,
	"LastUpdateStatus":     true,
	"LastUpdateStatusReason": true,
}

// filterAwsJSON strips noise keys from AWS JSON output.
func filterAwsJSON(output string, noiseKeys map[string]bool) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "aws: ok (no output)"
	}

	// Try to parse as JSON
	var data interface{}
	if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
		cleaned := removeNoiseKeys(data, noiseKeys)
		if b, err := json.MarshalIndent(cleaned, "", "  "); err == nil {
			result := string(b)
			// Truncate if too long
			lines := strings.Split(result, "\n")
			if len(lines) > 80 {
				lines = lines[:80]
				lines = append(lines, fmt.Sprintf("  ... +%d more lines", len(strings.Split(result, "\n"))-80))
			}
			return strings.Join(lines, "\n")
		}
	}

	// Non-JSON output: apply generic filter
	return Generic(output)
}

// removeNoiseKeys recursively removes noise keys from a JSON structure.
func removeNoiseKeys(data interface{}, noiseKeys map[string]bool) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		cleaned := make(map[string]interface{}, len(v))
		for key, val := range v {
			if noiseKeys[key] {
				continue
			}
			cleaned[key] = removeNoiseKeys(val, noiseKeys)
		}
		return cleaned
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = removeNoiseKeys(item, noiseKeys)
		}
		return result
	default:
		return data
	}
}

// filterAwsLogs filters `aws logs filter-log-events` and `describe-log-groups` output.
func filterAwsLogs(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "aws logs: ok (no output)"
	}

	// Try JSON — keep only message fields for log events
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &data); err == nil {
		// For filter-log-events: extract message fields only
		if events, ok := data["events"]; ok {
			if eventList, ok := events.([]interface{}); ok {
				var messages []string
				for i, ev := range eventList {
					if i >= 50 {
						messages = append(messages, fmt.Sprintf("... +%d more events", len(eventList)-50))
						break
					}
					if evMap, ok := ev.(map[string]interface{}); ok {
						if msg, ok := evMap["message"]; ok {
							messages = append(messages, truncate(fmt.Sprintf("%v", msg), 200))
						}
					}
				}
				if len(messages) > 0 {
					return strings.Join(messages, "\n")
				}
			}
		}
		// For describe-log-groups: strip noise
		return filterAwsJSON(output, awsNoiseKeys)
	}

	return Generic(output)
}

// filterAwsS3 filters `aws s3 ls`, `aws s3 cp`, `aws s3 sync` output.
func filterAwsS3(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		// Skip verbose transfer progress
		if strings.Contains(lower, "completed") && strings.Contains(lower, "%") {
			continue
		}
		count++
		if count <= 40 {
			result = append(result, truncate(trimmed, 200))
		}
	}

	if len(result) == 0 {
		return "aws s3: ok"
	}
	if count > 40 {
		result = append(result, fmt.Sprintf("... +%d more lines", count-40))
	}
	return strings.Join(result, "\n")
}
