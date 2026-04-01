package oauth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/Abraxas-365/claudio/internal/auth/oauth"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := oauth.GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	v2, _ := oauth.GenerateCodeVerifier()

	if v1 == v2 {
		t.Error("expected unique verifiers")
	}
	if len(v1) < 40 {
		t.Errorf("verifier too short: %d chars", len(v1))
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test-verifier-string"
	challenge := oauth.GenerateCodeChallenge(verifier)

	// Verify it's a valid S256 challenge
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expected {
		t.Errorf("challenge mismatch: got %q, want %q", challenge, expected)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := oauth.GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := oauth.GenerateState()

	if s1 == s2 {
		t.Error("expected unique state values")
	}
}
