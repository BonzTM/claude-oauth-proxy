package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterFetchAndLookup(t *testing.T) {
	models := openRouterResponse{Data: []openRouterModel{
		{ID: "anthropic/claude-sonnet-4-20250514", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000003", Completion: "0.000015"}},
		{ID: "anthropic/claude-opus-4-20250514", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000015", Completion: "0.000075"}},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	if err := src.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if src.ModelCount() != 2 {
		t.Fatalf("expected 2 models, got %d", src.ModelCount())
	}

	p, ok := src.Lookup("anthropic/claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected exact match lookup to succeed")
	}
	if p.InputPerToken != 0.000003 {
		t.Fatalf("unexpected input price: %v", p.InputPerToken)
	}

	p, ok = src.Lookup("claude-opus-4-20250514")
	if !ok {
		t.Fatal("expected prefix fallback lookup to succeed")
	}
	if p.OutputPerToken != 0.000075 {
		t.Fatalf("unexpected output price: %v", p.OutputPerToken)
	}

	_, ok = src.Lookup("nonexistent-model")
	if ok {
		t.Fatal("expected lookup for nonexistent model to fail")
	}
}

func TestOpenRouterFetchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	if err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestOpenRouterFetchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	if err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error on invalid json")
	}
}

func TestOpenRouterSkipsZeroPricingAndInvalidNumbers(t *testing.T) {
	models := openRouterResponse{Data: []openRouterModel{
		{ID: "free/model", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0", Completion: "0"}},
		{ID: "bad/model", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "notanumber", Completion: "0.001"}},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	if err := src.Fetch(context.Background()); err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if src.ModelCount() != 0 {
		t.Fatalf("expected 0 models (all filtered), got %d", src.ModelCount())
	}
}

func TestOpenRouterDefaultURL(t *testing.T) {
	src := NewOpenRouterSource("", nil)
	if src.url != DefaultOpenRouterURL {
		t.Fatalf("expected default URL, got %s", src.url)
	}
}
