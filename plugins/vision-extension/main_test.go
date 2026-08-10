package main

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/maximhq/bifrost/core/schemas"
)

func TestGetName(t *testing.T) {
	if name := GetName(); name == "" {
		t.Error("GetName() returned empty string")
	}
}

func TestInit(t *testing.T) {
	resetCfg := func() { cfg.Store(defaultConfig()) }

	tests := []struct {
		name    string
		input   any
		wantCfg *Config
	}{
		{
			name:    "nil config",
			input:   nil,
			wantCfg: defaultConfig(),
		},
		{
			name:    "string instead of map",
			input:   "not a map",
			wantCfg: defaultConfig(),
		},
		{
			name:    "empty map",
			input:   map[string]interface{}{},
			wantCfg: defaultConfig(),
		},
		{
			name:  "enabled false",
			input: map[string]interface{}{"enabled": false},
			wantCfg: func() *Config {
				c := defaultConfig()
				c.Enabled = false
				return c
			}(),
		},
		{
			name:  "custom placeholder",
			input: map[string]interface{}{"image_placeholder": "[IMG: {url}]"},
			wantCfg: func() *Config {
				c := defaultConfig()
				c.ImagePlaceholder = "[IMG: {url}]"
				return c
			}(),
		},
		{
			name:  "target models",
			input: map[string]interface{}{"target_models": []interface{}{"gpt-4", "gpt-3.5"}},
			wantCfg: func() *Config {
				c := defaultConfig()
				c.TargetModels = []string{"gpt-4", "gpt-3.5"}
				return c
			}(),
		},
		{
			name:    "full config",
			input:   map[string]interface{}{"enabled": false, "image_placeholder": "no img", "target_models": []interface{}{"m1"}},
			wantCfg: &Config{Enabled: false, ImagePlaceholder: "no img", TargetModels: []string{"m1"}},
		},
		{
			name:    "wrong type for enabled keeps default",
			input:   map[string]interface{}{"enabled": "yes"},
			wantCfg: defaultConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetCfg)
			if err := Init(tt.input); err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			got := cfg.Load()
			if diff := cmp.Diff(tt.wantCfg, got, cmpopts.IgnoreUnexported(Config{})); diff != "" {
				t.Errorf("config mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCleanup(t *testing.T) {
	if err := Cleanup(); err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}
}

func TestInit_Reload(t *testing.T) {
	resetCfg := func() { cfg.Store(defaultConfig()) }
	t.Cleanup(resetCfg)

	Init(map[string]interface{}{"enabled": false})
	if cfg.Load().Enabled {
		t.Error("expected Enabled=false after first Init")
	}
	Init(map[string]interface{}{"enabled": true})
	if !cfg.Load().Enabled {
		t.Error("expected Enabled=true after reload")
	}
}

func TestPreLLMHook(t *testing.T) {
	newCtx := func() *schemas.BifrostContext { return &schemas.BifrostContext{} }

	tests := []struct {
		name    string
		cfg     *Config
		req     *schemas.BifrostRequest
		wantErr bool
		check   func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit)
	}{
		{
			name:    "nil request",
			cfg:     defaultConfig(),
			req:     nil,
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				if req != nil {
					t.Error("expected nil request returned")
				}
			},
		},
		{
			name:    "nil ChatRequest",
			cfg:     defaultConfig(),
			req:     &schemas.BifrostRequest{},
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				if req.ChatRequest != nil {
					t.Error("expected nil ChatRequest")
				}
			},
		},
		{
			name:    "disabled plugin",
			cfg:     &Config{Enabled: false},
			req:     newChatRequest("gpt-4", textMsg("hello")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				content := req.ChatRequest.Input[0].Content
				if content.ContentStr == nil || *content.ContentStr != "hello" {
					t.Error("request should pass through unchanged when disabled")
				}
			},
		},
		{
			name:    "pass-through: text-only message",
			cfg:     defaultConfig(),
			req:     newChatRequest("gpt-4", textMsg("hello world")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				content := req.ChatRequest.Input[0].Content
				if content.ContentStr == nil || *content.ContentStr != "hello world" {
					t.Error("text-only message should pass through unchanged")
				}
			},
		},
		{
			name: "pass-through: ContentBlocks without images",
			cfg:  defaultConfig(),
			req: newChatRequest("gpt-4", schemas.ChatMessage{
				Role: "user",
				Content: &schemas.ChatMessageContent{
					ContentBlocks: []schemas.ChatContentBlock{
						{Type: schemas.ChatContentBlockTypeText, Text: strPtr("just text")},
					},
				},
			}),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if len(blocks) != 1 || *blocks[0].Text != "just text" {
					t.Error("non-image ContentBlocks should pass through unchanged")
				}
			},
		},
		{
			name: "not in target models",
			cfg: &Config{
				Enabled:      true,
				TargetModels: []string{"claude-3-opus"},
			},
			req:     newChatRequest("gpt-4", imageMsg("http://example.com/img.png", "high")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if blocks[0].Type != schemas.ChatContentBlockTypeImage {
					t.Error("image block should not be converted when model is not targeted")
				}
			},
		},
		{
			name:    "convert single image to text",
			cfg:     defaultConfig(),
			req:     newChatRequest("gpt-3.5-turbo", imageMsg("http://example.com/img.png", "auto")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if len(blocks) != 1 {
					t.Fatalf("expected 1 block, got %d", len(blocks))
				}
				if blocks[0].Type != schemas.ChatContentBlockTypeText {
					t.Errorf("expected text block, got %s", blocks[0].Type)
				}
				if blocks[0].Text == nil || !strings.Contains(*blocks[0].Text, "http://example.com/img.png") {
					t.Errorf("expected placeholder containing image URL, got %v", blocks[0].Text)
				}
			},
		},
		{
			name: "convert image mixed with text blocks",
			cfg:  defaultConfig(),
			req: newChatRequest("gpt-3.5-turbo", schemas.ChatMessage{
				Role: "user",
				Content: &schemas.ChatMessageContent{
					ContentBlocks: []schemas.ChatContentBlock{
						{Type: schemas.ChatContentBlockTypeText, Text: strPtr("describe this:")},
						{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: &schemas.ChatInputImage{URL: "http://example.com/img.png", Detail: strPtr("low")}},
					},
				},
			}),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if len(blocks) != 2 {
					t.Fatalf("expected 2 blocks, got %d", len(blocks))
				}
				if blocks[0].Type != schemas.ChatContentBlockTypeText || *blocks[0].Text != "describe this:" {
					t.Error("text block should be preserved")
				}
				if blocks[1].Type != schemas.ChatContentBlockTypeText {
					t.Error("image block should be converted to text")
				}
				if blocks[1].Text == nil || !strings.Contains(*blocks[1].Text, "low") {
					t.Error("placeholder should contain detail value")
				}
			},
		},
		{
			name:    "empty target_models applies to all",
			cfg:     defaultConfig(), // TargetModels empty
			req:     newChatRequest("any-model", imageMsg("http://example.com/img.png", "")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if blocks[0].Type != schemas.ChatContentBlockTypeText {
					t.Error("image should be converted when target_models is empty")
				}
			},
		},
		{
			name:    "nil config fallback",
			req:     newChatRequest("gpt-4", imageMsg("http://example.com/img.png", "")),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				// nil config — should pass through
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if blocks[0].Type != schemas.ChatContentBlockTypeImage {
					t.Error("should pass through when cfg is nil")
				}
			},
		},
		{
			name: "ContentBlocks with nil ImageURLStruct skipped",
			cfg:  defaultConfig(),
			req: newChatRequest("gpt-4", schemas.ChatMessage{
				Role: "user",
				Content: &schemas.ChatMessageContent{
					ContentBlocks: []schemas.ChatContentBlock{
						{Type: schemas.ChatContentBlockTypeImage, ImageURLStruct: nil},
						{Type: schemas.ChatContentBlockTypeText, Text: strPtr("text")},
					},
				},
			}),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest, sc *schemas.LLMPluginShortCircuit) {
				blocks := req.ChatRequest.Input[0].Content.ContentBlocks
				if len(blocks) != 2 {
					t.Fatalf("expected 2 blocks, got %d", len(blocks))
				}
				// image block with nil ImageURLStruct is kept as-is
				if blocks[0].Type != schemas.ChatContentBlockTypeImage {
					t.Error("image block with nil ImageURLStruct should not be converted")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg != nil {
				cfg.Store(tt.cfg)
			} else {
				cfg.Store(nil)
			}
			t.Cleanup(func() { cfg.Store(defaultConfig()) })

			req, sc, err := PreLLMHook(newCtx(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PreLLMHook() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, req, sc)
			}
		})
	}
}

