package main

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

type Config struct {
	Enabled          bool
	TargetModels     []string
	ImagePlaceholder string
}

var cfg atomic.Pointer[Config]

func defaultConfig() *Config {
	return &Config{
		Enabled: true,
		ImagePlaceholder: "[Image provided: this model does not support image input. " +
			"Image URL: {url}, Detail: {detail}]",
	}
}

func parseConfig(raw any, c *Config) error {
	m, ok := raw.(map[string]interface{})
	if !ok || m == nil {
		return nil
	}
	if v, ok := m["enabled"].(bool); ok {
		c.Enabled = v
	}
	if v, ok := m["image_placeholder"].(string); ok && v != "" {
		c.ImagePlaceholder = v
	}
	if arr, ok := m["target_models"].([]interface{}); ok {
		c.TargetModels = make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				c.TargetModels = append(c.TargetModels, s)
			}
		}
	}
	return nil
}

func GetName() string { return "vision-extension" }

func Init(config any) error {
	c := defaultConfig()
	if err := parseConfig(config, c); err != nil {
		return fmt.Errorf("vision-extension: invalid config: %w", err)
	}
	cfg.Store(c)
	return nil
}

func Cleanup() error { return nil }

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	c := cfg.Load()
	if c == nil || !c.Enabled {
		return req, nil, nil
	}
	if req == nil || req.ChatRequest == nil {
		return req, nil, nil
	}

	if len(c.TargetModels) > 0 && !isTargetModel(req.ChatRequest.Model, c.TargetModels) {
		return req, nil, nil
	}

	for i := range req.ChatRequest.Input {
		msg := &req.ChatRequest.Input[i]
		if msg.Content == nil || msg.Content.ContentBlocks == nil {
			continue
		}
		blocks := msg.Content.ContentBlocks
		hasImage := false
		for _, block := range blocks {
			if block.Type == schemas.ChatContentBlockTypeImage {
				hasImage = true
				break
			}
		}
		if !hasImage {
			continue
		}

		newBlocks := make([]schemas.ChatContentBlock, 0, len(blocks))
		for _, block := range blocks {
			if block.Type == schemas.ChatContentBlockTypeImage && block.ImageURLStruct != nil {
				detail := ""
				if block.ImageURLStruct.Detail != nil {
					detail = *block.ImageURLStruct.Detail
				}
				placeholder := formatPlaceholder(c.ImagePlaceholder, block.ImageURLStruct.URL, detail)
				newBlocks = append(newBlocks, schemas.ChatContentBlock{
					Type: schemas.ChatContentBlockTypeText,
					Text: &placeholder,
				})
			} else {
				newBlocks = append(newBlocks, block)
			}
		}
		req.ChatRequest.Input[i].Content = &schemas.ChatMessageContent{
			ContentBlocks: newBlocks,
		}
	}

	return req, nil, nil
}

func isTargetModel(model string, targetModels []string) bool {
	for _, m := range targetModels {
		if m == model {
			return true
		}
	}
	return false
}

func formatPlaceholder(template, url, detail string) string {
	r := strings.NewReplacer("{url}", url, "{detail}", detail)
	return r.Replace(template)
}
