package trader

import (
	"fmt"
	"math"
	"nofx/kernel"
	"nofx/market"
	"strings"
)

func canTightenAIFreeStop(side string, currentStop, requestedStop float64) bool {
	if currentStop <= 0 || requestedStop <= 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "long":
		return requestedStop >= currentStop
	case "short":
		return requestedStop <= currentStop
	default:
		return false
	}
}

func stopPriceFromOpenOrders(orders []OpenOrder) float64 {
	for _, order := range orders {
		t := strings.ToUpper(strings.TrimSpace(order.Type))
		if strings.Contains(t, "STOP") && !strings.Contains(t, "TAKE_PROFIT") {
			if order.StopPrice > 0 {
				return order.StopPrice
			}
			if order.Price > 0 {
				return order.Price
			}
		}
	}
	return 0
}

func findLivePositionForAIFree(positions []map[string]interface{}, symbol string) (string, float64, bool) {
	target := market.Normalize(symbol)
	for _, pos := range positions {
		raw, _ := pos["symbol"].(string)
		if market.Normalize(raw) != target {
			continue
		}
		qty, _ := pos["positionAmt"].(float64)
		if qty == 0 {
			continue
		}
		if qty < 0 {
			qty = -qty
		}
		side, _ := pos["side"].(string)
		return strings.ToLower(strings.TrimSpace(side)), qty, true
	}
	return "", 0, false
}

func (at *AutoTrader) maintainAIFreeProtection(d *kernel.Decision) error {
	if !at.isAIFreeMode() || d == nil || (d.UpdateStopLoss == nil && d.UpdateTakeProfit == nil) {
		return nil
	}

	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("get positions for protection update: %w", err)
	}
	side, qty, ok := findLivePositionForAIFree(positions, d.Symbol)
	if !ok {
		return fmt.Errorf("cannot update protection: no live %s position", d.Symbol)
	}

	marketPrice, err := at.trader.GetMarketPrice(d.Symbol)
	if err != nil {
		return fmt.Errorf("get market price for protection update: %w", err)
	}
	orders, err := at.trader.GetOpenOrders(d.Symbol)
	if err != nil {
		return fmt.Errorf("get protection orders: %w", err)
	}
	currentStop := stopPriceFromOpenOrders(orders)

	if d.UpdateStopLoss != nil {
		requested := *d.UpdateStopLoss
		if currentStop <= 0 {
			return fmt.Errorf("cannot change stop: current exchange stop not found")
		}
		if !canTightenAIFreeStop(side, currentStop, requested) {
			return fmt.Errorf("stop update would increase risk: current %.8f requested %.8f", currentStop, requested)
		}
		if side == "long" && requested >= marketPrice {
			return fmt.Errorf("long stop %.8f must remain below market %.8f", requested, marketPrice)
		}
		if side == "short" && requested <= marketPrice {
			return fmt.Errorf("short stop %.8f must remain above market %.8f", requested, marketPrice)
		}
		if math.Abs(requested-currentStop) > 1e-12 {
			if err := at.trader.CancelStopLossOrders(d.Symbol); err != nil {
				return fmt.Errorf("cancel old stop: %w", err)
			}
			if err := at.trader.SetStopLoss(d.Symbol, strings.ToUpper(side), qty, requested); err != nil {
				if restoreErr := at.trader.SetStopLoss(d.Symbol, strings.ToUpper(side), qty, currentStop); restoreErr != nil {
					return at.closeUnprotectedPosition(d.Symbol, side, qty, fmt.Errorf("new stop failed: %v; restore old stop failed: %w", err, restoreErr))
				}
				return fmt.Errorf("new stop failed; old stop restored: %w", err)
			}
		}
	}

	if d.UpdateTakeProfit != nil {
		requested := *d.UpdateTakeProfit
		if requested < 0 {
			return fmt.Errorf("take profit update cannot be negative")
		}
		if requested > 0 {
			if side == "long" && requested <= marketPrice {
				return fmt.Errorf("long take profit %.8f must be above market %.8f", requested, marketPrice)
			}
			if side == "short" && requested >= marketPrice {
				return fmt.Errorf("short take profit %.8f must be below market %.8f", requested, marketPrice)
			}
		}
		if err := at.trader.CancelTakeProfitOrders(d.Symbol); err != nil {
			return fmt.Errorf("cancel old take profit: %w", err)
		}
		if requested > 0 {
			if err := at.trader.SetTakeProfit(d.Symbol, strings.ToUpper(side), qty, requested); err != nil {
				return fmt.Errorf("set optional take profit: %w", err)
			}
		}
	}
	return nil
}
