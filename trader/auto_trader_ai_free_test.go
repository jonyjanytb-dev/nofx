package trader

import (
	"path/filepath"
	"testing"
	"time"

	"nofx/kernel"
	"nofx/store"
)

func TestSortAIFreeDecisionsCloseFirstNoSameCycleFlip(t *testing.T) {
	in := []kernel.Decision{
		{Symbol: "SOLUSDT", Action: "open_long", Confidence: 70},
		{Symbol: "ETHUSDT", Action: "open_short", Confidence: 90},
		{Symbol: "BTCUSDT", Action: "close_long"},
		{Symbol: "BTCUSDT", Action: "open_short", Confidence: 99},
	}
	out := sortAIFreeDecisions(in)
	if len(out) != 3 {
		t.Fatalf("same-cycle BTC flip should be removed, got %+v", out)
	}
	if out[0].Action != "close_long" {
		t.Fatalf("close must be first: %+v", out)
	}
	if out[1].Symbol != "ETHUSDT" || out[2].Symbol != "SOLUSDT" {
		t.Fatalf("opens should be confidence-desc: %+v", out)
	}
}

func TestNext15mCloseDelay(t *testing.T) {
	now := time.Date(2026, 9, 23, 3, 7, 20, 0, time.UTC)
	d := next15mCloseDelay(now)
	want := 7*time.Minute + 43*time.Second
	if d != want {
		t.Fatalf("got %v want %v", d, want)
	}
}

