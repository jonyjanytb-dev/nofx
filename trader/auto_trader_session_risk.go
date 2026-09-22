package trader

import (
	"fmt"
	"nofx/store"
	"strings"
	"time"
)

const aiFreeSessionRiskPoll = 10 * time.Second

func aiFreeTotalEquity(balance map[string]interface{}) float64 {
	if x, ok := balance["totalEquity"].(float64); ok && x > 0 {
		return x
	}
	wallet, _ := balance["totalWalletBalance"].(float64)
	unrealized, _ := balance["totalUnrealizedProfit"].(float64)
	return wallet + unrealized
}

func (at *AutoTrader) sessionStore() *store.TradingSessionStore {
	if at == nil || at.store == nil || at.store.GormDB() == nil {
		return nil
	}
	return store.NewTradingSessionStore(at.store.GormDB())
}

func (at *AutoTrader) ensureAIFreeSession() (*store.TradingSession, error) {
	if !at.isAIFreeMode() {
		return nil, nil
	}
	ss := at.sessionStore()
	if ss == nil {
		return nil, fmt.Errorf("ai_free requires persistent store")
	}
	if err := ss.InitTables(); err != nil {
		return nil, err
	}
	latest, err := ss.GetLatest(at.id)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		switch latest.Status {
		case store.TradingSessionRunning:
			return latest, nil
		case store.TradingSessionTPLocked, store.TradingSessionSLLocked:
			// A lock is persisted before flattening. If the process died between
			// those two steps, restart must finish the cleanup while keeping the
			// session locked; it must never silently resume trading.
			if cleanupErr := at.flattenLockedAIFreeSession(); cleanupErr != nil {
				return nil, fmt.Errorf("session locked: %s; cleanup failed: %w", latest.Status, cleanupErr)
			}
			return nil, fmt.Errorf("session locked: %s", latest.Status)
		}
	}

	r := at.config.StrategyConfig.RiskControl
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("read account balance at session start: %w", err)
	}
	startEquity := aiFreeTotalEquity(balance)
	if startEquity <= 0 {
		return nil, fmt.Errorf("account equity must be positive at session start")
	}
	startAvailable := 0.0
	if x, ok := balance["availableBalance"].(float64); ok {
		startAvailable = x
	}
	if r.ManagedCapitalUSDT <= 0 {
		return nil, fmt.Errorf("managed capital must be configured")
	}
	if r.ManagedCapitalUSDT > startEquity {
		return nil, fmt.Errorf("managed capital %.2f exceeds start equity %.2f", r.ManagedCapitalUSDT, startEquity)
	}

	v := &store.TradingSession{
		TraderID: at.id, Status: store.TradingSessionRunning,
		StartEquityUSDT: startEquity, StartAvailableBalanceUSDT: startAvailable,
		ManagedCapitalUSDT: r.ManagedCapitalUSDT, MaxLossPerTradeUSDT: r.MaxLossPerTradeUSDT,
		MaxLeverage: r.MaxLeverage, MaxPositions: r.MaxPositions,
		AccountTakeProfitUSDT: r.AccountTakeProfitUSDT, AccountStopLossUSDT: r.AccountStopLossUSDT,
	}
	if err := ss.Create(v); err != nil {
		return nil, err
	}
	return v, nil
}

func (at *AutoTrader) aiFreeSessionAllowsOpen() error {
	if !at.isAIFreeMode() {
		return nil
	}
	v, err := at.sessionStore().GetLatest(at.id)
	if err != nil {
		return err
	}
	if v == nil || v.Status != store.TradingSessionRunning {
		return fmt.Errorf("ai_free session is not RUNNING")
	}
	return nil
}

func (at *AutoTrader) startAIFreeSessionRiskMonitor() {
	if !at.isAIFreeMode() {
		return
	}
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()
		t := time.NewTicker(aiFreeSessionRiskPoll)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := at.checkAIFreeSessionRisk(); err != nil {
					at.logWarnf("⚠️ ai_free risk check: %v", err)
				}
			case <-at.stopMonitorCh:
				return
			}
		}
	}()
}

