package anthropicprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ant "github.com/anthropics/anthropic-sdk-go"
	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type fakeTokenSource struct {
	tokens []auth.AccessTokenOutput
	calls  []bool
	err    *core.Error
}

func (f *fakeTokenSource) AccessToken(_ context.Context, input auth.AccessTokenInput) (auth.AccessTokenOutput, *core.Error) {
	f.calls = append(f.calls, input.ForceRefresh)
	if f.err != nil {
		return auth.AccessTokenOutput{}, f.err
	}
	index := len(f.calls) - 1
	if index >= len(f.tokens) {
		index = len(f.tokens) - 1
	}
	return f.tokens[index], nil
}

func TestListModelsAndChatCompletion(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	var capturedAuth string
	var capturedBeta string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedBeta = r.Header.Get("Anthropic-Beta")
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-test","display_name":"Claude Sonnet Test","created_at":"2026-03-22T12:00:00Z","type":"model","capabilities":{},"max_input_tokens":200000,"max_tokens":8192}],"has_more":false,"first_id":"claude-sonnet-test","last_id":"claude-sonnet-test"}`))
		case "/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","model":"claude-sonnet-test","role":"assistant","content":[{"type":"text","text":"hello from claude"}],"stop_reason":"end_turn","stop_sequence":"","usage":{"input_tokens":12,"output_tokens":4}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", BillingHeader: "x-anthropic-billing-header: test", HTTPClient: server.Client(), Now: func() time.Time { return now }}, tokenSource)

	models, apiErr := svc.ListModels(context.Background(), provider.ListModelsInput{})
	if apiErr != nil {
		t.Fatalf("list models: %v", apiErr)
	}
	if len(models.Response.Data) != 1 || models.Response.Data[0].ID != "claude-sonnet-test" {
		t.Fatalf("unexpected models response: %+v", models.Response)
	}

	request := openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "system", Content: openai.MessageContent{Text: "be brief"}}, {Role: "user", Content: openai.MessageContent{Text: "say hi"}}}, MaxTokens: 64}
	completion, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{Request: request})
	if apiErr != nil {
		t.Fatalf("create chat completion: %v", apiErr)
	}
	if completion.Response.Choices[0].Message.Content.Text != "hello from claude" {
		t.Fatalf("unexpected chat completion response: %+v", completion.Response)
	}
	if capturedAuth != "Bearer token-1" || capturedBeta != "oauth-2025-04-20" {
		t.Fatalf("unexpected auth headers: auth=%q beta=%q", capturedAuth, capturedBeta)
	}
	system, ok := capturedBody["system"].([]any)
	if !ok || len(system) != 2 {
		t.Fatalf("unexpected system blocks: %+v", capturedBody["system"])
	}
	if tokenSource.calls[0] {
		t.Fatalf("expected initial token fetch without force refresh: %+v", tokenSource.calls)
	}
}

func TestCreateChatCompletionRetriesUnauthorized(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"authentication_error","message":"expired token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","model":"claude-sonnet-test","role":"assistant","content":[{"type":"text","text":"retry success"}],"stop_reason":"end_turn","stop_sequence":"","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}, {Token: "token-2"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", HTTPClient: server.Client(), Now: func() time.Time { return now }}, tokenSource)
	response, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{Request: openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}}})
	if apiErr != nil {
		t.Fatalf("chat completion retry: %v", apiErr)
	}
	if response.Response.Choices[0].Message.Content.Text != "retry success" {
		t.Fatalf("unexpected retry response: %+v", response.Response)
	}
	if requestCount != 2 || len(tokenSource.calls) != 2 || tokenSource.calls[1] != true {
		t.Fatalf("expected forced refresh retry, requests=%d calls=%+v", requestCount, tokenSource.calls)
	}
}

