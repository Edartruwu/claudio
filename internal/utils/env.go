package utils

import (
	"os"
	"strconv"
	"strings"
)

// GetEnv returns an environment variable value or a default.
func GetEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// GetEnvBool returns an environment variable as a boolean.
func GetEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	lower := strings.ToLower(val)
	return lower == "true" || lower == "1" || lower == "yes"
}

// GetEnvInt returns an environment variable as an integer.
func GetEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}

// GetEnvFloat returns an environment variable as a float64.
func GetEnvFloat(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

// RequireEnv returns the value of a required env var, panicking if not set.
func RequireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

// ValidateEnv checks that required environment variables are set.
// Returns a list of missing variable names.
func ValidateEnv(required ...string) []string {
	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

// ExpandEnvVars expands ${VAR} and $VAR patterns in a string.
func ExpandEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// ClaudioEnvVars returns all CLAUDIO_* environment variables.
func ClaudioEnvVars() map[string]string {
	result := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "CLAUDIO_") {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
