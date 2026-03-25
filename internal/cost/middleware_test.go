package cost

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type stubPricingSource struct {
	prices map[string]ModelPricing
}

func (s *stubPricingSource) Lookup(model string) (ModelPricing, bool) {
	p, ok := s.prices[model]
	return p, ok
}

type stubProvider struct {
	response      openai.ChatCompletionResponse
	streamChunks  []openai.ChatCompletionChunk
	err           *core.Error
}

func (s *stubProvider) ListModels(_ context.Context, input provider.ListModelsInput) (provider.ListModelsOutput, *core.Error) {
	return provider.ListModelsOutput{}, nil
}

func (s *stubProvider) CreateChatCompletion(_ context.Context, _ provider.CreateChatCompletionInput) (provider.CreateChatCompletionOutput, *core.Error) {
	if s.err != nil {
		return provider.CreateChatCompletionOutput{}, s.err
	}
	return provider.CreateChatCompletionOutput{Response: s.response}, nil
}

func (s *stubProvider) CreateChatCompletionStream(_ context.Context, _ provider.CreateChatCompletionStreamInput) (provider.CreateChatCompletionStreamOutput, *core.Error) {
	if s.err != nil {
		return provider.CreateChatCompletionStreamOutput{}, s.err
	}
	idx := 0
	stream := provider.NewChunkStream(func() (openai.ChatCompletionChunk, error) {
		if idx >= len(s.streamChunks) {
			return openai.ChatCompletionChunk{}, io.EOF
		}
		chunk := s.streamChunks[idx]
		idx++
		return chunk, nil
	}, func() error { return nil })
	return provider.CreateChatCompletionStreamOutput{Stream: stream}, nil
}

func TestWithCostTrackingNonStreaming(t *testing.T) {
	pricing := &stubPricingSource{prices: map[string]ModelPricing{
		"claude-sonnet-4-20250514": {InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000},
	}}
	inner := &stubProvider{response: openai.ChatCompletionResponse{
		Model: "claude-sonnet-4-20250514",
		Usage: openai.Usage{PromptTokens: 1000, CompletionTokens: 200, TotalTokens: 1200},
	}}
	recorder := logging.NewRecorder()
	svc := WithCostTracking(inner, pricing, recorder)

	result, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{})
	if apiErr != nil {
		t.Fatalf("unexpected error: %v", apiErr)
	}
	if result.Response.Usage.Cost == nil {
		t.Fatal("expected cost to be populated")
	}
	if result.Response.Usage.Cost.Currency != "USD" {
		t.Fatalf("expected USD, got %s", result.Response.Usage.Cost.Currency)
	}
	if result.Response.Usage.Cost.TotalCost <= 0 {
		t.Fatalf("expected positive total cost, got %f", result.Response.Usage.Cost.TotalCost)
	}

	found := false
	for _, entry := range recorder.Entries() {
		if entry.Event == "cost.tracked" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected cost.tracked log entry")
	}
}

func TestWithCostTrackingStreaming(t *testing.T) {
	pricing := &stubPricingSource{prices: map[string]ModelPricing{
		"claude-sonnet-4-20250514": {InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000},
	}}
	finish := "stop"
	inner := &stubProvider{streamChunks: []openai.ChatCompletionChunk{
		{ID: "1", Model: "claude-sonnet-4-20250514", Choices: []openai.ChatCompletionChunkChoice{{Delta: openai.ChatCompletionDelta{Content: "hello"}}}},
		{ID: "1", Model: "claude-sonnet-4-20250514", Choices: []openai.ChatCompletionChunkChoice{{Delta: openai.ChatCompletionDelta{}, FinishReason: &finish}}, Usage: &openai.Usage{PromptTokens: 500, CompletionTokens: 100, TotalTokens: 600}},
	}}
	recorder := logging.NewRecorder()
	svc := WithCostTracking(inner, pricing, recorder)

	result, apiErr := svc.CreateChatCompletionStream(context.Background(), provider.CreateChatCompletionStreamInput{
		Request: openai.ChatCompletionRequest{Model: "claude-sonnet-4-20250514"},
	})
	if apiErr != nil {
		t.Fatalf("unexpected error: %v", apiErr)
	}

	chunk1, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("first chunk error: %v", err)
	}
	if chunk1.Usage != nil {
		t.Fatal("first chunk should not have usage")
	}

	chunk2, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("second chunk error: %v", err)
	}
	if chunk2.Usage == nil || chunk2.Usage.Cost == nil {
		t.Fatal("final chunk should have cost")
	}
	if chunk2.Usage.Cost.TotalCost <= 0 {
		t.Fatalf("expected positive cost, got %f", chunk2.Usage.Cost.TotalCost)
	}

	if _, err := result.Stream.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if err := result.Stream.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
}

func TestWithCostTrackingPricingNotFound(t *testing.T) {
	pricing := &stubPricingSource{prices: map[string]ModelPricing{}}
	inner := &stubProvider{response: openai.ChatCompletionResponse{
		Model: "unknown-model",
		Usage: openai.Usage{PromptTokens: 100, CompletionTokens: 50},
	}}
	recorder := logging.NewRecorder()
	svc := WithCostTracking(inner, pricing, recorder)

	result, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{})
	if apiErr != nil {
		t.Fatalf("unexpected error: %v", apiErr)
	}
	if result.Response.Usage.Cost != nil {
		t.Fatal("expected nil cost when pricing not found")
	}

	found := false
	for _, entry := range recorder.Entries() {
		if entry.Event == "cost.pricing_not_found" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected cost.pricing_not_found log entry")
	}
}

func TestWithCostTrackingErrorPassthrough(t *testing.T) {
	pricing := &stubPricingSource{prices: map[string]ModelPricing{}}
	inner := &stubProvider{err: core.NewError("UPSTREAM_ERROR", http.StatusBadGateway, "test error", nil)}
	recorder := logging.NewRecorder()
	svc := WithCostTracking(inner, pricing, recorder)

	_, apiErr := svc.CreateChatCompletion(context.Background(), provider.CreateChatCompletionInput{})
	if apiErr == nil {
		t.Fatal("expected error passthrough")
	}
	if apiErr.Code != "UPSTREAM_ERROR" {
		t.Fatalf("expected UPSTREAM_ERROR, got %s", apiErr.Code)
	}
}

func TestWithCostTrackingNilService(t *testing.T) {
	if WithCostTracking(nil, nil, nil) != nil {
		t.Fatal("expected nil for nil service")
	}
}

func TestWithCostTrackingListModelsPassthrough(t *testing.T) {
	inner := &stubProvider{}
	svc := WithCostTracking(inner, &stubPricingSource{}, logging.NewRecorder())
	_, apiErr := svc.ListModels(context.Background(), provider.ListModelsInput{})
	if apiErr != nil {
		t.Fatalf("unexpected error: %v", apiErr)
	}
}
