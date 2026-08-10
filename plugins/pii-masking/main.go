package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	maskReplacement    = "******"
	ipMaskReplacement  = "***.***.***.***"
	maskPhoneLength    = 11
	maskIDCardLength   = 18
	minBankCardLength  = 13
	bankCardKeepPrefix = 4
	bankCardKeepSuffix = 4
)

// MaskingRule defines a provider/model pair whose traffic should be masked.
// Empty Provider or Model matches any value (wildcard).
type MaskingRule struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// PluginConfig holds the PII masking plugin configuration.
type PluginConfig struct {
	Enable         bool          `json:"enable"`
	MaskPhone      bool          `json:"mask_phone"`
	MaskEmail      bool          `json:"mask_email"`
	MaskIDCard     bool          `json:"mask_id_card"`
	MaskBankCard   bool          `json:"mask_bank_card"`
	MaskIP         bool          `json:"mask_ip"`
	CustomKeywords []string      `json:"custom_keywords"`
	CustomRegex    []string      `json:"custom_regex"`
	LogMasked      bool          `json:"log_desensitized"`
	MaskingRules   []MaskingRule `json:"desensitization_rules"`

	compiledCustomRegex []*regexp.Regexp
	keywordReplacer     *strings.Replacer
}

var globalConfig atomic.Pointer[PluginConfig]

// 全局预编译正则
var (
	// 中国手机号: 1[3-9] + 9位数字
	phoneRegex = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	// 邮箱
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	// 18位身份证号
	idCardRegex = regexp.MustCompile(`\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
	// 银行卡号: 3-6 开头, 13-19 位
	bankCardRegex = regexp.MustCompile(`\b[3-6]\d{12,18}\b`)
	// IPv4: 每段 0-255, 排除全0和全255可自行判断
	ipRegex = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b`)
)

// GetName returns the plugin name.
func GetName() string {
	return "pii-masking"
}

// ── 配置初始化 ───────────────────────────────────────────────────

// Init parses and applies the plugin configuration.
func Init(config any) error {
	cfg := defaultConfig()
	parseConfig(config, cfg)
	cfg.initKeywordReplacer()
	validateConfig(cfg)
	globalConfig.Store(cfg)
	return nil
}

func defaultConfig() *PluginConfig {
	return &PluginConfig{
		Enable:       true,
		MaskPhone:    true,
		MaskEmail:    true,
		MaskIDCard:   true,
		MaskBankCard: true,
		MaskIP:       true,
	}
}

func parseConfig(raw any, c *PluginConfig) {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return
	}

	parseBoolField(m, "enable", &c.Enable)
	parseBoolField(m, "mask_phone", &c.MaskPhone)
	parseBoolField(m, "mask_email", &c.MaskEmail)
	parseBoolField(m, "mask_id_card", &c.MaskIDCard)
	parseBoolField(m, "mask_bank_card", &c.MaskBankCard)
	parseBoolField(m, "mask_ip", &c.MaskIP)
	parseBoolField(m, "log_desensitized", &c.LogMasked)

	c.CustomKeywords = parseStringList(m, "custom_keywords")
	c.CustomRegex, c.compiledCustomRegex = parseRegexList(m, "custom_regex")
	c.MaskingRules = parseMaskingRules(m, "desensitization_rules")
}

// validateConfig logs warnings for suspicious configurations.
func validateConfig(c *PluginConfig) {
	if !c.Enable {
		return
	}
	if len(c.MaskingRules) == 0 {
		log.Printf("[pii-masking] WARNING: enable=true but desensitization_rules is empty — no traffic will be masked")
	}
	if !c.MaskPhone && !c.MaskEmail && !c.MaskIDCard && !c.MaskBankCard && !c.MaskIP &&
		len(c.CustomKeywords) == 0 && len(c.CustomRegex) == 0 {
		log.Printf("[pii-masking] WARNING: enable=true but all mask types are disabled — nothing will be masked")
	}
}

// initKeywordReplacer pre-builds a keyword replacer to avoid multiple scan passes.
func (c *PluginConfig) initKeywordReplacer() {
	if len(c.CustomKeywords) == 0 {
		return
	}
	pairs := make([]string, 0, len(c.CustomKeywords)*2)
	for _, kw := range c.CustomKeywords {
		pairs = append(pairs, kw, maskReplacement)
	}
	c.keywordReplacer = strings.NewReplacer(pairs...)
}

// ── 配置解析辅助函数 ────────────────────────────────────────────

func parseBoolField(m map[string]any, key string, target *bool) {
	v, ok := m[key]
	if !ok {
		return
	}
	switch val := v.(type) {
	case bool:
		*target = val
	case string:
		switch strings.ToLower(val) {
		case "true":
			*target = true
		case "false":
			*target = false
		default:
			log.Printf("[pii-masking] WARNING: invalid value for %q: %q (expected true/false), using default %v", key, val, *target)
		}
	default:
		log.Printf("[pii-masking] WARNING: unexpected type for %q: %T, using default %v", key, v, *target)
	}
}

func parseStringList(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if ok && strings.TrimSpace(s) != "" {
			result = append(result, strings.TrimSpace(s))
		}
	}
	return result
}

func parseRegexList(m map[string]any, key string) ([]string, []*regexp.Regexp) {
	raw, ok := m[key].([]any)
	if !ok {
		return nil, nil
	}
	patterns := make([]string, 0, len(raw))
	compiled := make([]*regexp.Regexp, 0, len(raw))
	for _, item := range raw {
		pattern, ok := item.(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("[pii-masking] WARNING: invalid custom regex %q skipped: %v", pattern, err)
			continue
		}
		patterns = append(patterns, pattern)
		compiled = append(compiled, re)
	}
	return patterns, compiled
}

