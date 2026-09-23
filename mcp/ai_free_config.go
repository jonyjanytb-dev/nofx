package mcp

import "time"

const AIFreeDeepSeekModel = "deepseek-v4-flash"

// ConfigureAIFreeClient applies transport behavior required by ai_free without
// changing legacy provider behavior.
func ConfigureAIFreeClient(client AIClient) {
	if client == nil {
		return
	}
	client.SetTimeout(60 * time.Second)

	var base *Client
	switch c := client.(type) {
	case *Client:
		base = c
	case ClientEmbedder:
		base = c.BaseClient()
	}
	if base == nil || base.Cfg == nil {
		return
	}
	base.Cfg.MaxRetries = 1
	base.Cfg.Temperature = 0.2
	base.Cfg.DeepSeekThinkingDisabled = base.Provider == ProviderDeepSeek
}
