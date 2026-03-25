package runtime

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/logging"
)

func TestNewAppWithLoggerBuildsDependencies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	app, err := NewAppWithLogger(cfg, logging.NewRecorder())
	if err != nil {
		t.Fatalf("new app with logger: %v", err)
	}
	if app.Auth == nil || app.Handler == nil || app.Config.TokenFile == "" {
		t.Fatalf("unexpected app: %+v", app)
	}
	if app.RefreshInterval != time.Minute {
		t.Fatalf("unexpected refresh interval: %v", app.RefreshInterval)
	}
}

func TestNewAppWithLoggerRejectsBadRefreshSkew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	cfg.RefreshSkew = "not-a-duration"
	if _, err := NewAppWithLogger(cfg, logging.NewRecorder()); err == nil {
		t.Fatal("expected refresh skew parse error")
	}
	cfg = DefaultConfig()
	cfg.APIKey = ""
	if _, err := NewAppWithLogger(cfg, nil); err == nil {
		t.Fatal("expected config validation error")
	}
}

func TestNewAppWithLoggerRejectsBadRequestTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	cfg.RequestTimeout = "invalid"
	if _, err := NewAppWithLogger(cfg, logging.NewRecorder()); err == nil {
		t.Fatal("expected request timeout parse error")
	}
}

func TestNewAppWithLoggerRejectsBadRefreshInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	cfg.RefreshInterval = "invalid"
	if _, err := NewAppWithLogger(cfg, logging.NewRecorder()); err == nil {
		t.Fatal("expected refresh interval parse error")
	}
}

func TestNewAppWithLoggerRejectsBadMaxRequestBody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	cfg.MaxRequestBody = "not-a-size"
	if _, err := NewAppWithLogger(cfg, logging.NewRecorder()); err == nil {
		t.Fatal("expected max request body parse error")
	}
}

func TestNewAppWithLoggerDoesNotFetchOpenRouterAtStartup(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.TokenFile = filepath.Join(t.TempDir(), "tokens.json")
	cfg.CostTracking = true
	cfg.OpenRouterURL = server.URL
	app, err := NewAppWithLogger(cfg, logging.NewRecorder())
	if err != nil {
		t.Fatalf("new app with cost tracking: %v", err)
	}
	if app.Auth == nil || app.Handler == nil {
		t.Fatalf("unexpected app: %+v", app)
	}
	if atomic.LoadInt32(&requests) != 0 {
		t.Fatalf("expected no OpenRouter calls during startup, got %d", atomic.LoadInt32(&requests))
	}
}
