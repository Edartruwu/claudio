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

// OpenAI implements the Provider interface for any OpenAI-compatible API
// (OpenAI, Groq, Ollama, Together, vLLM, etc.).
type OpenAI struct {
	name    string // provider name (e.g. "groq", "openai", "ollama")
	baseURL string
	apiKey  string
	numCtx  int // Ollama num_ctx override; 0 means use server default
}

// NewOpenAI creates a new OpenAI-compatible provider.
func NewOpenAI(name, baseURL, apiKey string) *OpenAI {
	return &OpenAI{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
}

// WithNumCtx sets the num_ctx (context window size) sent in every request.
// Ollama defaults to 2048 tokens regardless of the model's actual context
// length, silently truncating conversation history once the system prompt +
// history exceeds that limit. Set this to e.g. 32768 for local models.
func (o *OpenAI) WithNumCtx(n int) *OpenAI {
	o.numCtx = n
	return o
}

func (o *OpenAI) Name() string { return o.name }

func (o *OpenAI) StreamMessages(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (<-chan api.StreamEvent, <-chan error) {
	eventCh := make(chan api.StreamEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := translateRequest(req, o.numCtx)
		if err != nil {
			errCh <- fmt.Errorf("failed to translate request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST",
			o.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}

		o.setHeaders(httpReq)

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

		// Emit a synthetic message_start event so the query engine knows the response began
		msgStartJSON, _ := json.Marshal(api.MessageResp{
			ID:    "msg_openai",
			Type:  "message",
			Role:  "assistant",
			Model: req.Model,
		})
		select {
		case eventCh <- api.StreamEvent{Type: "message_start", MessageField: msgStartJSON}:
		case <-ctx.Done():
			return
		}

		state := newStreamState()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := line[6:]
			if data == "[DONE]" {
				break
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			events := translateStreamChunk(chunk, state)
			for _, event := range events {
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

func (o *OpenAI) SendMessage(ctx context.Context, httpClient *http.Client, req *api.MessagesRequest) (*api.MessageResp, error) {
	req.Stream = false

	body, err := translateRequest(req, o.numCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to translate request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	o.setHeaders(httpReq)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var oaiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return translateNonStreamingResponse(oaiResp), nil
}

func (o *OpenAI) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
}

// ListModels queries the /v1/models endpoint to discover available models.
// This works with OpenAI, Groq, Ollama, vLLM, and other OpenAI-compatible APIs.
// Returns api.ModelInfo slices (satisfies api.ModelLister interface).
func (o *OpenAI) ListModels(ctx context.Context, httpClient *http.Client) ([]api.ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	o.setHeaders(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model discovery failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("model discovery returned %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}

	var result struct {
		Data []api.ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse model list: %w", err)
	}
	return result.Data, nil
}
