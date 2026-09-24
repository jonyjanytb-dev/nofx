package kernel

import (
	"nofx/store"
	"strings"
	"testing"
)

func TestAIFreePromptKeepsCustomerStrategyWithoutLegacyAlpha(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	cfg.CustomPrompt = "  Trade breakouts only when I say so.\nKeep this line exactly.  "
	eng := NewStrategyEngine(&cfg)
	got := eng.BuildSystemPrompt(1000, "balanced")
	if !strings.Contains(got, cfg.CustomPrompt) {
		t.Fatalf("customer prompt missing: %s", got)
	}
	for _, forbidden := range []string{"2-4 trades/day", "45-90 minutes", "confidence ≥", "3:1"} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(forbidden)) {
			t.Fatalf("legacy platform alpha leaked into ai_free prompt: %q", forbidden)
		}
	}
}

func TestAIFreePromptRequiresStructuredOpenOrderFields(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	eng := NewStrategyEngine(&cfg)
	prompt := eng.BuildSystemPrompt(20, "balanced")
	for _, required := range []string{
		"<reasoning>", "</reasoning>", "<decision>", "</decision>",
		`"action": "open_long"`, `"position_size_usd":`, `"stop_loss":`, `"reasoning":`,
		"position_size_usd is required for open_long and open_short",
		"If you cannot provide all required OPEN fields, output wait instead",
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("AI-free decision contract missing %q", required)
		}
	}
	example, err := extractDecisions(prompt)
	if err != nil || len(example) != 1 || example[0].PositionSizeUSD <= 0 || example[0].StopLoss <= 0 {
		t.Fatalf("AI-free prompt must contain a parseable NOFX OPEN example: decisions=%+v err=%v", example, err)
	}
}

func TestAIFreeOpenInProseWithoutOrderSizeStillFailsClosed(t *testing.T) {
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}}}
	response := `<reasoning>准备以 300 USDT 名义价值做空 BTC。</reasoning><decision>[{"symbol":"BTCUSDT","action":"open_short","leverage":5,"stop_loss":84500,"reasoning":"价格走弱"}]</decision>`
	_, err := parseAIFreeDecisionResponse(response, ctx, 5)
	if err == nil || !strings.Contains(err.Error(), "position_size_usd") {
		t.Fatalf("OPEN without structured position_size_usd must fail closed, got %v", err)
	}
}

func TestAIFreeStructuredOpenShortParses(t *testing.T) {
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}}}
	response := `<reasoning>BTC 走弱，计划做空。</reasoning><decision>[{"symbol":"BTCUSDT","action":"open_short","leverage":5,"position_size_usd":50,"stop_loss":84500,"take_profit":0,"reasoning":"价格走弱"}]</decision>`
	out, err := parseAIFreeDecisionResponse(response, ctx, 5)
	if err != nil || len(out.Decisions) != 1 || out.Decisions[0].PositionSizeUSD != 50 {
		t.Fatalf("valid structured OPEN must parse without guessing prose: decision=%+v err=%v", out, err)
	}
}

func TestAIFreeValidatorRequiresSLButAllowsNoTP(t *testing.T) {
	d := []Decision{{Symbol: "BTCUSDT", Action: "open_long", Leverage: 8, PositionSizeUSD: 100, StopLoss: 99, TakeProfit: 0, Reasoning: "test"}}
	if err := validateAIFreeDecisions(d, 5); err != nil {
		t.Fatal(err)
	}
	if d[0].Leverage != 5 {
		t.Fatalf("expected leverage clamp to 5, got %d", d[0].Leverage)
	}
	d[0].StopLoss = 0
	if err := validateAIFreeDecisions(d, 5); err == nil {
		t.Fatal("expected missing stop loss to fail")
	}
}

func TestCompleteAIFreeDecisionsFillsOmissionsSafely(t *testing.T) {
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}, {Symbol: "ETHUSDT"}}, Positions: []PositionInfo{{Symbol: "ETHUSDT", Side: "long"}}}
	out, err := completeAIFreeDecisions(ctx, []Decision{{Symbol: "BTCUSDT", Action: "wait", Reasoning: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(out))
	}
	foundHold := false
	for _, d := range out {
		if d.Symbol == "ETHUSDT" && d.Action == "hold" {
			foundHold = true
		}
	}
	if !foundHold {
		t.Fatalf("expected omitted live ETH position to safe HOLD, got %+v", out)
	}
}

func TestAIFreeMalformedResponseFallsBackToExplicitWait(t *testing.T) {
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}, {Symbol: "ETHUSDT"}}}
	out, err := parseAIFreeDecisionResponse("this is not decision json", ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Decisions) != 2 {
		t.Fatalf("expected explicit fallback for both symbols, got %+v", out.Decisions)
	}
	for _, d := range out.Decisions {
		if d.Action != "wait" {
			t.Fatalf("malformed response must not open a position: %+v", d)
		}
	}
}

func TestAIFreeAcceptsFractionalConfidenceAsPercentage(t *testing.T) {
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}}}
	response := `<reasoning>等待更清晰的行情。</reasoning><decision>[{"symbol":"BTCUSDT","action":"wait","confidence":0.6,"reasoning":"行情不足，继续等待。"}]</decision>`
	out, err := parseAIFreeDecisionResponse(response, ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Decisions) != 1 || out.Decisions[0].Confidence != 60 || out.Decisions[0].Action != "wait" {
		t.Fatalf("unexpected parsed decision: %+v", out.Decisions)
	}
}

func TestAIFreeRejectsDecisionWithoutMarketData(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)
	ctx := &Context{CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT"}}}
	if err := requireAIFreeMarketData(ctx, engine); err == nil {
		t.Fatal("AI must not decide or place orders when every market candle feed is empty")
	}
}

func TestAIFreePresetUsesLockedTimeframesAndUniverse(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	if cfg.DecisionMode != "ai_free" || cfg.Indicators.Klines.PrimaryTimeframe != "15m" || cfg.Indicators.Klines.PrimaryCount != 30 {
		t.Fatalf("unexpected primary config: %+v", cfg)
	}
	if cfg.Indicators.Klines.LongerTimeframe != "1h" || cfg.Indicators.Klines.LongerCount != 24 {
		t.Fatalf("unexpected longer timeframe config: %+v", cfg.Indicators.Klines)
	}
	if len(cfg.CoinSource.StaticCoins) != 6 {
		t.Fatalf("expected six default symbols, got %+v", cfg.CoinSource.StaticCoins)
	}
}

func TestLegacyDefaultDoesNotEnableAIFree(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	if cfg.DecisionMode != "" {
		t.Fatalf("legacy default unexpectedly changed decision mode: %q", cfg.DecisionMode)
	}
	eng := NewStrategyEngine(&cfg)
	prompt := eng.BuildSystemPrompt(1000, "balanced")
	if prompt == "" {
		t.Fatal("legacy NOFX prompt unexpectedly empty")
	}
	if strings.Contains(prompt, "The customer's trading instructions below define the trading philosophy") {
		t.Fatal("legacy NOFX unexpectedly routed through ai_free prompt")
	}
}
