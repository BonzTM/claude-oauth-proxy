package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/logging"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
)

type fakeService struct {
	errByOperation map[string]*core.Error
}

func (f fakeService) ListModels(_ context.Context, _ ListModelsInput) (ListModelsOutput, *core.Error) {
	return ListModelsOutput{}, f.errByOperation[logging.OperationProviderModels]
}

func (f fakeService) CreateChatCompletion(_ context.Context, _ CreateChatCompletionInput) (CreateChatCompletionOutput, *core.Error) {
	return CreateChatCompletionOutput{}, f.errByOperation[logging.OperationProviderChat]
}

func (f fakeService) CreateChatCompletionStream(_ context.Context, _ CreateChatCompletionStreamInput) (CreateChatCompletionStreamOutput, *core.Error) {
	return CreateChatCompletionStreamOutput{}, f.errByOperation[logging.OperationProviderChatStreaming]
}

func TestChunkStreamAndLoggingDecorator(t *testing.T) {
	recorder := logging.NewRecorder()
	svc := WithLoggingClock(fakeService{}, recorder, time.Now)
	for _, call := range []func(){
		func() { _, _ = svc.ListModels(context.Background(), ListModelsInput{}) },
		func() { _, _ = svc.CreateChatCompletion(context.Background(), CreateChatCompletionInput{}) },
		func() { _, _ = svc.CreateChatCompletionStream(context.Background(), CreateChatCompletionStreamInput{}) },
	} {
		recorder.Reset()
		call()
		if got := len(recorder.Entries()); got != 2 {
			t.Fatalf("expected two log entries, got %d", got)
		}
	}

	recorder.Reset()
	svc = WithLoggingClock(fakeService{errByOperation: map[string]*core.Error{logging.OperationProviderChat: core.NewError("CHAT_FAILED", http.StatusBadGateway, "chat failed", nil)}}, recorder, time.Now)
	_, apiErr := svc.CreateChatCompletion(context.Background(), CreateChatCompletionInput{})
	if apiErr == nil {
		t.Fatal("expected chat error")
	}
	if recorder.Entries()[1].Fields["error_code"] != "CHAT_FAILED" {
		t.Fatalf("unexpected log entries: %+v", recorder.Entries())
	}
	if WithLogging(nil, recorder) != nil {
		t.Fatal("expected nil provider to remain nil")
	}

	stream := NewChunkStream(func() (openai.ChatCompletionChunk, error) {
		return openai.ChatCompletionChunk{}, io.EOF
	}, func() error { return errors.New("closed") })
	if _, err := stream.Next(); err != io.EOF {
		t.Fatalf("expected eof, got %v", err)
	}
	if err := stream.Close(); err == nil || err.Error() != "closed" {
		t.Fatalf("unexpected close error: %v", err)
	}
	if _, err := (*chunkStream)(nil).Next(); err != io.EOF {
		t.Fatalf("expected nil chunk stream eof, got %v", err)
	}
	if err := (*chunkStream)(nil).Close(); err != nil {
		t.Fatalf("expected nil chunk stream close success, got %v", err)
	}
}
