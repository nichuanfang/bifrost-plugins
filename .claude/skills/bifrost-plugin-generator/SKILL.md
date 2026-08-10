---
name: bifrost-plugin-generator
description: Generate complete, buildable Bifrost Go plugin from natural language. Use when the user wants to create, scaffold, or build a Bifrost plugin, add hooks to the request/response pipeline, intercept LLM calls, modify responses, or configure Bifrost gateway extensions. Triggers on phrases like "create a Bifrost plugin", "write a plugin that intercepts requests", "add a plugin for PII masking / logging / caching / rate limiting", or any mention of Bifrost plugin development.
---

# Bifrost Plugin Generator

Generate a complete, buildable Bifrost Go plugin under `plugins/<plugin-name>/`.

## Workflow

### 1. Clarify requirements

Infer, or ask if unclear:

- **Purpose** — logging, caching, PII masking, rate limiting, header injection, content filtering, MCP governance, etc.
- **Hooks** — minimal set only. Most plugins need just `PreLLMHook` and/or `PostLLMHook`.
- **Config** — keys, toggles, lists → typed struct.
- **Name** — kebab-case (directory, `GetName()`, config `name`).
- Short-circuit needed? (Only via `PreLLMHook`/`PreMCPHook`; `PreRequestHook` does not support short-circuit.)

| Hook | When |
|------|------|
| `PreRequestHook` | Once per request. Routing (provider/model/fallbacks). No short-circuit. Errors non-blocking. |
| `PreLLMHook` | Per provider attempt. Modify request or short-circuit with synthetic response. |
| `PostLLMHook` | Per provider attempt. Transform response / handle errors. |
| `HTTPTransportPreHook` | Raw HTTP request before core (bifrost-http only). |
| `HTTPTransportPostHook` | Raw HTTP response after core, non-streaming (bifrost-http only). |
| `HTTPTransportStreamChunkHook` | Streaming chunks to client (bifrost-http only). |
| `PreMCPHook` / `PostMCPHook` | Per MCP envelope call (ping / list_tools / execute_tool). |
| `PreMCPConnectionHook` / `PostMCPConnectionHook` | Once per MCP client transport setup. |

Do **not** implement unused hooks.

### 2. Resolve versions from README.md

**Before generating any file**, read the workspace root `README.md` and parse `## 版本信息`:

```
## 版本信息
- **go**: <go-version>
- **bifrost-version**: <unused-by-plugins>
- **bifrost-tag**: <unused-by-plugins>
- **core-version**: <core-version>
```

| Field | Meaning | Used for |
|-------|---------|----------|
| `go` | Go toolchain / `go.mod` directive | `go <go>` in plugin `go.mod` |
| `core-version` | bifrost core module version | `require github.com/maximhq/bifrost/core <core-version>` |
| `bifrost-version` | Bifrost main binary version | Not used when generating plugins |
| `bifrost-tag` | GitHub tag for Bifrost | Not used when generating plugins |

Rules:

1. Always read `README.md` first. Use the exact `go` and `core-version` values found there.
2. If `README.md` or the `## 版本信息` block is missing, fall back to an existing `plugins/*/go.mod` (match its `go` directive and `bifrost/core` require).
3. If neither exists, **stop and ask the user** for `go` and `core-version`. Do not invent versions.
4. Never embed literal version numbers in this skill's instructions as defaults for generation.

Module path: match existing workspace plugins, else `github.com/nichuanfang/bifrost-plugins/<plugin-name>`.

### 3. Generate files

Create `plugins/<plugin-name>/` with `main.go`, `go.mod`, `config.example.json`.

#### `main.go`

Export at least:

```go
func GetName() string
func Init(config any) error
func Cleanup() error
```

Plus only needed hooks. Rules:

- Types from `github.com/maximhq/bifrost/core/schemas`
- Log with `ctx.Log` (`LogLevelDebug` / `Info` / `Warn` / `Error`) — not `fmt.Println`
- Config: defensive parse of `map[string]interface{}` with defaults; prefer `sync/atomic.Pointer` for reload safety (see `references/config-pattern.md`)
- Pass-through: `(req, nil, nil)` / `(resp, bifrostErr, nil)`
- Short-circuit: `&schemas.LLMPluginShortCircuit{Response: ...}` from `PreLLMHook`
- Nil-guard nested fields (`req.ChatRequest`, `resp.ChatResponse`, …)
- HTTP transport hooks run only under bifrost-http
- `Provider`/`Model`/`Fallbacks` mutations only stick in `PreRequestHook`

Signatures, skeleton, MCP short-circuit types → `references/hooks-and-skeleton.md`.

#### `go.mod`

Substitute values resolved in step 2:

```go
module github.com/nichuanfang/bifrost-plugins/<plugin-name>

go <go>

require github.com/maximhq/bifrost/core <core-version>
```

Do not copy version numbers from memory — only from README (or fallback rules above).

#### `config.example.json`

```json
{
  "plugins": [
    {
      "enabled": true,
      "name": "<plugin-name>",
      "path": "./build/<plugin-name>.so",
      "version": 1,
      "config": {}
    }
  ]
}
```

Document every config key the plugin reads.

### 4. Summarize

- Plugin dir
- Versions used (`go` and `core-version` from README)
- Config wiring (from `config.example.json`)
- Hooks implemented and why
- Notable design choices

## Principles

- Real working code, not TODOs (pass-through is fine when intentional)
- Minimal hook surface
- Defensive config + nil checks; `ctx.Log` only
- Versions only from README (or existing go.mod / user); never invent
- Official examples for API patterns over any local/outdated docs