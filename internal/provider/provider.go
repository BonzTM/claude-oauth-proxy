package provider

import (
	"context"
	"io"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
)

type Stream interface {
	Next() (openai.ChatCompletionChunk, error)
	Close() error
}

type Service interface {
	ListModels(ctx context.Context, input ListModelsInput) (ListModelsOutput, *core.Error)
	CreateChatCompletion(ctx context.Context, input CreateChatCompletionInput) (CreateChatCompletionOutput, *core.Error)
	CreateChatCompletionStream(ctx context.Context, input CreateChatCompletionStreamInput) (CreateChatCompletionStreamOutput, *core.Error)
}

type ListModelsInput struct{}

type ListModelsOutput struct {
	Response openai.ModelsResponse
}

type CreateChatCompletionInput struct {
	Request openai.ChatCompletionRequest
}

type CreateChatCompletionOutput struct {
	Response openai.ChatCompletionResponse
}

type CreateChatCompletionStreamInput struct {
	Request openai.ChatCompletionRequest
}

type CreateChatCompletionStreamOutput struct {
	Stream Stream
}

type loggingService struct {
	next   Service
	logger logging.Logger
	now    func() time.Time
}

func WithLogging(next Service, logger logging.Logger) Service {
	return WithLoggingClock(next, logger, time.Now)
}

func WithLoggingClock(next Service, logger logging.Logger, now func() time.Time) Service {
	if next == nil {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &loggingService{next: next, logger: logging.Normalize(logger), now: now}
}

func (s *loggingService) ListModels(ctx context.Context, input ListModelsInput) (ListModelsOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationProviderModels, func() (ListModelsOutput, *core.Error) {
		return s.next.ListModels(ctx, input)
	})
}

func (s *loggingService) CreateChatCompletion(ctx context.Context, input CreateChatCompletionInput) (CreateChatCompletionOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationProviderChat, func() (CreateChatCompletionOutput, *core.Error) {
		return s.next.CreateChatCompletion(ctx, input)
	})
}

func (s *loggingService) CreateChatCompletionStream(ctx context.Context, input CreateChatCompletionStreamInput) (CreateChatCompletionStreamOutput, *core.Error) {
	return withOperation(ctx, s.now, s.logger, logging.OperationProviderChatStreaming, func() (CreateChatCompletionStreamOutput, *core.Error) {
		return s.next.CreateChatCompletionStream(ctx, input)
	})
}

func withOperation[T any](ctx context.Context, now func() time.Time, logger logging.Logger, operation string, run func() (T, *core.Error)) (T, *core.Error) {
	startedAt := now()
	logger.Info(ctx, logging.EventServiceOperationStart, "operation", operation)
	result, err := run()
	fields := []any{"operation", operation, "duration_ms", now().Sub(startedAt).Milliseconds(), "ok", err == nil}
	if err != nil {
		fields = append(fields, "error_code", err.Code)
		logger.Error(ctx, logging.EventServiceOperationFinish, fields...)
	} else {
		logger.Info(ctx, logging.EventServiceOperationFinish, fields...)
	}
	return result, err
}

type chunkStream struct {
	next  func() (openai.ChatCompletionChunk, error)
	close func() error
}

func NewChunkStream(next func() (openai.ChatCompletionChunk, error), closeFn func() error) Stream {
	return &chunkStream{next: next, close: closeFn}
}

func (s *chunkStream) Next() (openai.ChatCompletionChunk, error) {
	if s == nil || s.next == nil {
		return openai.ChatCompletionChunk{}, io.EOF
	}
	return s.next()
}

func (s *chunkStream) Close() error {
	if s == nil || s.close == nil {
		return nil
	}
	return s.close()
}
