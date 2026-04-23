package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromLocal_HappyPath(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	// Write a valid harness at src.
	writeManifest(t, src, Manifest{Name: "my-harness", Version: "1.2.3"})
	// Write some extra files.
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	installed, err := InstallFromLocal(src, dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(dest, "my-harness")
	if installed != want {
		t.Errorf("installed path: got %q, want %q", installed, want)
	}

	// harness.json should be present in destination.
	if _, err := os.Stat(filepath.Join(installed, ManifestFile)); err != nil {
		t.Errorf("harness.json missing: %v", err)
	}
	// README.md should be present.
	if _, err := os.Stat(filepath.Join(installed, "README.md")); err != nil {
		t.Errorf("README.md missing: %v", err)
	}
}

func TestInstallFromLocal_InvalidManifest(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	// Manifest missing required fields.
	writeManifest(t, src, Manifest{}) // no name, no version

	_, err := InstallFromLocal(src, dest)
	if err == nil {
		t.Fatal("expected error for invalid manifest")
	}
}

func TestInstallFromLocal_MissingManifest(t *testing.T) {
	src := t.TempDir() // no harness.json
	dest := t.TempDir()

	_, err := InstallFromLocal(src, dest)
	if err == nil {
		t.Fatal("expected error when harness.json is missing")
	}
}

func TestUninstall(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "my-harness")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(root, "my-harness"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("harness dir should be removed")
	}
}

func TestUninstall_NonExistentOK(t *testing.T) {
	root := t.TempDir()
	// Should not error if harness doesn't exist (RemoveAll is idempotent).
	if err := Uninstall(root, "nonexistent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	setupHarness(t, root, "alpha", "1.0.0")
	setupHarness(t, root, "beta", "2.0.0")

	manifests, err := List(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("want 2 manifests, got %d", len(manifests))
	}
	// DiscoverHarnesses sorts by name, so alpha comes first.
	if manifests[0].Name != "alpha" {
		t.Errorf("want alpha first, got %q", manifests[0].Name)
	}
}

func TestIsGitURL(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/user/repo":   true,
		"http://example.com/repo.git":    true,
		"git@github.com:user/repo.git":   true,
		"gh:user/repo":                   true,
		"/local/path/to/harness":         false,
		"./relative/path":                false,
		"C:\\Windows\\path":              false,
	}
	for url, want := range cases {
		got := isGitURL(url)
		if got != want {
			t.Errorf("isGitURL(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestExpandGitURL_GhShorthand(t *testing.T) {
	got := expandGitURL("gh:user/my-harness")
	want := "https://github.com/user/my-harness"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandGitURL_NoChange(t *testing.T) {
	url := "https://github.com/user/repo"
	if got := expandGitURL(url); got != url {
		t.Errorf("got %q, want %q", got, url)
	}
}

func TestInstall_DispatchesLocal(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	writeManifest(t, src, Manifest{Name: "local-h", Version: "1.0.0"})

	installed, err := Install(src, dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if installed != filepath.Join(dest, "local-h") {
		t.Errorf("unexpected install path: %q", installed)
	}
}
