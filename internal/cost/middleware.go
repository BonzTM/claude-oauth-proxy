package cost

import (
	"context"
	"fmt"
	"io"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type trackingService struct {
	next    provider.Service
	pricing PricingSource
	logger  logging.Logger
}

// WithCostTracking wraps a provider.Service and attaches cost information
// to every response. For non-streaming responses the cost is computed from
// the Usage block. For streaming responses the wrapping stream computes the
// cost from the final chunk's usage data.
func WithCostTracking(next provider.Service, pricing PricingSource, logger logging.Logger) provider.Service {
	if next == nil {
		return nil
	}
	return &trackingService{
		next:    next,
		pricing: pricing,
		logger:  logging.Normalize(logger),
	}
}

func (s *trackingService) ListModels(ctx context.Context, input provider.ListModelsInput) (provider.ListModelsOutput, *core.Error) {
	return s.next.ListModels(ctx, input)
}

func (s *trackingService) CreateChatCompletion(ctx context.Context, input provider.CreateChatCompletionInput) (provider.CreateChatCompletionOutput, *core.Error) {
	result, err := s.next.CreateChatCompletion(ctx, input)
	if err != nil {
		return result, err
	}
	model := result.Response.Model
	if p, ok := s.pricing.Lookup(model); ok {
		result.Response.Usage.Cost = Calculate(model, result.Response.Usage, p)
		s.logCost(ctx, model, result.Response.Usage.Cost)
	} else {
		s.logger.Info(ctx, "cost.pricing_not_found", "model", model)
	}
	return result, nil
}

func (s *trackingService) CreateChatCompletionStream(ctx context.Context, input provider.CreateChatCompletionStreamInput) (provider.CreateChatCompletionStreamOutput, *core.Error) {
	result, err := s.next.CreateChatCompletionStream(ctx, input)
	if err != nil {
		return result, err
	}
	model := input.Request.Model
	wrapped := &costTrackingStream{
		inner:   result.Stream,
		model:   model,
		pricing: s.pricing,
		logger:  s.logger,
		ctx:     ctx,
	}
	return provider.CreateChatCompletionStreamOutput{
		Stream: wrapped,
	}, nil
}

func (s *trackingService) logCost(ctx context.Context, model string, cost *openai.CostDetails) {
	if cost == nil {
		return
	}
	s.logger.Info(ctx, "cost.tracked",
		"model", model,
		"input_cost", fmt.Sprintf("%.6f", cost.InputCost),
		"output_cost", fmt.Sprintf("%.6f", cost.OutputCost),
		"cache_write_cost", fmt.Sprintf("%.6f", cost.CacheWriteCost),
		"cache_read_cost", fmt.Sprintf("%.6f", cost.CacheReadCost),
		"total_cost", fmt.Sprintf("%.6f", cost.TotalCost),
		"currency", cost.Currency,
	)
}

// costTrackingStream wraps a provider.Stream and attaches cost data to
// the final chunk that contains usage information.
type costTrackingStream struct {
	inner   provider.Stream
	model   string
	pricing PricingSource
	logger  logging.Logger
	ctx     context.Context
}

func (s *costTrackingStream) Next() (openai.ChatCompletionChunk, error) {
	chunk, err := s.inner.Next()
	if err != nil {
		return chunk, err
	}
	if chunk.Usage != nil {
		if p, ok := s.pricing.Lookup(s.model); ok {
			chunk.Usage.Cost = Calculate(s.model, openai.Usage{
				PromptTokens:             chunk.Usage.PromptTokens,
				CompletionTokens:         chunk.Usage.CompletionTokens,
				TotalTokens:              chunk.Usage.TotalTokens,
				CacheCreationInputTokens: chunk.Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     chunk.Usage.CacheReadInputTokens,
			}, p)
			logCostFields(s.ctx, s.logger, s.model, chunk.Usage.Cost)
		} else {
			s.logger.Info(s.ctx, "cost.pricing_not_found", "model", s.model)
		}
	}
	return chunk, nil
}

func (s *costTrackingStream) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func logCostFields(ctx context.Context, logger logging.Logger, model string, cost *openai.CostDetails) {
	if cost == nil {
		return
	}
	logger.Info(ctx, "cost.tracked",
		"model", model,
		"input_cost", fmt.Sprintf("%.6f", cost.InputCost),
		"output_cost", fmt.Sprintf("%.6f", cost.OutputCost),
		"cache_write_cost", fmt.Sprintf("%.6f", cost.CacheWriteCost),
		"cache_read_cost", fmt.Sprintf("%.6f", cost.CacheReadCost),
		"total_cost", fmt.Sprintf("%.6f", cost.TotalCost),
		"currency", cost.Currency,
	)
}

var _ io.Closer = (*costTrackingStream)(nil)
