package anthropicprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ant "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/bonztm/claude-oauth-proxy/internal/auth"
	"github.com/bonztm/claude-oauth-proxy/internal/core"
	"github.com/bonztm/claude-oauth-proxy/internal/openai"
	"github.com/bonztm/claude-oauth-proxy/internal/provider"
)

type accessTokenSource interface {
	AccessToken(ctx context.Context, input auth.AccessTokenInput) (auth.AccessTokenOutput, *core.Error)
}

type Config struct {
	BaseURL       string
	BetaHeader    string
	BillingHeader string
	HTTPClient    *http.Client
	Now           func() time.Time
}

type service struct {
	cfg         Config
	tokenSource accessTokenSource
}

func New(cfg Config, tokenSource accessTokenSource) provider.Service {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 0, Transport: &http.Transport{ResponseHeaderTimeout: 5 * time.Minute, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 10, DisableCompression: true}}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &service{cfg: cfg, tokenSource: tokenSource}
}

func (s *service) ListModels(ctx context.Context, _ provider.ListModelsInput) (provider.ListModelsOutput, *core.Error) {
	client, apiErr := s.client(ctx, false)
	if apiErr != nil {
		return provider.ListModelsOutput{}, apiErr
	}
	out, err := s.fetchModels(ctx, client)
	if unauthorized(err) {
		client, apiErr = s.client(ctx, true)
		if apiErr != nil {
			return provider.ListModelsOutput{}, apiErr
		}
		out, err = s.fetchModels(ctx, client)
	}
	if err != nil {
		return provider.ListModelsOutput{}, translateError("list anthropic models", err)
	}
	return provider.ListModelsOutput{Response: out}, nil
}

func (s *service) CreateChatCompletion(ctx context.Context, input provider.CreateChatCompletionInput) (provider.CreateChatCompletionOutput, *core.Error) {
	client, apiErr := s.client(ctx, false)
	if apiErr != nil {
		return provider.CreateChatCompletionOutput{}, apiErr
	}
	params, apiErr := s.messageParams(input.Request)
	if apiErr != nil {
		return provider.CreateChatCompletionOutput{}, apiErr
	}
	response, err := client.Messages.New(ctx, params)
	if unauthorized(err) {
		client, apiErr = s.client(ctx, true)
		if apiErr != nil {
			return provider.CreateChatCompletionOutput{}, apiErr
		}
		response, err = client.Messages.New(ctx, params)
	}
	if err != nil {
		return provider.CreateChatCompletionOutput{}, translateError("create anthropic message", err)
	}
	return provider.CreateChatCompletionOutput{Response: mapResponse(input.Request.Model, response, s.cfg.Now())}, nil
}

func (s *service) CreateChatCompletionStream(ctx context.Context, input provider.CreateChatCompletionStreamInput) (provider.CreateChatCompletionStreamOutput, *core.Error) {
	client, apiErr := s.client(ctx, false)
	if apiErr != nil {
		return provider.CreateChatCompletionStreamOutput{}, apiErr
	}
	params, apiErr := s.messageParams(input.Request)
	if apiErr != nil {
		return provider.CreateChatCompletionStreamOutput{}, apiErr
	}
	stream := client.Messages.NewStreaming(ctx, params)
	firstChunkSent := false
	finished := false
	chunkID := fmt.Sprintf("chatcmpl-%d", s.cfg.Now().UnixNano())
	var streamUsage openai.Usage
	finishChunk := func() openai.ChatCompletionChunk {
		finish := "stop"
		return openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{}, FinishReason: &finish}}, Usage: &streamUsage}
	}
	next := func() (openai.ChatCompletionChunk, error) {
		if finished {
			return openai.ChatCompletionChunk{}, io.EOF
		}
		for stream.Next() {
			event := stream.Current()
			switch variant := event.AsAny().(type) {
			case ant.ContentBlockDeltaEvent:
				switch delta := variant.Delta.AsAny().(type) {
				case ant.TextDelta:
					chunk := openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{Content: delta.Text}}}}
					if !firstChunkSent {
						chunk.Choices[0].Delta.Role = "assistant"
						firstChunkSent = true
					}
					return chunk, nil
				}
			case ant.MessageStartEvent:
				if msg := variant.Message; msg.Usage.InputTokens > 0 {
					streamUsage.PromptTokens = int64(msg.Usage.InputTokens)
					streamUsage.TotalTokens = int64(msg.Usage.InputTokens + msg.Usage.OutputTokens)
					streamUsage.CacheCreationInputTokens = msg.Usage.CacheCreationInputTokens
					streamUsage.CacheReadInputTokens = msg.Usage.CacheReadInputTokens
				}
			case ant.MessageDeltaEvent:
				streamUsage.CompletionTokens = int64(variant.Usage.OutputTokens)
				streamUsage.CacheCreationInputTokens = variant.Usage.CacheCreationInputTokens
				streamUsage.CacheReadInputTokens = variant.Usage.CacheReadInputTokens
				streamUsage.TotalTokens = streamUsage.PromptTokens + streamUsage.CompletionTokens
			case ant.MessageStopEvent:
				finished = true
				return finishChunk(), nil
			}
		}
		if err := stream.Err(); err != nil {
			return openai.ChatCompletionChunk{}, translateError("stream anthropic message", err)
		}
		finished = true
		return finishChunk(), nil
	}
	closeFn := func() error { return stream.Close() }
	return provider.CreateChatCompletionStreamOutput{Stream: provider.NewChunkStream(next, closeFn)}, nil
}

