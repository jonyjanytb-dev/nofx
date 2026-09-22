package kernel

import (
	"fmt"
	"strings"
)

func (e *StrategyEngine) usesAIFreeMode() bool {
	return e != nil && e.config != nil && strings.EqualFold(strings.TrimSpace(e.config.DecisionMode), "ai_free")
}

func (e *StrategyEngine) buildAIFreeSystemPrompt() string {
	r := e.config.RiskControl
	return fmt.Sprintf(`You are an autonomous cryptocurrency trading agent.
The customer's instructions below define the trading philosophy and style. Do not replace them with a platform trading strategy.

<CUSTOMER_TRADING_INSTRUCTIONS>
%s
</CUSTOMER_TRADING_INSTRUCTIONS>

HARD RUNTIME BOUNDARIES
- managed margin pool: %.2f USDT
- maximum loss budget per trade: %.2f USDT
- maximum leverage: %dx
- maximum simultaneous positions: %d
- session take-profit: %.2f USDT
- session stop-loss: %.2f USDT
These boundaries are absolute. Runtime independently recalculates risk and may reduce or block an OPEN.

DECISION AUTHORITY
You may independently decide whether to trade, which selected symbol to trade, long or short, leverage, requested notional size, stop-loss, optional take-profit, hold and close timing.

INTERFACE RULES
1. Base decisions only on supplied data. Do not invent missing data.
2. OPEN requires a valid stop_loss.
3. take_profit is optional and may be 0.
4. One live position per symbol. Do not request hedged or stacked positions.
5. Do not request an immediate same-cycle reversal. Close first; reconsider next cycle.
6. Compare all selected symbols before consuming limited position slots.
7. Return one decision for every selected symbol. Existing positions require HOLD or the matching CLOSE action. Flat symbols require OPEN or WAIT.
8. For HOLD, you may optionally send update_stop_loss and update_take_profit. Omit a field to leave it unchanged. update_take_profit=0 explicitly removes TP. Runtime allows stop changes only when they reduce risk.
9. Confidence is descriptive; it never overrides Runtime risk.
10. Keep the visible analysis concise. Do not expose hidden/private chain-of-thought.

Allowed actions: open_long, open_short, close_long, close_short, hold, wait.

Return concise user-visible analysis in <reasoning> and a strict JSON array in <decision>.`,
		strings.TrimSpace(e.config.CustomPrompt),
		r.ManagedCapitalUSDT,
		r.MaxLossPerTradeUSDT,
		r.MaxLeverage,
		r.MaxPositions,
		r.AccountTakeProfitUSDT,
		r.AccountStopLossUSDT,
	)
}

func validateAIFreeDecisions(decisions []Decision, maxLeverage int) error {
	for i := range decisions {
		d := &decisions[i]
		d.Action = strings.ToLower(strings.TrimSpace(d.Action))
		d.Symbol = strings.TrimSpace(d.Symbol)
		switch d.Action {
		case "open_long", "open_short", "close_long", "close_short", "hold", "wait":
		default:
			return fmt.Errorf("decision #%d invalid action %q", i+1, d.Action)
		}
		if d.Symbol == "" {
			return fmt.Errorf("decision #%d symbol required", i+1)
		}
		if strings.TrimSpace(d.Reasoning) == "" {
			return fmt.Errorf("decision #%d reasoning required", i+1)
		}
		if d.Action == "hold" {
			if d.UpdateStopLoss != nil && *d.UpdateStopLoss <= 0 {
				return fmt.Errorf("decision #%d update_stop_loss must be >0 when supplied", i+1)
			}
			if d.UpdateTakeProfit != nil && *d.UpdateTakeProfit < 0 {
				return fmt.Errorf("decision #%d update_take_profit cannot be negative", i+1)
			}
		}
		if d.Action != "open_long" && d.Action != "open_short" {
			continue
		}
		if d.Leverage <= 0 {
			return fmt.Errorf("decision #%d leverage must be >0", i+1)
		}
		if maxLeverage > 0 && d.Leverage > maxLeverage {
			d.Leverage = maxLeverage
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("decision #%d position_size_usd must be >0", i+1)
		}
		if d.StopLoss <= 0 {
			return fmt.Errorf("decision #%d stop_loss is mandatory", i+1)
		}
	}
	return nil
}

func parseAIFreeDecisionResponse(aiResponse string, ctx *Context, maxLeverage int) (*FullDecision, error) {
	cotTrace := extractCoTTrace(aiResponse)
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{CoTTrace: cotTrace}, err
	}
	if len(decisions) == 1 && strings.EqualFold(decisions[0].Symbol, "ALL") && decisions[0].Action == "wait" {
		decisions = nil
	}
	if err := validateAIFreeDecisions(decisions, maxLeverage); err != nil {
		return &FullDecision{CoTTrace: cotTrace, Decisions: decisions}, err
	}
	decisions, err = completeAIFreeDecisions(ctx, decisions)
	if err != nil {
		return &FullDecision{CoTTrace: cotTrace, Decisions: decisions}, err
	}
	return &FullDecision{CoTTrace: cotTrace, Decisions: decisions}, nil
}

func completeAIFreeDecisions(ctx *Context, in []Decision) ([]Decision, error) {
	if ctx == nil {
		return in, nil
	}
	positionSide := make(map[string]string, len(ctx.Positions))
	canonical := make(map[string]string, len(ctx.Positions)+len(ctx.CandidateCoins))
	order := make([]string, 0, len(ctx.Positions)+len(ctx.CandidateCoins))
	seenOrder := map[string]bool{}
	add := func(symbol string) {
		key := strings.ToUpper(strings.TrimSpace(symbol))
		if key == "" || seenOrder[key] {
			return
		}
		seenOrder[key] = true
		canonical[key] = symbol
		order = append(order, key)
	}
	for _, p := range ctx.Positions {
		key := strings.ToUpper(strings.TrimSpace(p.Symbol))
		positionSide[key] = strings.ToLower(strings.TrimSpace(p.Side))
		add(p.Symbol)
	}
	for _, c := range ctx.CandidateCoins {
		add(c.Symbol)
	}
	bySymbol := make(map[string]Decision, len(in))
	for _, d := range in {
		key := strings.ToUpper(strings.TrimSpace(d.Symbol))
		if key == "" {
			continue
		}
		if _, exists := bySymbol[key]; exists {
			return nil, fmt.Errorf("duplicate AI decisions for %s", d.Symbol)
		}
		bySymbol[key] = d
	}
	out := make([]Decision, 0, len(order))
	for _, key := range order {
		if d, ok := bySymbol[key]; ok {
			out = append(out, d)
			continue
		}
		symbol := canonical[key]
		if _, hasPosition := positionSide[key]; hasPosition {
			out = append(out, Decision{Symbol: symbol, Action: "hold", Reasoning: "Model omitted this existing position; safe fallback is HOLD."})
			continue
		}
		out = append(out, Decision{Symbol: symbol, Action: "wait", Reasoning: "Model omitted this selected symbol; safe fallback is WAIT."})
	}
	return out, nil
}
