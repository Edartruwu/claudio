package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultDenyPaths are always blocked unless explicitly overridden.
var DefaultDenyPaths = []string{
	"~/.ssh/**",
	"~/.aws/**",
	"~/.gnupg/**",
	"~/.config/gcloud/**",
	"**/.env",
	"**/.env.*",
	"**/.env.local",
	"**/credentials.json",
	"**/service-account*.json",
	"**/*.pem",
	"**/*.key",
}

// DefaultDenyCommands are blocked shell command patterns.
var DefaultDenyCommands = []string{
	`curl\s+.*\|\s*bash`,
	`curl\s+.*\|\s*sh`,
	`wget\s+.*\|\s*bash`,
	`ssh\s+`,
	`scp\s+`,
	`nc\s+-`,
	`netcat\s+`,
	`rm\s+-rf\s+/`,
	`mkfs\.`,
	`dd\s+if=/dev/zero`,
	`:\(\)\{\s*:\|:&\s*\};:`, // fork bomb
}

// SecretPatterns detects potential secrets in output.
var SecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`(?i)(secret|password|passwd|pwd)\s*[:=]\s*['"]?[^\s'"]{8,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),          // OpenAI/Anthropic API keys
	regexp.MustCompile(`sk-ant-[A-Za-z0-9\-]{20,}`),    // Anthropic specific
	regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),          // GitHub PAT
	regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),          // GitHub OAuth
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{40,}`),  // GitHub fine-grained PAT
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),              // AWS access key
	regexp.MustCompile(`-----BEGIN (RSA |EC |)PRIVATE KEY-----`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), // JWT
}

// CheckPathAccess verifies a file path against deny/allow rules.
func CheckPathAccess(path string, denyPaths, allowPaths []string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	home, _ := os.UserHomeDir()

	// Check allow list first (overrides deny)
	for _, pattern := range allowPaths {
		expanded := expandHome(pattern, home)
		if matchPath(absPath, expanded) {
			return nil
		}
	}

	// Check deny list
	allDeny := append(DefaultDenyPaths, denyPaths...)
	for _, pattern := range allDeny {
		expanded := expandHome(pattern, home)
		if matchPath(absPath, expanded) {
			return fmt.Errorf("access denied: %s matches deny rule %q", path, pattern)
		}
	}

	return nil
}

// CheckCommandSafety validates a shell command against deny patterns.
func CheckCommandSafety(command string, extraDeny []string) error {
	allPatterns := append(DefaultDenyCommands, extraDeny...)

	for _, pattern := range allPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(command) {
			return fmt.Errorf("command blocked: matches deny pattern %q", pattern)
		}
	}

	return nil
}

// ScanForSecrets checks text output for potential secret leakage.
func ScanForSecrets(text string) []string {
	var found []string
	for _, re := range SecretPatterns {
		matches := re.FindAllString(text, 3) // limit to 3 per pattern
		for _, m := range matches {
			// Mask the match for reporting
			if len(m) > 20 {
				found = append(found, m[:10]+"..."+m[len(m)-4:])
			} else {
				found = append(found, m[:len(m)/2]+"...")
			}
		}
	}
	return found
}

// RedactSecrets replaces detected secrets with [REDACTED].
func RedactSecrets(text string) string {
	result := text
	for _, re := range SecretPatterns {
		result = re.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func expandHome(pattern, home string) string {
	if strings.HasPrefix(pattern, "~/") {
		return filepath.Join(home, pattern[2:])
	}
	return pattern
}

// NetworkEgressCheck validates if a command might make unauthorized network requests.
// Returns true if the command contains network activity, along with a description.
func NetworkEgressCheck(command string) (bool, string) {
	lower := strings.ToLower(command)

	checks := []struct {
		pattern string
		reason  string
	}{
		{"curl ", "HTTP request via curl"},
		{"wget ", "HTTP request via wget"},
		{"nc -", "netcat connection"},
		{"netcat ", "netcat connection"},
		{"ssh ", "SSH connection"},
		{"scp ", "SCP file transfer"},
		{"rsync ", "rsync transfer"},
		{"ftp ", "FTP connection"},
		{"telnet ", "telnet connection"},
		{"nmap ", "network scan"},
	}

	for _, c := range checks {
		if strings.Contains(lower, c.pattern) {
			return true, c.reason
		}
	}
	return false, ""
}

// SanitizeForLog removes sensitive data from strings before logging.
func SanitizeForLog(s string) string {
	return RedactSecrets(s)
}

func matchPath(path, pattern string) bool {
	// Handle ** patterns
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := strings.TrimRight(parts[0], "/")
		suffix := strings.TrimLeft(parts[1], "/")

		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}
		if suffix != "" {
			matched, _ := filepath.Match(suffix, filepath.Base(path))
			return matched
		}
		return true
	}

	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Also try matching just the base name
	matched, _ = filepath.Match(pattern, filepath.Base(path))
	return matched
}
