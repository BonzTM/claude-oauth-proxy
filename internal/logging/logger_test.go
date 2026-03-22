package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNormalizeDefaultsToDiscardLogger(t *testing.T) {
	logger := Normalize(nil)
	logger.Info(context.Background(), "event.one")
	logger.Error(context.Background(), "event.two")
	if Normalize(logger) != logger {
		t.Fatal("expected normalize to preserve logger")
	}
}

func TestNewJSONLoggerWritesEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(&buf)
	logger.Info(context.Background(), "event.info", "key", "value")
	logger.Error(context.Background(), "event.error", "key", "value")
	output := buf.String()
	for _, snippet := range []string{"event.info", "event.error", "\"key\":\"value\""} {
		if !strings.Contains(output, snippet) {
			t.Fatalf("missing %q in %q", snippet, output)
		}
	}
}

func TestRecorderCapturesAndClonesEntries(t *testing.T) {
	recorder := NewRecorder()
	recorder.Info(context.Background(), "event.info", "a", 1, 2)
	recorder.Error(context.Background(), "event.error")
	entries := recorder.Entries()
	if len(entries) != 2 {
		t.Fatalf("unexpected entries: %d", len(entries))
	}
	if entries[0].Fields["a"] != 1 {
		t.Fatalf("unexpected fields: %+v", entries[0].Fields)
	}
	if _, ok := entries[0].Fields["field_unpaired"]; !ok {
		t.Fatalf("expected unpaired field: %+v", entries[0].Fields)
	}
	entries[0].Fields["a"] = 99
	if recorder.Entries()[0].Fields["a"] != 1 {
		t.Fatal("expected cloned fields")
	}
	recorder.Reset()
	if len(recorder.Entries()) != 0 {
		t.Fatal("expected recorder reset to clear entries")
	}
	var nilSlog *slogLogger
	nilSlog.Info(context.Background(), "ignored")
	nilSlog.Error(context.Background(), "ignored")
}
