package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/tokens"
)

type fakeOAuthProvider struct {
	authURL       string
	authorizeIn   AuthorizationURLInput
	exchangeOut   tokens.TokenSet
	exchangeErr   *core.Error
	refreshOut    tokens.TokenSet
	refreshErr    *core.Error
	refreshCalls  int
	exchangeCalls int
}

func (f *fakeOAuthProvider) AuthorizationURL(input AuthorizationURLInput) (string, *core.Error) {
	f.authorizeIn = input
	return f.authURL, nil
}

func (f *fakeOAuthProvider) ExchangeCode(_ context.Context, _ ExchangeCodeInput) (tokens.TokenSet, *core.Error) {
	f.exchangeCalls++
	if f.exchangeErr != nil {
		return tokens.TokenSet{}, f.exchangeErr
	}
	return f.exchangeOut, nil
}

func (f *fakeOAuthProvider) RefreshToken(_ context.Context, _ RefreshTokenInput) (tokens.TokenSet, *core.Error) {
	f.refreshCalls++
	if f.refreshErr != nil {
		return tokens.TokenSet{}, f.refreshErr
	}
	return f.refreshOut, nil
}

type fakeBrowserOpener struct {
	openedURL string
	err       error
}

func (f *fakeBrowserOpener) Open(target string) error {
	f.openedURL = target
	return f.err
}

type errorStore struct {
	loadErr   error
	saveErr   error
	deleteErr error
	path      string
}

func (s errorStore) Load(context.Context) (tokens.TokenSet, error) {
	return tokens.TokenSet{}, s.loadErr
}
func (s errorStore) Save(context.Context, tokens.TokenSet) error { return s.saveErr }
func (s errorStore) Delete(context.Context) error                { return s.deleteErr }
func (s errorStore) Path() string                                { return s.path }

type fakeAuthService struct {
	errByOperation map[string]*core.Error
}

func (f fakeAuthService) PrepareLogin(_ context.Context, _ PrepareLoginInput) (PrepareLoginOutput, *core.Error) {
	return PrepareLoginOutput{}, f.errByOperation[logging.OperationAuthPrepareLogin]
}

func (f fakeAuthService) CompleteLogin(_ context.Context, _ CompleteLoginInput) (CompleteLoginOutput, *core.Error) {
	return CompleteLoginOutput{}, f.errByOperation[logging.OperationAuthCompleteLogin]
}

func (f fakeAuthService) Status(_ context.Context, _ StatusInput) (StatusOutput, *core.Error) {
	return StatusOutput{}, f.errByOperation[logging.OperationAuthStatus]
}

func (f fakeAuthService) Logout(_ context.Context, _ LogoutInput) (LogoutOutput, *core.Error) {
	return LogoutOutput{}, f.errByOperation[logging.OperationAuthLogout]
}

func (f fakeAuthService) AccessToken(_ context.Context, _ AccessTokenInput) (AccessTokenOutput, *core.Error) {
	return AccessTokenOutput{}, f.errByOperation[logging.OperationAuthAccessToken]
}

