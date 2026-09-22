package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setCommonEnv(t *testing.T, exchange string, prompt string) {
	t.Helper()
	promptPath := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(promptPath, []byte(prompt), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERMAN_AI_FREE_TRADER_ID", "customer-test")
	t.Setenv("HERMAN_AI_FREE_USER_ID", "customer-test")
	t.Setenv("HERMAN_AI_FREE_DB_PATH", filepath.Join(t.TempDir(), "nofx.sqlite"))
	t.Setenv("HERMAN_AI_FREE_EXCHANGE", exchange)
	t.Setenv("HERMAN_AI_FREE_LANGUAGE", "zh")
	t.Setenv("HERMAN_AI_FREE_PROMPT_FILE", promptPath)
	t.Setenv("HERMAN_AI_FREE_MANAGED_CAPITAL_USDT", "300")
	t.Setenv("HERMAN_AI_FREE_MAX_LOSS_PER_TRADE_USDT", "10")
	t.Setenv("HERMAN_AI_FREE_MAX_LEVERAGE", "5")
	t.Setenv("HERMAN_AI_FREE_MAX_POSITIONS", "3")
	t.Setenv("HERMAN_AI_FREE_ACCOUNT_TP_USDT", "30")
	t.Setenv("HERMAN_AI_FREE_ACCOUNT_SL_USDT", "20")
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
}

func TestLoadRuntimeConfigOKXPreservesPromptVerbatim(t *testing.T) {
	prompt := "  only trade my setup\nsecond line exactly  "
	setCommonEnv(t, "okx", prompt)
	t.Setenv("OKX_API_KEY", "key")
	t.Setenv("OKX_SECRET_KEY", "secret")
	t.Setenv("OKX_PASSPHRASE", "pass")

	cfg, err := loadRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CustomerPrompt != prompt {
		t.Fatalf("prompt changed: %q", cfg.CustomerPrompt)
	}
	strategy := buildStrategy(cfg)
	if strategy.CustomPrompt != prompt {
		t.Fatalf("strategy prompt changed: %q", strategy.CustomPrompt)
	}
	if strategy.DecisionMode != "ai_free" {
		t.Fatalf("unexpected decision mode: %q", strategy.DecisionMode)
	}
	if strategy.RiskControl.ManagedCapitalUSDT != 300 ||
		strategy.RiskControl.MaxLossPerTradeUSDT != 10 ||
		strategy.RiskControl.MaxLeverage != 5 ||
		strategy.RiskControl.MaxPositions != 3 ||
		strategy.RiskControl.AccountTakeProfitUSDT != 30 ||
		strategy.RiskControl.AccountStopLossUSDT != 20 {
		t.Fatalf("unexpected risk config: %+v", strategy.RiskControl)
	}
}

func TestLoadRuntimeConfigLighterAllowsAccountIndexZero(t *testing.T) {
	setCommonEnv(t, "lighter", "test")
	t.Setenv("LIGHTER_ACCOUNT_INDEX", "0")
	t.Setenv("LIGHTER_API_KEY_INDEX", "2")
	t.Setenv("LIGHTER_API_KEY_PRIVATE", "private")

	cfg, err := loadRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	strategy := buildStrategy(cfg)
	autoCfg := buildAutoTraderConfig(cfg, &strategy)
	if !autoCfg.LighterUseAccountIndex || autoCfg.LighterAccountIndex != 0 {
		t.Fatalf("lighter account-index mode not configured: %+v", autoCfg)
	}
	if autoCfg.LighterAPIKeyIndex != 2 || autoCfg.LighterAPIKeyPrivateKey != "private" {
		t.Fatalf("lighter credentials not mapped")
	}
}

func TestLoadRuntimeConfigHyperliquidMapsCredentials(t *testing.T) {
	setCommonEnv(t, "hyperliquid", "test")
	t.Setenv("HYPERLIQUID_ACCOUNT_ADDRESS", "0x1111111111111111111111111111111111111111")
	t.Setenv("HYPERLIQUID_API_PRIVATE_KEY", "0xabc")

	cfg, err := loadRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	strategy := buildStrategy(cfg)
	autoCfg := buildAutoTraderConfig(cfg, &strategy)
	if autoCfg.HyperliquidWalletAddr != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("hyperliquid address not mapped")
	}
	if autoCfg.HyperliquidPrivateKey != "0xabc" {
		t.Fatalf("hyperliquid key not mapped")
	}
}

func TestLoadRuntimeConfigRejectsUnsupportedExchange(t *testing.T) {
	setCommonEnv(t, "binance", "test")
	if _, err := loadRuntimeConfig(); err == nil {
		t.Fatal("expected unsupported Herman exchange to fail")
	}
}

func TestLoadRuntimeConfigRejectsRiskOutsideManagedCapital(t *testing.T) {
	setCommonEnv(t, "okx", "test")
	t.Setenv("OKX_API_KEY", "key")
	t.Setenv("OKX_SECRET_KEY", "secret")
	t.Setenv("OKX_PASSPHRASE", "pass")
	t.Setenv("HERMAN_AI_FREE_MAX_LOSS_PER_TRADE_USDT", "301")
	if _, err := loadRuntimeConfig(); err == nil {
		t.Fatal("expected per-trade risk above managed capital to fail")
	}
}
