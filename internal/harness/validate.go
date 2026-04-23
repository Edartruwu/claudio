package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidationError is a single finding from ValidateHarness.
type ValidationError struct {
	Severity string // "error" or "warning"
	Path     string
	Message  string
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", v.Severity, v.Path, v.Message)
}

// ValidateHarness runs all structural checks on an installed harness.
// It always returns all findings rather than stopping at the first error.
func ValidateHarness(h *Harness) []ValidationError {
	var errs []ValidationError

	// 1. Manifest name+version.
	if err := h.Manifest.Validate(); err != nil {
		errs = append(errs, ValidationError{
			Severity: "error",
			Path:     filepath.Join(h.Dir, ManifestFile),
			Message:  err.Error(),
		})
	}

	// 2. Agent dirs exist; .md files have content.
	for _, dir := range h.Manifest.AgentDirs(h.Dir) {
		errs = append(errs, checkDirExists(dir, "agents")...)
		if dirExists(dir) {
			errs = append(errs, checkMDFiles(dir)...)
		}
	}

	// 3. Skill dirs exist.
	for _, dir := range h.Manifest.SkillDirs(h.Dir) {
		errs = append(errs, checkDirExists(dir, "skills")...)
	}

	// 4. Plugin dirs exist; executables are executable (non-Windows only).
	for _, dir := range h.Manifest.PluginDirs(h.Dir) {
		errs = append(errs, checkDirExists(dir, "plugins")...)
		if dirExists(dir) && runtime.GOOS != "windows" {
			errs = append(errs, checkExecutables(dir)...)
		}
	}

	// 5. Template dirs exist; JSON files are valid.
	for _, dir := range h.Manifest.TemplateDirs(h.Dir) {
		errs = append(errs, checkDirExists(dir, "templates")...)
		if dirExists(dir) {
			errs = append(errs, checkJSONFiles(dir)...)
		}
	}

	// 6. Rules paths exist.
	for _, path := range h.Manifest.RulePaths(h.Dir) {
		errs = append(errs, checkPathExists(path, "rules")...)
	}

	return errs
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// checkDirExists returns a warning if dir does not exist.
func checkDirExists(dir, kind string) []ValidationError {
	if !dirExists(dir) {
		return []ValidationError{{
			Severity: "warning",
			Path:     dir,
			Message:  fmt.Sprintf("%s directory does not exist", kind),
		}}
	}
	return nil
}

// checkPathExists returns a warning if path does not exist (file or dir).
func checkPathExists(path, kind string) []ValidationError {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []ValidationError{{
			Severity: "warning",
			Path:     path,
			Message:  fmt.Sprintf("%s path does not exist", kind),
		}}
	}
	return nil
}

// checkMDFiles verifies that .md files inside dir exist and have content.
func checkMDFiles(dir string) []ValidationError {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []ValidationError{{Severity: "error", Path: dir, Message: err.Error()}}
	}
	var errs []ValidationError
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			errs = append(errs, ValidationError{Severity: "error", Path: path, Message: err.Error()})
			continue
		}
		if info.Size() == 0 {
			errs = append(errs, ValidationError{
				Severity: "warning",
				Path:     path,
				Message:  "agent .md file is empty",
			})
		}
	}
	return errs
}

// checkJSONFiles verifies that .json files inside dir are valid JSON.
func checkJSONFiles(dir string) []ValidationError {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []ValidationError{{Severity: "error", Path: dir, Message: err.Error()}}
	}
	var errs []ValidationError
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, ValidationError{Severity: "error", Path: path, Message: err.Error()})
			continue
		}
		var v interface{}
		if err := json.Unmarshal(data, &v); err != nil {
			errs = append(errs, ValidationError{
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("invalid JSON: %v", err),
			})
		}
	}
	return errs
}

// checkExecutables verifies that files inside dir are executable.
func checkExecutables(dir string) []ValidationError {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []ValidationError{{Severity: "error", Path: dir, Message: err.Error()}}
	}
	var errs []ValidationError
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			errs = append(errs, ValidationError{Severity: "error", Path: path, Message: err.Error()})
			continue
		}
		if info.Mode()&0111 == 0 {
			errs = append(errs, ValidationError{
				Severity: "warning",
				Path:     path,
				Message:  "plugin file is not executable",
			})
		}
	}
	return errs
}
