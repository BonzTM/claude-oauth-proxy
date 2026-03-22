package cli

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/runtime"
)

type fakeAuthService struct {
	prepareOut   auth.PrepareLoginOutput
	prepareErr   *core.Error
	completeOut  auth.CompleteLoginOutput
	completeErr  *core.Error
	statusOut    auth.StatusOutput
	statusErr    *core.Error
	logoutOut    auth.LogoutOutput
	logoutErr    *core.Error
	accessOut    auth.AccessTokenOutput
	accessErr    *core.Error
	prepareHits  int
	accessHits   int
	completeHits int
}

func (f *fakeAuthService) PrepareLogin(_ context.Context, _ auth.PrepareLoginInput) (auth.PrepareLoginOutput, *core.Error) {
	f.prepareHits++
	return f.prepareOut, f.prepareErr
}

func (f *fakeAuthService) CompleteLogin(_ context.Context, _ auth.CompleteLoginInput) (auth.CompleteLoginOutput, *core.Error) {
	f.completeHits++
	return f.completeOut, f.completeErr
}

func (f *fakeAuthService) Status(_ context.Context, _ auth.StatusInput) (auth.StatusOutput, *core.Error) {
	return f.statusOut, f.statusErr
}

func (f *fakeAuthService) Logout(_ context.Context, _ auth.LogoutInput) (auth.LogoutOutput, *core.Error) {
	return f.logoutOut, f.logoutErr
}

func (f *fakeAuthService) AccessToken(_ context.Context, _ auth.AccessTokenInput) (auth.AccessTokenOutput, *core.Error) {
	f.accessHits++
	return f.accessOut, f.accessErr
}

func TestRunCommandsAndNormalizeCode(t *testing.T) {
	authService := &fakeAuthService{
		prepareOut:  auth.PrepareLoginOutput{AuthURL: "https://claude.ai/oauth/authorize?client_id=test", State: "state", CodeVerifier: "verifier"},
		completeOut: auth.CompleteLoginOutput{TokenPath: "/tmp/tokens.json", ExpiresAt: time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)},
		statusOut:   auth.StatusOutput{TokenPath: "/tmp/tokens.json", Exists: true, ExpiresAt: time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)},
		logoutOut:   auth.LogoutOutput{TokenPath: "/tmp/tokens.json"},
		accessOut:   auth.AccessTokenOutput{Token: "token"},
	}
	factory := func(cfg runtime.Config, logger logging.Logger) (runtime.App, error) {
		return runtime.App{Config: cfg, Auth: authService, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })}, nil
	}
	baseConfig := runtime.DefaultConfig()
	baseConfig.ListenAddr = "bad address"
	var usageOut bytes.Buffer
	if code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader(""), &usageOut, &usageOut, nil); code != 0 {
		t.Fatalf("unexpected zero-args exit code: %d", code)
	}

	for _, tc := range []struct {
		name    string
		args    []string
		want    int
		wantOut string
	}{
		{name: "help", args: []string{"help"}, want: 0, wantOut: "claude-oauth-proxy serve"},
		{name: "version", args: []string{"version"}, want: 0, wantOut: "claude-oauth-proxy "},
		{name: "config", args: []string{"config", "validate"}, want: 0, wantOut: "config is valid"},
		{name: "status", args: []string{"status"}, want: 0, wantOut: "saved oauth session"},
		{name: "logout", args: []string{"logout"}, want: 0, wantOut: "deleted oauth session"},
		{name: "login", args: []string{"login", "--no-browser", "--code", "https://example.com/callback?code=abc123#state"}, want: 0, wantOut: "saved oauth session"},
		{name: "unknown", args: []string{"wat"}, want: 2, wantOut: "unknown command"},
		{name: "serve", args: []string{"serve"}, want: 1, wantOut: "server failed"},
		{name: "serve relogin", args: []string{"serve", "--relogin", "--no-browser", "--code", "abc123"}, want: 1, wantOut: "server failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader("pasted-code\n"), &stdout, &stderr, tc.args)
			if code != tc.want {
				t.Fatalf("unexpected exit code: got=%d want=%d stdout=%q stderr=%q", code, tc.want, stdout.String(), stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, tc.wantOut) {
				t.Fatalf("expected output to contain %q, got stdout=%q stderr=%q", tc.wantOut, stdout.String(), stderr.String())
			}
		})
	}
	if authService.prepareHits == 0 || authService.completeHits == 0 {
		t.Fatalf("expected login flow to run, prepare=%d complete=%d", authService.prepareHits, authService.completeHits)
	}
	if authService.accessHits == 0 {
		t.Fatal("expected serve path to call AccessToken")
	}
	if got := normalizeCode("https://example.com/callback?code=xyz789&state=s"); got != "xyz789" {
		t.Fatalf("unexpected normalized code: %q", got)
	}
	if got := normalizeCode("abc123#state"); got != "abc123" {
		t.Fatalf("unexpected fragment normalized code: %q", got)
	}
}

