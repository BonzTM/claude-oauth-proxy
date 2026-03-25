package cost

import "github.com/bonztm/claude-oauth-proxy/internal/openai"

// ModelPricing holds per-token prices in USD.
// InputPerToken and OutputPerToken are the cost for a single token.
type ModelPricing struct {
	InputPerToken  float64
	OutputPerToken float64
}

const (
	cacheWriteMultiplier = 1.25
	cacheReadMultiplier  = 0.1
	tokensPerMillion     = 1_000_000.0
)

// Calculate computes cost details for a completed request.
// It applies Anthropic cache pricing rules: cache writes cost 1.25x input,
// cache reads cost 0.1x input. Regular input tokens are calculated after
// subtracting cache tokens from the total prompt tokens reported.
func Calculate(model string, usage openai.Usage, pricing ModelPricing) *openai.CostDetails {
	if pricing.InputPerToken == 0 && pricing.OutputPerToken == 0 {
		return nil
	}
	regularInput := usage.PromptTokens - usage.CacheCreationInputTokens - usage.CacheReadInputTokens
	if regularInput < 0 {
		regularInput = 0
	}
	inputCost := float64(regularInput) * pricing.InputPerToken
	outputCost := float64(usage.CompletionTokens) * pricing.OutputPerToken
	cacheWriteCost := float64(usage.CacheCreationInputTokens) * pricing.InputPerToken * cacheWriteMultiplier
	cacheReadCost := float64(usage.CacheReadInputTokens) * pricing.InputPerToken * cacheReadMultiplier
	total := inputCost + outputCost + cacheWriteCost + cacheReadCost
	return &openai.CostDetails{
		InputCost:        inputCost,
		OutputCost:       outputCost,
		CacheWriteCost:   cacheWriteCost,
		CacheReadCost:    cacheReadCost,
		TotalCost:        total,
		Currency:         "USD",
		Model:            model,
		InputPricePer1M:  pricing.InputPerToken * tokensPerMillion,
		OutputPricePer1M: pricing.OutputPerToken * tokensPerMillion,
	}
}
