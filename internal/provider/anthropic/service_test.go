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
			defer func() {
				_ = r.Body.Close()
			}()
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
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", BillingHeader: "x-anthropic-billing-header: cc_version=%s; test", CCVersion: "1.0.0", HTTPClient: server.Client(), Now: func() time.Time { return now }}, tokenSource)

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
	defer func() {
		_ = result.Stream.Close()
	}()
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
	defer func() {
		_ = result.Stream.Close()
	}()
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
	base := &service{cfg: Config{BillingHeader: "billing: cc_version=%s", CCVersion: "1.0.0"}}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{}); apiErr == nil || apiErr.Code != "MODEL_REQUIRED" {
		t.Fatalf("expected missing model error, got %v", apiErr)
	}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{Model: "model"}); apiErr == nil || apiErr.Code != "MESSAGES_REQUIRED" {
		t.Fatalf("expected missing messages error, got %v", apiErr)
	}
	if _, apiErr := base.messageParams(openai.ChatCompletionRequest{Model: "model", Messages: []openai.ChatCompletionMessage{{Role: "invalid", Content: openai.MessageContent{Text: "bad"}}}}); apiErr == nil || apiErr.Code != "UNSUPPORTED_ROLE" {
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
	if got := estimateThinkingTokens(0); got != 0 {
		t.Fatalf("expected 0 tokens for 0 chars, got %d", got)
	}
	if got := estimateThinkingTokens(-1); got != 0 {
		t.Fatalf("expected 0 tokens for negative chars, got %d", got)
	}
	if got := estimateThinkingTokens(100); got != 25 {
		t.Fatalf("expected 25 tokens for 100 chars, got %d", got)
	}
	if got := estimateThinkingTokens(3); got != 1 {
		t.Fatalf("expected 1 token for 3 chars, got %d", got)
	}
}

