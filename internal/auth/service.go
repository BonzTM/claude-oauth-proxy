package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/tokens"
)

var randomReader io.Reader = rand.Reader
var commandStarter = func(name string, args ...string) error { return exec.Command(name, args...).Start() }

type BrowserOpener interface {
	Open(url string) error
}

type OAuthProvider interface {
	AuthorizationURL(input AuthorizationURLInput) (string, *core.Error)
	ExchangeCode(ctx context.Context, input ExchangeCodeInput) (tokens.TokenSet, *core.Error)
	RefreshToken(ctx context.Context, input RefreshTokenInput) (tokens.TokenSet, *core.Error)
}

type AuthorizationURLInput struct {
	State         string
	CodeChallenge string
	RedirectURI   string
}

type ExchangeCodeInput struct {
	Code         string
	State        string
	CodeVerifier string
	RedirectURI  string
}

type RefreshTokenInput struct {
	RefreshToken string
}

type Service interface {
	PrepareLogin(ctx context.Context, input PrepareLoginInput) (PrepareLoginOutput, *core.Error)
	CompleteLogin(ctx context.Context, input CompleteLoginInput) (CompleteLoginOutput, *core.Error)
	Status(ctx context.Context, input StatusInput) (StatusOutput, *core.Error)
	Logout(ctx context.Context, input LogoutInput) (LogoutOutput, *core.Error)
	AccessToken(ctx context.Context, input AccessTokenInput) (AccessTokenOutput, *core.Error)
}

type PrepareLoginInput struct {
	OpenBrowser bool
}

type PrepareLoginOutput struct {
	AuthURL      string
	State        string
	CodeVerifier string
}

type CompleteLoginInput struct {
	Code         string
	State        string
	CodeVerifier string
	RedirectURI  string
}

type CompleteLoginOutput struct {
	TokenPath string
	ExpiresAt time.Time
}

type StatusInput struct{}

type StatusOutput struct {
	TokenPath string
	Exists    bool
	ExpiresAt time.Time
	Expired   bool
}

type LogoutInput struct{}

type LogoutOutput struct {
	TokenPath string
}

type AccessTokenInput struct {
	ForceRefresh bool
}

type AccessTokenOutput struct {
	Token     string
	ExpiresAt time.Time
}

type Config struct {
	RedirectURI string
	RefreshSkew time.Duration
}

type service struct {
	store    tokens.Store
	provider OAuthProvider
	opener   BrowserOpener
	now      func() time.Time
	cfg      Config
	mu       sync.Mutex
}

func NewService(cfg Config, store tokens.Store, provider OAuthProvider, opener BrowserOpener, now func() time.Time) Service {
	if now == nil {
		now = time.Now
	}
	if cfg.RefreshSkew <= 0 {
		cfg.RefreshSkew = 5 * time.Minute
	}
	return &service{store: store, provider: provider, opener: opener, now: now, cfg: cfg}
}

func (s *service) PrepareLogin(ctx context.Context, input PrepareLoginInput) (PrepareLoginOutput, *core.Error) {
	if s == nil || s.provider == nil {
		return PrepareLoginOutput{}, core.NewError("AUTH_UNAVAILABLE", http.StatusInternalServerError, "auth service is unavailable", nil)
	}
	verifier, challenge, err := generatePKCE()
	if err != nil {
		return PrepareLoginOutput{}, core.NewError("PKCE_GENERATION_FAILED", http.StatusInternalServerError, "generate pkce challenge", err)
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return PrepareLoginOutput{}, core.NewError("STATE_GENERATION_FAILED", http.StatusInternalServerError, "generate oauth state", err)
	}
	authURL, apiErr := s.provider.AuthorizationURL(AuthorizationURLInput{State: state, CodeChallenge: challenge, RedirectURI: s.cfg.RedirectURI})
	if apiErr != nil {
		return PrepareLoginOutput{}, apiErr
	}
	if input.OpenBrowser && s.opener != nil {
		if err := s.opener.Open(authURL); err != nil {
			return PrepareLoginOutput{}, core.NewError("BROWSER_OPEN_FAILED", http.StatusBadGateway, "open browser", err)
		}
	}
	return PrepareLoginOutput{AuthURL: authURL, State: state, CodeVerifier: verifier}, nil
}

func (s *service) CompleteLogin(ctx context.Context, input CompleteLoginInput) (CompleteLoginOutput, *core.Error) {
	if strings.TrimSpace(input.Code) == "" {
		return CompleteLoginOutput{}, core.NewError("CODE_REQUIRED", http.StatusBadRequest, "oauth authorization code is required", nil)
	}
	redirectURI := input.RedirectURI
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = s.cfg.RedirectURI
	}
	tokenSet, apiErr := s.provider.ExchangeCode(ctx, ExchangeCodeInput{Code: strings.TrimSpace(input.Code), State: input.State, CodeVerifier: input.CodeVerifier, RedirectURI: redirectURI})
	if apiErr != nil {
		return CompleteLoginOutput{}, apiErr
	}
	if err := s.store.Save(ctx, tokenSet); err != nil {
		return CompleteLoginOutput{}, core.NewError("TOKEN_SAVE_FAILED", http.StatusInternalServerError, "save tokens", err)
	}
	return CompleteLoginOutput{TokenPath: s.store.Path(), ExpiresAt: tokenSet.ExpiresAt}, nil
}

