package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// SHA256String returns the SHA-256 hash of a string.
func SHA256String(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// SHA256File returns the SHA-256 hash of a file's contents.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ShortHash returns the first n characters of a SHA-256 hash.
func ShortHash(s string, n int) string {
	full := SHA256String(s)
	if n > len(full) {
		return full
	}
	return full[:n]
}

// ContentHash returns a content-addressable hash for caching.
func ContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:8]))
}
