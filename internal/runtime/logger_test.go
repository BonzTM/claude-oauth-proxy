package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerConfigFromEnvDefaults(t *testing.T) {
	cfg := loggerConfigFromEnv(func(string) string { return "" })
	if cfg.level != slog.LevelInfo || cfg.sink != loggerSinkStderr {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoggerConfigFromEnvParsesValues(t *testing.T) {
	cfg := loggerConfigFromEnv(func(key string) string {
		if key == EnvLogLevel {
			return "error"
		}
		return "stdout"
	})
	if cfg.level != slog.LevelError || cfg.sink != loggerSinkStdout {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if parseLoggerLevel("warn") != slog.LevelWarn || parseLoggerLevel("wat") != slog.LevelInfo {
		t.Fatal("unexpected logger level parsing")
	}
	if parseLoggerSink("discard") != loggerSinkDiscard || parseLoggerSink("wat") != loggerSinkStderr {
		t.Fatal("unexpected logger sink parsing")
	}
}

func TestNewLoggerFromEnvWithOutputsWritesToSelectedSink(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	logger := newLoggerFromEnvWithOutputs(func(key string) string {
		if key == EnvLogSink {
			return "stdout"
		}
		return "debug"
	}, loggerOutputs{stdout: &stdout, stderr: &stderr, discard: &bytes.Buffer{}})
	logger.Info(context.Background(), "logger.stdout")
	if !strings.Contains(stdout.String(), "logger.stdout") {
		t.Fatalf("expected stdout log, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
	if selectLoggerWriter(loggerSinkDiscard, loggerOutputs{discard: &stdout}) != &stdout {
		t.Fatal("expected discard writer selection")
	}
	if selectLoggerWriter(loggerSinkStderr, loggerOutputs{}) == nil {
		t.Fatal("expected fallback writer")
	}
	t.Setenv(EnvLogSink, "discard")
	NewLogger().Info(context.Background(), "ignored")
}
