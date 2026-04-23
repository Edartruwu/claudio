package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// MetaFile is written alongside harness.json for harnesses installed from git.
const MetaFile = "harness.meta.json"

// Meta holds installation provenance.
type Meta struct {
	InstalledFrom string `json:"installed_from"`
	InstalledAt   string `json:"installed_at"` // RFC3339
}

// Install detects whether source is a git URL or local path and dispatches accordingly.
// Returns the absolute path of the installed harness directory.
func Install(source, destDir string) (string, error) {
	if isGitURL(source) {
		return InstallFromGit(source, destDir)
	}
	return InstallFromLocal(source, destDir)
}

// InstallFromGit clones a git repository (shallow) to a temp dir, validates the manifest,
// then copies the result to destDir/<name>/. Returns the destination path.
func InstallFromGit(url, destDir string) (string, error) {
	url = expandGitURL(url)

	tmp, err := os.MkdirTemp("", "claudio-harness-*")
	if err != nil {
		return "", fmt.Errorf("harness: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("git", "clone", "--depth", "1", url, tmp)
	cmd.Stdout = os.Stderr // progress output to stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("harness: git clone %s: %w", url, err)
	}

	m, err := LoadManifest(tmp)
	if err != nil {
		return "", err
	}
	if err := m.Validate(); err != nil {
		return "", err
	}

	dest, err := copyHarness(tmp, destDir, m.Name)
	if err != nil {
		return "", err
	}

	// Write metadata file.
	meta := Meta{
		InstalledFrom: url,
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeMeta(dest, meta); err != nil {
		return dest, fmt.Errorf("harness: write meta: %w", err)
	}

	return dest, nil
}

// InstallFromLocal validates the manifest at srcDir and copies to destDir/<name>/.
// Returns the destination path.
func InstallFromLocal(srcDir, destDir string) (string, error) {
	m, err := LoadManifest(srcDir)
	if err != nil {
		return "", err
	}
	if err := m.Validate(); err != nil {
		return "", err
	}

	return copyHarness(srcDir, destDir, m.Name)
}

// Uninstall removes harnessesDir/<name>/.
func Uninstall(harnessesDir, name string) error {
	target := filepath.Join(harnessesDir, name)
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("harness: uninstall %s: %w", name, err)
	}
	return nil
}

// List returns manifests for all installed harnesses.
func List(harnessesDir string) ([]*Manifest, error) {
	harnesses, err := DiscoverHarnesses(harnessesDir)
	if err != nil {
		return nil, err
	}
	out := make([]*Manifest, 0, len(harnesses))
	for _, h := range harnesses {
		out = append(out, h.Manifest)
	}
	return out, nil
}

// isGitURL returns true if source looks like a git remote URL.
func isGitURL(source string) bool {
	return strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "gh:")
}

// expandGitURL expands the gh: shorthand to a full GitHub URL.
func expandGitURL(url string) string {
	if strings.HasPrefix(url, "gh:") {
		return "https://github.com/" + strings.TrimPrefix(url, "gh:")
	}
	return url
}

// copyHarness copies src into destDir/<name>/ using filepath.WalkDir.
func copyHarness(src, destDir, name string) (string, error) {
	dest := filepath.Join(destDir, name)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", fmt.Errorf("harness: create dest %s: %w", dest, err)
	}

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Skip .git directory entirely.
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		return "", fmt.Errorf("harness: copy harness: %w", err)
	}
	return dest, nil
}

// copyFile copies a single file preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// writeMeta writes harness.meta.json to dir.
func writeMeta(dir string, meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, MetaFile), data, 0644)
}
