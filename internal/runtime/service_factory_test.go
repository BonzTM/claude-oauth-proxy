package runtime

import (
	"path/filepath"
	"testing"

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