func TestMessageParamsToolCalls(t *testing.T) {
	base := &service{cfg: Config{BillingHeader: "", CCVersion: "1.0.0"}}
	params, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model: "claude-sonnet-test",
		Messages: []openai.ChatCompletionMessage{
			{
				Role: "assistant",
				ToolCalls: []openai.ToolCall{
					{ID: "tool_1", Type: "function", Function: openai.FunctionCall{Name: "get_weather", Arguments: ""}},
					{ID: "tool_2", Type: "function", Function: openai.FunctionCall{Name: "get_time", Arguments: `{"tz":"utc"}`}},
				},
			},
			{
				Role:    "assistant",
				Content: openai.MessageContent{Text: "calling tool now"},
				ToolCalls: []openai.ToolCall{
					{ID: "tool_3", Type: "function", Function: openai.FunctionCall{Name: "lookup_city", Arguments: `{"city":"Boston"}`}},
				},
			},
			{Role: "tool", ToolCallID: "tool_3", Content: openai.MessageContent{Text: "sunny"}},
			{Role: "tool", ToolCallID: "tool_4", Content: openai.MessageContent{Text: ""}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params: %v", apiErr)
	}
	if len(params.Messages) != 4 {
		t.Fatalf("expected 4 params messages, got %d", len(params.Messages))
	}

	first := params.Messages[0]
	if first.Role != "assistant" || len(first.Content) != 2 {
		t.Fatalf("unexpected first assistant message: %+v", first)
	}
	if first.Content[0].OfToolUse == nil || first.Content[0].OfToolUse.Name != "get_weather" {
		t.Fatalf("unexpected first tool call block: %+v", first.Content[0].OfToolUse)
	}
	firstInput, err := json.Marshal(first.Content[0].OfToolUse.Input)
	if err != nil || string(firstInput) != "{}" {
		t.Fatalf("unexpected first tool call input: %s err=%v", string(firstInput), err)
	}
	if first.Content[1].OfToolUse == nil || first.Content[1].OfToolUse.Name != "get_time" {
		t.Fatalf("unexpected second tool call block: %+v", first.Content[1].OfToolUse)
	}
	secondInput, err := json.Marshal(first.Content[1].OfToolUse.Input)
	if err != nil || string(secondInput) != `{"tz":"utc"}` {
		t.Fatalf("unexpected second tool call input: %s err=%v", string(secondInput), err)
	}

	second := params.Messages[1]
	if second.Role != "assistant" || len(second.Content) != 2 {
		t.Fatalf("unexpected second assistant message: %+v", second)
	}
	if second.Content[0].OfText == nil || second.Content[0].OfText.Text != "calling tool now" {
		t.Fatalf("expected assistant text block before tool call, got %+v", second.Content[0])
	}
	if second.Content[1].OfToolUse == nil || second.Content[1].OfToolUse.ID != "tool_3" || second.Content[1].OfToolUse.Name != "lookup_city" {
		t.Fatalf("unexpected assistant tool block: %+v", second.Content[1].OfToolUse)
	}

	toolWithText := params.Messages[2]
	if toolWithText.Role != "user" || len(toolWithText.Content) != 1 || toolWithText.Content[0].OfToolResult == nil {
		t.Fatalf("unexpected tool result message with text: %+v", toolWithText)
	}
	if toolWithText.Content[0].OfToolResult.ToolUseID != "tool_3" || len(toolWithText.Content[0].OfToolResult.Content) != 1 || toolWithText.Content[0].OfToolResult.Content[0].OfText == nil || toolWithText.Content[0].OfToolResult.Content[0].OfText.Text != "sunny" {
		t.Fatalf("unexpected tool result payload with text: %+v", toolWithText.Content[0].OfToolResult)
	}

	toolWithoutText := params.Messages[3]
	if toolWithoutText.Role != "user" || len(toolWithoutText.Content) != 1 || toolWithoutText.Content[0].OfToolResult == nil {
		t.Fatalf("unexpected empty tool result message: %+v", toolWithoutText)
	}
	if toolWithoutText.Content[0].OfToolResult.ToolUseID != "tool_4" || len(toolWithoutText.Content[0].OfToolResult.Content) != 0 {
		t.Fatalf("expected empty tool result content, got %+v", toolWithoutText.Content[0].OfToolResult)
	}
}

func TestMessageParamsToolDefinitions(t *testing.T) {
	base := &service{cfg: Config{BillingHeader: "", CCVersion: "1.0.0"}}
	params, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model: "claude-sonnet-test",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: openai.MessageContent{Text: "hello"}},
		},
		Tools: []openai.Tool{
			{
				Type: "function",
				Function: openai.ToolFunction{
					Name:        "get_weather",
					Description: "Fetch weather for a city",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
				},
			},
			{
				Type: "function",
				Function: openai.ToolFunction{
					Name:       "no_description",
					Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
				},
			},
			{Type: "server", Function: openai.ToolFunction{Name: "skip_non_function"}},
			{Type: "function", Function: openai.ToolFunction{Name: ""}},
			{Type: "function", Function: openai.ToolFunction{Name: "last_tool"}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params: %v", apiErr)
	}
	if len(params.Tools) != 3 {
		t.Fatalf("expected 3 translated tools, got %d", len(params.Tools))
	}

	first := params.Tools[0].OfTool
	if first == nil || first.Name != "get_weather" {
		t.Fatalf("unexpected first tool: %+v", first)
	}
	if !first.Description.Valid() || first.Description.Value != "Fetch weather for a city" {
		t.Fatalf("expected first tool description to be set, got %+v", first.Description)
	}
	if first.InputSchema.Type != "object" {
		t.Fatalf("expected first tool schema type object, got %q", first.InputSchema.Type)
	}

	second := params.Tools[1].OfTool
	if second == nil || second.Name != "no_description" {
		t.Fatalf("unexpected second tool: %+v", second)
	}
	if second.Description.Valid() {
		t.Fatalf("expected second tool description to be omitted, got %+v", second.Description)
	}

	last := params.Tools[2].OfTool
	if last == nil || last.Name != "last_tool" {
		t.Fatalf("unexpected last tool: %+v", last)
	}
	if last.CacheControl.Type != "ephemeral" {
		t.Fatalf("expected last tool cache control breakpoint, got %+v", last.CacheControl)
	}
}