func (s *service) Status(ctx context.Context, _ StatusInput) (StatusOutput, *core.Error) {
	tokenSet, err := s.store.Load(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return StatusOutput{TokenPath: s.store.Path(), Exists: false}, nil
	}
	if err != nil {
		return StatusOutput{}, core.NewError("TOKEN_LOAD_FAILED", http.StatusInternalServerError, "load tokens", err)
	}
	return StatusOutput{TokenPath: s.store.Path(), Exists: true, ExpiresAt: tokenSet.ExpiresAt, Expired: tokenSet.Expired(s.now, s.cfg.RefreshSkew)}, nil
}

func (s *service) Logout(ctx context.Context, _ LogoutInput) (LogoutOutput, *core.Error) {
	if err := s.store.Delete(ctx); err != nil {
		return LogoutOutput{}, core.NewError("TOKEN_DELETE_FAILED", http.StatusInternalServerError, "delete token file", err)
	}
	return LogoutOutput{TokenPath: s.store.Path()}, nil
}

func (s *service) AccessToken(ctx context.Context, input AccessTokenInput) (AccessTokenOutput, *core.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenSet, err := s.store.Load(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return AccessTokenOutput{}, core.NewError("TOKEN_MISSING", http.StatusServiceUnavailable, "no saved claude oauth tokens were found", err)
	}
	if err != nil {
		return AccessTokenOutput{}, core.NewError("TOKEN_LOAD_FAILED", http.StatusInternalServerError, "load tokens", err)
	}
	if !input.ForceRefresh && !tokenSet.Expired(s.now, s.cfg.RefreshSkew) {
		return AccessTokenOutput{Token: tokenSet.AccessToken, ExpiresAt: tokenSet.ExpiresAt}, nil
	}
	if strings.TrimSpace(tokenSet.RefreshToken) == "" {
		return AccessTokenOutput{}, core.NewError("REFRESH_TOKEN_MISSING", http.StatusServiceUnavailable, "refresh token is missing", nil)
	}
	refreshed, apiErr := s.provider.RefreshToken(ctx, RefreshTokenInput{RefreshToken: tokenSet.RefreshToken})
	if apiErr != nil {
		return AccessTokenOutput{}, apiErr
	}
	if strings.TrimSpace(refreshed.RefreshToken) == "" {
		refreshed.RefreshToken = tokenSet.RefreshToken
	}
	if err := s.store.Save(ctx, refreshed); err != nil {
		return AccessTokenOutput{}, core.NewError("TOKEN_SAVE_FAILED", http.StatusInternalServerError, "save refreshed tokens", err)
	}
	return AccessTokenOutput{Token: refreshed.AccessToken, ExpiresAt: refreshed.ExpiresAt}, nil
}

type ExecBrowserOpener struct{}

func (ExecBrowserOpener) Open(target string) error {
	name, args := browserCommand(runtime.GOOS, target)
	return commandStarter(name, args...)
}

func browserCommand(goos, target string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{target}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		return "xdg-open", []string{target}
	}
}

type ClaudeOAuthProviderConfig struct {
	AuthURL     string
	TokenURL    string
	ClientID    string
	Scopes      string
	RedirectURI string
}

type claudeOAuthProvider struct {
	client *http.Client
	cfg    ClaudeOAuthProviderConfig
	now    func() time.Time
}

func NewClaudeOAuthProvider(client *http.Client, cfg ClaudeOAuthProviderConfig, now func() time.Time) OAuthProvider {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if now == nil {
		now = time.Now
	}
	return &claudeOAuthProvider{client: client, cfg: cfg, now: now}
}

func (p *claudeOAuthProvider) AuthorizationURL(input AuthorizationURLInput) (string, *core.Error) {
	values := url.Values{}
	values.Set("client_id", p.cfg.ClientID)
	values.Set("response_type", "code")
	values.Set("redirect_uri", input.RedirectURI)
	values.Set("scope", p.cfg.Scopes)
	values.Set("code_challenge", input.CodeChallenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", input.State)
	return p.cfg.AuthURL + "?" + values.Encode(), nil
}

func (p *claudeOAuthProvider) ExchangeCode(ctx context.Context, input ExchangeCodeInput) (tokens.TokenSet, *core.Error) {
	return p.postToken(ctx, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     p.cfg.ClientID,
		"code":          input.Code,
		"state":         input.State,
		"redirect_uri":  input.RedirectURI,
		"code_verifier": input.CodeVerifier,
	}, "")
}

