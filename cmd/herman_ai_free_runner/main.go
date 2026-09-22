package main

import (
	"encoding/json"
	"fmt"
	"nofx/logger"
	"nofx/store"
	"nofx/trader"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type runtimeConfig struct {
	TraderID              string
	UserID                string
	Exchange              string
	DatabasePath          string
	Language              string
	CustomerPrompt        string
	DeepSeekAPIKey        string
	ManagedCapitalUSDT    float64
	MaxLossPerTradeUSDT   float64
	MaxLeverage           int
	MaxPositions          int
	AccountTakeProfitUSDT float64
	AccountStopLossUSDT   float64

	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	LighterAccountIndex int64
	LighterAPIKeyIndex  int
	LighterAPIPrivate   string

	HyperliquidAddress    string
	HyperliquidPrivateKey string
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func floatEnv(name string) (float64, error) {
	raw, err := requiredEnv(name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric", name)
	}
	return value, nil
}

func intEnv(name string) (int, error) {
	raw, err := requiredEnv(name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}

func optionalFloatEnv(name string) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be numeric", name)
	}
	return value, nil
}

func loadRuntimeConfig() (runtimeConfig, error) {
	var cfg runtimeConfig
	var err error

	if cfg.TraderID, err = requiredEnv("HERMAN_AI_FREE_TRADER_ID"); err != nil {
		return cfg, err
	}
	if cfg.UserID, err = requiredEnv("HERMAN_AI_FREE_USER_ID"); err != nil {
		return cfg, err
	}
	if cfg.DatabasePath, err = requiredEnv("HERMAN_AI_FREE_DB_PATH"); err != nil {
		return cfg, err
	}
	if cfg.DeepSeekAPIKey, err = requiredEnv("DEEPSEEK_API_KEY"); err != nil {
		return cfg, err
	}

	cfg.Exchange = strings.ToLower(strings.TrimSpace(os.Getenv("HERMAN_AI_FREE_EXCHANGE")))
	if cfg.Exchange == "" {
		return cfg, fmt.Errorf("HERMAN_AI_FREE_EXCHANGE is required")
	}
	cfg.Language = strings.ToLower(strings.TrimSpace(os.Getenv("HERMAN_AI_FREE_LANGUAGE")))
	if cfg.Language == "" {
		cfg.Language = "zh"
	}
	if cfg.Language != "zh" && cfg.Language != "en" {
		return cfg, fmt.Errorf("HERMAN_AI_FREE_LANGUAGE must be zh or en")
	}

	promptPath, err := requiredEnv("HERMAN_AI_FREE_PROMPT_FILE")
	if err != nil {
		return cfg, err
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		return cfg, fmt.Errorf("read customer prompt: %w", err)
	}
	cfg.CustomerPrompt = string(prompt)

	if cfg.ManagedCapitalUSDT, err = floatEnv("HERMAN_AI_FREE_MANAGED_CAPITAL_USDT"); err != nil {
		return cfg, err
	}
	if cfg.MaxLossPerTradeUSDT, err = floatEnv("HERMAN_AI_FREE_MAX_LOSS_PER_TRADE_USDT"); err != nil {
		return cfg, err
	}
	if cfg.MaxLeverage, err = intEnv("HERMAN_AI_FREE_MAX_LEVERAGE"); err != nil {
		return cfg, err
	}
	if cfg.MaxPositions, err = intEnv("HERMAN_AI_FREE_MAX_POSITIONS"); err != nil {
		return cfg, err
	}
	if cfg.AccountTakeProfitUSDT, err = optionalFloatEnv("HERMAN_AI_FREE_ACCOUNT_TP_USDT"); err != nil {
		return cfg, err
	}
	if cfg.AccountStopLossUSDT, err = optionalFloatEnv("HERMAN_AI_FREE_ACCOUNT_SL_USDT"); err != nil {
		return cfg, err
	}

	if cfg.ManagedCapitalUSDT <= 0 {
		return cfg, fmt.Errorf("managed capital must be > 0")
	}
	if cfg.MaxLossPerTradeUSDT <= 0 {
		return cfg, fmt.Errorf("max loss per trade must be > 0")
	}
	if cfg.MaxLossPerTradeUSDT > cfg.ManagedCapitalUSDT {
		return cfg, fmt.Errorf("max loss per trade cannot exceed managed capital")
	}
	if cfg.MaxLeverage < 1 || cfg.MaxLeverage > 20 {
		return cfg, fmt.Errorf("max leverage must be between 1 and 20")
	}
	if cfg.MaxPositions < 1 || cfg.MaxPositions > store.MaxPositions {
		return cfg, fmt.Errorf("max positions must be between 1 and %d", store.MaxPositions)
	}
	if cfg.AccountTakeProfitUSDT < 0 || cfg.AccountStopLossUSDT < 0 {
		return cfg, fmt.Errorf("account TP/SL cannot be negative")
	}

	switch cfg.Exchange {
	case "okx":
		if cfg.OKXAPIKey, err = requiredEnv("OKX_API_KEY"); err != nil {
			return cfg, err
		}
		if cfg.OKXSecretKey, err = requiredEnv("OKX_SECRET_KEY"); err != nil {
			return cfg, err
		}
		if cfg.OKXPassphrase, err = requiredEnv("OKX_PASSPHRASE"); err != nil {
			return cfg, err
		}
	case "lighter":
		rawIndex, indexErr := requiredEnv("LIGHTER_ACCOUNT_INDEX")
		if indexErr != nil {
			return cfg, indexErr
		}
		cfg.LighterAccountIndex, err = strconv.ParseInt(strings.TrimSpace(rawIndex), 10, 64)
		if err != nil || cfg.LighterAccountIndex < 0 {
			return cfg, fmt.Errorf("LIGHTER_ACCOUNT_INDEX must be an integer >= 0")
		}
		if cfg.LighterAPIKeyIndex, err = intEnv("LIGHTER_API_KEY_INDEX"); err != nil {
			return cfg, err
		}
		if cfg.LighterAPIKeyIndex < 0 || cfg.LighterAPIKeyIndex > 255 {
			return cfg, fmt.Errorf("LIGHTER_API_KEY_INDEX must be between 0 and 255")
		}
		if cfg.LighterAPIPrivate, err = requiredEnv("LIGHTER_API_KEY_PRIVATE"); err != nil {
			return cfg, err
		}
	case "hyperliquid":
		if cfg.HyperliquidAddress, err = requiredEnv("HYPERLIQUID_ACCOUNT_ADDRESS"); err != nil {
			return cfg, err
		}
		if cfg.HyperliquidPrivateKey, err = requiredEnv("HYPERLIQUID_API_PRIVATE_KEY"); err != nil {
			return cfg, err
		}
	default:
		return cfg, fmt.Errorf("unsupported Herman ai_free exchange: %s", cfg.Exchange)
	}

	return cfg, nil
}

func buildStrategy(cfg runtimeConfig) store.StrategyConfig {
	strategy := store.GetAIFreeStrategyConfig(cfg.Language)
	strategy.CustomPrompt = cfg.CustomerPrompt
	strategy.RiskControl.ManagedCapitalUSDT = cfg.ManagedCapitalUSDT
	strategy.RiskControl.MaxLossPerTradeUSDT = cfg.MaxLossPerTradeUSDT
	strategy.RiskControl.MaxLeverage = cfg.MaxLeverage
	strategy.RiskControl.BTCETHMaxLeverage = cfg.MaxLeverage
	strategy.RiskControl.AltcoinMaxLeverage = cfg.MaxLeverage
	strategy.RiskControl.MaxPositions = cfg.MaxPositions
	strategy.RiskControl.AccountTakeProfitUSDT = cfg.AccountTakeProfitUSDT
	strategy.RiskControl.AccountStopLossUSDT = cfg.AccountStopLossUSDT
	return strategy
}

func buildAutoTraderConfig(cfg runtimeConfig, strategy *store.StrategyConfig) trader.AutoTraderConfig {
	out := trader.AutoTraderConfig{
		ID:             cfg.TraderID,
		Name:           "Herman AI Free",
		AIModel:        "deepseek",
		Exchange:       cfg.Exchange,
		ExchangeID:     cfg.TraderID,
		DeepSeekKey:    cfg.DeepSeekAPIKey,
		IsCrossMargin:  true,
		StrategyConfig: strategy,
	}

	switch cfg.Exchange {
	case "okx":
		out.OKXAPIKey = cfg.OKXAPIKey
		out.OKXSecretKey = cfg.OKXSecretKey
		out.OKXPassphrase = cfg.OKXPassphrase
	case "lighter":
		out.LighterUseAccountIndex = true
		out.LighterAccountIndex = cfg.LighterAccountIndex
		out.LighterAPIKeyPrivateKey = cfg.LighterAPIPrivate
		out.LighterAPIKeyIndex = cfg.LighterAPIKeyIndex
	case "hyperliquid":
		out.HyperliquidWalletAddr = cfg.HyperliquidAddress
		out.HyperliquidPrivateKey = cfg.HyperliquidPrivateKey
	}

	return out
}

func checkPayload(cfg runtimeConfig) map[string]interface{} {
	return map[string]interface{}{
		"mode":          "ai_free",
		"model":         "deepseek-flash",
		"exchange":      cfg.Exchange,
		"trader_id":     cfg.TraderID,
		"max_positions": cfg.MaxPositions,
		"max_leverage":  cfg.MaxLeverage,
	}
}

func run(cfg runtimeConfig) error {
	if dir := filepath.Dir(cfg.DatabasePath); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create database directory: %w", err)
		}
	}

	st, err := store.New(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	strategy := buildStrategy(cfg)
	autoCfg := buildAutoTraderConfig(cfg, &strategy)
	at, err := trader.NewAutoTrader(autoCfg, st, cfg.UserID)
	if err != nil {
		return err
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- at.Run()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-runErr:
		return err
	case <-signals:
		at.Stop()
		return nil
	}
}

func main() {
	logger.Init(nil)

	cfg, err := loadRuntimeConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if len(os.Args) > 1 && os.Args[1] == "--check" {
		payload, _ := json.Marshal(checkPayload(cfg))
		fmt.Println(string(payload))
		return
	}

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
