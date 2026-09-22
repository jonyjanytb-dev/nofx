# Live NOFX code audit

Audited against `jonyjanytb-dev/nofx@638d4042118995fbf1a38d3822b1139aa3c6b467`.

## Reuse directly

- `trader/auto_trader_loop.go`: live strategy reload, context, decision audit, Safe Mode, CLOSE-before-OPEN.
- `buildTradingContext()`: real balance/positions and `GetRecentTrades(at.id, 10)` already implemented.
- `trader/auto_trader_orders.go`: existing exchange adapters, order confirmation, max positions, mandatory exchange-side SL, emergency close when SL protection fails.
- `store/decision.go`, `store/order.go`, `store/position.go`: reuse.
- existing exchange sync/reconciliation: reuse; exchange remains position truth.

## Conflicts requiring ai_free override

- `kernel/engine_position.go`: current generic validator forces TP and hard 3:1 R/R and equity-ratio sizing.
- generic System Prompt injects platform strategy (frequency/entry/risk heuristics) instead of letting customer prompt lead.
- `auto_trader_throttle.go`: 90m minimum hold, 3h noise gate, 4h re-entry cooldown conflict with free AI behavior.
- existing drawdown monitor can autonomously close positions; disable only in ai_free.
- `market.GetWithTimeframes()` routes ordinary crypto multi-TF data to Binance/CoinAnk and Binance OI/Funding even when execution exchange differs.
- open-order code currently blocks same symbol only when the direction is the same; ai_free must block either direction.
- non-Vergex OPEN currently requires TP; ai_free TP must be optional.

## PnL accounting finding

`TraderPosition` stores `RealizedPnL`, `Fee`, and `ExitTime`. This supports session PnL by querying trade history since session start. Deposits/withdrawals therefore never need to enter risk PnL. Funding is not represented by one universal field on the shared Trader interface; do not invent it where an adapter cannot supply it reliably.
