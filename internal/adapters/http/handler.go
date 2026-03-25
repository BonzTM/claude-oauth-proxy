package httpadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

type contextKey string

const requestIDKey contextKey = "request_id"

type HandlerConfig struct {
	CORSOrigins    string
	MaxRequestBody int64
}

type Handler struct {
	provider       provider.Service
	auth           auth.Service
	apiKey         string
	logger         logging.Logger
	now            func() time.Time
	corsOrigins    []string
	maxRequestBody int64
}

func NewHandler(provider provider.Service, authService auth.Service, apiKey string, logger logging.Logger, now func() time.Time, cfg HandlerConfig) http.Handler {
	if now == nil {
		now = time.Now
	}
	var origins []string
	if strings.TrimSpace(cfg.CORSOrigins) != "" {
		for _, o := range strings.Split(cfg.CORSOrigins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}
	maxBody := cfg.MaxRequestBody
	if maxBody <= 0 {
		maxBody = 10 * 1024 * 1024
	}
	h := &Handler{provider: provider, auth: authService, apiKey: apiKey, logger: logging.Normalize(logger), now: now, corsOrigins: origins, maxRequestBody: maxBody}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/healthz", h.handleHealth)
	mux.HandleFunc("/livez", h.handleHealth)
	mux.HandleFunc("/ready", h.handleReady)
	mux.HandleFunc("/readyz", h.handleReady)
	// NOTE: /v1/models is intentionally unauthenticated to allow client model
	// discovery without credentials. This matches common proxy conventions where
	// model listing is a read-only metadata operation.
	mux.HandleFunc("/v1/models", h.handleModels)
	mux.Handle("/v1/chat/completions", h.requireAPIKey(http.HandlerFunc(h.handleChatCompletions)))
	mux.Handle("/v1/", h.requireAPIKey(http.HandlerFunc(h.handleUnsupportedEndpoint)))
	return h.withRequestID(h.withCORS(h.withRequestLogging(mux)))
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
	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBody)
	defer func() {
		_ = r.Body.Close()
	}()
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
	if c := result.Response.Usage.Cost; c != nil {
		w.Header().Set("X-Request-Cost", fmt.Sprintf("%.6f %s", c.TotalCost, c.Currency))
	}
	writeJSON(w, http.StatusOK, result.Response)
}

func (h *Handler) handleStreamingChatCompletions(w http.ResponseWriter, r *http.Request, request openai.ChatCompletionRequest) {
	result, apiErr := h.provider.CreateChatCompletionStream(r.Context(), provider.CreateChatCompletionStreamInput{Request: request})
	if apiErr != nil {
		writeError(w, apiErr)
		return
	}
	defer func() {
		_ = result.Stream.Close()
	}()
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
			h.logger.Error(r.Context(), "stream.error", "error", err.Error())
			return
		}
		data, marshalErr := json.Marshal(chunk)
		if marshalErr != nil {
			h.logger.Error(r.Context(), "stream.marshal_error", "error", marshalErr.Error())
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

var generateRequestID = func() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

func (h *Handler) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = generateRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) withCORS(next http.Handler) http.Handler {
	if len(h.corsOrigins) == 0 {
		return next
	}
	allowed := make(map[string]bool, len(h.corsOrigins))
	allowAll := false
	for _, o := range h.corsOrigins {
		if o == "*" {
			allowAll = true
		}
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Request-Cost")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := h.now()
		requestID, _ := r.Context().Value(requestIDKey).(string)
		h.logger.Info(r.Context(), logging.EventHTTPRequestStart, "method", r.Method, "path", r.URL.Path, "request_id", requestID)
		capturingWriter := &statusCapturingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(capturingWriter, r)
		fields := []any{"method", r.Method, "path", r.URL.Path, "status_code", capturingWriter.statusCode, "duration_ms", h.now().Sub(startedAt).Milliseconds(), "request_id", requestID}
		if capturingWriter.statusCode >= http.StatusBadRequest {
			h.logger.Error(r.Context(), logging.EventHTTPRequestFinish, fields...)
		} else {
			h.logger.Info(r.Context(), logging.EventHTTPRequestFinish, fields...)
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
