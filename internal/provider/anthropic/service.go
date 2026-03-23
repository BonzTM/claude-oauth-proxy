package anthropicprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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
	BaseURL        string
	BetaHeader     string
	BillingHeader  string
	CCVersion      string
	UserAgent      string
	SDKVersion     string
	RuntimeVersion string
	StainlessOS    string
	StainlessArch  string
	HTTPClient     *http.Client
	Now            func() time.Time
}

type service struct {
	cfg         Config
	tokenSource accessTokenSource
	turnCounter atomic.Int64
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
	hasToolUse := false
	chunkID := fmt.Sprintf("chatcmpl-%d", s.cfg.Now().UnixNano())
	var streamUsage openai.Usage
	// Track current tool call state for streaming.
	var currentToolID, currentToolName string
	var toolCallIndex int
	var streamThinkingChars int64
	finishChunk := func() openai.ChatCompletionChunk {
		finish := "stop"
		if hasToolUse {
			finish = "tool_calls"
		}
		if streamUsage.CacheReadInputTokens > 0 || streamUsage.CacheCreationInputTokens > 0 {
			streamUsage.PromptTokensDetails = &openai.PromptTokensDetails{CachedTokens: streamUsage.CacheReadInputTokens}
		}
		if streamThinkingChars > 0 {
			streamUsage.CompletionTokensDetails = &openai.CompletionTokensDetails{ReasoningTokens: (streamThinkingChars + 3) / 4}
		}
		return openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{}, FinishReason: &finish}}, Usage: &streamUsage}
	}
	next := func() (openai.ChatCompletionChunk, error) {
		if finished {
			return openai.ChatCompletionChunk{}, io.EOF
		}
		for stream.Next() {
			event := stream.Current()
			switch variant := event.AsAny().(type) {
			case ant.ContentBlockStartEvent:
				if variant.ContentBlock.Type == "tool_use" {
					hasToolUse = true
					currentToolID = variant.ContentBlock.ID
					currentToolName = variant.ContentBlock.Name
					idx := toolCallIndex
					tc := openai.ToolCall{Index: &idx, ID: currentToolID, Type: "function", Function: openai.FunctionCall{Name: currentToolName, Arguments: ""}}
					chunk := openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{ToolCalls: []openai.ToolCall{tc}}}}}
					if !firstChunkSent {
						chunk.Choices[0].Delta.Role = "assistant"
						firstChunkSent = true
					}
					toolCallIndex++
					return chunk, nil
				}
			case ant.ContentBlockDeltaEvent:
				switch delta := variant.Delta.AsAny().(type) {
				case ant.TextDelta:
					chunk := openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{Content: delta.Text}}}}
					if !firstChunkSent {
						chunk.Choices[0].Delta.Role = "assistant"
						firstChunkSent = true
					}
					return chunk, nil
				case ant.ThinkingDelta:
					streamThinkingChars += int64(len(delta.Thinking))
				case ant.InputJSONDelta:
					idx := toolCallIndex - 1
					tc := openai.ToolCall{Index: &idx, ID: currentToolID, Type: "function", Function: openai.FunctionCall{Name: currentToolName, Arguments: delta.PartialJSON}}
					chunk := openai.ChatCompletionChunk{ID: chunkID, Object: "chat.completion.chunk", Created: s.cfg.Now().Unix(), Model: input.Request.Model, Choices: []openai.ChatCompletionChunkChoice{{Index: 0, Delta: openai.ChatCompletionDelta{ToolCalls: []openai.ToolCall{tc}}}}}
					return chunk, nil
				}
			case ant.MessageStartEvent:
				if msg := variant.Message; msg.Usage.InputTokens > 0 || msg.Usage.CacheCreationInputTokens > 0 || msg.Usage.CacheReadInputTokens > 0 {
					totalInput := int64(msg.Usage.InputTokens) + msg.Usage.CacheCreationInputTokens + msg.Usage.CacheReadInputTokens
					streamUsage.PromptTokens = totalInput
					streamUsage.TotalTokens = totalInput + int64(msg.Usage.OutputTokens)
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
		// Match Claude Code's request fingerprint.
		option.WithHeader("User-Agent", s.cfg.UserAgent),
		option.WithHeader("X-Stainless-Lang", "js"),
		option.WithHeader("X-Stainless-Package-Version", s.cfg.SDKVersion),
		option.WithHeader("X-Stainless-Runtime", "node"),
		option.WithHeader("X-Stainless-Runtime-Version", s.cfg.RuntimeVersion),
		option.WithHeader("X-Stainless-OS", s.cfg.StainlessOS),
		option.WithHeader("X-Stainless-Arch", s.cfg.StainlessArch),
		option.WithHeader("x-app", "cli"),
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
		turn := s.turnCounter.Add(1)
		ccVersion := fmt.Sprintf("%s.%d", s.cfg.CCVersion, turn)
		billingHeader := fmt.Sprintf(s.cfg.BillingHeader, ccVersion)
		systemBlocks = append(systemBlocks, ant.TextBlockParam{Text: billingHeader})
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
			if len(message.ToolCalls) > 0 {
				var blocks []ant.ContentBlockParamUnion
				if text != "" {
					blocks = append(blocks, ant.NewTextBlock(text))
				}
				for _, tc := range message.ToolCalls {
					var input json.RawMessage
					if tc.Function.Arguments != "" {
						input = json.RawMessage(tc.Function.Arguments)
					} else {
						input = json.RawMessage("{}")
					}
					blocks = append(blocks, ant.ContentBlockParamUnion{OfToolUse: &ant.ToolUseBlockParam{ID: tc.ID, Name: tc.Function.Name, Input: input}})
				}
				params.Messages = append(params.Messages, ant.MessageParam{Role: "assistant", Content: blocks})
			} else {
				params.Messages = append(params.Messages, ant.NewAssistantMessage(ant.NewTextBlock(text)))
			}
		case "tool":
			result := ant.ToolResultBlockParam{ToolUseID: message.ToolCallID}
			if text != "" {
				result.Content = []ant.ToolResultBlockParamContentUnion{{OfText: &ant.TextBlockParam{Text: text}}}
			}
			params.Messages = append(params.Messages, ant.MessageParam{Role: "user", Content: []ant.ContentBlockParamUnion{{OfToolResult: &result}}})
		default:
			return ant.MessageNewParams{}, core.NewError("UNSUPPORTED_ROLE", http.StatusBadRequest, fmt.Sprintf("unsupported message role %q", message.Role), nil)
		}
	}
	// Convert OpenAI tools to Anthropic tools; cache the last tool definition.
	if len(request.Tools) > 0 {
		for idx, tool := range request.Tools {
			if tool.Type != "function" || tool.Function.Name == "" {
				continue
			}
			tp := ant.ToolParam{Name: tool.Function.Name}
			if tool.Function.Description != "" {
				tp.Description = ant.String(tool.Function.Description)
			}
			if len(tool.Function.Parameters) > 0 {
				var schema ant.ToolInputSchemaParam
				if err := json.Unmarshal(tool.Function.Parameters, &schema); err == nil {
					tp.InputSchema = schema
				}
			}
			if idx == len(request.Tools)-1 {
				tp.CacheControl = ant.NewCacheControlEphemeralParam()
			}
			params.Tools = append(params.Tools, ant.ToolUnionParam{OfTool: &tp})
		}
	}
	if len(systemBlocks) > 0 {
		systemBlocks[len(systemBlocks)-1].CacheControl = ant.NewCacheControlEphemeralParam()
		params.System = systemBlocks
	}
	// Map OpenAI reasoning_effort to Anthropic extended thinking.
	if effort := strings.TrimSpace(strings.ToLower(request.ReasoningEffort)); effort != "" {
		var budgetTokens int64
		switch effort {
		case "low":
			budgetTokens = 1024
		case "medium":
			budgetTokens = 8192
		case "high":
			budgetTokens = 32768
		}
		if budgetTokens > 0 {
			if params.MaxTokens <= budgetTokens {
				params.MaxTokens = budgetTokens + 4096
			}
			params.Thinking = ant.ThinkingConfigParamOfEnabled(budgetTokens)
			// Extended thinking requires temperature to be unset (defaults to 1).
			params.Temperature = ant.Float(1)
		}
	}
	return params, nil
}

