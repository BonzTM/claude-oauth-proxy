# Prompt Caching

The proxy automatically applies Anthropic's prompt caching to every request. No client-side configuration is needed — caching is handled transparently between the proxy and the Anthropic API.

Prompt caching reduces latency and cost by reusing previously processed prompt prefixes. When a cache breakpoint is hit, Anthropic skips re-processing everything before that point.

## How It Works

The proxy sets `CacheControl: ephemeral` breakpoints on three types of content:

### System messages

All system messages (including the internal billing header) are collected into a single system block array. The last block in the array gets a cache breakpoint. This means your system prompt is cached across conversation turns.

### User messages

The proxy finds the last two user messages in the conversation and marks both with cache breakpoints:

- **Second-to-last user message**: caches the conversation history up to that point
- **Last user message**: caches the current turn

If there is only one user message, that message gets the breakpoint.

### Tool definitions

When tools are provided, the last tool definition in the list gets a cache breakpoint. This caches all tool definitions together, which is useful for tool-heavy workloads where the same tools are sent with every request.

## Cache Metrics in Responses

The proxy passes through Anthropic's cache metrics in the `usage` field of both streaming and non-streaming responses:

```json
{
  "usage": {
    "prompt_tokens": 1200,
    "completion_tokens": 45,
    "total_tokens": 1245,
    "cache_creation_input_tokens": 1100,
    "cache_read_input_tokens": 0
  }
}
```

- `cache_creation_input_tokens`: tokens written to the cache on this request (first time a prefix is seen)
- `cache_read_input_tokens`: tokens read from cache (subsequent requests with the same prefix)

On a cache hit, you will see `cache_read_input_tokens > 0` and `cache_creation_input_tokens = 0`. Cached input tokens are billed at a reduced rate.

## Minimum Token Thresholds

Anthropic only activates caching when the cached prefix exceeds a model-specific minimum:

| Model | Minimum cached prefix |
|-------|----------------------|
| Claude Sonnet 4.6 | 2,048 tokens |
| Claude Opus 4.6 | 4,096 tokens |
| Claude Haiku 4.5 | 4,096 tokens |

If the prefix is smaller, the request still succeeds but nothing is cached. In practice, any real conversation with a system prompt and tools will exceed this threshold after the first couple of messages.

## What Gets Cached in Practice

For a typical multi-turn conversation with tools:

| Turn | What is cached |
|------|---------------|
| First request | System prompt + tools + first user message are written to cache |
| Second request | System prompt + tools + first user message are read from cache; second user message is written |
| Nth request | Everything up to the (N-1)th user message is read from cache |

The cache has a 5-minute TTL on Anthropic's side. As long as requests arrive within that window, cached prefixes are reused.
