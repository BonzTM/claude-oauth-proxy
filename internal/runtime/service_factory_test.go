package runtime

import (
	"path/filepath"
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
