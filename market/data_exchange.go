package market

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nofx/logger"
	"nofx/provider/coinank/coinank_api"
	"nofx/provider/coinank/coinank_enum"
)

func getKlinesFromCoinAnkStrict(symbol, interval, exchange string, limit int) ([]Kline, error) {
	var coinankInterval coinank_enum.Interval
	switch interval {
	case "1m":
		coinankInterval = coinank_enum.Minute1
	case "3m":
		coinankInterval = coinank_enum.Minute3
	case "5m":
		coinankInterval = coinank_enum.Minute5
	case "15m":
		coinankInterval = coinank_enum.Minute15
	case "30m":
		coinankInterval = coinank_enum.Minute30
	case "1h":
		coinankInterval = coinank_enum.Hour1
	case "2h":
		coinankInterval = coinank_enum.Hour2
	case "4h":
		coinankInterval = coinank_enum.Hour4
	case "6h":
		coinankInterval = coinank_enum.Hour6
	case "8h":
		coinankInterval = coinank_enum.Hour8
	case "12h":
		coinankInterval = coinank_enum.Hour12
	case "1d":
		coinankInterval = coinank_enum.Day1
	case "3d":
		coinankInterval = coinank_enum.Day3
	case "1w":
		coinankInterval = coinank_enum.Week1
	default:
		return nil, fmt.Errorf("unsupported interval: %s", interval)
	}

	var coinankExchange coinank_enum.Exchange
	switch strings.ToLower(strings.TrimSpace(exchange)) {
	case "binance":
		coinankExchange = coinank_enum.Binance
	case "bybit":
		coinankExchange = coinank_enum.Bybit
	case "okx":
		coinankExchange = coinank_enum.Okex
	case "bitget":
		coinankExchange = coinank_enum.Bitget
	case "gate":
		coinankExchange = coinank_enum.Gate
	case "hyperliquid":
		coinankExchange = coinank_enum.Hyperliquid
	case "aster":
		coinankExchange = coinank_enum.Aster
	default:
		return nil, fmt.Errorf("unsupported ai_free kline exchange: %s", exchange)
	}

	ctx := context.Background()
	items, err := coinank_api.Kline(ctx, symbol, coinankExchange, time.Now().UnixMilli(), coinank_enum.To, limit, coinankInterval)
	if err != nil {
		return nil, fmt.Errorf("CoinAnk %s kline error: %w", exchange, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("CoinAnk %s returned empty kline data", exchange)
	}

	klines := make([]Kline, len(items))
	for i, ck := range items {
		klines[i] = Kline{
			OpenTime:  ck.StartTime,
			Open:      ck.Open,
			High:      ck.High,
			Low:       ck.Low,
			Close:     ck.Close,
			Volume:    ck.Volume,
			CloseTime: ck.EndTime,
		}
	}
	return klines, nil
}

// GetWithTimeframesAndExchange is the ai_free market-data path. It reuses
// NOFX indicator code while refusing to silently switch a non-Binance trader
// to Binance K-lines.
func GetWithTimeframesAndExchange(
	symbol string,
	timeframes []string,
	primaryTimeframe string,
	countByTimeframe map[string]int,
	exchange string,
) (*Data, error) {
	symbol = Normalize(symbol)
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	if exchange == "" {
		exchange = "binance"
	}
	if len(timeframes) == 0 {
		return nil, fmt.Errorf("at least one timeframe is required")
	}
	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	hasPrimary := false
	for _, tf := range timeframes {
		if tf == primaryTimeframe {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		timeframes = append([]string{primaryTimeframe}, timeframes...)
	}

	timeframeData := make(map[string]*TimeframeSeriesData)
	var primaryKlines []Kline
	isXyzAsset := IsXyzDexAsset(symbol)
	nowMs := time.Now().UTC().UnixMilli()

	for _, tf := range timeframes {
		count := countByTimeframe[tf]
		if count <= 0 {
			count = 30
		}
		fetchLimit := 200
		if fetchLimit < count {
			fetchLimit = count
		}

		var (
			klines []Kline
			err    error
		)
		if isXyzAsset || exchange == "hyperliquid" {
			klines, err = getKlinesFromHyperliquid(symbol, tf, fetchLimit)
		} else {
			klines, err = getKlinesFromCoinAnkStrict(symbol, tf, exchange, fetchLimit)
		}
		if err != nil {
			logger.Infof("⚠️ Failed to get %s %s K-line from %s: %v", symbol, tf, exchange, err)
			continue
		}

		klines = onlyClosedKlines(klines, nowMs)
		if len(klines) == 0 {
			logger.Infof("⚠️ %s %s has no closed K-line data", symbol, tf)
			continue
		}
		if tf == primaryTimeframe {
			primaryKlines = klines
		}

		tfData := calculateTimeframeSeries(klines, tf, count)
		if len(klines) >= 200 {
			tfData.EMA200Value = calculateEMA(klines, 200)
		}
		window := 20
		if len(klines) < window {
			window = len(klines)
		}
		if window > 0 {
			sum := 0.0
			for _, k := range klines[len(klines)-window:] {
				sum += k.Volume
			}
			tfData.VolumeMA20 = sum / float64(window)
		}
		if len(klines) >= 2 && klines[len(klines)-2].Volume > 0 {
			prev := klines[len(klines)-2].Volume
			tfData.VolumeChangePct = (klines[len(klines)-1].Volume - prev) / prev * 100
		}
		timeframeData[tf] = tfData
	}

	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("primary timeframe %s closed K-line data is empty", primaryTimeframe)
	}
	if isStaleData(primaryKlines, symbol) {
		return nil, fmt.Errorf("%s data is stale", symbol)
	}

	currentPrice := primaryKlines[len(primaryKlines)-1].Close
	var oiData *OIData
	var fundingRate float64

	// Existing shared OI/Funding helpers are Binance-native. For non-Binance
	// execution venues ai_free omits them rather than mislabeling Binance data.
	if exchange == "binance" {
		if oi, err := getOpenInterestData(symbol); err == nil {
			oiData = oi
		}
		if fr, err := getFundingRate(symbol); err == nil {
			fundingRate = fr
		}
	}

	return &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		PriceChange1h: calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60),
		PriceChange4h: calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240),
		CurrentEMA20:  calculateEMA(primaryKlines, 20),
		CurrentMACD:   calculateMACD(primaryKlines),
		CurrentRSI7:   calculateRSI(primaryKlines, 7),
		OpenInterest:  oiData,
		FundingRate:   fundingRate,
		TimeframeData: timeframeData,
	}, nil
}

func onlyClosedKlines(in []Kline, nowMs int64) []Kline {
	out := make([]Kline, 0, len(in))
	for _, k := range in {
		if k.CloseTime > 0 && k.CloseTime <= nowMs {
			out = append(out, k)
		}
	}
	return out
}
