package lua

import (
	"os"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/auth"
	"github.com/Abraxas-365/claudio/internal/auth/storage"
)

// newTestAPIClient creates a minimal api.Client for provider registration tests.
func newTestAPIClient() *api.Client {
	store := storage.NewPlaintextStorage(os.TempDir() + "/claudio-test-creds.json")
	resolver := auth.NewResolver(store)
	return api.NewClient(resolver)
}

func TestProviderAPI_Register_OpenAI(t *testing.T) {
	r := testRuntime(t)

	_, err := r.ExecString(`
		claudio.register_provider({
			name     = "groq",
			type     = "openai",
			base_url = "https://api.groq.com/openai/v1",
			api_key  = "test-key",
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.pendingProviders) != 1 {
		t.Fatalf("want 1 pending provider, got %d", len(r.pendingProviders))
	}
	p := r.pendingProviders[0]
	if p.Name != "groq" {
		t.Errorf("Name: want groq, got %q", p.Name)
	}
	if p.Type != "openai" {
		t.Errorf("Type: want openai, got %q", p.Type)
	}
	if p.BaseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("BaseURL: want https://api.groq.com/openai/v1, got %q", p.BaseURL)
	}
	if p.APIKey != "test-key" {
		t.Errorf("APIKey: want test-key, got %q", p.APIKey)
	}
}

func TestProviderAPI_Register_Ollama(t *testing.T) {
	r := testRuntime(t)

	_, err := r.ExecString(`
		claudio.register_provider({
			name           = "ollama",
			type           = "ollama",
			base_url       = "http://localhost:11434",
			context_window = 32768,
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.pendingProviders) != 1 {
		t.Fatalf("want 1 pending provider, got %d", len(r.pendingProviders))
	}
	p := r.pendingProviders[0]
	if p.Name != "ollama" {
		t.Errorf("Name: want ollama, got %q", p.Name)
	}
	if p.ContextWindow != 32768 {
		t.Errorf("ContextWindow: want 32768, got %d", p.ContextWindow)
	}
}

func TestProviderAPI_Models(t *testing.T) {
	r := testRuntime(t)

	_, err := r.ExecString(`
		claudio.register_provider({
			name = "groq",
			type = "openai",
			models = {
				llama   = "llama-3.3-70b-versatile",
				mixtral = "mixtral-8x7b",
			},
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p := r.pendingProviders[0]
	if p.Models["llama"] != "llama-3.3-70b-versatile" {
		t.Errorf("Models[llama]: want llama-3.3-70b-versatile, got %q", p.Models["llama"])
	}
	if p.Models["mixtral"] != "mixtral-8x7b" {
		t.Errorf("Models[mixtral]: want mixtral-8x7b, got %q", p.Models["mixtral"])
	}
}

func TestProviderAPI_Routes(t *testing.T) {
	r := testRuntime(t)

	_, err := r.ExecString(`
		claudio.register_provider({
			name   = "groq",
			type   = "openai",
			routes = { "llama-*", "mixtral-*" },
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p := r.pendingProviders[0]
	if len(p.Routes) != 2 {
		t.Fatalf("Routes: want 2, got %d", len(p.Routes))
	}
	routeSet := map[string]bool{}
	for _, rt := range p.Routes {
		routeSet[rt] = true
	}
	if !routeSet["llama-*"] {
		t.Error("missing route llama-*")
	}
	if !routeSet["mixtral-*"] {
		t.Error("missing route mixtral-*")
	}
}

func TestProviderAPI_EnvKeyResolution(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "secret-from-env")

	r := testRuntime(t)
	_, err := r.ExecString(`
		claudio.register_provider({
			name    = "groq",
			type    = "openai",
			api_key = "$GROQ_API_KEY",
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	// Raw value stored as-is (resolution happens at ApplyProviders time)
	r.mu.Lock()
	raw := r.pendingProviders[0].APIKey
	r.mu.Unlock()

	if raw != "$GROQ_API_KEY" {
		t.Fatalf("raw APIKey should be $GROQ_API_KEY, got %q", raw)
	}

	// Resolution via resolveEnvVar
	resolved := resolveEnvVar(raw)
	if resolved != "secret-from-env" {
		t.Errorf("resolveEnvVar: want secret-from-env, got %q", resolved)
	}

	// Also verify ApplyProviders correctly registers the provider
	client := newTestAPIClient()
	r.ApplyProviders(client)
	if !client.HasProvider("groq") {
		t.Error("groq provider not registered after ApplyProviders")
	}

	_ = os.Getenv // suppress unused import warning (already used via t.Setenv)
}

func TestProviderAPI_UnknownType(t *testing.T) {
	r := testRuntime(t)

	_, err := r.ExecString(`
		claudio.register_provider({
			name = "mystery",
			type = "totally-unknown",
		})
	`)
	if err != nil {
		t.Fatalf("ExecString: %v", err)
	}

	// pendingProviders has the entry
	r.mu.Lock()
	n := len(r.pendingProviders)
	r.mu.Unlock()
	if n != 1 {
		t.Fatalf("want 1 pending provider, got %d", n)
	}

	// ApplyProviders skips unknown types — no panic, no registration
	client := newTestAPIClient()
	r.ApplyProviders(client) // must not panic

	if client.HasProvider("mystery") {
		t.Error("mystery provider should not be registered for unknown type")
	}
}
