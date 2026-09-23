package market

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOnlyClosedKlines(t *testing.T) {
	in := []Kline{{CloseTime: 1000, Close: 1}, {CloseTime: 2000, Close: 2}, {CloseTime: 3000, Close: 3}}
	out := onlyClosedKlines(in, 2500)
	if len(out) != 2 || out[1].Close != 2 {
		t.Fatalf("unexpected closed bars: %+v", out)
	}
}

func TestStrictCoinAnkRejectsUnknownExchangeInsteadOfFallingBack(t *testing.T) {
	if _, err := getKlinesFromCoinAnkStrict("BTCUSDT", "15m", "definitely-unsupported", 30); err == nil {
		t.Fatal("ai_free strict market path must not silently fall back to Binance")
	}
}

func TestOKXPublicCandlesUseSwapMarketAndClosedBars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/candles" || r.URL.Query().Get("instId") != "BTC-USDT-SWAP" || r.URL.Query().Get("bar") != "15m" {
			t.Errorf("unexpected public candle request: %s", r.URL.String())
		}
		if r.Header.Get("OK-ACCESS-KEY") != "" {
			t.Error("public candle request must not carry trading credentials")
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[["1800000900000","12","13","11","12.5","5","0","0","0"],["1800000000000","10","12","9","11","4","0","0","1"]]}`))
	}))
	defer server.Close()

	got, err := getKlinesFromOKXWithClient("BTCUSDT", "15m", 30, server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Open != 10 || got[0].Close != 11 || got[0].CloseTime <= got[0].OpenTime {
		t.Fatalf("expected one chronological confirmed candle, got %+v", got)
	}
}

func TestOKXPublicCandlesIntegration(t *testing.T) {
	if os.Getenv("HERMAN_OKX_PUBLIC_CANDLES_TEST") != "1" {
		t.Skip("public read-only OKX network test is opt-in")
	}
	got, err := getKlinesFromOKX("BTCUSDT", "15m", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 30 {
		t.Fatalf("expected enough closed OKX 15m candles for the AI prompt, got %d", len(got))
	}
}
