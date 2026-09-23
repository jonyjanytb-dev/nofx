# Exact integration checklist

Base: NOFX `dev@638d4042118995fbf1a38d3822b1139aa3c6b467`.

## store/strategy.go

Add `DecisionMode` to StrategyConfig/AIStrategyConfig serialization. Extend `RiskControlConfig` with managed capital, per-trade max loss, max leverage and session TP/SL. Add a separate ai_free preset; do not replace legacy defaults.

Locked ai_free market contract:

```go
DecisionMode: "ai_free"
CoinSource.SourceType: "static"
StaticCoins: []string{"BTCUSDT","ETHUSDT","SOLUSDT","DOGEUSDT","HYPEUSDT","ZECUSDT"}
PrimaryTimeframe: "15m", PrimaryCount: 30
LongerTimeframe: "1h", LongerCount: 24
EnableMultiTimeframe: true
SelectedTimeframes: []string{"15m","1h"}
```

## kernel

- `engine_prompt.go`: ai_free branches to a neutral protocol prompt before legacy prompt generation.
- `engine_analysis.go`: ai_free reuses NOFX extraction but calls `validateAIFreeDecisions`, not the legacy mandatory-TP/RR validator.
- Context carries the execution exchange and market-price getter.
- ai_free skips the legacy OI candidate filter.

## trader

- DeepSeek `deepseek-v4-flash`, explicit non-thinking, 60s timeout, one attempt.
- 15m close-aligned loop.
- no legacy drawdown monitor in ai_free.
- no generic trade throttle or Vergex direction policy in ai_free.
- CLOSE before OPEN; no same-cycle flip; competing OPENs sort by confidence.
- same symbol cannot hold both directions.
- mandatory SL, optional TP.
- HOLD may tighten SL and modify/remove TP; SL may not be loosened.
- per-trade max loss and managed-margin pool can shrink requested notional.
- Session TP/SL uses trade PnL, persists lock, flattens and verifies exchange state.

## market

- ai_free K-lines are fetched for the actual execution exchange without silent Binance fallback.
- only closed 15m/1h candles are sent to the model.
- existing local indicator functions are reused.
- Binance OI/Funding are used only when Binance is the execution venue; unsupported same-exchange OI/Funding is omitted.

## TradingSession

`TradingSession` is lazily AutoMigrated when ai_free starts. This avoids changing legacy database initialization behavior.

## Required tests

1. prompt preserved verbatim
2. no hidden mandatory frequency/RR/confidence in ai_free prompt
3. SL mandatory, TP optional
4. leverage cap
5. max-loss shrinks notional
6. aggregate managed margin cap
7. no opposite/same-symbol simultaneous position
8. no same-cycle flip
9. CLOSE before OPEN; OPEN confidence ordering
10. deposits/withdrawals do not alter session trading PnL
11. account TP/SL lock survives restart and flattens
12. ai_free bypasses 90m/3h/4h throttle
13. legacy NOFX/Vergex tests unchanged
14. same-exchange Kline path and closed-bar filtering verified
15. malformed AI JSON = no new order
