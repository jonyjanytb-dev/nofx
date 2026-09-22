package kernel

import "nofx/market"

func fetchMarketDataForStrategySymbol(
	ctx *Context,
	engine *StrategyEngine,
	symbol string,
	timeframes []string,
	primaryTimeframe string,
	primaryCount int,
) (*market.Data, error) {
	if engine == nil || !engine.usesAIFreeMode() {
		return market.GetWithTimeframes(symbol, timeframes, primaryTimeframe, primaryCount)
	}

	cfg := engine.GetConfig().Indicators.Klines
	counts := make(map[string]int, len(timeframes))
	for _, tf := range timeframes {
		counts[tf] = primaryCount
	}
	if cfg.PrimaryTimeframe != "" && cfg.PrimaryCount > 0 {
		counts[cfg.PrimaryTimeframe] = cfg.PrimaryCount
	}
	if cfg.LongerTimeframe != "" && cfg.LongerCount > 0 {
		counts[cfg.LongerTimeframe] = cfg.LongerCount
	}

	data, err := market.GetWithTimeframesAndExchange(
		symbol,
		timeframes,
		primaryTimeframe,
		counts,
		ctx.MarketExchange,
	)
	if err != nil {
		return nil, err
	}
	if ctx.MarketPriceGetter != nil {
		if price, priceErr := ctx.MarketPriceGetter(symbol); priceErr == nil && price > 0 {
			data.CurrentPrice = price
		}
	}
	return data, nil
}
