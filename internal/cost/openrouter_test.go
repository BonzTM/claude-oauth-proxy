package cost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

func TestNormalizeModelVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-opus-4-6", "claude-opus-4.6"},
		{"claude-sonnet-4-6", "claude-sonnet-4.6"},
		{"claude-3-5-sonnet", "claude-3.5-sonnet"},
		{"claude-3-7-sonnet", "claude-3.7-sonnet"},
		{"claude-sonnet-4", "claude-sonnet-4"},
		{"claude-opus-4-20250514", "claude-opus-4.20250514"},
		{"no-digits-here", "no-digits-here"},
		{"already-4.6", "already-4.6"},
	}
	for _, tt := range tests {
		if got := normalizeModelVersion(tt.input); got != tt.want {
			t.Errorf("normalizeModelVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOpenRouterLookupVersionNormalization(t *testing.T) {
	models := openRouterResponse{Data: []openRouterModel{
		{ID: "anthropic/claude-opus-4.6", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000005", Completion: "0.000025"}},
		{ID: "anthropic/claude-sonnet-4.6", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000003", Completion: "0.000015"}},
		{ID: "anthropic/claude-3.5-sonnet", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000006", Completion: "0.000030"}},
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

	p, ok := src.Lookup("claude-opus-4-6")
	if !ok {
		t.Fatal("expected version-normalized lookup for claude-opus-4-6 to succeed")
	}
	if p.InputPerToken != 0.000005 {
		t.Fatalf("unexpected input price: %v", p.InputPerToken)
	}

	p, ok = src.Lookup("claude-sonnet-4-6")
	if !ok {
		t.Fatal("expected version-normalized lookup for claude-sonnet-4-6 to succeed")
	}
	if p.OutputPerToken != 0.000015 {
		t.Fatalf("unexpected output price: %v", p.OutputPerToken)
	}

	p, ok = src.Lookup("claude-3-5-sonnet")
	if !ok {
		t.Fatal("expected version-normalized lookup for claude-3-5-sonnet to succeed")
	}
	if p.InputPerToken != 0.000006 {
		t.Fatalf("unexpected input price: %v", p.InputPerToken)
	}

	p, ok = src.Lookup("anthropic/claude-opus-4.6")
	if !ok {
		t.Fatal("expected exact match for anthropic/claude-opus-4.6 to still work")
	}
	if p.InputPerToken != 0.000005 {
		t.Fatalf("unexpected input price: %v", p.InputPerToken)
	}
}

func TestOpenRouterDefaultURL(t *testing.T) {
	src := NewOpenRouterSource("", nil)
	if src.url != DefaultOpenRouterURL {
		t.Fatalf("expected default URL, got %s", src.url)
	}
}

func TestOpenRouterLookupFetchesLazily(t *testing.T) {
	var requests int32
	models := openRouterResponse{Data: []openRouterModel{
		{ID: "anthropic/claude-sonnet-4-20250514", Pricing: struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		}{Prompt: "0.000003", Completion: "0.000015"}},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	p, ok := src.Lookup("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("expected lazy lookup to fetch pricing and succeed")
	}
	if p.InputPerToken != 0.000003 {
		t.Fatalf("unexpected input price: %v", p.InputPerToken)
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected one fetch request, got %d", atomic.LoadInt32(&requests))
	}

	_, _ = src.Lookup("claude-sonnet-4-20250514")
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected no additional fetch after initial lazy fetch, got %d", atomic.LoadInt32(&requests))
	}
}

func TestOpenRouterLookupFetchFailureDoesNotRetry(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	src := NewOpenRouterSource(server.URL, server.Client())
	if _, ok := src.Lookup("claude-sonnet-4-20250514"); ok {
		t.Fatal("expected lookup to fail when lazy fetch fails")
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected one lazy fetch attempt, got %d", atomic.LoadInt32(&requests))
	}

	if _, ok := src.Lookup("claude-sonnet-4-20250514"); ok {
		t.Fatal("expected lookup to continue failing without prices")
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected no retry after first lazy fetch failure, got %d", atomic.LoadInt32(&requests))
	}
}
