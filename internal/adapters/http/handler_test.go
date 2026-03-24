package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type fakeProvider struct {
	modelsOut provider.ListModelsOutput
	modelsErr *core.Error
	chatOut   provider.CreateChatCompletionOutput
	chatErr   *core.Error
	streamOut provider.CreateChatCompletionStreamOutput
	streamErr *core.Error
}

func (f fakeProvider) ListModels(_ context.Context, _ provider.ListModelsInput) (provider.ListModelsOutput, *core.Error) {
	return f.modelsOut, f.modelsErr
}

func (f fakeProvider) CreateChatCompletion(_ context.Context, _ provider.CreateChatCompletionInput) (provider.CreateChatCompletionOutput, *core.Error) {
	return f.chatOut, f.chatErr
}

func (f fakeProvider) CreateChatCompletionStream(_ context.Context, _ provider.CreateChatCompletionStreamInput) (provider.CreateChatCompletionStreamOutput, *core.Error) {
	return f.streamOut, f.streamErr
}

type fakeAuthService struct {
	statusOut auth.StatusOutput
	statusErr *core.Error
}

func (f fakeAuthService) PrepareLogin(_ context.Context, _ auth.PrepareLoginInput) (auth.PrepareLoginOutput, *core.Error) {
	return auth.PrepareLoginOutput{}, nil
}

func (f fakeAuthService) CompleteLogin(_ context.Context, _ auth.CompleteLoginInput) (auth.CompleteLoginOutput, *core.Error) {
	return auth.CompleteLoginOutput{}, nil
}

func (f fakeAuthService) Status(_ context.Context, _ auth.StatusInput) (auth.StatusOutput, *core.Error) {
	return f.statusOut, f.statusErr
}

func (f fakeAuthService) Logout(_ context.Context, _ auth.LogoutInput) (auth.LogoutOutput, *core.Error) {
	return auth.LogoutOutput{}, nil
}

func (f fakeAuthService) AccessToken(_ context.Context, _ auth.AccessTokenInput) (auth.AccessTokenOutput, *core.Error) {
	return auth.AccessTokenOutput{}, nil
}

func defaultHandlerConfig() HandlerConfig {
	return HandlerConfig{MaxRequestBody: 10 * 1024 * 1024}
}

func TestHealthAndReadyEndpoints(t *testing.T) {
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true, Expired: false}}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())
	for path, wantStatus := range map[string]int{"/health": http.StatusOK, "/healthz": http.StatusOK, "/livez": http.StatusOK, "/ready": http.StatusOK, "/readyz": http.StatusOK} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("unexpected status for %s: got=%d want=%d", path, rec.Code, wantStatus)
		}
	}
	notReady := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: false}}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())
	rec := httptest.NewRecorder()
	notReady.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected not-ready status: %d", rec.Code)
	}
}

func TestModelsAuthAndRequestLogging(t *testing.T) {
	recorder := logging.NewRecorder()
	handler := NewHandler(fakeProvider{modelsOut: provider.ListModelsOutput{Response: openai.ModelsResponse{Object: "list", Data: []openai.ModelInfo{{ID: "claude", Object: "model"}}}}}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", recorder, func() time.Time { return time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC) }, defaultHandlerConfig())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "claude") {
		t.Fatalf("unexpected models response: status=%d body=%q", rec.Code, rec.Body.String())
	}
	entries := recorder.Entries()
	if len(entries) < 2 || entries[0].Event != logging.EventHTTPRequestStart || entries[1].Event != logging.EventHTTPRequestFinish {
		t.Fatalf("unexpected request logs: %+v", entries)
	}
}

func TestChatCompletionsAndUnsupportedEndpoints(t *testing.T) {
	sent := false
	stream := provider.NewChunkStream(func() (openai.ChatCompletionChunk, error) {
		if sent {
			return openai.ChatCompletionChunk{}, io.EOF
		}
		sent = true
		return openai.ChatCompletionChunk{ID: "chatcmpl-1", Object: "chat.completion.chunk", Created: 1, Model: "claude", Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{Role: "assistant", Content: "hello"}}}}, nil
	}, nil)
	handler := NewHandler(fakeProvider{chatOut: provider.CreateChatCompletionOutput{Response: openai.ChatCompletionResponse{ID: "chatcmpl-1", Object: "chat.completion", Created: 1, Model: "claude", Choices: []openai.ChatCompletionChoice{{Index: 0, Message: openai.ChatCompletionMessage{Role: "assistant", Content: openai.MessageContent{Text: "hello"}}, FinishReason: "stop"}}}}, streamOut: provider.CreateChatCompletionStreamOutput{Stream: stream}}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())
	body, _ := json.Marshal(openai.ChatCompletionRequest{Model: "claude", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("unexpected chat completion response: status=%d body=%q", rec.Code, rec.Body.String())
	}

	streamBody, _ := json.Marshal(openai.ChatCompletionRequest{Model: "claude", Stream: true, Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}})
	streamReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(streamBody))
	streamReq.Header.Set("X-API-Key", "secret")
	streamRec := httptest.NewRecorder()
	handler.ServeHTTP(streamRec, streamReq)
	if streamRec.Code != http.StatusOK || !strings.Contains(streamRec.Body.String(), "data: ") || !strings.Contains(streamRec.Body.String(), "[DONE]") {
		t.Fatalf("unexpected stream response: status=%d body=%q", streamRec.Code, streamRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
	invalidReq.Header.Set("X-API-Key", "secret")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected invalid json status: %d", invalidRec.Code)
	}

	unsupportedReq := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	unsupportedReq.Header.Set("X-API-Key", "secret")
	unsupportedRec := httptest.NewRecorder()
	handler.ServeHTTP(unsupportedRec, unsupportedReq)
	if unsupportedRec.Code != http.StatusNotImplemented {
		t.Fatalf("unexpected unsupported endpoint status: %d", unsupportedRec.Code)
	}
}

