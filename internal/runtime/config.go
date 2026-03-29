package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var userHomeDir = os.UserHomeDir

const (
	EnvListenAddr      = "CLAUDE_OAUTH_PROXY_LISTEN_ADDR"
	EnvAPIKey          = "CLAUDE_OAUTH_PROXY_API_KEY"
	EnvTokenFile       = "CLAUDE_OAUTH_PROXY_TOKEN_FILE"
	EnvAnthropicBase   = "CLAUDE_OAUTH_PROXY_ANTHROPIC_BASE_URL"
	EnvOAuthAuthURL    = "CLAUDE_OAUTH_PROXY_OAUTH_AUTH_URL"
	EnvOAuthTokenURL   = "CLAUDE_OAUTH_PROXY_OAUTH_TOKEN_URL"
	EnvOAuthClientID   = "CLAUDE_OAUTH_PROXY_OAUTH_CLIENT_ID"
	EnvOAuthScopes     = "CLAUDE_OAUTH_PROXY_OAUTH_SCOPES"
	EnvOAuthRedirect   = "CLAUDE_OAUTH_PROXY_OAUTH_REDIRECT_URI"
	EnvAnthropicBeta   = "CLAUDE_OAUTH_PROXY_ANTHROPIC_BETA"
	EnvRequestTimeout  = "CLAUDE_OAUTH_PROXY_REQUEST_TIMEOUT"
	EnvLogLevel        = "CLAUDE_OAUTH_PROXY_LOG_LEVEL"
	EnvLogSink         = "CLAUDE_OAUTH_PROXY_LOG_SINK"
	EnvRefreshInterval = "CLAUDE_OAUTH_PROXY_REFRESH_INTERVAL"
	EnvRefreshSkew     = "CLAUDE_OAUTH_PROXY_REFRESH_SKEW"
	EnvSeedFile        = "CLAUDE_OAUTH_PROXY_SEED_FILE"
	EnvCORSOrigins     = "CLAUDE_OAUTH_PROXY_CORS_ORIGINS"
	EnvMaxRequestBody  = "CLAUDE_OAUTH_PROXY_MAX_REQUEST_BODY"
	EnvCCVersion       = "CLAUDE_OAUTH_PROXY_CC_VERSION"
	EnvCCUserAgent     = "CLAUDE_OAUTH_PROXY_CC_USER_AGENT"
	EnvCCSDKVersion    = "CLAUDE_OAUTH_PROXY_CC_SDK_VERSION"
	EnvCCRuntimeVer    = "CLAUDE_OAUTH_PROXY_CC_RUNTIME_VERSION"
	EnvCCOS            = "CLAUDE_OAUTH_PROXY_CC_OS"
	EnvCCArch          = "CLAUDE_OAUTH_PROXY_CC_ARCH"
	EnvCostTracking    = "CLAUDE_OAUTH_PROXY_COST_TRACKING"
	EnvOpenRouterURL   = "CLAUDE_OAUTH_PROXY_OPENROUTER_URL"
)

const (
	DefaultListenAddr      = ":9999"
	DefaultAPIKey          = "sk-proxy-local-key"
	DefaultAnthropicBase   = "https://api.anthropic.com"
	DefaultOAuthAuthURL    = "https://claude.ai/oauth/authorize"
	DefaultOAuthTokenURL   = "https://platform.claude.com/v1/oauth/token"
	DefaultOAuthClientID   = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	DefaultOAuthScopes     = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	DefaultOAuthRedirect   = "https://platform.claude.com/oauth/code/callback"
	DefaultAnthropicBeta   = "oauth-2025-04-20,prompt-caching-2024-07-31"
	DefaultRequestTimeout  = "10m"
	DefaultRefreshInterval = "1m"
	DefaultRefreshSkew     = "5m"
	DefaultCORSOrigins     = ""
	DefaultMaxRequestBody  = "10MB"
	DefaultBillingHeader   = "x-anthropic-billing-header: cc_version=%s; cc_entrypoint=cli; cch=00000;"
	DefaultCCVersion       = "2.1.87"
	DefaultUserAgent       = "claude-cli/2.1.87 (external, cli)"
	DefaultSDKVersion      = "0.74.0"
	DefaultRuntimeVersion  = "v25.8.1"
	DefaultStainlessOS     = "Linux"
	DefaultStainlessArch   = "x64"
)

type Config struct {
	ListenAddr      string
	APIKey          string
	TokenFile       string
	AnthropicBase   string
	OAuthAuthURL    string
	OAuthTokenURL   string
	OAuthClientID   string
	OAuthScopes     string
	OAuthRedirect   string
	AnthropicBeta   string
	BillingHeader   string
	RequestTimeout  string
	RefreshInterval string
	RefreshSkew     string
	SeedFile        string
	CORSOrigins     string
	MaxRequestBody  string
	CCVersion       string
	CCUserAgent     string
	CCSDKVersion    string
	CCRuntimeVer    string
	CCOS            string
	CCArch          string
	CostTracking    bool
	OpenRouterURL   string
}

