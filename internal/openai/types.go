package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type MessageContent struct {
	Text string
}

func (c *MessageContent) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		c.Text = ""
		return nil
	}
	var stringValue string
	if err := json.Unmarshal(trimmed, &stringValue); err == nil {
		c.Text = stringValue
		return nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return fmt.Errorf("decode message content: %w", err)
	}
	var builder strings.Builder
	for _, part := range parts {
		if part.Type != "" && part.Type != "text" {
			return fmt.Errorf("unsupported message content type %q", part.Type)
		}
		builder.WriteString(part.Text)
	}
	c.Text = builder.String()
	return nil
}

func (c MessageContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Text)
}

type ChatCompletionMessage struct {
	Role       string         `json:"role"`
	Content    MessageContent `json:"content"`
	Name       string         `json:"name,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ChatCompletionRequest struct {
	Model           string                  `json:"model"`
	Messages        []ChatCompletionMessage `json:"messages"`
	MaxTokens       int64                   `json:"max_tokens,omitempty"`
	Temperature     *float64                `json:"temperature,omitempty"`
	TopP            *float64                `json:"top_p,omitempty"`
	Stop            []string                `json:"stop,omitempty"`
	Stream          bool                    `json:"stream,omitempty"`
	User            string                  `json:"user,omitempty"`
	Tools           []Tool                  `json:"tools,omitempty"`
	ReasoningEffort string                  `json:"reasoning_effort,omitempty"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	Name    string `json:"name,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   Usage                  `json:"usage,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason,omitempty"`
	Delta        *ChatCompletionDelta  `json:"delta,omitempty"`
}

type ChatCompletionDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *Usage                      `json:"usage,omitempty"`
}

type ChatCompletionChunkChoice struct {
	Index        int                 `json:"index"`
	Delta        ChatCompletionDelta `json:"delta"`
	FinishReason *string             `json:"finish_reason,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

type Usage struct {
	PromptTokens             int64                    `json:"prompt_tokens,omitempty"`
	CompletionTokens         int64                    `json:"completion_tokens,omitempty"`
	TotalTokens              int64                    `json:"total_tokens,omitempty"`
	PromptTokensDetails      *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails  *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
	CacheCreationInputTokens int64                    `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64                    `json:"cache_read_input_tokens,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}