func TestProviderAndAuthErrors(t *testing.T) {
	handler := NewHandler(fakeProvider{modelsErr: core.NewError("MODELS_FAILED", http.StatusBadGateway, "models failed", nil), chatErr: core.NewError("CHAT_FAILED", http.StatusBadGateway, "chat failed", nil), streamErr: core.NewError("STREAM_FAILED", http.StatusBadGateway, "stream failed", nil)}, fakeAuthService{statusErr: core.NewError("STATUS_FAILED", http.StatusInternalServerError, "status failed", nil)}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected ready error status: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-API-Key", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unexpected models error status: %d", rec.Code)
	}
	body, _ := json.Marshal(openai.ChatCompletionRequest{Model: "claude", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}})
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unexpected chat error status: %d", rec.Code)
	}
	body, _ = json.Marshal(openai.ChatCompletionRequest{Model: "claude", Stream: true, Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}})
	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unexpected stream error status: %d", rec.Code)
	}
}

func TestWriteErrorAndStatusCapturingFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected default error status: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	writeError(rec, core.NewError("FAIL", http.StatusBadGateway, "boom", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "FAIL") {
		t.Fatalf("unexpected explicit error response: status=%d body=%q", rec.Code, rec.Body.String())
	}
	writer := &statusCapturingResponseWriter{ResponseWriter: httptest.NewRecorder(), statusCode: http.StatusOK}
	writer.Flush()
}

func TestRequestIDMiddleware(t *testing.T) {
	prev := generateRequestID
	defer func() { generateRequestID = prev }()
	generateRequestID = func() string { return "test-request-id" }

	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if got := rec.Header().Get("X-Request-ID"); got != "test-request-id" {
		t.Fatalf("expected generated request ID, got %q", got)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "client-provided-id")
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "client-provided-id" {
		t.Fatalf("expected client-provided request ID, got %q", got)
	}
}

func TestRequestIDInLogs(t *testing.T) {
	prev := generateRequestID
	defer func() { generateRequestID = prev }()
	generateRequestID = func() string { return "log-test-id" }

	recorder := logging.NewRecorder()
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", recorder, time.Now, defaultHandlerConfig())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	for _, entry := range recorder.Entries() {
		if entry.Event == logging.EventHTTPRequestStart || entry.Event == logging.EventHTTPRequestFinish {
			if entry.Fields["request_id"] != "log-test-id" {
				t.Fatalf("expected request_id in log entry, got fields=%+v", entry.Fields)
			}
		}
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, HandlerConfig{CORSOrigins: "http://localhost:3000,http://example.com", MaxRequestBody: 10 * 1024 * 1024})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected CORS origin, got %q", got)
	}
	vary := strings.Join(rec.Header().Values("Vary"), ",")
	if !strings.Contains(vary, "Origin") {
		t.Fatalf("expected Vary to include Origin, got %q", vary)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://evil.com")
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header for disallowed origin, got %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for allowed preflight, got %d", rec.Code)
	}
	vary = strings.Join(rec.Header().Values("Vary"), ",")
	for _, expected := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
		if !strings.Contains(vary, expected) {
			t.Fatalf("expected Vary to include %q, got %q", expected, vary)
		}
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://evil.com")
	handler.ServeHTTP(rec, req)
	if rec.Code == http.StatusNoContent {
		t.Fatal("expected disallowed-origin preflight to fall through to route handler, got 204")
	}
}

func TestCORSWildcard(t *testing.T) {
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, HandlerConfig{CORSOrigins: "*", MaxRequestBody: 10 * 1024 * 1024})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://anything.example.com")
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://anything.example.com" {
		t.Fatalf("expected wildcard CORS to allow any origin, got %q", got)
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, defaultHandlerConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header when disabled, got %q", got)
	}
}

func TestMaxRequestBodyLimit(t *testing.T) {
	handler := NewHandler(fakeProvider{}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", logging.NewRecorder(), time.Now, HandlerConfig{MaxRequestBody: 64})
	largeBody := strings.Repeat("x", 1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(largeBody))
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", rec.Code)
	}
}

func TestStreamingErrorIsLogged(t *testing.T) {
	streamErr := fmt.Errorf("upstream connection reset")
	errReturned := false
	stream := provider.NewChunkStream(func() (openai.ChatCompletionChunk, error) {
		if !errReturned {
			errReturned = true
			return openai.ChatCompletionChunk{}, streamErr
		}
		return openai.ChatCompletionChunk{}, io.EOF
	}, nil)
	recorder := logging.NewRecorder()
	handler := NewHandler(fakeProvider{streamOut: provider.CreateChatCompletionStreamOutput{Stream: stream}}, fakeAuthService{statusOut: auth.StatusOutput{Exists: true}}, "secret", recorder, time.Now, defaultHandlerConfig())
	body, _ := json.Marshal(openai.ChatCompletionRequest{Model: "claude", Stream: true, Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	found := false
	for _, entry := range recorder.Entries() {
		if entry.Event == "stream.error" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected stream.error log entry for mid-flight streaming error")
	}
}
