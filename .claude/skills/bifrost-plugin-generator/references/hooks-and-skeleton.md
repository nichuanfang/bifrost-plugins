# Hook Signatures and Minimal Skeleton

Based on official docs (writing-go-plugin.mdx v1.5.x+/v1.6.x+) and examples (hello-world, llm-only, mcp-only).

## Required exports

```go
func GetName() string
func Init(config any) error
func Cleanup() error
```

## LLM / routing hooks

```go
// Once per top-level request. Routing only.
// Mutations to Provider/Model/Fallbacks commit across all attempts.
// Errors are non-blocking (logged, pipeline continues). No short-circuit.
func PreRequestHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) error

// Before each provider attempt. Can modify request or short-circuit.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error)

// After each provider attempt (or short-circuit).
func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error)
```

### Short-circuit (PreLLMHook)

```go
return req, &schemas.LLMPluginShortCircuit{
    Response: cachedOrSyntheticResponse, // *schemas.BifrostResponse
}, nil
```

Pass-through:

```go
return req, nil, nil
```

## HTTP transport hooks (bifrost-http only)

```go
// Before request enters Bifrost core. Modify req in-place.
// Return (*HTTPResponse, nil) to short-circuit with a response.
// Return (nil, error) to short-circuit with error.
// Return (nil, nil) to continue.
func HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error)

// After response exits Bifrost core (NON-streaming only).
// Modify resp in-place. Return error to short-circuit remaining post-hooks.
func HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error

// Streaming chunks before they reach the client.
// Return (chunk, nil) pass-through, (nil, nil) skip chunk, (modified, nil) replace, (nil, err) abort stream.
func HTTPTransportStreamChunkHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, chunk *schemas.BifrostStreamChunk) (*schemas.BifrostStreamChunk, error)
```

Header / query helpers:

```go
contentType := req.CaseInsensitiveHeaderLookup("Content-Type")
apiKey := req.CaseInsensitiveQueryLookup("api_key")
req.Headers["X-Custom"] = "value" // set via direct map access
```

## MCP hooks (optional, v1.5.x+)

```go
// Per envelope call (ping / list_tools / execute_tool)
func PreMCPHook(ctx *schemas.BifrostContext, req *schemas.BifrostMCPRequest) (*schemas.BifrostMCPRequest, *schemas.MCPPluginShortCircuit, error)
func PostMCPHook(ctx *schemas.BifrostContext, resp *schemas.BifrostMCPResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostMCPResponse, *schemas.BifrostError, error)

// Once per MCP client when transport is established
func PreMCPConnectionHook(ctx *schemas.BifrostContext, req *schemas.BifrostMCPConnectRequest) (*schemas.BifrostMCPConnectRequest, *schemas.MCPConnectionShortCircuit, error)
func PostMCPConnectionHook(ctx *schemas.BifrostContext, resp *schemas.BifrostMCPConnectResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostMCPConnectResponse, *schemas.BifrostError, error)
```

Connect short-circuit uses `*MCPConnectionShortCircuit` (Error or Response). Envelope short-circuit uses `*MCPPluginShortCircuit`. They are not interchangeable.

## Execution order (v1.4.x+ / v1.6.x+)

1. `HTTPTransportPreHook` (HTTP transport only, registration order)
2. `PreRequestHook` (once per request, routing)
3. `PreLLMHook` / `PreMCPHook` (per attempt, registration order; can short-circuit)
4. Provider / MCP call
5. `PostLLMHook` / `PostMCPHook` (per attempt, reverse order)
6. `HTTPTransportPostHook` or `HTTPTransportStreamChunkHook` (HTTP transport only, reverse order)

## Minimal skeleton (LLM logging + optional system-message inject)

```go
package main

import (
	"fmt"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

type Config struct {
	EnableLogging       bool
	InjectSystemMessage bool
	SystemMessageText   string
}

var cfg atomic.Pointer[Config]

func GetName() string { return "example-logger" }

func Init(config any) error {
	c := &Config{
		EnableLogging:       true,
		InjectSystemMessage: false,
		SystemMessageText:   "You are a helpful assistant.",
	}
	if m, ok := config.(map[string]interface{}); ok {
		if v, ok := m["enable_logging"].(bool); ok {
			c.EnableLogging = v
		}
		if v, ok := m["inject_system_message"].(bool); ok {
			c.InjectSystemMessage = v
		}
		if v, ok := m["system_message_text"].(string); ok && v != "" {
			c.SystemMessageText = v
		}
	}
	cfg.Store(c)
	return nil
}

func Cleanup() error { return nil }

func PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	c := cfg.Load()
	if c == nil {
		return req, nil, nil
	}
	if c.EnableLogging {
		ctx.Log(schemas.LogLevelInfo, "PreLLMHook")
	}
	if c.InjectSystemMessage && req != nil && req.ChatRequest != nil && req.ChatRequest.Input != nil {
		msg := schemas.ChatMessage{
			Role:    "system",
			Content: &schemas.ChatMessageContent{ContentStr: &c.SystemMessageText},
		}
		req.ChatRequest.Input = append([]schemas.ChatMessage{msg}, req.ChatRequest.Input...)
	}
	return req, nil, nil
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	c := cfg.Load()
	if c == nil {
		return resp, bifrostErr, nil
	}
	if bifrostErr != nil {
		ctx.Log(schemas.LogLevelError, fmt.Sprintf("LLM error: %v", bifrostErr))
		return resp, bifrostErr, nil
	}
	if c.EnableLogging && resp != nil && resp.ChatResponse != nil {
		ctx.Log(schemas.LogLevelInfo, fmt.Sprintf("response id=%s model=%s", resp.ChatResponse.ID, resp.ChatResponse.Model))
	}
	return resp, bifrostErr, nil
}
```

## Nil-safety checklist

```go
if req == nil || req.ChatRequest == nil {
	return req, nil, nil
}
if resp == nil || resp.ChatResponse == nil {
	return resp, bifrostErr, nil
}
```

## Context helpers (useful in hooks)

```go
ctx.SetValue(schemas.BifrostContextKey("my-key"), value)
val := ctx.Value(schemas.BifrostContextKey("my-key"))

// v1.6.7+
info := ctx.GetModelInfo(provider, model) // nil if unknown / no catalog
cost := ctx.CalculateCost(resp)           // compute synchronously inside the hook
```