func TestListModelsRetriesUnauthorized(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"type":"authentication_error","message":"expired token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-test","display_name":"Claude Sonnet Test","created_at":"2026-03-22T12:00:00Z","type":"model","capabilities":{},"max_input_tokens":200000,"max_tokens":8192}],"has_more":false,"first_id":"claude-sonnet-test","last_id":"claude-sonnet-test"}`))
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}, {Token: "token-2"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", HTTPClient: server.Client(), Now: time.Now}, tokenSource)
	models, apiErr := svc.ListModels(context.Background(), provider.ListModelsInput{})
	if apiErr != nil || len(models.Response.Data) != 1 || requestCount != 2 || !tokenSource.calls[1] {
		t.Fatalf("unexpected list models retry result: models=%+v err=%v requests=%d calls=%+v", models, apiErr, requestCount, tokenSource.calls)
	}
}

func TestCreateChatCompletionStreamMapsTextDeltas(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}",
			"",
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}",
			"",
			"event: message_stop",
			"data: {\"type\":\"message_stop\"}",
			"",
		}, "\n")))
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", HTTPClient: server.Client(), Now: func() time.Time { return now }}, tokenSource)
	result, apiErr := svc.CreateChatCompletionStream(context.Background(), provider.CreateChatCompletionStreamInput{Request: openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}}})
	if apiErr != nil {
		t.Fatalf("create chat completion stream: %v", apiErr)
	}
	defer result.Stream.Close()
	chunkOne, err := result.Stream.Next()
	if err != nil || chunkOne.Choices[0].Delta.Role != "assistant" || chunkOne.Choices[0].Delta.Content != "hello" {
		t.Fatalf("unexpected first chunk: %+v err=%v", chunkOne, err)
	}
	chunkTwo, err := result.Stream.Next()
	if err != nil || chunkTwo.Choices[0].Delta.Content != " world" {
		t.Fatalf("unexpected second chunk: %+v err=%v", chunkTwo, err)
	}
	chunkThree, err := result.Stream.Next()
	if err != nil || chunkThree.Choices[0].FinishReason == nil || *chunkThree.Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected final chunk: %+v err=%v", chunkThree, err)
	}
	if _, err := result.Stream.Next(); err != io.EOF {
		t.Fatalf("expected eof after final chunk, got %v", err)
	}
}

func TestCreateChatCompletionStreamTranslatesStreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: error\ndata: upstream failed\n\n"))
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", HTTPClient: server.Client(), Now: time.Now}, tokenSource)
	result, apiErr := svc.CreateChatCompletionStream(context.Background(), provider.CreateChatCompletionStreamInput{Request: openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}}})
	if apiErr != nil {
		t.Fatalf("unexpected stream creation error: %v", apiErr)
	}
	defer result.Stream.Close()
	if _, err := result.Stream.Next(); err == nil || !strings.Contains(err.Error(), "stream anthropic message") {
		t.Fatalf("expected translated stream error, got %v", err)
	}
}

func TestMessageParamsRejectsInvalidRequestsAndTranslatesErrors(t *testing.T) {
	tokenSource := &fakeTokenSource{err: core.NewError("TOKEN_FAILED", http.StatusServiceUnavailable, "token failed", nil)}
	svc := New(Config{Now: time.Now}, tokenSource)
	if _, apiErr := svc.ListModels(context.Background(), provider.ListModelsInput{}); apiErr == nil || apiErr.Code != "TOKEN_FAILED" {
		t.Fatalf("expected token error, got %v", apiErr)
	}
	if _, ok := svc.(*service); !ok {
		t.Fatal("expected concrete service implementation")
	}
	if _, ok := New(Config{}, tokenSource).(*service); !ok {
		t.Fatal("expected default-config service implementation")
	}
	base := &service{cfg: Config{BillingHeader: "billing"}}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{}); apiErr == nil || apiErr.Code != "MODEL_REQUIRED" {
		t.Fatalf("expected missing model error, got %v", apiErr)
	}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{Model: "model"}); apiErr == nil || apiErr.Code != "MESSAGES_REQUIRED" {
		t.Fatalf("expected missing messages error, got %v", apiErr)
	}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{Model: "model", Messages: []openai.ChatCompletionMessage{{Role: "tool", Content: openai.MessageContent{Text: "bad"}}}}); apiErr == nil || apiErr.Code != "UNSUPPORTED_ROLE" {
		t.Fatalf("expected unsupported role error, got %v", apiErr)
	}
	if apiErr := translateError("oops", errors.New("boom")); apiErr.Code != "UPSTREAM_ERROR" || apiErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected translated error: %+v", apiErr)
	}
	unauthorizedErr := &ant.Error{StatusCode: http.StatusUnauthorized, Request: &http.Request{Method: http.MethodGet, URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/v1/messages"}}, Response: &http.Response{StatusCode: http.StatusUnauthorized, Status: "401 Unauthorized"}}
	if apiErr := translateError("oops", unauthorizedErr); apiErr.Code != "UPSTREAM_UNAUTHORIZED" || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected unauthorized translated error: %+v", apiErr)
	}
	_ = unauthorized(nil)
	if unauthorized(nil) {
		t.Fatal("expected nil error to be non-unauthorized")
	}
	mapped := mapResponse("model", nil, time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC))
	if mapped.Choices[0].Message.Content.Text != "" || mapped.Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected mapped nil response: %+v", mapped)
	}
}
