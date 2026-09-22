package store

// GetAIFreeStrategyConfig returns the product preset. It is intentionally
// separate from GetDefaultStrategyConfig so legacy NOFX/Vergex defaults stay
// behaviorally compatible unless the new mode is selected.
func GetAIFreeStrategyConfig(lang string) StrategyConfig {
	if lang != "zh" {
		lang = "en"
	}
	return StrategyConfig{
		StrategyType: "ai_trading",
		DecisionMode: "ai_free",
		Language:     lang,
		CoinSource: CoinSourceConfig{
			SourceType: "static",
			StaticCoins: []string{
				"BTCUSDT", "ETHUSDT", "SOLUSDT", "DOGEUSDT", "HYPEUSDT", "ZECUSDT",
			},
		},
		Indicators: IndicatorConfig{
			Klines: KlineConfig{
				PrimaryTimeframe:     "15m",
				PrimaryCount:         30,
				LongerTimeframe:      "1h",
				LongerCount:          24,
				EnableMultiTimeframe: true,
				SelectedTimeframes:   []string{"15m", "1h"},
			},
			EnableRawKlines:   true,
			EnableEMA:         true,
			EnableMACD:        true,
			EnableRSI:         true,
			EnableATR:         true,
			EnableBOLL:        true,
			EnableVolume:      true,
			EnableOI:          true,
			EnableFundingRate: true,
			EMAPeriods:        []int{20, 50, 200},
			RSIPeriods:        []int{7, 14},
			ATRPeriods:        []int{14},
			BOLLPeriods:       []int{20},
		},
		RiskControl: RiskControlConfig{
			MaxPositions:                 3,
			BTCETHMaxLeverage:            5,
			AltcoinMaxLeverage:           5,
			BTCETHMaxPositionValueRatio:  5,
			AltcoinMaxPositionValueRatio: 5,
			MaxMarginUsage:               1,
			MinPositionSize:              12,
			MinRiskRewardRatio:           1,
			MinConfidence:                50,
			MaxLeverage:                  5,
			ManagedCapitalUSDT:           300,
			MaxLossPerTradeUSDT:          10,
			AccountTakeProfitUSDT:        0,
			AccountStopLossUSDT:          0,
		},
	}
}
