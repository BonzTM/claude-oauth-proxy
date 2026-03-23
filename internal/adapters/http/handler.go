package httpadapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type Handler struct {
	provider provider.Service
	auth     auth.Service
	apiKey   string
	logger   logging.Logger
	now      func() time.Time
}

func NewHandler(provider provider.Service, authService auth.Service, apiKey string, logger logging.Logger, now func() time.Time) http.Handler {
	if now == nil {
		now = time.Now
	}
	h := &Handler{provider: provider, auth: authService, apiKey: apiKey, logger: logging.Normalize(logger), now: now}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/livez", h.handleHealth)
	mux.HandleFunc("/ready", h.handleReady)
	mux.HandleFunc("/readyz", h.handleReady)
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.Handle("/v1/chat/completions", h.requireAPIKey(http.HandlerFunc(h.handleChatCompletions)))
	mux.Handle("/v1/", h.requireAPIKey(http.HandlerFunc(h.handleUnsupportedEndpoint)))
	return h.withRequestLogging(mux)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	status, apiErr := h.auth.Status(r.Context(), auth.StatusInput{})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	if !status.Exists || status.Expired {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleModels(w http.ResponseWriter, r *http.Request) {
	result, apiErr := h.provider.ListModels(r.Context(), provider.ListModelsInput{})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	writeJSON(w, http.StatusOK, result.Response)
}

func (h *Handler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, core.NewError("INVALID_JSON", http.StatusBadRequest, "decode request body", err))
		return
	}
	if request.Stream {
		h.handleStreamingChatCompletions(w, r, request)
		return
	}
	result, apiErr := h.provider.CreateChatCompletion(r.Context(), provider.CreateChatCompletionInput{Request: request})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	writeJSON(w, http.StatusOK, result.Response)
}

func (h *Handler) handleStreamingChatCompletions(w http.ResponseWriter, r *http.Request, request openai.ChatCompletionRequest) {
	result, apiErr := h.provider.CreateChatCompletionStream(r.Context(), provider.CreateChatCompletionStreamInput{Request: request})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	defer result.Stream.Close()
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, core.NewError("STREAMING_UNAVAILABLE", http.StatusInternalServerError, "http flusher is unavailable", nil))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	for {
		chunk, err := result.Stream.Next()
		if err != nil {
			if err == io.EOF {
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
				flusher.Flush()
				return
			}
			return
		}
		data, marshalErr := json.Marshal(chunk)
		if marshalErr != nil {
			return
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}

func (h *Handler) handleUnsupportedEndpoint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotImplemented, openai.ErrorEnvelope{Error: openai.ErrorPayload{Message: "this OpenAI-compatible endpoint is not implemented", Type: "not_implemented_error", Code: "ENDPOINT_NOT_IMPLEMENTED"}})
}

func (h *Handler) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if key == "" {
			key = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		if key != h.apiKey {
			writeJSON(w, http.StatusUnauthorized, openai.ErrorEnvelope{Error: openai.ErrorPayload{Message: "invalid api key", Type: "authentication_error", Code: "INVALID_API_KEY"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := h.now()
		h.logger.Info(r.Context(), logging.EventHTTPRequestStart, "method", r.Method, "path", r.URL.Path)
		capturingWriter := &statusCapturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(capturingWriter, r)
		fields := []any{"method", r.Method, "path", r.URL.Path, "status_code", capturingWriter.statusCode, "duration_ms", h.now().Sub(startedAt).Milliseconds()}
		if capturingWriter.statusCode >= http.StatusBadRequest {
			h.logger.Error(context.Background(), logging.EventHTTPRequestFinish, fields...)
		} else {
			h.logger.Info(context.Background(), logging.EventHTTPRequestFinish, fields...)
		}
	})
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCapturingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusCapturingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, apiErr *core.Error) {
	if apiErr == nil {
		writeJSON(w, http.StatusInternalServerError, openai.ErrorEnvelope{Error: openai.ErrorPayload{Message: "internal server error", Type: "server_error", Code: "INTERNAL_ERROR"}})
		return
	}
	statusCode := apiErr.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	writeJSON(w, statusCode, openai.ErrorEnvelope{Error: openai.ErrorPayload{Message: apiErr.Error(), Type: "proxy_error", Code: apiErr.Code}})
}
