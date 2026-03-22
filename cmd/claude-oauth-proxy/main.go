package main

import (
	"context"
	"io"
	"os"

	"github.com/bonztm/claude-oauth-proxy/internal/adapters/cli"
	"github.com/bonztm/claude-oauth-proxy/internal/runtime"
)

var exitFunc = os.Exit

func main() {
	exitFunc(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	logger := runtime.NewLogger()
	return cli.Run(context.Background(), cli.NewDefaultFactory(logger), runtime.ConfigFromEnv(), logger, stdin, stdout, stderr, args)
}