func DefaultConfig() Config {
	return Config{
		ListenAddr:      DefaultListenAddr,
		APIKey:          DefaultAPIKey,
		TokenFile:       defaultTokenFile(),
		AnthropicBase:   DefaultAnthropicBase,
		OAuthAuthURL:    DefaultOAuthAuthURL,
		OAuthTokenURL:   DefaultOAuthTokenURL,
		OAuthClientID:   DefaultOAuthClientID,
		OAuthScopes:     DefaultOAuthScopes,
		OAuthRedirect:   DefaultOAuthRedirect,
		AnthropicBeta:   DefaultAnthropicBeta,
		BillingHeader:   DefaultBillingHeader,
		CCVersion:       DefaultCCVersion,
		CCUserAgent:     DefaultUserAgent,
		CCSDKVersion:    DefaultSDKVersion,
		CCRuntimeVer:    DefaultRuntimeVersion,
		CCOS:            DefaultStainlessOS,
		CCArch:          DefaultStainlessArch,
		RequestTimeout:  DefaultRequestTimeout,
		RefreshInterval: DefaultRefreshInterval,
		RefreshSkew:     DefaultRefreshSkew,
		CORSOrigins:     DefaultCORSOrigins,
		MaxRequestBody:  DefaultMaxRequestBody,
	}
}

func ConfigFromEnv() Config {
	return configFromEnv(func(key string) string { return os.Getenv(key) }, defaultTokenFile)
}

func configFromEnv(getenv func(string) string, tokenFile func() string) Config {
	if getenv == nil {
		getenv = os.Getenv
	}
	if tokenFile == nil {
		tokenFile = defaultTokenFile
	}
	cfg := DefaultConfig()
	cfg.TokenFile = tokenFile()
	apply := func(value string, target *string) {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			*target = trimmed
		}
	}
	apply(getenv(EnvListenAddr), &cfg.ListenAddr)
	apply(getenv(EnvAPIKey), &cfg.APIKey)
	apply(getenv(EnvTokenFile), &cfg.TokenFile)
	apply(getenv(EnvAnthropicBase), &cfg.AnthropicBase)
	apply(getenv(EnvOAuthAuthURL), &cfg.OAuthAuthURL)
	apply(getenv(EnvOAuthTokenURL), &cfg.OAuthTokenURL)
	apply(getenv(EnvOAuthClientID), &cfg.OAuthClientID)
	apply(getenv(EnvOAuthScopes), &cfg.OAuthScopes)
	apply(getenv(EnvOAuthRedirect), &cfg.OAuthRedirect)
	apply(getenv(EnvAnthropicBeta), &cfg.AnthropicBeta)
	apply(getenv(EnvRequestTimeout), &cfg.RequestTimeout)
	apply(getenv(EnvRefreshInterval), &cfg.RefreshInterval)
	apply(getenv(EnvRefreshSkew), &cfg.RefreshSkew)
	apply(getenv(EnvSeedFile), &cfg.SeedFile)
	apply(getenv(EnvCORSOrigins), &cfg.CORSOrigins)
	apply(getenv(EnvMaxRequestBody), &cfg.MaxRequestBody)
	apply(getenv(EnvCCVersion), &cfg.CCVersion)
	apply(getenv(EnvCCUserAgent), &cfg.CCUserAgent)
	apply(getenv(EnvCCSDKVersion), &cfg.CCSDKVersion)
	apply(getenv(EnvCCRuntimeVer), &cfg.CCRuntimeVer)
	apply(getenv(EnvCCOS), &cfg.CCOS)
	apply(getenv(EnvCCArch), &cfg.CCArch)
	apply(getenv(EnvOpenRouterURL), &cfg.OpenRouterURL)
	if v := strings.TrimSpace(getenv(EnvCostTracking)); v != "" {
		cfg.CostTracking = v == "true" || v == "1" || v == "yes"
	}
	return cfg
}

func (c Config) Validate() error {
	checks := map[string]string{
		"listen address":        c.ListenAddr,
		"api key":               c.APIKey,
		"token file":            c.TokenFile,
		"anthropic base url":    c.AnthropicBase,
		"oauth auth url":        c.OAuthAuthURL,
		"oauth token url":       c.OAuthTokenURL,
		"oauth client id":       c.OAuthClientID,
		"oauth scopes":          c.OAuthScopes,
		"oauth redirect uri":    c.OAuthRedirect,
		"anthropic beta header": c.AnthropicBeta,
	}
	for label, value := range checks {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s must not be empty", label)
		}
	}
	for label, value := range map[string]string{
		"request timeout":  c.RequestTimeout,
		"refresh interval": c.RefreshInterval,
		"refresh skew":     c.RefreshSkew,
	} {
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	if _, err := ParseByteSize(c.MaxRequestBody); err != nil {
		return fmt.Errorf("max request body: %w", err)
	}
	return nil
}

func ParseByteSize(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	s = strings.ToUpper(s)
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("byte size must be positive, got %d", n)
	}
	return n * multiplier, nil
}

func defaultTokenFile() string {
	home, err := userHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".config", "claude-oauth-proxy", "tokens.json")
	}
	return filepath.Join(home, ".config", "claude-oauth-proxy", "tokens.json")
}
