package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Abraxas-365/claudio/internal/api"
	"github.com/Abraxas-365/claudio/internal/api/provider"
)

func TestOpenAI_ListModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("expected /models, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "llama-3.3-70b", "owned_by": "meta"},
				{"id": "mixtral-8x7b", "owned_by": "mistral"},
			},
		})
	}))
	defer srv.Close()

	p := provider.NewOpenAI("test", srv.URL, "test-key")
	models, err := p.ListModels(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "llama-3.3-70b" {
		t.Fatalf("expected llama-3.3-70b, got %s", models[0].ID)
	}
}

func TestOpenAI_ListModels_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	p := provider.NewOpenAI("test", srv.URL, "key")
	_, err := p.ListModels(context.Background(), http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestOpenAI_ListModels_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-key" {
			t.Fatalf("expected Bearer auth header, got %q", auth)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	p := provider.NewOpenAI("test", srv.URL, "my-secret-key")
	_, err := p.ListModels(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAI_ImplementsModelLister(t *testing.T) {
	p := provider.NewOpenAI("test", "http://localhost", "key")
	if _, ok := interface{}(p).(api.ModelLister); !ok {
		t.Fatal("OpenAI provider should implement api.ModelLister interface")
	}
}
