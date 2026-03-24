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
		EnvListenAddr:     "127.0.0.1:7777",
		EnvAPIKey:         "key-1",
		EnvTokenFile:      "/tmp/custom.json",
		EnvAnthropicBase:  "https://example.com",
		EnvCORSOrigins:    "http://localhost:3000",
		EnvMaxRequestBody: "5MB",
	}
	cfg := configFromEnv(func(key string) string { return values[key] }, func() string { return filepath.Join("ignored", "tokens.json") })
	if cfg.ListenAddr != "127.0.0.1:7777" || cfg.APIKey != "key-1" || cfg.TokenFile != "/tmp/custom.json" || cfg.AnthropicBase != "https://example.com" {
		t.Fatalf("unexpected env overrides: %+v", cfg)
	}
	if cfg.CORSOrigins != "http://localhost:3000" {
		t.Fatalf("unexpected cors origins: %q", cfg.CORSOrigins)
	}
	if cfg.MaxRequestBody != "5MB" {
		t.Fatalf("unexpected max request body: %q", cfg.MaxRequestBody)
	}
}

func TestConfigValidateRejectsMissingFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKey = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestConfigValidateRejectsBadDurations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestTimeout = "not-a-duration"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected request timeout validation error")
	}
	cfg = DefaultConfig()
	cfg.RefreshInterval = "bad"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected refresh interval validation error")
	}
	cfg = DefaultConfig()
	cfg.RefreshSkew = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected refresh skew validation error")
	}
}

func TestConfigValidateRejectsBadMaxRequestBody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRequestBody = "not-a-size"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected max request body validation error")
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

func TestParseByteSize(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{"10MB", 10 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"512KB", 512 * 1024},
		{"100B", 100},
		{"42", 42},
		{" 5 MB ", 5 * 1024 * 1024},
	} {
		got, err := ParseByteSize(tc.input)
		if err != nil {
			t.Fatalf("ParseByteSize(%q): unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseByteSize(%q): got=%d want=%d", tc.input, got, tc.want)
		}
	}
	for _, bad := range []string{"", "abc", "-1MB", "0MB"} {
		if _, err := ParseByteSize(bad); err == nil {
			t.Fatalf("ParseByteSize(%q): expected error", bad)
		}
	}
}
