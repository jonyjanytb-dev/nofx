package mcp

import (
	"testing"
	"time"
)

func TestConfigureAIFreeClientDisablesDeepSeekThinking(t *testing.T){
	client:=NewClient(WithProvider(ProviderDeepSeek),WithAPIKey("test"))
	ConfigureAIFreeClient(client)
	base:=client.(*Client)
	body:=base.BuildMCPRequestBody("system","user")
	thinking,ok:=body["thinking"].(map[string]string)
	if !ok||thinking["type"]!="disabled"{t.Fatalf("expected thinking disabled, got %#v",body["thinking"])}
	if base.Cfg.MaxRetries!=1{t.Fatalf("expected exactly one attempt, got MaxRetries=%d",base.Cfg.MaxRetries)}
	if base.HTTPClient.Timeout!=60*time.Second{t.Fatalf("expected 60s timeout, got %v",base.HTTPClient.Timeout)}
	if base.Cfg.Temperature!=0.2{t.Fatalf("expected temperature 0.2, got %v",base.Cfg.Temperature)}
}
