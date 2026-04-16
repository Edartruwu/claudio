package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Abraxas-365/claudio/internal/imageutil"
)

// ImageAttachment represents an image attached to the prompt.
type ImageAttachment struct {
	ID        int
	FileName  string // display name
	MediaType string // MIME type (image/png, image/jpeg, etc.)
	Data      string // base64-encoded image data
}

// IsImageFile returns true if the path has a supported image extension.
// Delegates to imageutil.IsImageFile.
func IsImageFile(path string) bool {
	return imageutil.IsImageFile(path)
}

// ReadImageFile reads an image from disk and returns its base64 data and MIME type.
// Delegates to imageutil.ReadImageFile.
func ReadImageFile(path string) (data string, mediaType string, err error) {
	return imageutil.ReadImageFile(path)
}

// ReadClipboardImage attempts to read an image from the system clipboard.
// Returns the base64 data and MIME type, or an error if no image is available.
func ReadClipboardImage() (data string, mediaType string, err error) {
	switch runtime.GOOS {
	case "darwin":
		return readClipboardImageDarwin()
	case "linux":
		return readClipboardImageLinux()
	default:
		return "", "", fmt.Errorf("clipboard image not supported on %s", runtime.GOOS)
	}
}

// HasClipboardImage does a fast check to see if the clipboard
// contains image data. On macOS uses osascript clipboard info (~50ms).
func HasClipboardImage() bool {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e",
			`try
	clipboard info
on error
	return ""
end try`).Output()
		if err != nil {
			return false
		}
		s := string(out)
		return strings.Contains(s, "PNGf") ||
			strings.Contains(s, "TIFF") ||
			strings.Contains(s, "JPEG") ||
			strings.Contains(s, "«class jp2 »")
	case "linux":
		// Check if xclip or wl-paste can find image content
		for _, tool := range []struct {
			name string
			args []string
		}{
			{"xclip", []string{"-selection", "clipboard", "-target", "TARGETS", "-o"}},
			{"wl-paste", []string{"--list-types"}},
		} {
			if _, err := exec.LookPath(tool.name); err != nil {
				continue
			}
			out, err := exec.Command(tool.name, tool.args...).Output()
			if err != nil {
				continue
			}
			s := string(out)
			if strings.Contains(s, "image/png") || strings.Contains(s, "image/jpeg") {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func readClipboardImageDarwin() (string, string, error) {
	tmpFile, err := os.CreateTemp("", "claudio-clip-*.png")
	if err != nil {
		return "", "", err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Try PNG first, then TIFF, then JPEG — macOS stores images in various formats
	// depending on the source (screenshot = PNG, copy from Preview = TIFF, etc.)
	script := fmt.Sprintf(`
try
	-- Try PNG first
	set imgData to the clipboard as «class PNGf»
	set fileRef to open for access POSIX file "%s" with write permission
	write imgData to fileRef
	close access fileRef
	return "png"
on error
	try
		-- Try TIFF
		set imgData to the clipboard as «class TIFF»
		set fileRef to open for access POSIX file "%s" with write permission
		write imgData to fileRef
		close access fileRef
		return "tiff"
	on error
		try
			-- Try JPEG
			set imgData to the clipboard as «class JPEG»
			set fileRef to open for access POSIX file "%s" with write permission
			write imgData to fileRef
			close access fileRef
			return "jpeg"
		on error errMsg
			return "error:" & errMsg
		end try
	end try
end try`, tmpPath, tmpPath, tmpPath)

	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", "", fmt.Errorf("clipboard read failed: %w", err)
	}

	result := strings.TrimSpace(string(out))
	if strings.HasPrefix(result, "error:") {
		return "", "", fmt.Errorf("no image in clipboard: %s", result[6:])
	}

	raw, err := os.ReadFile(tmpPath)
	if err != nil || len(raw) == 0 {
		return "", "", fmt.Errorf("no image data in clipboard")
	}

	// Convert non-PNG formats to PNG using sips (built into macOS)
	mediaType := "image/png"
	switch result {
	case "tiff":
		pngPath := tmpPath + ".png"
		defer os.Remove(pngPath)
		if err := exec.Command("sips", "-s", "format", "png", tmpPath, "--out", pngPath).Run(); err != nil {
			// If conversion fails, try sending raw data anyway
			return base64.StdEncoding.EncodeToString(raw), "image/tiff", nil
		}
		converted, err := os.ReadFile(pngPath)
		if err == nil && len(converted) > 0 {
			raw = converted
		}
	case "jpeg":
		mediaType = "image/jpeg"
	}

	compressed, mediaType := imageutil.CompressImage(raw, mediaType)
	return base64.StdEncoding.EncodeToString(compressed), mediaType, nil
}

func readClipboardImageLinux() (string, string, error) {
	// Try xclip first (X11), then wl-paste (Wayland)
	for _, tool := range []struct {
		name string
		args []string
	}{
		{"xclip", []string{"-selection", "clipboard", "-target", "image/png", "-o"}},
		{"wl-paste", []string{"--type", "image/png"}},
	} {
		if _, err := exec.LookPath(tool.name); err != nil {
			continue
		}
		out, err := exec.Command(tool.name, tool.args...).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		return base64.StdEncoding.EncodeToString(out), "image/png", nil
	}
	return "", "", fmt.Errorf("no image in clipboard (install xclip or wl-paste)")
}