func TestIsTargetModel(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		targets []string
		want    bool
	}{
		{name: "empty targets", model: "gpt-4", targets: nil, want: false},
		{name: "exact match", model: "gpt-4", targets: []string{"gpt-4"}, want: true},
		{name: "no match", model: "gpt-3.5", targets: []string{"gpt-4", "claude-3"}, want: false},
		{name: "multi match", model: "claude-3", targets: []string{"gpt-4", "claude-3"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTargetModel(tt.model, tt.targets); got != tt.want {
				t.Errorf("isTargetModel(%q, %v) = %v, want %v", tt.model, tt.targets, got, tt.want)
			}
		})
	}
}

func TestFormatPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		template string
		url      string
		detail   string
		want     string
	}{
		{
			name:     "replaces url and detail",
			template: "[IMG: {url} ({detail})]",
			url:      "http://example.com/a.png",
			detail:   "high",
			want:     "[IMG: http://example.com/a.png (high)]",
		},
		{
			name:     "empty detail",
			template: "[IMG: {url}]",
			url:      "http://example.com/a.png",
			detail:   "",
			want:     "[IMG: http://example.com/a.png]",
		},
		{
			name:     "no replacements when no placeholders",
			template: "static text",
			url:      "http://example.com/a.png",
			detail:   "high",
			want:     "static text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPlaceholder(tt.template, tt.url, tt.detail); got != tt.want {
				t.Errorf("formatPlaceholder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newChatRequest(model string, messages ...schemas.ChatMessage) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Model: model,
			Input: messages,
		},
	}
}

func textMsg(text string) schemas.ChatMessage {
	return schemas.ChatMessage{
		Role: "user",
		Content: &schemas.ChatMessageContent{
			ContentStr: &text,
		},
	}
}

func imageMsg(url, detail string) schemas.ChatMessage {
	var d *string
	if detail != "" {
		d = &detail
	}
	return schemas.ChatMessage{
		Role: "user",
		Content: &schemas.ChatMessageContent{
			ContentBlocks: []schemas.ChatContentBlock{
				{
					Type:           schemas.ChatContentBlockTypeImage,
					ImageURLStruct: &schemas.ChatInputImage{URL: url, Detail: d},
				},
			},
		},
	}
}

func strPtr(s string) *string { return &s }
