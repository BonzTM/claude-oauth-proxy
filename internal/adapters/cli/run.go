package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/buildinfo"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/runtime"
)

type Factory func(runtime.Config, logging.Logger) (runtime.App, error)

func Run(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	logger = logging.Normalize(logger)
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	logger.Info(ctx, logging.EventCLICommandStart, "command", strings.Join(args, " "))
	code := run(ctx, factory, baseConfig, logger, stdin, stdout, stderr, args)
	logger.Info(ctx, logging.EventCLICommandFinish, "command", strings.Join(args, " "), "exit_code", code)
	return code
}

func run(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	switch args[0] {
	case "serve":
		return runServe(ctx, factory, baseConfig, logger, stdin, stdout, stderr, args[1:])
	case "login":
		return runLogin(ctx, factory, baseConfig, logger, stdin, stdout, stderr, args[1:])
	case "status":
		return runStatus(ctx, factory, baseConfig, logger, stdout, stderr)
	case "logout":
		return runLogout(ctx, factory, baseConfig, logger, stdout, stderr)
	case "config":
		return runConfig(baseConfig, stdout, stderr, args[1:])
	case "version", "--version", "-v":
		_, _ = fmt.Fprintln(stdout, buildinfo.Banner("claude-oauth-proxy"))
		return 0
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runServe(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listenAddr := fs.String("listen-addr", baseConfig.ListenAddr, "listen address")
	apiKey := fs.String("api-key", baseConfig.APIKey, "local OpenAI-compatible API key")
	relogin := fs.Bool("relogin", false, "force browser re-authentication before serving")
	noBrowser := fs.Bool("no-browser", false, "print the Claude OAuth URL instead of opening a browser")
	code := fs.String("code", "", "authorization code to complete login without prompting")
	costTracking := fs.Bool("cost-tracking", baseConfig.CostTracking, "enable theoretical cost tracking via OpenRouter pricing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	baseConfig.ListenAddr = *listenAddr
	baseConfig.APIKey = *apiKey
	baseConfig.CostTracking = *costTracking
	app, err := factory(baseConfig, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to initialize app: %v\n", err)
		return 1
	}
	if *relogin {
		if loginCode := executeLoginFlow(ctx, app.Auth, stdin, stdout, stderr, !*noBrowser, *code); loginCode != 0 {
			return loginCode
		}
	} else {
		status, apiErr := app.Auth.Status(ctx, auth.StatusInput{})
		if apiErr != nil {
			_, _ = fmt.Fprintf(stderr, "failed to inspect token state: %v\n", apiErr)
			return 1
		}
		if !status.Exists {
			if loginCode := executeLoginFlow(ctx, app.Auth, stdin, stdout, stderr, !*noBrowser, *code); loginCode != 0 {
				return loginCode
			}
		} else if _, apiErr := app.Auth.AccessToken(ctx, auth.AccessTokenInput{}); apiErr != nil {
			if loginCode := executeLoginFlow(ctx, app.Auth, stdin, stdout, stderr, !*noBrowser, *code); loginCode != 0 {
				return loginCode
			}
		}
	}
	refreshCtx, refreshCancel := context.WithCancel(ctx)
	defer refreshCancel()
	go autoRefreshWithInterval(refreshCtx, app.Auth, logger, app.RefreshInterval)
	server := &http.Server{Addr: baseConfig.ListenAddr, Handler: app.Handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	_, _ = fmt.Fprintf(stdout, "serving OpenAI-compatible API on http://127.0.0.1%s\n", baseConfig.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		_, _ = fmt.Fprintf(stderr, "server failed: %v\n", err)
		return 1
	}
	return 0
}

func runLogin(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	noBrowser := fs.Bool("no-browser", false, "print the Claude OAuth URL instead of opening a browser")
	code := fs.String("code", "", "authorization code to complete login without prompting")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	app, err := factory(baseConfig, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to initialize app: %v\n", err)
		return 1
	}
	return executeLoginFlow(ctx, app.Auth, stdin, stdout, stderr, !*noBrowser, *code)
}

func runStatus(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdout, stderr io.Writer) int {
	app, err := factory(baseConfig, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to initialize app: %v\n", err)
		return 1
	}
	status, apiErr := app.Auth.Status(ctx, auth.StatusInput{})
	if apiErr != nil {
		_, _ = fmt.Fprintf(stderr, "failed to load status: %v\n", apiErr)
		return 1
	}
	if !status.Exists {
		_, _ = fmt.Fprintf(stdout, "no saved oauth session at %s\n", status.TokenPath)
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "saved oauth session at %s\n", status.TokenPath)
	_, _ = fmt.Fprintf(stdout, "expires at %s\n", status.ExpiresAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(stdout, "expired=%t\n", status.Expired)
	return 0
}

func runLogout(ctx context.Context, factory Factory, baseConfig runtime.Config, logger logging.Logger, stdout, stderr io.Writer) int {
	app, err := factory(baseConfig, logger)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to initialize app: %v\n", err)
		return 1
	}
	result, apiErr := app.Auth.Logout(ctx, auth.LogoutInput{})
	if apiErr != nil {
		_, _ = fmt.Fprintf(stderr, "failed to delete token file: %v\n", apiErr)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "deleted oauth session at %s\n", result.TokenPath)
	return 0
}

func runConfig(baseConfig runtime.Config, stdout, stderr io.Writer, args []string) int {
	if len(args) == 1 && args[0] == "validate" {
		if err := baseConfig.Validate(); err != nil {
			_, _ = fmt.Fprintf(stderr, "config is invalid: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, "config is valid")
		_, _ = fmt.Fprintf(stdout, "listen_addr=%s\n", baseConfig.ListenAddr)
		_, _ = fmt.Fprintf(stdout, "token_file=%s\n", baseConfig.TokenFile)
		return 0
	}
	printUsage(stderr)
	return 2
}

func executeLoginFlow(ctx context.Context, authService auth.Service, stdin io.Reader, stdout, stderr io.Writer, openBrowser bool, code string) int {
	prepared, apiErr := authService.PrepareLogin(ctx, auth.PrepareLoginInput{OpenBrowser: openBrowser})
	if apiErr != nil {
		_, _ = fmt.Fprintf(stderr, "failed to prepare oauth login: %v\n", apiErr)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "open this URL in your browser:\n\n%s\n\n", prepared.AuthURL)
	code = strings.TrimSpace(code)
	if code == "" {
		_, _ = fmt.Fprintln(stdout, "after logging in, copy the ?code=... value from the redirect URL and paste it here:")
		_, _ = fmt.Fprint(stdout, "code: ")
		line, err := readLineWithContext(ctx, stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "failed to read oauth code: %v\n", err)
			return 1
		}
		code = strings.TrimSpace(line)
	}
	code = normalizeCode(code)
	result, apiErr := authService.CompleteLogin(ctx, auth.CompleteLoginInput{Code: code, State: prepared.State, CodeVerifier: prepared.CodeVerifier})
	if apiErr != nil {
		_, _ = fmt.Fprintf(stderr, "failed to complete oauth login: %v\n", apiErr)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "saved oauth session to %s\n", result.TokenPath)
	_, _ = fmt.Fprintf(stdout, "token expires at %s\n", result.ExpiresAt.Format(time.RFC3339))
	return 0
}

func readLineWithContext(ctx context.Context, stdin io.Reader) (string, error) {
	reader := bufio.NewReader(stdin)
	type readResult struct {
		line string
		err  error
	}
	resultCh := make(chan readResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			resultCh <- readResult{line: line}
			return
		}
		resultCh <- readResult{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultCh:
		return result.line, result.err
	}
}

func normalizeCode(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "code=") {
		if parsed, err := url.Parse(trimmed); err == nil {
			if value := strings.TrimSpace(parsed.Query().Get("code")); value != "" {
				return value
			}
		}
		queryIndex := strings.Index(trimmed, "code=")
		trimmed = trimmed[queryIndex+len("code="):]
	}
	for _, separator := range []string{"&", "#"} {
		if index := strings.Index(trimmed, separator); index >= 0 {
			trimmed = trimmed[:index]
		}
	}
	return strings.TrimSpace(trimmed)
}

func autoRefreshWithInterval(ctx context.Context, authService auth.Service, logger logging.Logger, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, apiErr := authService.AccessToken(ctx, auth.AccessTokenInput{})
			if apiErr != nil {
				logging.Normalize(logger).Error(ctx, logging.EventServiceOperationFinish, "operation", logging.OperationAuthAccessToken, "ok", false, "error_code", apiErr.Code)
			}
		}
	}
}

func printUsage(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	_, _ = fmt.Fprintln(w, "claude-oauth-proxy - OpenAI-compatible Claude OAuth proxy")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Usage:")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy serve [--listen-addr :9999] [--api-key sk-proxy-local-key] [--relogin] [--no-browser] [--code <code>] [--cost-tracking]")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy login [--no-browser] [--code <code>]")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy status")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy logout")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy config validate")
	_, _ = fmt.Fprintln(w, "  claude-oauth-proxy version")
}

func NewDefaultFactory(logger logging.Logger) Factory {
	_ = logger
	return func(cfg runtime.Config, logger logging.Logger) (runtime.App, error) {
		return runtime.NewAppWithLogger(cfg, logger)
	}
}
