package trader

import (
	"fmt"
	"math"
	"nofx/kernel"
	"nofx/market"
	"strings"
)

const aiFreeSlippageBufferRate = 0.0015

type aiFreeRiskAdjustment struct {
	RequestedNotional float64
	AllowedNotional float64
	EstimatedRiskUSDT float64
	RemainingMargin float64
	Adjusted bool
}

func (at *AutoTrader) isAIFreeMode() bool {
	return at != nil && at.config.StrategyConfig != nil && strings.EqualFold(strings.TrimSpace(at.config.StrategyConfig.DecisionMode), "ai_free")
}

func positionMapMarginUsed(pos map[string]interface{}) float64 {
	qty, _ := pos["positionAmt"].(float64)
	if qty < 0 { qty = -qty }
	mark, _ := pos["markPrice"].(float64)
	lev := 1.0
	if v, ok := pos["leverage"].(float64); ok && v > 0 { lev = v }
	if qty <= 0 || mark <= 0 { return 0 }
	return qty * mark / lev
}

func sameSymbolPositionExists(positions []map[string]interface{}, symbol string) bool {
	target := market.Normalize(symbol)
	for _, p := range positions {
		raw, _ := p["symbol"].(string)
		qty, _ := p["positionAmt"].(float64)
		if qty != 0 && market.Normalize(raw) == target { return true }
	}
	return false
}

func validateAIFreeProtectionPrices(action string, marketPrice, sl, tp float64) error {
	if marketPrice <= 0 || sl <= 0 { return fmt.Errorf("market price and mandatory stop loss must be positive") }
	switch action {
	case "open_long":
		if sl >= marketPrice { return fmt.Errorf("long stop must be below market") }
		if tp > 0 && tp <= marketPrice { return fmt.Errorf("long take profit must be above market") }
	case "open_short":
		if sl <= marketPrice { return fmt.Errorf("short stop must be above market") }
		if tp > 0 && tp >= marketPrice { return fmt.Errorf("short take profit must be below market") }
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	return nil
}

func (at *AutoTrader) applyAIFreeOpenRiskBudget(d *kernel.Decision, currentPrice, availableBalance float64, positions []map[string]interface{}) (*aiFreeRiskAdjustment, error) {
	if !at.isAIFreeMode() { return nil, nil }
	r := at.config.StrategyConfig.RiskControl
	if r.ManagedCapitalUSDT <= 0 { return nil, fmt.Errorf("managed capital must be configured") }
	if r.MaxLossPerTradeUSDT <= 0 { return nil, fmt.Errorf("max loss per trade must be configured") }
	if r.MaxLeverage > 0 && d.Leverage > r.MaxLeverage { d.Leverage = r.MaxLeverage }
	if d.Leverage <= 0 { return nil, fmt.Errorf("leverage must be positive") }

	var stopMove float64
	switch d.Action {
	case "open_long":
		if d.StopLoss >= currentPrice { return nil, fmt.Errorf("invalid long stop") }
		stopMove = (currentPrice - d.StopLoss) / currentPrice
	case "open_short":
		if d.StopLoss <= currentPrice { return nil, fmt.Errorf("invalid short stop") }
		stopMove = (d.StopLoss - currentPrice) / currentPrice
	default:
		return nil, fmt.Errorf("risk budget applies to OPEN")
	}

	riskRate := stopMove + 2*takerFeeRate + aiFreeSlippageBufferRate
	maxByLoss := r.MaxLossPerTradeUSDT / riskRate
	used := 0.0
	for _, p := range positions { used += positionMapMarginUsed(p) }
	remaining := math.Max(0, r.ManagedCapitalUSDT-used)
	maxByManaged := remaining * float64(d.Leverage)
	marginFactor := marginOverheadFactor/float64(d.Leverage) + takerFeeRate
	maxByBalance := availableBalance / marginFactor * positionSizeSafetyFactor

	allowed := d.PositionSizeUSD
	if allowed > maxByLoss { allowed = maxByLoss }
	if allowed > maxByManaged { allowed = maxByManaged }
	if allowed > maxByBalance { allowed = maxByBalance }
	if allowed <= 0 { return nil, fmt.Errorf("no margin available") }

	out := &aiFreeRiskAdjustment{
		RequestedNotional: d.PositionSizeUSD,
		AllowedNotional: allowed,
		EstimatedRiskUSDT: allowed*riskRate,
		RemainingMargin: remaining,
		Adjusted: allowed+1e-9 < d.PositionSizeUSD,
	}
	d.PositionSizeUSD = allowed
	return out, nil
}