func (at *AutoTrader) checkAIFreeSessionRisk() error {
	ss := at.sessionStore()
	v, err := ss.GetRunning(at.id)
	if err != nil || v == nil {
		return err
	}
	realized, fees, err := at.store.Position().SessionRealizedPnL(at.id, v.StartedAt)
	if err != nil {
		return err
	}
	positions, err := at.trader.GetPositions()
	if err != nil {
		return err
	}
	unreal := 0.0
	for _, p := range positions {
		if x, ok := p["unRealizedProfit"].(float64); ok {
			unreal += x
		}
	}
	net := realized - fees
	if err := ss.UpdatePnL(v.ID, net, unreal); err != nil {
		return err
	}
	total := net + unreal
	if v.AccountTakeProfitUSDT > 0 && total >= v.AccountTakeProfitUSDT {
		return at.lockAndFlattenAIFreeSession(v, store.TradingSessionTPLocked, fmt.Sprintf("PnL %.4f reached TP %.4f", total, v.AccountTakeProfitUSDT))
	}
	if v.AccountStopLossUSDT > 0 && total <= -v.AccountStopLossUSDT {
		return at.lockAndFlattenAIFreeSession(v, store.TradingSessionSLLocked, fmt.Sprintf("PnL %.4f reached SL -%.4f", total, v.AccountStopLossUSDT))
	}
	return nil
}

func (at *AutoTrader) lockAndFlattenAIFreeSession(v *store.TradingSession, status, reason string) error {
	ss := at.sessionStore()
	if err := ss.Lock(v.ID, status, reason); err != nil {
		return err
	}
	return at.flattenLockedAIFreeSession()
}

func (at *AutoTrader) flattenLockedAIFreeSession() error {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return err
	}

	var first error
	touchedSymbols := make(map[string]struct{}, len(positions))
	for _, p := range positions {
		sym, _ := p["symbol"].(string)
		side, _ := p["side"].(string)
		qty, _ := p["positionAmt"].(float64)
		if qty < 0 {
			qty = -qty
		}
		if qty == 0 || sym == "" {
			continue
		}
		touchedSymbols[sym] = struct{}{}
		if e := at.trader.CancelAllOrders(sym); e != nil && first == nil {
			first = fmt.Errorf("cancel pending orders for %s: %w", sym, e)
		}
		if e := at.emergencyClosePositionAndVerify(sym, strings.ToLower(side), qty); e != nil && first == nil {
			first = e
		}
	}

	// Closing can leave exchange-side protection orders behind. Cancel them
	// again after flattening so a locked session has no residual orders.
	for sym := range touchedSymbols {
		if e := at.trader.CancelAllOrders(sym); e != nil && first == nil {
			first = fmt.Errorf("cancel residual orders for %s: %w", sym, e)
		}
	}

	after, e := at.trader.GetPositions()
	if e != nil {
		if first == nil {
			first = e
		}
		return first
	}
	for _, p := range after {
		q, _ := p["positionAmt"].(float64)
		if q != 0 && first == nil {
			first = fmt.Errorf("locked but position remains")
		}
	}
	return first
}

func (at *AutoTrader) aiFreeContextPnL(fallbackPnL, fallbackPct float64) (float64, float64) {
	if !at.isAIFreeMode() {
		return fallbackPnL, fallbackPct
	}
	ss := at.sessionStore()
	if ss == nil {
		return fallbackPnL, fallbackPct
	}
	v, err := ss.GetLatest(at.id)
	if err != nil || v == nil {
		return fallbackPnL, fallbackPct
	}
	pnl := v.LastTradingPnLUSDT
	pct := 0.0
	if v.ManagedCapitalUSDT > 0 {
		pct = pnl / v.ManagedCapitalUSDT * 100
	}
	return pnl, pct
}