func TestMessageParamsReasoningEffort(t *testing.T) {
	base := &service{cfg: Config{BillingHeader: "", CCVersion: "1.0.0"}}
	cases := []struct {
		name              string
		effort            string
		maxTokens         int64
		expectedBudget    int64
		expectedMaxTokens int64
		expectsThinking   bool
	}{
		{name: "low", effort: "low", maxTokens: 500, expectedBudget: 1024, expectedMaxTokens: 5120, expectsThinking: true},
		{name: "medium", effort: "medium", maxTokens: 2000, expectedBudget: 8192, expectedMaxTokens: 12288, expectsThinking: true},
		{name: "high", effort: "high", maxTokens: 32768, expectedBudget: 32768, expectedMaxTokens: 36864, expectsThinking: true},
		{name: "unknown", effort: "custom", maxTokens: 777, expectedBudget: 0, expectedMaxTokens: 777, expectsThinking: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, apiErr := base.messageParams(openai.ChatCompletionRequest{
				Model:           "claude-sonnet-test",
				ReasoningEffort: tc.effort,
				MaxTokens:       tc.maxTokens,
				Messages: []openai.ChatCompletionMessage{
					{Role: "user", Content: openai.MessageContent{Text: "reason"}},
				},
			})
			if apiErr != nil {
				t.Fatalf("message params: %v", apiErr)
			}
			if params.MaxTokens != tc.expectedMaxTokens {
				t.Fatalf("expected max tokens %d, got %d", tc.expectedMaxTokens, params.MaxTokens)
			}
			budget := params.Thinking.GetBudgetTokens()
			if tc.expectsThinking {
				if budget == nil || *budget != tc.expectedBudget {
					t.Fatalf("expected thinking budget %d, got %v", tc.expectedBudget, budget)
				}
				if !params.Temperature.Valid() || params.Temperature.Value != 1 {
					t.Fatalf("expected temperature forced to 1 with thinking, got %+v", params.Temperature)
				}
			} else {
				if budget != nil {
					t.Fatalf("expected no thinking config for unknown effort, got budget %d", *budget)
				}
			}
		})
	}
}

func TestMessageParamsOptionalFields(t *testing.T) {
	base := &service{cfg: Config{BillingHeader: "", CCVersion: "1.0.0"}}
	temp := 0.3
	topP := 0.8
	params, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model:       "claude-sonnet-test",
		MaxTokens:   0,
		Temperature: &temp,
		TopP:        &topP,
		Stop:        []string{"END", "STOP"},
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: openai.MessageContent{Text: "hi"}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params: %v", apiErr)
	}
	if params.MaxTokens != 1024 {
		t.Fatalf("expected default max tokens 1024, got %d", params.MaxTokens)
	}
	if !params.Temperature.Valid() || params.Temperature.Value != temp {
		t.Fatalf("unexpected temperature: %+v", params.Temperature)
	}
	if !params.TopP.Valid() || params.TopP.Value != topP {
		t.Fatalf("unexpected top_p: %+v", params.TopP)
	}
	if len(params.StopSequences) != 2 || params.StopSequences[0] != "END" || params.StopSequences[1] != "STOP" {
		t.Fatalf("unexpected stop sequences: %+v", params.StopSequences)
	}
	if len(params.System) != 0 {
		t.Fatalf("expected no billing header system block when billing header is empty, got %+v", params.System)
	}
}

func TestMessageParamsCacheBreakpoints(t *testing.T) {
	base := &service{cfg: Config{BillingHeader: "", CCVersion: "1.0.0"}}

	threeUserParams, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model: "claude-sonnet-test",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: openai.MessageContent{Text: "u1"}},
			{Role: "assistant", Content: openai.MessageContent{Text: "a1"}},
			{Role: "user", Content: openai.MessageContent{Text: "u2"}},
			{Role: "user", Content: openai.MessageContent{Text: "u3"}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params with 3 users: %v", apiErr)
	}
	if threeUserParams.Messages[0].Content[0].OfText.CacheControl.Type != "" {
		t.Fatalf("expected first user message without cache breakpoint, got %+v", threeUserParams.Messages[0].Content[0].OfText.CacheControl)
	}
	if threeUserParams.Messages[2].Content[0].OfText.CacheControl.Type != "ephemeral" {
		t.Fatalf("expected second-to-last user message cached, got %+v", threeUserParams.Messages[2].Content[0].OfText.CacheControl)
	}
	if threeUserParams.Messages[3].Content[0].OfText.CacheControl.Type != "ephemeral" {
		t.Fatalf("expected last user message cached, got %+v", threeUserParams.Messages[3].Content[0].OfText.CacheControl)
	}

	oneUserParams, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model: "claude-sonnet-test",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: openai.MessageContent{Text: "u1"}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params with one user: %v", apiErr)
	}
	if oneUserParams.Messages[0].Content[0].OfText.CacheControl.Type != "ephemeral" {
		t.Fatalf("expected single user message cached, got %+v", oneUserParams.Messages[0].Content[0].OfText.CacheControl)
	}

	zeroUserParams, apiErr := base.messageParams(openai.ChatCompletionRequest{
		Model: "claude-sonnet-test",
		Messages: []openai.ChatCompletionMessage{
			{Role: "system", Content: openai.MessageContent{Text: "policy"}},
			{Role: "assistant", Content: openai.MessageContent{Text: "ready"}},
		},
	})
	if apiErr != nil {
		t.Fatalf("message params with zero user messages: %v", apiErr)
	}
	if len(zeroUserParams.Messages) != 1 || zeroUserParams.Messages[0].Role != "assistant" {
		t.Fatalf("unexpected non-user message mapping: %+v", zeroUserParams.Messages)
	}
	if len(zeroUserParams.Messages[0].Content) != 1 || zeroUserParams.Messages[0].Content[0].OfText == nil || zeroUserParams.Messages[0].Content[0].OfText.CacheControl.Type != "" {
		t.Fatalf("expected no cache breakpoint on assistant-only message path, got %+v", zeroUserParams.Messages[0])
	}
}

