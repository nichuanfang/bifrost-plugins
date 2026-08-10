package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/maximhq/bifrost/core/schemas"
)

var maskConfigCmpOpts = cmpopts.IgnoreUnexported(PluginConfig{})

func TestGetName(t *testing.T) {
	if name := GetName(); name == "" {
		t.Error("GetName() returned empty string")
	}
}

func TestCleanup(t *testing.T) {
	if err := Cleanup(); err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}
}

// ── Init tests ───────────────────────────────────────────────────────

func TestInit(t *testing.T) {
	resetCfg := func() { globalConfig.Store(defaultConfig()) }

	tests := []struct {
		name    string
		input   any
		wantCfg *PluginConfig
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
			input:   map[string]any{},
			wantCfg: defaultConfig(),
		},
		{
			name:  "enable false",
			input: map[string]any{"enable": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.Enable = false
				return c
			}(),
		},
		{
			name:  "enable string false",
			input: map[string]any{"enable": "false"},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.Enable = false
				return c
			}(),
		},
		{
			name:    "enable string true",
			input:   map[string]any{"enable": "true"},
			wantCfg: defaultConfig(),
		},
		{
			name:  "mask_phone false",
			input: map[string]any{"mask_phone": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskPhone = false
				return c
			}(),
		},
		{
			name:  "mask_email false",
			input: map[string]any{"mask_email": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskEmail = false
				return c
			}(),
		},
		{
			name:  "mask_id_card false",
			input: map[string]any{"mask_id_card": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskIDCard = false
				return c
			}(),
		},
		{
			name:  "mask_bank_card false",
			input: map[string]any{"mask_bank_card": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskBankCard = false
				return c
			}(),
		},
		{
			name:  "mask_ip false",
			input: map[string]any{"mask_ip": false},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskIP = false
				return c
			}(),
		},
		{
			name:  "log_desensitized true",
			input: map[string]any{"log_desensitized": true},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.LogMasked = true
				return c
			}(),
		},
		{
			name:  "custom_keywords",
			input: map[string]any{"custom_keywords": []any{"secret", "classified"}},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.CustomKeywords = []string{"secret", "classified"}
				c.initKeywordReplacer()
				return c
			}(),
		},
		{
			name:  "custom_regex",
			input: map[string]any{"custom_regex": []any{`token_\w+`}},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.CustomRegex = []string{`token_\w+`}
				return c
			}(),
		},
		{
			name: "desensitization_rules",
			input: map[string]any{
				"desensitization_rules": []any{
					map[string]any{"provider": "openai", "model": "gpt-4"},
				},
			},
			wantCfg: func() *PluginConfig {
				c := defaultConfig()
				c.MaskingRules = []MaskingRule{
					{Provider: "openai", Model: "gpt-4"},
				}
				return c
			}(),
		},
		{
			name:    "wrong type for enable keeps default",
			input:   map[string]any{"enable": 123},
			wantCfg: defaultConfig(),
		},
		{
			name: "full config",
			input: map[string]any{
				"enable":           false,
				"mask_phone":       false,
				"mask_email":       false,
				"mask_id_card":     false,
				"mask_bank_card":   false,
				"mask_ip":          false,
				"log_desensitized": true,
				"custom_keywords":  []any{"top-secret"},
				"desensitization_rules": []any{
					map[string]any{"provider": "openai", "model": ""},
				},
			},
			wantCfg: &PluginConfig{
				Enable:         false,
				MaskPhone:      false,
				MaskEmail:      false,
				MaskIDCard:     false,
				MaskBankCard:   false,
				MaskIP:         false,
				LogMasked:      true,
				CustomKeywords: []string{"top-secret"},
				MaskingRules: []MaskingRule{
					{Provider: "openai"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetCfg)
			if err := Init(tt.input); err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			got := globalConfig.Load()

			if len(tt.wantCfg.compiledCustomRegex) > 0 {
				if len(got.compiledCustomRegex) != len(tt.wantCfg.compiledCustomRegex) {
					t.Errorf("compiledCustomRegex count = %d, want %d",
						len(got.compiledCustomRegex), len(tt.wantCfg.compiledCustomRegex))
				}
			}

			if tt.wantCfg.keywordReplacer != nil {
				if got.keywordReplacer == nil {
					t.Error("keywordReplacer is nil, want non-nil")
				}
			}

			gotCopy := *got
			gotCopy.compiledCustomRegex = tt.wantCfg.compiledCustomRegex
			gotCopy.keywordReplacer = tt.wantCfg.keywordReplacer

			if diff := cmp.Diff(tt.wantCfg, &gotCopy, maskConfigCmpOpts); diff != "" {
				t.Errorf("config mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInit_Reload(t *testing.T) {
	resetCfg := func() { globalConfig.Store(defaultConfig()) }
	t.Cleanup(resetCfg)

	Init(map[string]any{"enable": false})
	if globalConfig.Load().Enable {
		t.Error("expected Enable=false after first Init")
	}
	Init(map[string]any{"enable": true})
	if !globalConfig.Load().Enable {
		t.Error("expected Enable=true after reload")
	}
}

// ── shouldMask tests ──────────────────────────────────────────────────

func TestShouldMask(t *testing.T) {
	cfgWithRules := func(rules ...MaskingRule) *PluginConfig {
		return &PluginConfig{MaskingRules: rules}
	}

	tests := []struct {
		name string
		cfg  *PluginConfig
		req  *schemas.BifrostRequest
		want bool
	}{
		{
			name: "empty rules returns false",
			cfg:  cfgWithRules(),
			req:  newChatReq("openai", "gpt-4"),
			want: false,
		},
		{
			name: "nil request returns false",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai"}),
			req:  nil,
			want: false,
		},
		{
			name: "nil ChatRequest returns false",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai"}),
			req:  &schemas.BifrostRequest{},
			want: false,
		},
		{
			name: "exact provider and model match",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai", Model: "gpt-4"}),
			req:  newChatReq("openai", "gpt-4"),
			want: true,
		},
		{
			name: "no match: different provider",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai"}),
			req:  newChatReq("anthropic", "claude-3"),
			want: false,
		},
		{
			name: "no match: different model",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai", Model: "gpt-4"}),
			req:  newChatReq("openai", "gpt-3.5"),
			want: false,
		},
		{
			name: "wildcard provider (empty string)",
			cfg:  cfgWithRules(MaskingRule{Provider: "", Model: "gpt-4"}),
			req:  newChatReq("openai", "gpt-4"),
			want: true,
		},
		{
			name: "wildcard model (empty string)",
			cfg:  cfgWithRules(MaskingRule{Provider: "openai", Model: ""}),
			req:  newChatReq("openai", "any-model"),
			want: true,
		},
		{
			name: "both wildcard matches all",
			cfg:  cfgWithRules(MaskingRule{}),
			req:  newChatReq("any", "any"),
			want: true,
		},
		{
			name: "case insensitive match",
			cfg:  cfgWithRules(MaskingRule{Provider: "OpenAI", Model: "GPT-4"}),
			req:  newChatReq("openai", "gpt-4"),
			want: true,
		},
		{
			name: "multi rule: second matches",
			cfg: cfgWithRules(
				MaskingRule{Provider: "anthropic"},
				MaskingRule{Provider: "openai"},
			),
			req:  newChatReq("openai", "gpt-4"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldMask(tt.cfg, tt.req); got != tt.want {
				t.Errorf("shouldMask() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── Masking function tests ───────────────────────────────────────────

func TestMaskPhoneFunc(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"11-digit phone", "13812345678", "138****5678"},
		{"correct length", "15900001111", "159****1111"},
		{"too short", "1381234", "1381234"},
		{"too long", "138123456789", "138123456789"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskPhoneFunc(tt.input); got != tt.want {
				t.Errorf("maskPhoneFunc(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskEmailFunc(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard email", "test@example.com", "tes****@example.com"},
		{"short local part", "a@b.com", "****@b.com"},
		{"three char local", "abc@test.com", "****@test.com"},
		{"no at sign", "noatsign", "noatsign"},
		{"at at start", "@domain.com", "@domain.com"},
		{"empty string", "", ""},
		{"long local part", "verylongname@domain.com", "ver****@domain.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskEmailFunc(tt.input); got != tt.want {
				t.Errorf("maskEmailFunc(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskIDCardFunc(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"18-digit ID", "110101199001011234", "110101********1234"},
		{"18-digit ID with X", "11010119900101123X", "110101********123X"},
		{"too short", "11010119900101", "11010119900101"},
		{"too long", "1101011990010112345", "1101011990010112345"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskIDCardFunc(tt.input); got != tt.want {
				t.Errorf("maskIDCardFunc(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskBankCardFunc(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"16-digit card", "6222021234567890", "6222********7890"},
		{"19-digit card", "6222021234567890123", "6222********0123"},
		{"13-digit card", "4000123456789", "4000********6789"},
		{"with spaces", "6222 0212 3456 7890", "6222 **** **** 7890"},
		{"with hyphens", "6222-0212-3456-7890", "6222-****-****-7890"},
		{"with mixed separators", "6222 0212-3456 7890", "6222 ****-**** 7890"},
		{"too short", "123456789012", "123456789012"},
		{"too short with spaces", "1234 5678 9012", "1234 5678 9012"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskBankCardFunc(tt.input); got != tt.want {
				t.Errorf("maskBankCardFunc(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripCardSeparators(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no separators", "6222021234567890", "6222021234567890"},
		{"with spaces", "6222 0212 3456 7890", "6222021234567890"},
		{"with hyphens", "6222-0212-3456-7890", "6222021234567890"},
		{"mixed", "6222 0212-3456 7890", "6222021234567890"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCardSeparators(tt.input); got != tt.want {
				t.Errorf("stripCardSeparators(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── maskSensitiveText tests ──────────────────────────────────────────

func TestMaskSensitiveText(t *testing.T) {
	fullCfg := defaultConfig()

	tests := []struct {
		name  string
		cfg   *PluginConfig
		input string
		want  string
	}{
		{
			name:  "empty string",
			cfg:   fullCfg,
			input: "",
			want:  "",
		},
		{
			name:  "no sensitive data",
			cfg:   fullCfg,
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "mask phone number",
			cfg:   fullCfg,
			input: "call me at 13812345678 please",
			want:  "call me at 138****5678 please",
		},
		{
			name:  "mask email",
			cfg:   fullCfg,
			input: "contact: user@example.com for help",
			want:  "contact: use****@example.com for help",
		},
		{
			name:  "mask ID card",
			cfg:   fullCfg,
			input: "my id is 110101199001011234",
			want:  "my id is 110101********1234",
		},
		{
			name:  "mask bank card",
			cfg:   fullCfg,
			input: "card: 6222021234567890 expired",
			want:  "card: 6222********7890 expired",
		},
		{
			name:  "mask IP",
			cfg:   fullCfg,
			input: "server 192.168.1.1 is down",
			want:  "server ***.***.***.*** is down",
		},
		{
			name:  "invalid IP not masked",
			cfg:   fullCfg,
			input: "address 999.999.999.999 is invalid",
			want:  "address 999.999.999.999 is invalid",
		},
		{
			name:  "mask multiple types",
			cfg:   fullCfg,
			input: "phone: 13812345678, email: test@example.com",
			want:  "phone: 138****5678, email: tes****@example.com",
		},
		{
			name:  "custom keywords",
			cfg:   withKeywords("top-secret-project"),
			input: "the top-secret-project is launched",
			want:  "the ****** is launched",
		},
		{
			name:  "custom keywords multiple",
			cfg:   withKeywords("alpha", "beta"),
			input: "alpha and beta are projects",
			want:  "****** and ****** are projects",
		},
		{
			name:  "all masking disabled",
			cfg:   &PluginConfig{Enable: true},
			input: "phone: 13812345678, email: test@example.com, id: 110101199001011234, card: 6222021234567890, ip: 192.168.1.1",
			want:  "phone: 13812345678, email: test@example.com, id: 110101199001011234, card: 6222021234567890, ip: 192.168.1.1",
		},
		{
			name:  "only phone enabled",
			cfg:   &PluginConfig{MaskPhone: true},
			input: "phone: 13812345678, email: test@example.com",
			want:  "phone: 138****5678, email: test@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskSensitiveText(tt.cfg, tt.input); got != tt.want {
				t.Errorf("maskSensitiveText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMaskSensitiveText_CustomRegex(t *testing.T) {
	cfg := defaultConfig()
	cfg.CustomRegex = []string{`token_\w+`, `secret_\w+`}
	cfg.compiledCustomRegex = make([]*regexp.Regexp, 0)
	for _, p := range cfg.CustomRegex {
		cfg.compiledCustomRegex = append(cfg.compiledCustomRegex, regexp.MustCompile(p))
	}

	input := "api key: token_abc123 and secret_xyz789 are sensitive"
	want := "api key: ****** and ****** are sensitive"
	if got := maskSensitiveText(cfg, input); got != want {
		t.Errorf("maskSensitiveText() = %q, want %q", got, want)
	}
}

func TestMaskSensitiveText_IPEdgeCases(t *testing.T) {
	cfg := defaultConfig()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid IP masked",
			input: "connect to 192.168.1.1 now",
			want:  "connect to ***.***.***.*** now",
		},
		{
			name:  "IP preceded by hostname is masked",
			input: "localhost 127.0.0.1 test",
			want:  "localhost ***.***.***.*** test",
		},
		{
			name:  "invalid octet >255 not masked",
			input: "address 999.999.999.999",
			want:  "address 999.999.999.999",
		},
		{
			name:  "valid IP at sentence start",
			input: "10.0.0.1 is the gateway",
			want:  "***.***.***.*** is the gateway",
		},
		{
			name:  "valid IP at sentence end",
			input: "the gateway is 10.0.0.1",
			want:  "the gateway is ***.***.***.***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskSensitiveText(cfg, tt.input); got != tt.want {
				t.Errorf("maskSensitiveText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── PreLLMHook tests ─────────────────────────────────────────────────

func TestPreLLMHook(t *testing.T) {
	newCtx := func() *schemas.BifrostContext { return &schemas.BifrostContext{} }

	resetCfg := func() { globalConfig.Store(defaultConfig()) }

	tests := []struct {
		name    string
		cfg     *PluginConfig
		req     *schemas.BifrostRequest
		wantErr bool
		check   func(t *testing.T, req *schemas.BifrostRequest)
	}{
		{
			name:    "nil request",
			cfg:     fullCfgWithRules(),
			req:     nil,
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				if req != nil {
					t.Error("expected nil request returned")
				}
			},
		},
		{
			name:    "nil ChatRequest",
			cfg:     fullCfgWithRules(),
			req:     &schemas.BifrostRequest{},
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				if req.ChatRequest != nil {
					t.Error("expected nil ChatRequest")
				}
			},
		},
		{
			name:    "disabled plugin passes through",
			cfg:     &PluginConfig{Enable: false},
			req:     newChatReqWithText("openai", "gpt-4", "phone: 13812345678"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				content := req.ChatRequest.Input[0].Content.ContentStr
				if content == nil || !strings.Contains(*content, "13812345678") {
					t.Error("text should pass through unchanged when disabled")
				}
			},
		},
		{
			name:    "pass-through: no masking rules",
			cfg:     fullCfgNoRules(),
			req:     newChatReqWithText("openai", "gpt-4", "phone: 13812345678"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				content := req.ChatRequest.Input[0].Content.ContentStr
				if content == nil || !strings.Contains(*content, "13812345678") {
					t.Error("text should pass through when no rules match")
				}
			},
		},
		{
			name:    "pass-through: model not in rules",
			cfg:     fullCfgWithRules(MaskingRule{Provider: "openai", Model: "gpt-4"}),
			req:     newChatReqWithText("openai", "gpt-3.5", "phone: 13812345678"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				content := req.ChatRequest.Input[0].Content.ContentStr
				if content == nil || !strings.Contains(*content, "13812345678") {
					t.Error("text should pass through when model doesn't match rules")
				}
			},
		},
		{
			name:    "masks phone when rule matches",
			cfg:     fullCfgWithRules(MaskingRule{Provider: "openai", Model: "gpt-4"}),
			req:     newChatReqWithText("openai", "gpt-4", "call 13812345678"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				content := req.ChatRequest.Input[0].Content.ContentStr
				if content == nil || strings.Contains(*content, "13812345678") {
					t.Error("phone should be masked")
				}
				if !strings.Contains(*content, "****") {
					t.Error("expected masked content")
				}
			},
		},
		{
			name:    "wildcard rule matches all models",
			cfg:     fullCfgWithRules(MaskingRule{Provider: "openai", Model: ""}),
			req:     newChatReqWithText("openai", "gpt-3.5", "email: user@example.com"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				content := req.ChatRequest.Input[0].Content.ContentStr
				if content == nil || strings.Contains(*content, "user@example.com") {
					t.Error("email should be masked when wildcard rule matches")
				}
			},
		},
		{
			name:    "msg with nil Content skipped",
			cfg:     fullCfgWithRules(MaskingRule{Provider: "", Model: ""}),
			req:     newChatReq("openai", "gpt-4"),
			wantErr: false,
			check: func(t *testing.T, req *schemas.BifrostRequest) {
				// should not panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalConfig.Store(tt.cfg)
			t.Cleanup(resetCfg)

			req, _, err := PreLLMHook(newCtx(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PreLLMHook() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, req)
			}
		})
	}
}

func TestPreLLMHook_MultipleMessages(t *testing.T) {
	resetCfg := func() { globalConfig.Store(defaultConfig()) }
	t.Cleanup(resetCfg)

	globalConfig.Store(fullCfgWithRules(MaskingRule{Provider: "", Model: ""}))

	text1 := "my phone: 13812345678"
	text2 := "my email: test@example.com"
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: "openai",
			Model:    "gpt-4",
			Input: []schemas.ChatMessage{
				{Role: "user", Content: &schemas.ChatMessageContent{ContentStr: &text1}},
				{Role: "user", Content: &schemas.ChatMessageContent{ContentStr: &text2}},
			},
		},
	}

	req, _, err := PreLLMHook(&schemas.BifrostContext{}, req)
	if err != nil {
		t.Fatalf("PreLLMHook() error = %v", err)
	}

	if strings.Contains(*req.ChatRequest.Input[0].Content.ContentStr, "13812345678") {
		t.Error("first message phone should be masked")
	}
	if strings.Contains(*req.ChatRequest.Input[1].Content.ContentStr, "test@example.com") {
		t.Error("second message email should be masked")
	}
}

// ── IP regex validation tests ────────────────────────────────────────

func TestIPRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		setup    func(*PluginConfig)
	}{
		{
			name:     "valid private IP",
			input:    "server 192.168.1.1 is down",
			expected: "server ***.***.***.*** is down",
		},
		{
			name:     "valid public IP",
			input:    "address 8.8.8.8 dns",
			expected: "address ***.***.***.*** dns",
		},
		{
			name:     "IP with leading zeros in octet still masked",
			input:    "ip 10.0.0.01",
			expected: "ip ***.***.***.***",
		},
		{
			name:     "out of range octet not masked",
			input:    "ip 999.999.999.999 invalid",
			expected: "ip 999.999.999.999 invalid",
		},
		{
			name:     "incomplete IP",
			input:    "version 192.168.1",
			expected: "version 192.168.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.MaskPhone = false
			cfg.MaskEmail = false
			cfg.MaskIDCard = false
			cfg.MaskBankCard = false
			got := maskSensitiveText(cfg, tt.input)
			if got != tt.expected {
				t.Errorf("maskSensitiveText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ── Helper functions ─────────────────────────────────────────────────

func newChatReq(provider, model string) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.ModelProvider(provider),
			Model:    model,
		},
	}
}

func newChatReqWithText(provider, model, text string) *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.ModelProvider(provider),
			Model:    model,
			Input: []schemas.ChatMessage{
				{
					Role: "user",
					Content: &schemas.ChatMessageContent{
						ContentStr: &text,
					},
				},
			},
		},
	}
}

func fullCfgNoRules() *PluginConfig {
	c := defaultConfig()
	c.MaskingRules = nil
	return c
}

func fullCfgWithRules(rules ...MaskingRule) *PluginConfig {
	c := defaultConfig()
	c.MaskingRules = rules
	return c
}

func withKeywords(keywords ...string) *PluginConfig {
	c := defaultConfig()
	c.CustomKeywords = keywords
	c.initKeywordReplacer()
	return c
}
