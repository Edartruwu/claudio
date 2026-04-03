package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Abraxas-365/claudio/internal/api"
)

// Anthropic implements the Provider interface for the Anthropic Messages API.
// This is used when the user configures an additional Anthropic-compatible endpoint
// (e.g. a proxy). The main client still uses its own Anthropic logic directly.
type Anthropic struct {
	baseURL    string
	apiKey     string
	apiVersion string
}

// NewAnthropic creates a new Anthropic provider with the given configuration.
func NewAnthropic(baseURL, apiKey string) *Anthropic {
	return &Anthropic{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		apiVersion: "2023-06-01",
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) StreamMessages(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (<-chan api.StreamEvent, <-chan error) {
	req.Stream = true

	eventCh := make(chan api.StreamEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := json.Marshal(req)
		if err != nil {
			errCh <- fmt.Errorf("failed to marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			a.baseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("anthropic-version", a.apiVersion)
		httpReq.Header.Set("x-api-key", a.apiKey)

		resp, err := httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("API request failed: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := line[6:]
				if data == "[DONE]" {
					return
				}
				var event api.StreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}
				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stream read error: %w", err)
		}
	}()

	return eventCh, errCh
}

func (a *Anthropic) SendMessage(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (*api.MessageResp, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", a.apiVersion)
	httpReq.Header.Set("x-api-key", a.apiKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var msgResp api.MessageResp
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &msgResp, nil
}