func TestPrepareLoginCompleteLoginStatusLogoutAndAccessToken(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	store := tokens.NewFileStore(filepath.Join(t.TempDir(), "tokens.json"))
	provider := &fakeOAuthProvider{
		authURL:     "https://claude.ai/oauth/authorize?client_id=test",
		exchangeOut: tokens.TokenSet{AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: now.Add(time.Hour)},
		refreshOut:  tokens.TokenSet{AccessToken: "access-2", ExpiresAt: now.Add(2 * time.Hour)},
	}
	opener := &fakeBrowserOpener{}
	service := NewService(Config{RedirectURI: "https://platform.claude.com/oauth/code/callback", RefreshSkew: 5 * time.Minute}, store, provider, opener, func() time.Time { return now })

	prepared, apiErr := service.PrepareLogin(context.Background(), PrepareLoginInput{OpenBrowser: true})
	if apiErr != nil {
		t.Fatalf("prepare login: %v", apiErr)
	}
	if prepared.AuthURL != provider.authURL || opener.openedURL != provider.authURL {
		t.Fatalf("unexpected prepared login output: %+v opener=%q", prepared, opener.openedURL)
	}
	if prepared.State == "" || prepared.CodeVerifier == "" || provider.authorizeIn.CodeChallenge == "" {
		t.Fatalf("expected generated login data: %+v %+v", prepared, provider.authorizeIn)
	}
	preparedNoBrowser, apiErr := service.PrepareLogin(context.Background(), PrepareLoginInput{})
	if apiErr != nil || preparedNoBrowser.AuthURL != provider.authURL {
		t.Fatalf("unexpected prepare login without browser result: %+v err=%v", preparedNoBrowser, apiErr)
	}

	completed, apiErr := service.CompleteLogin(context.Background(), CompleteLoginInput{Code: "abc123", State: prepared.State, CodeVerifier: prepared.CodeVerifier})
	if apiErr != nil {
		t.Fatalf("complete login: %v", apiErr)
	}
	if completed.TokenPath != store.Path() {
		t.Fatalf("unexpected token path: %+v", completed)
	}
	status, apiErr := service.Status(context.Background(), StatusInput{})
	if apiErr != nil || !status.Exists || status.Expired {
		t.Fatalf("unexpected status: %+v err=%v", status, apiErr)
	}
	access, apiErr := service.AccessToken(context.Background(), AccessTokenInput{})
	if apiErr != nil || access.Token != "access-1" {
		t.Fatalf("unexpected access token: %+v err=%v", access, apiErr)
	}

	if err := store.Save(context.Background(), tokens.TokenSet{AccessToken: "access-old", RefreshToken: "refresh-old", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("seed expiring token: %v", err)
	}
	access, apiErr = service.AccessToken(context.Background(), AccessTokenInput{})
	if apiErr != nil || access.Token != "access-2" || provider.refreshCalls != 1 {
		t.Fatalf("unexpected refreshed access token: %+v err=%v refreshCalls=%d", access, apiErr, provider.refreshCalls)
	}

	loggedOut, apiErr := service.Logout(context.Background(), LogoutInput{})
	if apiErr != nil {
		t.Fatalf("logout: %v", apiErr)
	}
	if loggedOut.TokenPath != store.Path() {
		t.Fatalf("unexpected logout output: %+v", loggedOut)
	}
	status, apiErr = service.Status(context.Background(), StatusInput{})
	if apiErr != nil || status.Exists {
		t.Fatalf("expected missing status after logout: %+v err=%v", status, apiErr)
	}
}

func TestServiceErrorsAndRefreshRequirements(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	store := tokens.NewFileStore(filepath.Join(t.TempDir(), "tokens.json"))
	provider := &fakeOAuthProvider{authURL: "https://claude.ai/oauth/authorize", exchangeErr: core.NewError("EXCHANGE_FAILED", http.StatusBadGateway, "exchange failed", nil), refreshErr: core.NewError("REFRESH_FAILED", http.StatusBadGateway, "refresh failed", nil)}
	opener := &fakeBrowserOpener{err: errors.New("open failed")}
	service := NewService(Config{RedirectURI: "redirect", RefreshSkew: time.Minute}, store, provider, opener, func() time.Time { return now })

	// Browser open failure is non-fatal; PrepareLogin should succeed even when opener fails.
	if _, apiErr := service.PrepareLogin(context.Background(), PrepareLoginInput{OpenBrowser: true}); apiErr != nil {
		t.Fatalf("expected no error when browser fails to open, got %v", apiErr)
	}
	if _, apiErr := service.CompleteLogin(context.Background(), CompleteLoginInput{}); apiErr == nil || apiErr.Code != "CODE_REQUIRED" {
		t.Fatalf("expected code required error, got %v", apiErr)
	}
	if err := store.Save(context.Background(), tokens.TokenSet{AccessToken: "expired", ExpiresAt: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("save expired token: %v", err)
	}
	if _, apiErr := service.AccessToken(context.Background(), AccessTokenInput{}); apiErr == nil || apiErr.Code != "REFRESH_TOKEN_MISSING" {
		t.Fatalf("expected refresh token missing error, got %v", apiErr)
	}
	if _, apiErr := service.CompleteLogin(context.Background(), CompleteLoginInput{Code: "code", State: "state", CodeVerifier: "verifier"}); apiErr == nil || apiErr.Code != "EXCHANGE_FAILED" {
		t.Fatalf("expected exchange failure, got %v", apiErr)
	}
	if _, apiErr := service.AccessToken(context.Background(), AccessTokenInput{}); apiErr == nil || apiErr.Code != "REFRESH_TOKEN_MISSING" {
		t.Fatalf("expected refresh token missing error after failed exchange, got %v", apiErr)
	}
	if _, apiErr := NewService(Config{}, store, nil, nil, nil).PrepareLogin(context.Background(), PrepareLoginInput{}); apiErr == nil || apiErr.Code != "AUTH_UNAVAILABLE" {
		t.Fatalf("expected auth unavailable error, got %v", apiErr)
	}
}

func TestClaudeOAuthProviderBuildsURLAndPostsTokens(t *testing.T) {
	requests := make([]url.Values, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		requests = append(requests, mapToValues(payload))
		if payload["grant_type"] == "refresh_token" {
			_, _ = w.Write([]byte(`{"access_token":"access-2","token_type":"Bearer","expires_in":60,"scope":"scope"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer","expires_in":60,"scope":"scope"}`))
	}))
	defer server.Close()
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	provider := NewClaudeOAuthProvider(server.Client(), ClaudeOAuthProviderConfig{AuthURL: "https://claude.ai/oauth/authorize", TokenURL: server.URL, ClientID: "client-id", Scopes: "scope-a scope-b", RedirectURI: "https://platform.claude.com/oauth/code/callback"}, func() time.Time { return now })
	authURL, apiErr := provider.AuthorizationURL(AuthorizationURLInput{State: "state-1", CodeChallenge: "challenge-1", RedirectURI: "https://platform.claude.com/oauth/code/callback"})
	if apiErr != nil {
		t.Fatalf("authorization url: %v", apiErr)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	if parsed.Host != "claude.ai" || parsed.Query().Get("code_challenge") != "challenge-1" || parsed.Query().Get("state") != "state-1" {
		t.Fatalf("unexpected auth url: %s", authURL)
	}
	exchanged, apiErr := provider.ExchangeCode(context.Background(), ExchangeCodeInput{Code: "code-1", State: "state-1", CodeVerifier: "verifier-1", RedirectURI: "https://platform.claude.com/oauth/code/callback"})
	if apiErr != nil || exchanged.AccessToken != "access-1" || exchanged.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected exchange result: %+v err=%v", exchanged, apiErr)
	}
	refreshed, apiErr := provider.RefreshToken(context.Background(), RefreshTokenInput{RefreshToken: "refresh-1"})
	if apiErr != nil || refreshed.AccessToken != "access-2" || refreshed.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected refresh result: %+v err=%v", refreshed, apiErr)
	}
	if len(requests) != 2 || requests[0].Get("grant_type") != "authorization_code" || requests[1].Get("grant_type") != "refresh_token" {
		t.Fatalf("unexpected requests: %+v", requests)
	}
}

func TestClaudeOAuthProviderRejectsBadResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad code"}`))
	}))
	defer server.Close()
	provider := NewClaudeOAuthProvider(server.Client(), ClaudeOAuthProviderConfig{AuthURL: "https://claude.ai/oauth/authorize", TokenURL: server.URL, ClientID: "client", Scopes: "scope"}, time.Now)
	if _, apiErr := provider.ExchangeCode(context.Background(), ExchangeCodeInput{Code: "bad", State: "state", CodeVerifier: "verifier", RedirectURI: "redirect"}); apiErr == nil || apiErr.Code != "TOKEN_EXCHANGE_FAILED" {
		t.Fatalf("expected token exchange failure, got %v", apiErr)
	}
	badProvider := NewClaudeOAuthProvider(server.Client(), ClaudeOAuthProviderConfig{AuthURL: "https://claude.ai/oauth/authorize", TokenURL: "://bad-url", ClientID: "client", Scopes: "scope"}, time.Now)
	if _, apiErr := badProvider.ExchangeCode(context.Background(), ExchangeCodeInput{Code: "bad", State: "state", CodeVerifier: "verifier", RedirectURI: "redirect"}); apiErr == nil || apiErr.Code != "TOKEN_REQUEST_FAILED" {
		t.Fatalf("expected token request failure, got %v", apiErr)
	}
}

func TestWithLoggingEmitsStartAndFinish(t *testing.T) {
	recorder := logging.NewRecorder()
	svc := WithLoggingClock(fakeAuthService{}, recorder, func() time.Time {
		return time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	})
	for _, call := range []func(){
		func() { _, _ = svc.PrepareLogin(context.Background(), PrepareLoginInput{}) },
		func() { _, _ = svc.CompleteLogin(context.Background(), CompleteLoginInput{Code: "x"}) },
		func() { _, _ = svc.Status(context.Background(), StatusInput{}) },
		func() { _, _ = svc.Logout(context.Background(), LogoutInput{}) },
		func() { _, _ = svc.AccessToken(context.Background(), AccessTokenInput{}) },
	} {
		recorder.Reset()
		call()
		entries := recorder.Entries()
		if len(entries) != 2 {
			t.Fatalf("expected 2 log entries, got %d", len(entries))
		}
	}
}