func TestMapResponseWithToolUseAndThinking(t *testing.T) {
	createdAt := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	message := &ant.Message{
		Content: []ant.ContentBlockUnion{
			{Type: "text", Text: "hello "},
			{Type: "tool_use", ID: "tool_1", Name: "get_weather", Input: json.RawMessage(`{"city":"Paris"}`)},
			{Type: "thinking", Thinking: "reasoning"},
			{Type: "text", Text: "world"},
		},
		StopReason: ant.StopReasonToolUse,
		Usage: ant.Usage{
			InputTokens:              10,
			OutputTokens:             5,
			CacheCreationInputTokens: 3,
			CacheReadInputTokens:     2,
		},
	}

	mapped := mapResponse("claude-sonnet-test", message, createdAt)
	if mapped.Choices[0].Message.Content.Text != "hello world" {
		t.Fatalf("unexpected mapped content: %q", mapped.Choices[0].Message.Content.Text)
	}
	if mapped.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_use to map to tool_calls, got %q", mapped.Choices[0].FinishReason)
	}
	if len(mapped.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected one mapped tool call, got %+v", mapped.Choices[0].Message.ToolCalls)
	}
	if mapped.Choices[0].Message.ToolCalls[0].ID != "tool_1" || mapped.Choices[0].Message.ToolCalls[0].Function.Name != "get_weather" || mapped.Choices[0].Message.ToolCalls[0].Function.Arguments != `{"city":"Paris"}` {
		t.Fatalf("unexpected mapped tool call: %+v", mapped.Choices[0].Message.ToolCalls[0])
	}
	if mapped.Usage.PromptTokens != 15 || mapped.Usage.CompletionTokens != 5 || mapped.Usage.TotalTokens != 20 {
		t.Fatalf("unexpected usage totals: %+v", mapped.Usage)
	}
	if mapped.Usage.CacheCreationInputTokens != 3 || mapped.Usage.CacheReadInputTokens != 2 {
		t.Fatalf("unexpected cache usage tokens: %+v", mapped.Usage)
	}
	if mapped.Usage.PromptTokensDetails == nil || mapped.Usage.PromptTokensDetails.CachedTokens != 2 {
		t.Fatalf("expected cached token details, got %+v", mapped.Usage.PromptTokensDetails)
	}
	if mapped.Usage.CompletionTokensDetails == nil || mapped.Usage.CompletionTokensDetails.ReasoningTokens != estimateThinkingTokens(int64(len("reasoning"))) {
		t.Fatalf("expected reasoning token details, got %+v", mapped.Usage.CompletionTokensDetails)
	}
}