func mapResponse(model string, message *ant.Message, createdAt time.Time) openai.ChatCompletionResponse {
	content := ""
	finishReason := "stop"
	var toolCalls []openai.ToolCall
	var thinkingTokens int64
	if message != nil {
		for _, block := range message.Content {
			switch block.Type {
			case "text":
				content += block.Text
			case "thinking":
				// Estimate thinking tokens: ~4 chars per token is a rough heuristic.
				thinkingTokens += int64(len(block.Thinking)+3) / 4
			case "tool_use":
				args, _ := json.Marshal(block.Input)
				toolCalls = append(toolCalls, openai.ToolCall{ID: block.ID, Type: "function", Function: openai.FunctionCall{Name: block.Name, Arguments: string(args)}})
			}
		}
		if message.StopReason != "" {
			finishReason = string(message.StopReason)
			if finishReason == "tool_use" {
				finishReason = "tool_calls"
			}
		}
	}
	usage := openai.Usage{}
	if message != nil {
		totalInput := message.Usage.InputTokens + message.Usage.CacheCreationInputTokens + message.Usage.CacheReadInputTokens
		usage.PromptTokens = totalInput
		usage.CompletionTokens = message.Usage.OutputTokens
		usage.TotalTokens = totalInput + message.Usage.OutputTokens
		usage.CacheCreationInputTokens = message.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens = message.Usage.CacheReadInputTokens
		if message.Usage.CacheReadInputTokens > 0 || message.Usage.CacheCreationInputTokens > 0 {
			usage.PromptTokensDetails = &openai.PromptTokensDetails{CachedTokens: message.Usage.CacheReadInputTokens}
		}
		if thinkingTokens > 0 {
			usage.CompletionTokensDetails = &openai.CompletionTokensDetails{ReasoningTokens: thinkingTokens}
		}
	}
	msg := openai.ChatCompletionMessage{Role: "assistant", Content: openai.MessageContent{Text: content}, ToolCalls: toolCalls}
	return openai.ChatCompletionResponse{ID: fmt.Sprintf("chatcmpl-%d", createdAt.UnixNano()), Object: "chat.completion", Created: createdAt.Unix(), Model: model, Choices: []openai.ChatCompletionChoice{{Index: 0, Message: msg, FinishReason: finishReason}}, Usage: usage}
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
