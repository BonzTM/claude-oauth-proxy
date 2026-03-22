package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunHelpAndVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{"help"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("unexpected help exit code: %d", code)
	}
	if !strings.Contains(stdout.String(), "claude-oauth-proxy serve") {
		t.Fatalf("unexpected help output: %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"version"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("unexpected version exit code: %d", code)
	}
	if !strings.Contains(stdout.String(), "claude-oauth-proxy ") {
		t.Fatalf("unexpected version output: %q", stdout.String())
	}
}

func TestMainUsesExitFunc(t *testing.T) {
	previousArgs := os.Args
	previousExit := exitFunc
	defer func() {
		os.Args = previousArgs
		exitFunc = previousExit
	}()
	os.Args = []string{"claude-oauth-proxy", "help"}
	exitCode := -1
	exitFunc = func(code int) {
		exitCode = code
	}
	main()
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
}
