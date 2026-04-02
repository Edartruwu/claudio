package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// FileExists checks if a file exists and is not a directory.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists checks if a directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// EnsureDir creates a directory and all parents if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// ReadFileIfExists reads a file and returns its contents, or empty string if not found.
func ReadFileIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// FindProjectRoot walks up from the given directory to find a project root.
// A project root is identified by the presence of .git, .claudio, go.mod, package.json, etc.
func FindProjectRoot(startDir string) string {
	markers := []string{".git", ".claudio", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "pom.xml"}

	dir := startDir
	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir // fallback to start dir
		}
		dir = parent
	}
}

// CollectMarkdownFiles finds all .md files in a directory (non-recursive).
func CollectMarkdownFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files
}

// CollectMarkdownFilesRecursive finds all .md files in a directory tree.
func CollectMarkdownFilesRecursive(dir string) []string {
	var files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files
}
