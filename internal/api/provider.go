package api

import (
	"context"
	"net/http"
)

// Provider handles sending requests to a specific API backend.
// Implementations translate between claudio's internal Anthropic-style types
// and whatever wire format the backend expects (e.g. OpenAI chat completions).
type Provider interface {
	// StreamMessages sends a streaming request and returns channels for events and errors.
	StreamMessages(ctx context.Context, httpClient *http.Client, req *MessagesRequest) (<-chan StreamEvent, <-chan error)

	// SendMessage sends a non-streaming request and returns the response.
	SendMessage(ctx context.Context, httpClient *http.Client, req *MessagesRequest) (*MessageResp, error)
}
