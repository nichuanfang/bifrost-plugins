package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

type Config struct {
	Enabled          bool
	ThresholdSeconds float64
	BotToken         string
	ChatID           string
}

var cfg atomic.Pointer[Config]

const (
	startTimeKey schemas.BifrostContextKey = "long_execution_alert_start"
	reqInfoKey   schemas.BifrostContextKey = "long_execution_alert_req_info"
)

type reqInfo struct {
	Provider string
	Model    string
}

func GetName() string { return "long-execution-alert" }

func Init(config any) error {
	c := &Config{
		Enabled:          true,
		ThresholdSeconds: 10.0,
	}
	if m, ok := config.(map[string]interface{}); ok {
		if v, ok := m["enabled"].(bool); ok {
			c.Enabled = v
		}
		if v, ok := m["threshold_seconds"].(float64); ok {
			c.ThresholdSeconds = v
		}
		if v, ok := m["telegram_bot_token"].(string); ok {
			c.BotToken = v
		}
		if v, ok := m["telegram_chat_id"].(string); ok {
			c.ChatID = v
		}
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
	ctx.SetValue(startTimeKey, time.Now())
	if req != nil {
		p, m, _ := req.GetRequestFields()
		ctx.SetValue(reqInfoKey, reqInfo{Provider: string(p), Model: m})
	}
	return req, nil, nil
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	c := cfg.Load()
	if c == nil || !c.Enabled {
		return resp, bifrostErr, nil
	}

	v := ctx.Value(startTimeKey)
	if v == nil {
		return resp, bifrostErr, nil
	}
	start, ok := v.(time.Time)
	if !ok {
		return resp, bifrostErr, nil
	}

	elapsed := time.Since(start).Seconds()
	if elapsed <= c.ThresholdSeconds {
		return resp, bifrostErr, nil
	}

	provider := ""
	model := ""
	if ri, ok := ctx.Value(reqInfoKey).(reqInfo); ok {
		provider = ri.Provider
		model = ri.Model
	}

	msg := fmt.Sprintf(
		"[长执行告警]\n耗时: %.1fs (阈值: %.1fs)\nProvider: %s\nModel: %s",
		elapsed, c.ThresholdSeconds, provider, model,
	)
	if bifrostErr != nil {
		msg += fmt.Sprintf("\n错误: %v", bifrostErr)
	}

	sendTelegramAlert(c.BotToken, c.ChatID, msg, ctx)
	return resp, bifrostErr, nil
}

func sendTelegramAlert(botToken, chatID, message string, ctx *schemas.BifrostContext) {
	if botToken == "" || chatID == "" {
		ctx.Log(schemas.LogLevelWarn, "[long-execution-alert] Telegram bot token or chat ID not configured, skipping alert")
		return
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	body := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}
	jsonBody, _ := json.Marshal(body)

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		ctx.Log(schemas.LogLevelError, fmt.Sprintf("[long-execution-alert] Failed to send Telegram alert: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ctx.Log(schemas.LogLevelError, fmt.Sprintf("[long-execution-alert] Telegram API returned status %d", resp.StatusCode))
		return
	}

	ctx.Log(schemas.LogLevelInfo, fmt.Sprintf("[long-execution-alert] Alert sent: %s", message))
}
