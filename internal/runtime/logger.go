package runtime

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/bonztm/claude-oauth-proxy/internal/logging"
)

type loggerSink string

const (
	loggerSinkStderr  loggerSink = "stderr"
	loggerSinkStdout  loggerSink = "stdout"
	loggerSinkDiscard loggerSink = "discard"
)

type loggerConfig struct {
	level slog.Level
	sink  loggerSink
}

type loggerOutputs struct {
	stdout  io.Writer
	stderr  io.Writer
	discard io.Writer
}

func NewLogger() logging.Logger {
	return newLoggerFromEnvWithOutputs(func(key string) string { return os.Getenv(key) }, loggerOutputs{stdout: os.Stdout, stderr: os.Stderr, discard: io.Discard})
}

func newLoggerFromEnvWithOutputs(getenv func(string) string, outputs loggerOutputs) logging.Logger {
	cfg := loggerConfigFromEnv(getenv)
	return logging.NewJSONLoggerWithLevel(selectLoggerWriter(cfg.sink, outputs), cfg.level)
}

func loggerConfigFromEnv(getenv func(string) string) loggerConfig {
	if getenv == nil {
		getenv = os.Getenv
	}
	return loggerConfig{level: parseLoggerLevel(getenv(EnvLogLevel)), sink: parseLoggerSink(getenv(EnvLogSink))}
}

func parseLoggerLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func parseLoggerSink(raw string) loggerSink {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(loggerSinkStdout):
		return loggerSinkStdout
	case string(loggerSinkDiscard):
		return loggerSinkDiscard
	default:
		return loggerSinkStderr
	}
}

func selectLoggerWriter(sink loggerSink, outputs loggerOutputs) io.Writer {
	switch sink {
	case loggerSinkStdout:
		if outputs.stdout != nil {
			return outputs.stdout
		}
	case loggerSinkDiscard:
		if outputs.discard != nil {
			return outputs.discard
		}
	}
	if outputs.stderr != nil {
		return outputs.stderr
	}
	if outputs.stdout != nil {
		return outputs.stdout
	}
	return io.Discard
}