func (p *claudeOAuthProvider) RefreshToken(ctx context.Context, input RefreshTokenInput) (tokens.TokenSet, *core.Error) {
	return p.postToken(ctx, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     p.cfg.ClientID,
		"refresh_token": input.RefreshToken,
	}, input.RefreshToken)
}

func (p *claudeOAuthProvider) postToken(ctx context.Context, body map[string]string, previousRefreshToken string) (tokens.TokenSet, *core.Error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return tokens.TokenSet{}, core.NewError("TOKEN_ENCODE_FAILED", http.StatusInternalServerError, "encode oauth token request", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(string(payload)))
	if err != nil {
		return tokens.TokenSet{}, core.NewError("TOKEN_REQUEST_FAILED", http.StatusBadGateway, "build oauth token request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return tokens.TokenSet{}, core.NewError("TOKEN_REQUEST_FAILED", http.StatusBadGateway, "execute oauth token request", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokens.TokenSet{}, core.NewError("TOKEN_RESPONSE_READ_FAILED", http.StatusBadGateway, "read oauth token response", err)
	}
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(data, &tokenResponse); err != nil {
		return tokens.TokenSet{}, core.NewError("TOKEN_RESPONSE_INVALID", http.StatusBadGateway, "decode oauth token response", err)
	}
	if resp.StatusCode >= http.StatusBadRequest || tokenResponse.Error != "" {
		message := tokenResponse.ErrorDesc
		if strings.TrimSpace(message) == "" {
			message = string(data)
		}
		return tokens.TokenSet{}, core.NewError("TOKEN_EXCHANGE_FAILED", http.StatusBadGateway, message, nil)
	}
	refreshToken := tokenResponse.RefreshToken
	if strings.TrimSpace(refreshToken) == "" {
		refreshToken = previousRefreshToken
	}
	return tokens.TokenSet{AccessToken: tokenResponse.AccessToken, RefreshToken: refreshToken, TokenType: tokenResponse.TokenType, Scope: tokenResponse.Scope, ExpiresAt: p.now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)}, nil
}

func generatePKCE() (string, string, error) {
	b := make([]byte, 64)
	if _, err := io.ReadFull(randomReader, b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	if len(verifier) > 128 {
		verifier = verifier[:128]
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomBase64URL(length int) (string, error) {
	b := make([]byte, length)
	if _, err := io.ReadFull(randomReader, b); err != nil {
		return "", err
	}
	value := base64.RawURLEncoding.EncodeToString(b)
	if len(value) > length {
		value = value[:length]
	}
	return value, nil
}

type loggingService struct {
	next   Service
	logger logging.Logger
	now    func() time.Time
}

func WithLogging(next Service, logger logging.Logger) Service {
	return WithLoggingClock(next, logger, time.Now)
}

func WithLoggingClock(next Service, logger logging.Logger, now func() time.Time) Service {
	if next == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &loggingService{next: next, logger: logging.Normalize(logger), now: now}
}

func (s *loggingService) PrepareLogin(ctx context.Context, input PrepareLoginInput) (PrepareLoginOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationAuthPrepareLogin, func() (PrepareLoginOutput, *core.Error) {
		return s.next.PrepareLogin(ctx, input)
	})
}

func (s *loggingService) CompleteLogin(ctx context.Context, input CompleteLoginInput) (CompleteLoginOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationAuthCompleteLogin, func() (CompleteLoginOutput, *core.Error) {
		return s.next.CompleteLogin(ctx, input)
	})
}

func (s *loggingService) Status(ctx context.Context, input StatusInput) (StatusOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationAuthStatus, func() (StatusOutput, *core.Error) {
		return s.next.Status(ctx, input)
	})
}

func (s *loggingService) Logout(ctx context.Context, input LogoutInput) (LogoutOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationAuthLogout, func() (LogoutOutput, *core.Error) {
		return s.next.Logout(ctx, input)
	})
}

func (s *loggingService) AccessToken(ctx context.Context, input AccessTokenInput) (AccessTokenOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationAuthAccessToken, func() (AccessTokenOutput, *core.Error) {
		return s.next.AccessToken(ctx, input)
	})
}

func withOperation[T any](ctx context.Context, now func() time.Time, logger logging.Logger, operation string, run func() (T, *core.Error)) (T, *core.Error) {
	startedAt := now()
	logger.Info(ctx, logging.EventServiceOperationStart, "operation", operation)
	result, err := run()
	durationMS := now().Sub(startedAt).Milliseconds()
	fields := []any{"operation", operation, "duration_ms", durationMS, "ok", err == nil}
	if err != nil && err.Code != "" {
		fields = append(fields, "error_code", err.Code)
		logger.Error(ctx, logging.EventServiceOperationFinish, fields...)
	} else {
		logger.Info(ctx, logging.EventServiceOperationFinish, fields...)
	}
	return result, err
}