func (s *service) client(ctx context.Context, forceRefresh bool) (ant.Client, *core.Error) {
	token, apiErr := s.tokenSource.AccessToken(ctx, auth.AccessTokenInput{ForceRefresh: forceRefresh})
	if apiErr != nil {
		return ant.Client{}, apiErr
	}
	return ant.NewClient(
		option.WithBaseURL(s.cfg.BaseURL),
		option.WithAuthToken(token.Token),
		option.WithHeaderAdd("anthropic-beta", s.cfg.BetaHeader),
		option.WithHTTPClient(s.cfg.HTTPClient),
	), nil
}

func (s *service) fetchModels(ctx context.Context, client ant.Client) (openai.ModelsResponse, error) {
	iter := client.Models.ListAutoPaging(ctx, ant.ModelListParams{})
	response := openai.ModelsResponse{Object: "list", Data: []openai.ModelInfo{}}
	for iter.Next() {
		model := iter.Current()
		response.Data = append(response.Data, openai.ModelInfo{ID: model.ID, Object: "model", Created: model.CreatedAt.Unix(), OwnedBy: "anthropic", Name: model.DisplayName})
	}
	return response, iter.Err()
}

func (s *service) messageParams(request openai.ChatCompletionRequest) (ant.MessageNewParams, *core.Error) {
	if strings.TrimSpace(request.Model) == "" {
		return ant.MessageNewParams{}, core.NewError("MODEL_REQUIRED", http.StatusBadRequest, "model is required", nil)
	}
	if len(request.Messages) == 0 {
		return ant.MessageNewParams{}, core.NewError("MESSAGES_REQUIRED", http.StatusBadRequest, "messages are required", nil)
	}
	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	params := ant.MessageNewParams{Model: ant.Model(request.Model), MaxTokens: maxTokens, Messages: []ant.MessageParam{}}
	if request.Temperature != nil {
		params.Temperature = ant.Float(*request.Temperature)
	}
	if request.TopP != nil {
		params.TopP = ant.Float(*request.TopP)
	}
	if len(request.Stop) > 0 {
		params.StopSequences = request.Stop
	}
	systemBlocks := []ant.TextBlockParam{}
	if strings.TrimSpace(s.cfg.BillingHeader) != "" {
		systemBlocks = append(systemBlocks, ant.TextBlockParam{Text: s.cfg.BillingHeader})
	}
	// Find the last two user message indices for cache breakpoints.
	// Breakpoint on second-to-last user message caches conversation history;
	// breakpoint on last user message caches the current turn.
	var userIndices []int
	for i, message := range request.Messages {
		if strings.TrimSpace(message.Role) == "user" {
			userIndices = append(userIndices, i)
		}
	}
	cacheSet := map[int]bool{}
	if n := len(userIndices); n >= 2 {
		cacheSet[userIndices[n-2]] = true
		cacheSet[userIndices[n-1]] = true
	} else if n == 1 {
		cacheSet[userIndices[0]] = true
	}
	for i, message := range request.Messages {
		text := strings.TrimSpace(message.Content.Text)
		switch strings.TrimSpace(message.Role) {
		case "system":
			if text != "" {
				systemBlocks = append(systemBlocks, ant.TextBlockParam{Text: text})
			}
		case "user":
			if cacheSet[i] {
				cached := ant.TextBlockParam{Text: text, CacheControl: ant.NewCacheControlEphemeralParam()}
				params.Messages = append(params.Messages, ant.NewUserMessage(ant.ContentBlockParamUnion{OfText: &cached}))
			} else {
				params.Messages = append(params.Messages, ant.NewUserMessage(ant.NewTextBlock(text)))
			}
		case "assistant":
			params.Messages = append(params.Messages, ant.NewAssistantMessage(ant.NewTextBlock(text)))
		default:
			return ant.MessageNewParams{}, core.NewError("UNSUPPORTED_ROLE", http.StatusBadRequest, fmt.Sprintf("unsupported message role %q", message.Role), nil)
		}
	}
	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = ant.NewCacheControlEphemeralParam()
		params.System = systemBlocks
	}
	return params, nil
}

func mapResponse(model string, message *ant.Message, createdAt time.Time) openai.ChatCompletionResponse {
	content := ""
	finishReason := "stop"
	if message != nil {
		for _, block := range message.Content {
			content += block.Text
		}
		if message.StopReason != "" {
			finishReason = string(message.StopReason)
		}
	}
	usage := openai.Usage{}
	if message != nil {
		usage.PromptTokens = message.Usage.InputTokens
		usage.CompletionTokens = message.Usage.OutputTokens
		usage.TotalTokens = message.Usage.InputTokens + message.Usage.OutputTokens
		usage.CacheCreationInputTokens = message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = message.Usage.CacheReadInputTokens
	}
	return openai.ChatCompletionResponse{ID: fmt.Sprintf("chatcmpl-%d", createdAt.UnixNano()), Object: "chat.completion", Created: createdAt.Unix(), Model: model, Choices: []openai.ChatCompletionChoice{{Index: 0, Message: openai.ChatCompletionMessage{Role: "assistant", Content: openai.MessageContent{Text: content}}, FinishReason: finishReason}}, Usage: usage}
}

func unauthorized(err error) bool {
	var apiErr *ant.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

func translateError(message string, err error) *core.Error {
	var apiErr *ant.Error
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status == 0 {
			status = http.StatusBadGateway
		}
		code := "UPSTREAM_ERROR"
		if status == http.StatusUnauthorized {
			code = "UPSTREAM_UNAUTHORIZED"
		}
		return core.NewError(code, status, message, err)
	}
	return core.NewError("UPSTREAM_ERROR", http.StatusBadGateway, message, err)
}
