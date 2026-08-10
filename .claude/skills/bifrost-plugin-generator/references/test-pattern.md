# Test Pattern

Generate tests in `package main` covering every exported function. Use table-driven tests. Zero external dependencies — everything is in-process.

## Required tests (every plugin)

**`TestGetName`** — assert non-empty string.

**`TestInit`** — table-driven over these cases:

| Input | Expected |
|-------|----------|
| `nil` | no error, defaults applied |
| `"not a map"` | no error, defaults applied (graceful) |
| `map[string]interface{}{}` | no error, defaults applied |
| `map[string]interface{}{"enabled": false}` | no error, `Enabled == false` |
| Full valid config | no error, all fields match |
| Wrong type per field (e.g. `"enabled": "yes"`) | no error, that field keeps default |

Pattern:

```go
func TestInit(t *testing.T) {
    tests := []struct {
        name    string
        input   any
        wantCfg *Config // expected state after Init
    }{
        {name: "nil config", input: nil, wantCfg: defaultConfig()},
        {name: "empty map", input: map[string]interface{}{}, wantCfg: defaultConfig()},
        // ... per-field cases
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if err := Init(tt.input); err != nil {
                t.Fatalf("Init() error = %v", err)
            }
            got := cfg.Load()
            if diff := cmp.Diff(tt.wantCfg, got); diff != "" {
                t.Errorf("config mismatch (-want +got):\n%s", diff)
            }
        })
    }
}
```

If the plugin has **no config fields** beyond a toggle, keep it minimal: just `nil`, empty map, and `enabled: false`.

**`TestCleanup`** — assert `Cleanup() == nil`.

**`TestInit_Reload`** — verify that calling `Init` a second time overwrites config:

```go
func TestInit_Reload(t *testing.T) {
    Init(map[string]interface{}{"enabled": false})
    Init(map[string]interface{}{"enabled": true})
    if !cfg.Load().Enabled {
        t.Error("expected Enabled=true after reload")
    }
}
```

## Hook tests (one per implemented hook)

Only generate tests for hooks the plugin actually exports. Each hook gets its own `Test<HookName>` function.

**Nil-safety** — every hook test MUST include a `"nil request"` sub-case:

```go
{name: "nil request", req: nil, wantErr: false}
```

For `PostLLMHook` / `PostMCPHook`, also test `"nil response"` and `"nil error"`.

**Pass-through** — with the hook effectively disabled (`Enabled: false` or minimal config), assert the hook returns its inputs unchanged.

**Business logic** — one sub-case per distinct behavior the hook implements. Examples by hook type:

| Hook | Typical test cases (beyond nil/pass-through) |
|------|---------------------------------------------|
| `PreLLMHook` (inject) | `req.ChatRequest.Input` has injected message at front; `GetName()` not included in injected messages |
| `PreLLMHook` (block) | deny-listed model returns `(nil, &LLMPluginShortCircuit{Response: ...}, nil)` |
| `PreLLMHook` (short-circuit) | short-circuit response has expected content, `resp.Object == "chat.completion"` |
| `PostLLMHook` (log/mask) | mutated response field; original untouched when disabled |
| `PreRequestHook` (route) | `req.Provider` / `req.Model` mutated as expected |
| HTTP transport hooks | `req.Headers` set/modified; `nil` chunk handled; short-circuit returns `(*HTTPResponse, nil)` |

**Short-circuit assertion pattern:**

```go
_, sc, err := PreLLMHook(ctx, req)
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
if sc == nil {
    t.Fatal("expected short-circuit, got nil")
}
if sc.Response == nil {
    t.Fatal("short-circuit response is nil")
}
```

## Test helpers

Include these in `main_test.go` when useful:

```go
func newBifrostContext() *schemas.BifrostContext {
    return &schemas.BifrostContext{}
}

func newChatRequest(model string, messages ...schemas.ChatMessage) *schemas.BifrostRequest {
    return &schemas.BifrostRequest{
        ChatRequest: &schemas.ChatRequest{
            Model: model,
            Input: messages,
        },
    }
}

func newChatResponse(id, model string) *schemas.BifrostResponse {
    return &schemas.BifrostResponse{
        ChatResponse: &schemas.ChatResponse{
            ID:    id,
            Model: model,
        },
    }
}
```

Only generate helpers actually used by the tests.

## Rules

1. Tests live in `package main` — same package, no import cycle.
2. Tests must pass with `go test ./plugins/<name>/` — no external test deps beyond `github.com/google/go-cmp/cmp` (already a transitive dep of bifrost/core).
3. Do not test bifrost framework internals (hook dispatch order, context propagation) — only test the plugin's own logic.
4. Each table sub-test uses `t.Run` for clean failure attribution.
5. Config-dependent tests must call `Init` with the desired config inside the sub-test (or in a `t.Cleanup` reset).
6. Do not write tests for hooks the plugin does not implement.
7. If the plugin is stateless and has no hooks, only `TestGetName`, `TestInit`, and `TestCleanup` are needed.