func TestWithLoggingEmitsFailures(t *testing.T) {
	recorder := logging.NewRecorder()
	svc := WithLoggingClock(fakeAuthService{errByOperation: map[string]*core.Error{logging.OperationAuthStatus: core.NewError("STATUS_FAILED", http.StatusInternalServerError, "status failed", nil)}}, recorder, time.Now)
	_, apiErr := svc.Status(context.Background(), StatusInput{})
	if apiErr == nil {
		t.Fatal("expected status error")
	}
	entries := recorder.Entries()
	if len(entries) != 2 || entries[1].Fields["error_code"] != "STATUS_FAILED" {
		t.Fatalf("unexpected logging entries: %+v", entries)
	}
	if WithLogging(nil, recorder) != nil {
		t.Fatal("expected nil service to remain nil")
	}
	if WithLoggingClock(fakeAuthService{}, recorder, nil) == nil {
		t.Fatal("expected logging clock wrapper")
	}
}

func TestBrowserCommandAndRandomHelpers(t *testing.T) {
	if name, args := browserCommand("darwin", "https://example.com"); name != "open" || args[0] != "https://example.com" {
		t.Fatalf("unexpected darwin browser command: %s %+v", name, args)
	}
	if name, _ := browserCommand("windows", "https://example.com"); name != "rundll32" {
		t.Fatalf("unexpected windows browser command: %s", name)
	}
	if name, _ := browserCommand("linux", "https://example.com"); name != "xdg-open" {
		t.Fatalf("unexpected default browser command: %s", name)
	}
	previousStarter := commandStarter
	defer func() { commandStarter = previousStarter }()
	called := ""
	commandStarter = func(name string, args ...string) error {
		called = name + " " + args[0]
		return nil
	}
	if err := (ExecBrowserOpener{}).Open("https://example.com"); err != nil || called == "" {
		t.Fatalf("unexpected browser open result: called=%q err=%v", called, err)
	}
	previousReader := randomReader
	defer func() { randomReader = previousReader }()
	randomReader = failingReader{}
	if _, _, err := generatePKCE(); err == nil {
		t.Fatal("expected pkce generation error")
	}
	if _, err := randomBase64URL(10); err == nil {
		t.Fatal("expected random state generation error")
	}
}

func TestServiceStoreErrorPathsAndProviderDefaults(t *testing.T) {
	provider := &fakeOAuthProvider{authURL: "https://claude.ai/oauth/authorize", exchangeOut: tokens.TokenSet{AccessToken: "a", ExpiresAt: time.Now().Add(time.Hour)}, refreshOut: tokens.TokenSet{AccessToken: "b", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)}}
	service := NewService(Config{RedirectURI: "redirect", RefreshSkew: time.Minute}, errorStore{saveErr: io.ErrClosedPipe, path: "/tmp/tokens.json"}, provider, nil, time.Now)
	if _, apiErr := service.CompleteLogin(context.Background(), CompleteLoginInput{Code: "code", State: "state", CodeVerifier: "verifier"}); apiErr == nil || apiErr.Code != "TOKEN_SAVE_FAILED" {
		t.Fatalf("expected token save failure, got %v", apiErr)
	}
	service = NewService(Config{RedirectURI: "redirect", RefreshSkew: time.Minute}, errorStore{loadErr: io.ErrUnexpectedEOF, path: "/tmp/tokens.json"}, provider, nil, time.Now)
	if _, apiErr := service.Status(context.Background(), StatusInput{}); apiErr == nil || apiErr.Code != "TOKEN_LOAD_FAILED" {
		t.Fatalf("expected token load failure, got %v", apiErr)
	}
	if _, apiErr := service.AccessToken(context.Background(), AccessTokenInput{}); apiErr == nil || apiErr.Code != "TOKEN_LOAD_FAILED" {
		t.Fatalf("expected access token load failure, got %v", apiErr)
	}
	service = NewService(Config{RedirectURI: "redirect", RefreshSkew: time.Minute}, tokens.NewFileStore(filepath.Join(t.TempDir(), "missing.json")), provider, nil, time.Now)
	if _, apiErr := service.AccessToken(context.Background(), AccessTokenInput{}); apiErr == nil || apiErr.Code != "TOKEN_MISSING" {
		t.Fatalf("expected token missing error, got %v", apiErr)
	}
	service = NewService(Config{RedirectURI: "redirect", RefreshSkew: time.Minute}, errorStore{deleteErr: io.ErrClosedPipe, path: "/tmp/tokens.json"}, provider, nil, time.Now)
	if _, apiErr := service.Logout(context.Background(), LogoutInput{}); apiErr == nil || apiErr.Code != "TOKEN_DELETE_FAILED" {
		t.Fatalf("expected token delete failure, got %v", apiErr)
	}
	if NewClaudeOAuthProvider(nil, ClaudeOAuthProviderConfig{}, nil) == nil {
		t.Fatal("expected default claude oauth provider")
	}
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func mapToValues(in map[string]string) url.Values {
	out := url.Values{}
	for key, value := range in {
		out.Set(key, value)
	}
	return out
}
