package cost

import (
	"math"
	"testing"

	"github.com/bonztm/claude-oauth-proxy/internal/openai"
)

func TestCalculateRegularTokens(t *testing.T) {
	pricing := ModelPricing{InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000}
	usage := openai.Usage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500}
	got := Calculate("claude-sonnet-4-20250514", usage, pricing)
	if got == nil {
		t.Fatal("expected non-nil cost details")
	}
	wantInput := 1000 * 3.0 / 1_000_000
	wantOutput := 500 * 15.0 / 1_000_000
	assertClose(t, "input_cost", got.InputCost, wantInput)
	assertClose(t, "output_cost", got.OutputCost, wantOutput)
	assertClose(t, "total_cost", got.TotalCost, wantInput+wantOutput)
	if got.Currency != "USD" {
		t.Fatalf("expected USD, got %s", got.Currency)
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected model to match, got %s", got.Model)
	}
	assertClose(t, "input_price_per_1m", got.InputPricePer1M, 3.0)
	assertClose(t, "output_price_per_1m", got.OutputPricePer1M, 15.0)
}

func TestCalculateWithCacheTokens(t *testing.T) {
	pricing := ModelPricing{InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000}
	usage := openai.Usage{
		PromptTokens:             2000,
		CompletionTokens:         100,
		CacheCreationInputTokens: 500,
		CacheReadInputTokens:     300,
	}
	got := Calculate("test-model", usage, pricing)
	if got == nil {
		t.Fatal("expected non-nil cost details")
	}
	regularInput := int64(2000 - 500 - 300)
	wantInput := float64(regularInput) * pricing.InputPerToken
	wantCacheWrite := float64(500) * pricing.InputPerToken * 1.25
	wantCacheRead := float64(300) * pricing.InputPerToken * 0.1
	wantOutput := float64(100) * pricing.OutputPerToken
	assertClose(t, "input_cost", got.InputCost, wantInput)
	assertClose(t, "cache_write_cost", got.CacheWriteCost, wantCacheWrite)
	assertClose(t, "cache_read_cost", got.CacheReadCost, wantCacheRead)
	assertClose(t, "output_cost", got.OutputCost, wantOutput)
	assertClose(t, "total_cost", got.TotalCost, wantInput+wantOutput+wantCacheWrite+wantCacheRead)
}

func TestCalculateZeroPricing(t *testing.T) {
	usage := openai.Usage{PromptTokens: 1000, CompletionTokens: 500}
	got := Calculate("model", usage, ModelPricing{})
	if got != nil {
		t.Fatalf("expected nil for zero pricing, got %+v", got)
	}
}

func TestCalculateZeroTokens(t *testing.T) {
	pricing := ModelPricing{InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000}
	got := Calculate("model", openai.Usage{}, pricing)
	if got == nil {
		t.Fatal("expected non-nil cost details")
	}
	assertClose(t, "total_cost", got.TotalCost, 0)
}

func TestCalculateNegativeRegularInputClampedToZero(t *testing.T) {
	pricing := ModelPricing{InputPerToken: 3.0 / 1_000_000, OutputPerToken: 15.0 / 1_000_000}
	usage := openai.Usage{
		PromptTokens:             100,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     200,
	}
	got := Calculate("model", usage, pricing)
	if got == nil {
		t.Fatal("expected non-nil cost details")
	}
	assertClose(t, "input_cost", got.InputCost, 0)
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("%s: got %.12f, want %.12f", name, got, want)
	}
}