func TestAIFreeRiskShrinksNotional(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	cfg.RiskControl.ManagedCapitalUSDT = 100
	cfg.RiskControl.MaxLossPerTradeUSDT = 5
	cfg.RiskControl.MaxLeverage = 5
	at := &AutoTrader{config: AutoTraderConfig{StrategyConfig: &cfg}}
	d := &kernel.Decision{Symbol: "BTCUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 500, StopLoss: 95}
	adj, err := at.applyAIFreeOpenRiskBudget(d, 100, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !adj.Adjusted || d.PositionSizeUSD >= 500 {
		t.Fatalf("expected risk shrink, got %+v decision=%+v", adj, d)
	}
	if adj.EstimatedRiskUSDT > 5.000001 {
		t.Fatalf("estimated risk exceeds hard max: %+v", adj)
	}
}

func TestAIFreeProtectionOptionalTP(t *testing.T) {
	if err := validateAIFreeProtectionPrices("open_long", 100, 95, 0); err != nil {
		t.Fatal(err)
	}
	if err := validateAIFreeProtectionPrices("open_short", 100, 105, 0); err != nil {
		t.Fatal(err)
	}
}

func TestCanTightenAIFreeStop(t *testing.T) {
	if !canTightenAIFreeStop("long", 95, 97) {
		t.Fatal("long stop should tighten upward")
	}
	if canTightenAIFreeStop("long", 95, 94) {
		t.Fatal("long stop must not loosen downward")
	}
	if !canTightenAIFreeStop("short", 105, 103) {
		t.Fatal("short stop should tighten downward")
	}
	if canTightenAIFreeStop("short", 105, 106) {
		t.Fatal("short stop must not loosen upward")
	}
}

func TestSameSymbolPositionExistsRegardlessOfSide(t *testing.T) {
	positions := []map[string]interface{}{{"symbol": "BTCUSDT", "side": "short", "positionAmt": 0.01}}
	if !sameSymbolPositionExists(positions, "BTCUSDT") {
		t.Fatal("same symbol must be occupied regardless of side")
	}
}

func TestAIFreeManagedCapitalCapsAggregateMargin(t *testing.T) {
	cfg := store.GetAIFreeStrategyConfig("en")
	cfg.RiskControl.ManagedCapitalUSDT = 100
	cfg.RiskControl.MaxLossPerTradeUSDT = 100
	cfg.RiskControl.MaxLeverage = 5
	at := &AutoTrader{config: AutoTraderConfig{StrategyConfig: &cfg}}
	positions := []map[string]interface{}{{"symbol": "ETHUSDT", "positionAmt": 1.0, "markPrice": 400.0, "leverage": 5.0}}
	d := &kernel.Decision{Symbol: "BTCUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 500, StopLoss: 99}
	adj, err := at.applyAIFreeOpenRiskBudget(d, 100, 1000, positions)
	if err != nil {
		t.Fatal(err)
	}
	if adj.AllowedNotional > 100.000001 {
		t.Fatalf("managed-capital cap exceeded: %+v", adj)
	}
}

type fakeAIFreeTrader struct {
	positions      []map[string]interface{}
	cancelAllCalls int
}

func (f *fakeAIFreeTrader) GetBalance() (map[string]interface{}, error) {
	return map[string]interface{}{"totalEquity": 100.0, "availableBalance": 100.0}, nil
}
func (f *fakeAIFreeTrader) GetPositions() ([]map[string]interface{}, error) { return f.positions, nil }
func (f *fakeAIFreeTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (f *fakeAIFreeTrader) OpenShort(string, float64, int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (f *fakeAIFreeTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	f.positions = nil
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (f *fakeAIFreeTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	f.positions = nil
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (f *fakeAIFreeTrader) SetLeverage(string, int) error                        { return nil }
func (f *fakeAIFreeTrader) SetMarginMode(string, bool) error                     { return nil }
func (f *fakeAIFreeTrader) GetMarketPrice(string) (float64, error)               { return 100, nil }
func (f *fakeAIFreeTrader) SetStopLoss(string, string, float64, float64) error   { return nil }
func (f *fakeAIFreeTrader) SetTakeProfit(string, string, float64, float64) error { return nil }
func (f *fakeAIFreeTrader) CancelStopLossOrders(string) error                    { return nil }
func (f *fakeAIFreeTrader) CancelTakeProfitOrders(string) error                  { return nil }
func (f *fakeAIFreeTrader) CancelAllOrders(string) error {
	f.cancelAllCalls++
	return nil
}
func (f *fakeAIFreeTrader) CancelStopOrders(string) error                        { return nil }
func (f *fakeAIFreeTrader) FormatQuantity(string, float64) (string, error)       { return "1", nil }
func (f *fakeAIFreeTrader) GetOrderStatus(string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "FILLED"}, nil
}
func (f *fakeAIFreeTrader) GetClosedPnL(time.Time, int) ([]ClosedPnLRecord, error) { return nil, nil }
func (f *fakeAIFreeTrader) GetOpenOrders(string) ([]OpenOrder, error)              { return nil, nil }

func TestAIFreeAccountTPLocksAndFlattens(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "ai-free.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := store.GetAIFreeStrategyConfig("en")
	cfg.RiskControl.ManagedCapitalUSDT = 100
	cfg.RiskControl.AccountTakeProfitUSDT = 10
	fake := &fakeAIFreeTrader{positions: []map[string]interface{}{{"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "unRealizedProfit": 11.0}}}
	at := &AutoTrader{id: "t1", config: AutoTraderConfig{StrategyConfig: &cfg}, trader: fake, store: st, exchange: "binance"}
	if _, err := at.ensureAIFreeSession(); err != nil {
		t.Fatal(err)
	}
	if err := at.checkAIFreeSessionRisk(); err != nil {
		t.Fatal(err)
	}
	if len(fake.positions) != 0 {
		t.Fatalf("expected positions flattened, got %+v", fake.positions)
	}
	latest, err := at.sessionStore().GetLatest("t1")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Status != store.TradingSessionTPLocked {
		t.Fatalf("expected TP lock, got %+v", latest)
	}
	if err := at.aiFreeSessionAllowsOpen(); err == nil {
		t.Fatal("locked session must block new opens")
	}
}


func TestAIFreeLockedSessionRestartFinishesFlattenAndStaysLocked(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "ai-free-restart.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := store.GetAIFreeStrategyConfig("en")
	cfg.RiskControl.ManagedCapitalUSDT = 100
	fake := &fakeAIFreeTrader{
		positions: []map[string]interface{}{{
			"symbol": "BTCUSDT", "side": "long", "positionAmt": 0.1, "unRealizedProfit": 0.0,
		}},
	}
	at := &AutoTrader{id: "restart-t1", config: AutoTraderConfig{StrategyConfig: &cfg}, trader: fake, store: st, exchange: "binance"}

	ss := at.sessionStore()
	if err := ss.InitTables(); err != nil {
		t.Fatal(err)
	}
	session := &store.TradingSession{
		TraderID: "restart-t1", Status: store.TradingSessionRunning,
		ManagedCapitalUSDT: 100, MaxLossPerTradeUSDT: 5, MaxLeverage: 5, MaxPositions: 3,
	}
	if err := ss.Create(session); err != nil {
		t.Fatal(err)
	}
	if err := ss.Lock(session.ID, store.TradingSessionSLLocked, "persisted before crash"); err != nil {
		t.Fatal(err)
	}

	if _, err := at.ensureAIFreeSession(); err == nil {
		t.Fatal("locked session must remain locked on restart")
	}
	if len(fake.positions) != 0 {
		t.Fatalf("restart cleanup must flatten residual positions, got %+v", fake.positions)
	}
	if fake.cancelAllCalls < 2 {
		t.Fatalf("expected pending and residual order cancellation, got %d calls", fake.cancelAllCalls)
	}
	latest, err := ss.GetLatest("restart-t1")
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Status != store.TradingSessionSLLocked {
		t.Fatalf("restart cleanup must preserve lock, got %+v", latest)
	}
}