func TestRunErrorsFromFactoryAndAuth(t *testing.T) {
	baseConfig := runtime.DefaultConfig()
	factoryErr := func(runtime.Config, logging.Logger) (runtime.App, error) {
		return runtime.App{}, context.DeadlineExceeded
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := Run(context.Background(), factoryErr, baseConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, []string{"status"}); code != 1 {
		t.Fatalf("unexpected factory error code: %d", code)
	}
	authService := &fakeAuthService{prepareErr: core.NewError("PREPARE_FAILED", http.StatusBadGateway, "prepare failed", nil), statusErr: core.NewError("STATUS_FAILED", http.StatusInternalServerError, "status failed", nil), logoutErr: core.NewError("LOGOUT_FAILED", http.StatusInternalServerError, "logout failed", nil)}
	factory := func(cfg runtime.Config, logger logging.Logger) (runtime.App, error) {
		return runtime.App{Config: cfg, Auth: authService, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}, nil
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, []string{"login", "--code", "abc"}); code != 1 {
		t.Fatalf("unexpected login error code: %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, []string{"status"}); code != 1 {
		t.Fatalf("unexpected status error code: %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, []string{"logout"}); code != 1 {
		t.Fatalf("unexpected logout error code: %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, []string{"config"}); code != 2 {
		t.Fatalf("unexpected config usage code: %d", code)
	}
	if NewDefaultFactory(logging.NewRecorder()) == nil {
		t.Fatal("expected default factory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	autoRefreshWithInterval(ctx, authService, logging.NewRecorder(), time.Millisecond)
	autoRefresh(ctx, authService, logging.NewRecorder())
}

func TestDirectCLIHelpers(t *testing.T) {
	baseConfig := runtime.DefaultConfig()
	baseConfig.ListenAddr = "bad address"
	authService := &fakeAuthService{prepareOut: auth.PrepareLoginOutput{AuthURL: "https://claude.ai/oauth/authorize", State: "state", CodeVerifier: "verifier"}, completeOut: auth.CompleteLoginOutput{TokenPath: "/tmp/tokens.json", ExpiresAt: time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)}, statusOut: auth.StatusOutput{TokenPath: "/tmp/tokens.json", Exists: false}, logoutOut: auth.LogoutOutput{TokenPath: "/tmp/tokens.json"}, accessErr: core.NewError("TOKEN_FAILED", http.StatusServiceUnavailable, "token failed", nil)}
	factory := func(cfg runtime.Config, logger logging.Logger) (runtime.App, error) {
		return runtime.App{Config: cfg, Auth: authService, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}, nil
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runLogin(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader("pasted\n"), &stdout, &stderr, []string{"--bad-flag"}); code != 2 {
		t.Fatalf("unexpected runLogin parse code: %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runServe(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader("pasted\n"), &stdout, &stderr, []string{"--bad-flag"}); code != 2 {
		t.Fatalf("unexpected runServe parse code: %d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := executeLoginFlow(context.Background(), authService, strings.NewReader("code-from-stdin\n"), &stdout, &stderr, false, ""); code != 0 || !strings.Contains(stdout.String(), "open this URL") {
		t.Fatalf("unexpected executeLoginFlow output: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	authService.prepareErr = core.NewError("PREPARE_FAILED", http.StatusBadGateway, "prepare failed", nil)
	if code := executeLoginFlow(context.Background(), authService, strings.NewReader("code\n"), &stdout, &stderr, true, ""); code != 1 {
		t.Fatalf("unexpected prepare failure code: %d", code)
	}
	authService.prepareErr = nil
	authService.completeErr = core.NewError("COMPLETE_FAILED", http.StatusBadGateway, "complete failed", nil)
	stdout.Reset()
	stderr.Reset()
	if code := executeLoginFlow(context.Background(), authService, strings.NewReader("code\n"), &stdout, &stderr, true, ""); code != 1 {
		t.Fatalf("unexpected complete failure code: %d", code)
	}
	authService.completeErr = nil
	stdout.Reset()
	stderr.Reset()
	if code := runStatus(context.Background(), factory, baseConfig, logging.NewRecorder(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "no saved oauth session") {
		t.Fatalf("unexpected missing status output: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runServe(context.Background(), factory, baseConfig, logging.NewRecorder(), strings.NewReader("code\n"), &stdout, &stderr, nil); code != 1 {
		t.Fatalf("unexpected runServe fallback code: %d", code)
	}
	successConfig := runtime.DefaultConfig()
	successConfig.ListenAddr = "127.0.0.1:0"
	successFactory := func(cfg runtime.Config, logger logging.Logger) (runtime.App, error) {
		return runtime.App{Config: cfg, Auth: &fakeAuthService{statusOut: auth.StatusOutput{TokenPath: "/tmp/tokens.json", Exists: true}, accessOut: auth.AccessTokenOutput{Token: "token"}}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	stdout.Reset()
	stderr.Reset()
	if code := runServe(ctx, successFactory, successConfig, logging.NewRecorder(), strings.NewReader(""), &stdout, &stderr, nil); code != 0 {
		t.Fatalf("unexpected successful runServe code: %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	defaultFactory := NewDefaultFactory(logging.NewRecorder())
	configForFactory := runtime.DefaultConfig()
	configForFactory.TokenFile = t.TempDir() + "/tokens.json"
	if _, err := defaultFactory(configForFactory, logging.NewRecorder()); err != nil {
		t.Fatalf("unexpected default factory error: %v", err)
	}
}
