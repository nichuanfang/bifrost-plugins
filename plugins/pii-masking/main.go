package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

const (
	maskReplacement   = "******"
	ipMaskReplacement = "***.***.***.***"
	maskPhoneLength   = 11
	maskIDCardLength  = 18
	minBankCardLength = 13
)

// DesensitizationRules defines a provider/model pair that should be desensitized.
// Empty Provider or Model matches any value (wildcard).
type DesensitizationRules struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// PluginConfig holds the desensitization plugin configuration.
type PluginConfig struct {
	Enable               bool                   `json:"enable"`
	MaskPhone            bool                   `json:"mask_phone"`
	MaskEmail            bool                   `json:"mask_email"`
	MaskIDCard           bool                   `json:"mask_id_card"`
	MaskBankCard         bool                   `json:"mask_bank_card"`
	MaskIP               bool                   `json:"mask_ip"`
	CustomKeywords       []string               `json:"custom_keywords"`
	CustomRegex          []string               `json:"custom_regex"`
	LogDesensitized      bool                   `json:"log_desensitized"`
	DesensitizationRules []DesensitizationRules `json:"desensitization_rules"`

	compiledCustomRegex []*regexp.Regexp
	keywordReplacer     *strings.Replacer // 预编译关键词替换器
}

var globalConfig atomic.Pointer[PluginConfig]

// 全局预编译正则
var (
	phoneRegex    = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	emailRegex    = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	idCardRegex   = regexp.MustCompile(`\b[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`)
	bankCardRegex = regexp.MustCompile(`\b[3-6]\d{12,18}\b`)
	ipRegex       = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
)

// GetName returns the plugin name.
func GetName() string {
	return "pii-masking"
}

// ── 配置初始化 ───────────────────────────────────────────────────

// Init parses and applies the plugin configuration.
func Init(config any) error {
	cfg := defaultConfig()
	configMap, ok := config.(map[string]interface{})
	if !ok {
		globalConfig.Store(cfg)
		return nil
	}

	parseBoolField(configMap, "enable", &cfg.Enable)
	parseBoolField(configMap, "mask_phone", &cfg.MaskPhone)
	parseBoolField(configMap, "mask_email", &cfg.MaskEmail)
	parseBoolField(configMap, "mask_id_card", &cfg.MaskIDCard)
	parseBoolField(configMap, "mask_bank_card", &cfg.MaskBankCard)
	parseBoolField(configMap, "mask_ip", &cfg.MaskIP)
	parseBoolField(configMap, "log_desensitized", &cfg.LogDesensitized)

	cfg.CustomKeywords = parseStringList(configMap, "custom_keywords")
	cfg.CustomRegex, cfg.compiledCustomRegex = parseRegexList(configMap, "custom_regex")
	cfg.DesensitizationRules = parseDesensitizationRules(configMap, "desensitization_rules")

	// 预编译关键词替换器，一次遍历替换所有关键词
	cfg.initKeywordReplacer()

	globalConfig.Store(cfg)
	return nil
}

func defaultConfig() *PluginConfig {
	return &PluginConfig{
		Enable:          true,
		MaskPhone:       true,
		MaskEmail:       true,
		MaskIDCard:      true,
		MaskBankCard:    true,
		MaskIP:          true,
		LogDesensitized: false,
	}
}

// initKeywordReplacer 预构建关键词替换器，避免多次 ReplaceAll 遍历
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

func parseBoolField(m map[string]interface{}, key string, target *bool) {
	if v, ok := m[key].(bool); ok {
		*target = v
	}
}

func parseStringList(m map[string]interface{}, key string) []string {
	raw, ok := m[key].([]interface{})
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

func parseRegexList(m map[string]interface{}, key string) ([]string, []*regexp.Regexp) {
	raw, ok := m[key].([]interface{})
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
			continue // 跳过无效正则，不阻断初始化
		}
		patterns = append(patterns, pattern)
		compiled = append(compiled, re)
	}
	return patterns, compiled
}

func parseDesensitizationRules(m map[string]interface{}, key string) []DesensitizationRules {
	raw, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	rules := make([]DesensitizationRules, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		rule := DesensitizationRules{}
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

// shouldDesensitize 判断请求是否应被执行脱敏。
//
// 语义：desensitization_rules是需要脱敏的规则集合。
//   - 规则集为空 → 不脱敏（返回 false）
//   - 不为空 → 匹配规则时才脱敏（返回 true）
//
// 规则匹配规则：
//   - Provider 为空字符串时视为通配，匹配所有 Provider
//   - Model 为空字符串时视为通配，匹配所有 Model
func shouldDesensitize(cfg *PluginConfig, req *schemas.BifrostRequest) bool {
	if len(cfg.DesensitizationRules) == 0 {
		return false // 白名单为空 → 不脱敏
	}
	if req == nil || req.ChatRequest == nil {
		return false
	}
	provider := strings.TrimSpace(string(req.ChatRequest.Provider))
	model := strings.TrimSpace(req.ChatRequest.Model)

	for _, rule := range cfg.DesensitizationRules {
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
		text = cfg.keywordReplacer.Replace(text) // 一次遍历替换所有关键词
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
	domain := s[atIndex:] // 包含 @ 符号

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
	// 先清理分隔符，再判断长度
	clean := stripCardSeparators(s)
	if len(clean) >= minBankCardLength {
		return clean[:4] + "********" + clean[len(clean)-4:]
	}
	return s
}

// stripCardSeparators 快速去除银行卡号中的空格和连字符
func stripCardSeparators(s string) string {
	// 快速路径：不包含任何分隔符，直接返回
	if !strings.Contains(s, " ") && !strings.Contains(s, "-") {
		return s
	}
	// 单次遍历构建新字符串，避免多次 ReplaceAll 分配
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

// PreRequestHook 官方 llm-only 示例
func PreRequestHook(_ *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	return nil
}

// PreLLMHook 在 LLM 请求发送前对消息内容进行脱敏。
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	cfg := globalConfig.Load()

	// 快速失败：插件未启用或请求无效
	if !cfg.Enable || req == nil || req.ChatRequest == nil {
		return req, nil, nil
	}
	// 白名单为空或不匹配 → 不脱敏
	if !shouldDesensitize(cfg, req) {
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
			if cfg.LogDesensitized {
				ctx.Log(schemas.LogLevelInfo, fmt.Sprintf(
					"[PII-Masking] Input text desensitized for %s/%s",
					req.ChatRequest.Provider, req.ChatRequest.Model,
				))
			}
			*msg.Content.ContentStr = masked
		}
	}
	return req, nil, nil
}

// PostLLMHook 透传 LLM 响应，不做处理。
func PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

// Cleanup 空实现，无需清理资源。
func Cleanup() error {
	return nil
}
