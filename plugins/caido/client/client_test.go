package client

import (
	"testing"

	"github.com/Abraxas-365/claudio-plugin-caido/config"
)

func TestClientConnectError(t *testing.T) {
	// Create client with unreachable endpoint
	cfg := &config.Config{
		URL:       "http://127.0.0.1:29999", // Port unlikely to be open
		PAT:       "dummy-token",
		BodyLimit: 2000,
	}

	client, err := New(cfg)
	// Should return error, not panic
	if err == nil {
		t.Fatal("expected error when connecting to unreachable Caido, got nil")
	}

	if client != nil {
		t.Fatal("expected nil client on connection error")
	}

	// Verify it's not a panic (would crash test)
	t.Logf("connection error as expected: %v", err)
}