func parseMaskingRules(m map[string]any, key string) []MaskingRule {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	rules := make([]MaskingRule, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		rule := MaskingRule{}
		if p, ok := entry["provider"].(string); ok {
			rule.Provider = strings.TrimSpace(p)
		}
		if mdl, ok := entry["model"].(string); ok {
			rule.Model = strings.TrimSpace(mdl)
		}
		rules = append(rules, rule)
	}
	return rules
}

// ── 核心判断逻辑 ──────────────────────────────────────────────────

// shouldMask 判断请求是否应执行脱敏。
//
// masking_rules 是需要脱敏的规则集合（白名单模式）：
//   - 规则集为空 → 不脱敏（返回 false）
//   - 不为空 → 匹配规则时才脱敏（返回 true）
//
// 规则匹配：
//   - Provider 为空字符串时视为通配，匹配所有 Provider
//   - Model 为空字符串时视为通配，匹配所有 Model
func shouldMask(cfg *PluginConfig, req *schemas.BifrostRequest) bool {
	if len(cfg.MaskingRules) == 0 {
		return false
	}
	if req == nil || req.ChatRequest == nil {
		return false
	}
	provider := strings.TrimSpace(string(req.ChatRequest.Provider))
	model := strings.TrimSpace(req.ChatRequest.Model)

	for _, rule := range cfg.MaskingRules {
		pMatch := rule.Provider == "" || strings.EqualFold(rule.Provider, provider)
		mMatch := rule.Model == "" || strings.EqualFold(rule.Model, model)
		if pMatch && mMatch {
			return true
		}
	}
	return false
}

// ── 脱敏引擎 ──────────────────────────────────────────────────────

func maskSensitiveText(cfg *PluginConfig, text string) string {
	if text == "" {
		return text
	}

	if cfg.MaskPhone {
		text = phoneRegex.ReplaceAllStringFunc(text, maskPhoneFunc)
	}
	if cfg.MaskEmail {
		text = emailRegex.ReplaceAllStringFunc(text, maskEmailFunc)
	}
	if cfg.MaskIDCard {
		text = idCardRegex.ReplaceAllStringFunc(text, maskIDCardFunc)
	}
	if cfg.MaskBankCard {
		text = bankCardRegex.ReplaceAllStringFunc(text, maskBankCardFunc)
	}
	if cfg.MaskIP {
		text = ipRegex.ReplaceAllString(text, ipMaskReplacement)
	}
	if cfg.keywordReplacer != nil {
		text = cfg.keywordReplacer.Replace(text)
	}
	for _, re := range cfg.compiledCustomRegex {
		text = re.ReplaceAllString(text, maskReplacement)
	}
	return text
}

// ── 各类型脱敏函数（独立可测试） ──────────────────────────────────

func maskPhoneFunc(s string) string {
	if len(s) == maskPhoneLength {
		return s[:3] + "****" + s[7:]
	}
	return s
}

func maskEmailFunc(s string) string {
	atIndex := strings.IndexByte(s, '@')
	if atIndex <= 0 {
		return s
	}
	local := s[:atIndex]
	domain := s[atIndex:]

	if len(local) >= 4 {
		local = local[:3] + "****"
	} else {
		local = "****"
	}
	return local + domain
}

func maskIDCardFunc(s string) string {
	if len(s) == maskIDCardLength {
		return s[:6] + "********" + s[14:]
	}
	return s
}

func maskBankCardFunc(s string) string {
	clean := stripCardSeparators(s)
	if len(clean) < minBankCardLength {
		return s
	}

	// Fast path: no separators
	if len(clean) == len(s) {
		return clean[:bankCardKeepPrefix] + "********" + clean[len(clean)-bankCardKeepSuffix:]
	}

	// Preserve separator positions in the output
	var buf strings.Builder
	buf.Grow(len(s) + 8)
	digitIdx := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '-' {
			buf.WriteByte(s[i])
			continue
		}
		if digitIdx < bankCardKeepPrefix || digitIdx >= len(clean)-bankCardKeepSuffix {
			buf.WriteByte(s[i])
		} else {
			buf.WriteByte('*')
		}
		digitIdx++
	}
	return buf.String()
}

// stripCardSeparators removes spaces and hyphens from a bank card number.
func stripCardSeparators(s string) string {
	if !strings.Contains(s, " ") && !strings.Contains(s, "-") {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '-' {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

// ── Hook 接口实现 ──────────────────────────────────────────────────

// PreRequestHook is a no-op passthrough.
func PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook masks PII in LLM request messages before they are sent.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	cfg := globalConfig.Load()

	if !cfg.Enable || req == nil || req.ChatRequest == nil {
		return req, nil, nil
	}
	if !shouldMask(cfg, req) {
		return req, nil, nil
	}

	for i := range req.ChatRequest.Input {
		msg := &req.ChatRequest.Input[i]
		if msg.Content == nil || msg.Content.ContentStr == nil {
			continue
		}
		original := *msg.Content.ContentStr
		masked := maskSensitiveText(cfg, original)
		if masked != original {
			if cfg.LogMasked {
				ctx.Log(schemas.LogLevelInfo, fmt.Sprintf(
					"[PII-Masking] Input text masked for %s/%s",
					req.ChatRequest.Provider, req.ChatRequest.Model,
				))
			}
			*msg.Content.ContentStr = masked
		}
	}
	return req, nil, nil
}

// PostLLMHook passes through the LLM response unchanged.
func PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

// Cleanup is a no-op.
func Cleanup() error {
	return nil
}
