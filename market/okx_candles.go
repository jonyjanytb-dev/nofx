package market

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OKX's public candle endpoint is the same venue as the customer's order
// execution. Do not substitute Binance candles when OKX market data is down.
func getKlinesFromOKX(symbol, interval string, limit int) ([]Kline, error) {
	baseURL := map[string]string{
		"eea": "https://eea.okx.com",
		"us":  "https://us.okx.com",
		"tr":  "https://tr.okx.com",
	}[strings.ToLower(strings.TrimSpace(os.Getenv("OKX_REGION")))]
	if baseURL == "" {
		baseURL = "https://www.okx.com"
	}
	return getKlinesFromOKXWithClient(symbol, interval, limit, baseURL, &http.Client{Timeout: 10 * time.Second})
}

func getKlinesFromOKXWithClient(symbol, interval string, limit int, baseURL string, client *http.Client) ([]Kline, error) {
	symbol = Normalize(symbol)
	base := strings.TrimSuffix(symbol, "USDT")
	if base == "" || base == symbol {
		return nil, fmt.Errorf("unsupported OKX swap symbol")
	}
	for _, char := range base {
		if !(char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return nil, fmt.Errorf("unsupported OKX swap symbol")
		}
	}
	bar := interval
	if strings.HasSuffix(bar, "h") {
		bar = strings.TrimSuffix(bar, "h") + "H"
	} else if strings.HasSuffix(bar, "d") {
		bar = strings.TrimSuffix(bar, "d") + "Dutc"
	}
	duration, err := TFDuration(interval)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 300 {
		limit = 300
	}
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/api/v5/market/candles")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("instId", base+"-USDT-SWAP")
	query.Set("bar", bar)
	query.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = query.Encode()

	var body []byte
	for attempt := 0; attempt < 3; attempt++ {
		response, requestErr := client.Get(endpoint.String())
		if requestErr != nil {
			if attempt == 2 {
				return nil, fmt.Errorf("OKX public candles request failed: %w", requestErr)
			}
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		body, err = io.ReadAll(io.LimitReader(response.Body, 2<<20))
		_ = response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("OKX public candles response failed: %w", err)
		}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout {
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
				continue
			}
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("OKX public candles HTTP %d", response.StatusCode)
		}
		break
	}
	var payload struct {
		Code string     `json:"code"`
		Data [][]string `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("OKX public candles JSON invalid: %w", err)
	}
	if payload.Code != "0" {
		return nil, fmt.Errorf("OKX public candles code %s", payload.Code)
	}
	klines := make([]Kline, 0, len(payload.Data))
	for _, row := range payload.Data {
		if len(row) < 9 || row[8] != "1" {
			continue
		}
		start, timeErr := strconv.ParseInt(row[0], 10, 64)
		open, openErr := strconv.ParseFloat(row[1], 64)
		high, highErr := strconv.ParseFloat(row[2], 64)
		low, lowErr := strconv.ParseFloat(row[3], 64)
		closePrice, closeErr := strconv.ParseFloat(row[4], 64)
		volume, volumeErr := strconv.ParseFloat(row[5], 64)
		if timeErr != nil || openErr != nil || highErr != nil || lowErr != nil || closeErr != nil || volumeErr != nil || start <= 0 || open <= 0 || high <= 0 || low <= 0 || closePrice <= 0 {
			return nil, fmt.Errorf("OKX public candles contain invalid OHLCV")
		}
		klines = append(klines, Kline{
			OpenTime: start, CloseTime: start + duration.Milliseconds() - 1,
			Open: open, High: high, Low: low, Close: closePrice, Volume: volume,
		})
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("OKX public candles returned no confirmed bars")
	}
	sort.Slice(klines, func(i, j int) bool { return klines[i].OpenTime < klines[j].OpenTime })
	return klines, nil
}
