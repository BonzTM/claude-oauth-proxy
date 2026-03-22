package runtime

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvUsesDefaults(t *testing.T) {
	cfg := configFromEnv(func(string) string { return "" }, func() string { return "/tmp/tokens.json" })
	if cfg.ListenAddr != DefaultListenAddr {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if cfg.TokenFile != "/tmp/tokens.json" {
		t.Fatalf("unexpected token file: %q", cfg.TokenFile)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestConfigFromEnvOverridesKnownValues(t *testing.T) {
	values := map[string]string{
		EnvListenAddr:    "127.0.0.1:7777",
		EnvAPIKey:        "key-1",
		EnvTokenFile:     "/tmp/custom.json",
		EnvAnthropicBase: "https://example.com",
	}
	cfg := configFromEnv(func(key string) string { return values[key] }, func() string { return filepath.Join("ignored", "tokens.json") })
	if cfg.ListenAddr != "127.0.0.1:7777" || cfg.APIKey != "key-1" || cfg.TokenFile != "/tmp/custom.json" || cfg.AnthropicBase != "https://example.com" {
		t.Fatalf("unexpected env overrides: %+v", cfg)
	}
}

func TestConfigValidateRejectsMissingFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKey = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestConfigFromEnvAndDefaultTokenFileFallback(t *testing.T) {
	t.Setenv(EnvListenAddr, "127.0.0.1:9998")
	if got := ConfigFromEnv().ListenAddr; got != "127.0.0.1:9998" {
		t.Fatalf("unexpected ConfigFromEnv listen addr: %q", got)
	}
	previousUserHomeDir := userHomeDir
	defer func() { userHomeDir = previousUserHomeDir }()
	userHomeDir = func() (string, error) { return "", filepath.ErrBadPattern }
	if got := defaultTokenFile(); got != filepath.Join(".config", "claude-oauth-proxy", "tokens.json") {
		t.Fatalf("unexpected fallback token file: %q", got)
	}
}
