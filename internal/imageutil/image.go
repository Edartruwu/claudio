package imageutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// SupportedImageExts maps file extensions to MIME types.
var SupportedImageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// IsImageFile returns true if path has a supported image extension.
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := SupportedImageExts[ext]
	return ok
}

// imageTargetBytes is the target raw size for images sent to the API.
// Images above this are compressed before base64 encoding.
const imageTargetBytes = 500_000

// imageMaxDimension is the maximum pixel width or height allowed before
// downscaling. Keeps images below the API 2000px multi-image limit.
const imageMaxDimension = 1568

// resizeImage scales img down so neither dimension exceeds imageMaxDimension.
// If both dimensions are within limit, returns img unchanged. Never upscales.
func resizeImage(img image.Image) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w <= imageMaxDimension && h <= imageMaxDimension {
		return img
	}

	// Compute scale factor to fit within imageMaxDimension.
	scaleW := float64(imageMaxDimension) / float64(w)
	scaleH := float64(imageMaxDimension) / float64(h)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newW := int(float64(w) * scale)
	newH := int(float64(h) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
	return dst
}

// CompressImage attempts to reduce image size by re-encoding as JPEG at
// progressively lower quality. Returns the (possibly compressed) bytes and
// the resulting media type. Falls back to original bytes if compression fails.
func CompressImage(raw []byte, originalMediaType string) ([]byte, string) {
	// Decode first so we can check/fix dimensions regardless of byte size.
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		// Can't decode — skip compression, return as-is.
		if len(raw) <= imageTargetBytes {
			return raw, originalMediaType
		}
		return raw, originalMediaType
	}

	// Downscale if either dimension exceeds the API limit.
	resized := resizeImage(img)

	// If no resize was needed and image is already small, return original bytes.
	if resized == img && len(raw) <= imageTargetBytes {
		return raw, originalMediaType
	}

	for _, quality := range []int{85, 70, 55, 40} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality}); err != nil {
			continue
		}
		if buf.Len() <= imageTargetBytes {
			return buf.Bytes(), "image/jpeg"
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 40}); err == nil {
		return buf.Bytes(), "image/jpeg"
	}
	return raw, originalMediaType
}

// ReadImageFile reads an image from disk and returns its base64 data and MIME type.
// Images larger than imageTargetBytes are compressed before encoding.
func ReadImageFile(path string) (data string, mediaType string, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	mt, ok := SupportedImageExts[ext]
	if !ok {
		return "", "", fmt.Errorf("unsupported image format: %s", ext)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("reading image: %w", err)
	}

	// Claude API hard limit: ~5MB base64 (~3.75MB raw)
	const maxSize = 3_750_000

	compressed, mt := CompressImage(raw, mt)
	if len(compressed) > maxSize {
		return "", "", fmt.Errorf("image too large (%s, max ~3.75MB)", HumanFileSize(len(compressed)))
	}

	return base64.StdEncoding.EncodeToString(compressed), mt, nil
}

// HumanFileSize returns a human-readable size string.
func HumanFileSize(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fkB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
