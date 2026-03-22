package core

import (
	"errors"
	"testing"
)

func TestErrorFormattingAndUnwrap(t *testing.T) {
	inner := errors.New("boom")
	err := NewError("X", 500, "explode", inner)
	if got := err.Error(); got != "explode: boom" {
		t.Fatalf("unexpected error string: %q", got)
	}
	if !errors.Is(err, inner) {
		t.Fatal("expected wrapped error")
	}
}

func TestErrorHandlesNilAndMessageOnly(t *testing.T) {
	var nilErr *Error
	if got := nilErr.Error(); got != "" {
		t.Fatalf("unexpected nil error string: %q", got)
	}
	err := NewError("X", 400, "plain", nil)
	if got := err.Error(); got != "plain" {
		t.Fatalf("unexpected plain error string: %q", got)
	}
	err = NewError("X", 400, "", errors.New("boom"))
	if got := err.Error(); got != "boom" {
		t.Fatalf("unexpected empty-message error string: %q", got)
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("expected nil unwrap")
	}
}