func TestStreamingWithToolUseAndUsageEvents(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: message_start",
			"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-test\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0,\"cache_creation_input_tokens\":3,\"cache_read_input_tokens\":2}}}",
			"",
			"event: content_block_start",
			"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tool_1\",\"name\":\"get_weather\",\"input\":{}}}",
			"",
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\":\\\"Boston\\\"}\"}}",
			"",
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"reasoning step\"}}",
			"",
			"event: message_delta",
			"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":\"\"},\"usage\":{\"output_tokens\":7,\"cache_creation_input_tokens\":3,\"cache_read_input_tokens\":2}}",
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
	defer func() {
		_ = result.Stream.Close()
	}()

	chunkOne, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("first stream chunk: %v", err)
	}
	if chunkOne.Choices[0].Delta.Role != "assistant" {
		t.Fatalf("expected assistant role on first chunk, got %+v", chunkOne)
	}
	if len(chunkOne.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("expected one tool call in first chunk, got %+v", chunkOne.Choices[0].Delta.ToolCalls)
	}
	firstToolCall := chunkOne.Choices[0].Delta.ToolCalls[0]
	if firstToolCall.Index == nil || *firstToolCall.Index != 0 || firstToolCall.ID != "tool_1" || firstToolCall.Function.Name != "get_weather" {
		t.Fatalf("unexpected first tool call chunk: %+v", firstToolCall)
	}

	chunkTwo, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("second stream chunk: %v", err)
	}
	if len(chunkTwo.Choices[0].Delta.ToolCalls) != 1 {
		t.Fatalf("expected one tool call in second chunk, got %+v", chunkTwo.Choices[0].Delta.ToolCalls)
	}
	secondToolCall := chunkTwo.Choices[0].Delta.ToolCalls[0]
	if secondToolCall.Index == nil || *secondToolCall.Index != 0 || secondToolCall.ID != "tool_1" || secondToolCall.Function.Name != "get_weather" || secondToolCall.Function.Arguments != `{"city":"Boston"}` {
		t.Fatalf("unexpected second tool call chunk: %+v", secondToolCall)
	}

	chunkThree, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("final stream chunk: %v", err)
	}
	if chunkThree.Choices[0].FinishReason == nil || *chunkThree.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %+v", chunkThree.Choices[0].FinishReason)
	}
	if chunkThree.Usage == nil {
		t.Fatalf("expected usage in final chunk")
	}
	if chunkThree.Usage.PromptTokens != 15 || chunkThree.Usage.CompletionTokens != 7 || chunkThree.Usage.TotalTokens != 22 {
		t.Fatalf("unexpected usage totals in final chunk: %+v", chunkThree.Usage)
	}
	if chunkThree.Usage.CacheCreationInputTokens != 3 || chunkThree.Usage.CacheReadInputTokens != 2 {
		t.Fatalf("unexpected cache usage in final chunk: %+v", chunkThree.Usage)
	}
	if chunkThree.Usage.PromptTokensDetails == nil || chunkThree.Usage.PromptTokensDetails.CachedTokens != 2 {
		t.Fatalf("expected cached token details in final chunk: %+v", chunkThree.Usage.PromptTokensDetails)
	}
	if chunkThree.Usage.CompletionTokensDetails == nil || chunkThree.Usage.CompletionTokensDetails.ReasoningTokens == 0 {
		t.Fatalf("expected reasoning tokens from thinking deltas, got %+v", chunkThree.Usage.CompletionTokensDetails)
	}

	if _, err := result.Stream.Next(); err != io.EOF {
		t.Fatalf("expected eof after final chunk, got %v", err)
	}
}

func TestCreateChatCompletionAppliesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","model":"claude-sonnet-test","role":"assistant","content":[{"type":"text","text":"late"}],"stop_reason":"end_turn","stop_sequence":"","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", RequestTimeout: 10 * time.Millisecond, HTTPClient: server.Client(), Now: time.Now}, tokenSource)
	_, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{Request: openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}}})
	if apiErr == nil {
		t.Fatal("expected timeout error")
	}
	if apiErr.Code != "UPSTREAM_ERROR" {
		t.Fatalf("expected upstream error code on timeout, got %+v", apiErr)
	}
}

func TestCreateChatCompletionStreamIgnoresRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}",
			"",
		}, "\n")))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte(strings.Join([]string{
			"event: content_block_delta",
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}",
			"",
		}, "\n")))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()
	tokenSource := &fakeTokenSource{tokens: []auth.AccessTokenOutput{{Token: "token-1"}}}
	svc := New(Config{BaseURL: server.URL + "/", BetaHeader: "oauth-2025-04-20", RequestTimeout: 5 * time.Millisecond, HTTPClient: server.Client(), Now: time.Now}, tokenSource)
	result, apiErr := svc.CreateChatCompletionStream(context.Background(), provider.CreateChatCompletionStreamInput{Request: openai.ChatCompletionRequest{Model: "claude-sonnet-test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: openai.MessageContent{Text: "hi"}}}}})
	if apiErr != nil {
		t.Fatalf("unexpected stream creation error: %v", apiErr)
	}
	defer result.Stream.Close()
	if _, err := result.Stream.Next(); err != nil {
		t.Fatalf("unexpected first chunk error: %v", err)
	}
	_, err := result.Stream.Next()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("expected stream not to be cut by request timeout, got err=%v", err)
	}
}
