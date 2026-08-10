# Config Parsing Pattern

Official examples parse `config any` as `map[string]interface{}` inside `Init`. Bifrost can reload plugins (`ReloadPlugin`), so prefer `sync/atomic.Pointer` for concurrent-safe reads from hooks.

## Canonical pattern

```go
package main

import (
	"fmt"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

type Config struct {
	Enabled   bool
	MaxTokens int
	DenyList  []string
	APIKey    string
}

var cfg atomic.Pointer[Config]

func Init(config any) error {
	c := defaultConfig()
	if err := parseConfig(config, c); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	cfg.Store(c)
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Enabled:   true,
		MaxTokens: 4096,
	}
}

func parseConfig(raw any, c *Config) error {
	m, ok := raw.(map[string]interface{})
	if !ok || m == nil {
		return nil // keep defaults
	}

	if v, ok := m["enabled"].(bool); ok {
		c.Enabled = v
	}
	// JSON numbers arrive as float64
	if v, ok := m["max_tokens"].(float64); ok {
		c.MaxTokens = int(v)
	}
	if v, ok := m["api_key"].(string); ok {
		c.APIKey = v
	}
	if arr, ok := m["deny_list"].([]interface{}); ok {
		c.DenyList = make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				c.DenyList = append(c.DenyList, s)
			}
		}
	}
	return nil
}

// In hooks — always Load() a snapshot; never mutate it.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	c := cfg.Load()
	if c == nil || !c.Enabled {
		return req, nil, nil
	}
	// use c.MaxTokens, c.DenyList, ...
	return req, nil, nil
}

func Cleanup() error { return nil }
```

## Rules

1. Never store config in a plain package-level variable if reload is possible.
2. `Init` may be called again on reload — always `Store` a complete new value.
3. Hooks only `Load()`; they must not mutate the loaded struct.
4. Provide sensible defaults so missing/partial config still works.
5. JSON numbers are `float64` when coming from `map[string]interface{}`.
6. Always use `atomic.Pointer` even for simple plugins — `Init` may be called again on reload.