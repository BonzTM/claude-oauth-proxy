package runtime

import (
	"fmt"
	"net/http"
	"time"

	httpadapter "github.com/bonztm/claude-oauth-proxy/internal/adapters/http"
	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
	antprovider "github.com/bonztm/claude-oauth-proxy/internal/provider/anthropic"
	"github.com/bonztm/claude-oauth-proxy/internal/tokens"
)

type App struct {
	Config          Config
	Auth            auth.Service
	Handler         http.Handler
	RefreshInterval time.Duration
}

func NewAppWithLogger(cfg Config, logger logging.Logger) (App, error) {
	logger = logging.Normalize(logger)
	if err := cfg.Validate(); err != nil {
		return App{}, err
	}
	refreshSkew, err := time.ParseDuration(cfg.RefreshSkew)
	if err != nil {
		return App{}, fmt.Errorf("refresh skew: %w", err)
	}
	refreshInterval, err := time.ParseDuration(cfg.RefreshInterval)
	if err != nil {
		return App{}, fmt.Errorf("refresh interval: %w", err)
	}
	requestTimeout, err := time.ParseDuration(cfg.RequestTimeout)
	if err != nil {
		return App{}, fmt.Errorf("request timeout: %w", err)
	}
	maxRequestBody, err := ParseByteSize(cfg.MaxRequestBody)
	if err != nil {
		return App{}, fmt.Errorf("max request body: %w", err)
	}
	var store tokens.Store = tokens.NewFileStore(cfg.TokenFile)
	if cfg.SeedFile != "" {
		store = tokens.NewFallbackStore(tokens.NewFileStore(cfg.TokenFile), tokens.NewFileStore(cfg.SeedFile))
	}
	authService := auth.WithLogging(auth.NewService(auth.Config{RedirectURI: cfg.OAuthRedirect, RefreshSkew: refreshSkew}, store, auth.NewClaudeOAuthProvider(&http.Client{Timeout: 15 * time.Second}, auth.ClaudeOAuthProviderConfig{AuthURL: cfg.OAuthAuthURL, TokenURL: cfg.OAuthTokenURL, ClientID: cfg.OAuthClientID, Scopes: cfg.OAuthScopes, RedirectURI: cfg.OAuthRedirect}, time.Now), auth.ExecBrowserOpener{}, time.Now), logger)
	providerService := provider.WithLogging(antprovider.New(antprovider.Config{BaseURL: cfg.AnthropicBase, BetaHeader: cfg.AnthropicBeta, BillingHeader: cfg.BillingHeader, CCVersion: cfg.CCVersion, UserAgent: cfg.CCUserAgent, SDKVersion: cfg.CCSDKVersion, RuntimeVersion: cfg.CCRuntimeVer, StainlessOS: cfg.CCOS, StainlessArch: cfg.CCArch, RequestTimeout: requestTimeout, HTTPClient: &http.Client{}, Now: time.Now}, authService), logger)
	handler := httpadapter.NewHandler(providerService, authService, cfg.APIKey, logger, time.Now, httpadapter.HandlerConfig{CORSOrigins: cfg.CORSOrigins, MaxRequestBody: maxRequestBody})
	return App{Config: cfg, Auth: authService, Handler: handler, RefreshInterval: refreshInterval}, nil
}
