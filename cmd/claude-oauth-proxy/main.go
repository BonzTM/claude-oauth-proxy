package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/bonztm/claude-oauth-proxy/internal/adapters/cli"
	"github.com/bonztm/claude-oauth-proxy/internal/runtime"
)

var exitFunc = os.Exit

func main() {
	exitFunc(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := runtime.NewLogger()
	return cli.Run(ctx, cli.NewDefaultFactory(logger), runtime.ConfigFromEnv(), logger, stdin, stdout, stderr, args)
}
