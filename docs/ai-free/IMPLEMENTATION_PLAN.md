# AI Free Trader – locked implementation plan

## Architecture

```text
customer prompt + selected symbols + 15m/1h market context + account/positions + last 10 completed trades + hard risk budget
        ↓
DeepSeek Flash
        ↓
open_long/open_short/close_long/close_short/hold/wait
        ↓
ai_free Runtime risk gate
        ↓
existing NOFX exchange/order/protection/sync engine
```

Runtime may reduce or block risk. It must not add hidden alpha rules.

## Universe and data

- decision cycle: fixed 15m, aligned just after a closed 15m candle
- default symbols: BTC, ETH, SOL, DOGE, HYPE, ZEC; user may uncheck
- selected symbols are sent in one model request
- 15m: 30 closed bars
- 1h: 24 closed bars
- last 10 completed trades (already native in NOFX)
- local NOFX indicators are reused
- same-exchange Kline/volume must be used; same-exchange OI/Funding where available; otherwise omit instead of silently substituting Binance

## AI

- provider: DeepSeek
- model: `deepseek-flash`
- non-thinking
- 60s timeout
- one request attempt in a cycle
- malformed/timeout → safe no-new-trade behavior
- 3 consecutive AI failures → existing NOFX Safe Mode
- customer prompt is preserved verbatim
- platform prompt defines protocol/data/output/risk only, not trading strategy
- confidence is display/ranking only, not a hard open threshold

## Hard risk

Customer config:
- managed capital (margin pool) USDT
- max loss per trade USDT
- max leverage
- max positions
- session account TP USDT
- session account SL USDT

Per-trade OPEN risk reserve:

```text
entry→SL price loss + open/close fee reserve + slippage buffer
```

If requested notional exceeds the loss budget, shrink notional automatically. If it falls below exchange/NOFX minimum, reject.

Managed capital is aggregate margin, not aggregate notional.

## Position invariants

- at most one live position per symbol, regardless of direction
- no same-cycle reversal; close now, reconsider opposite side next 15m
- close decisions execute before opens
- when slots are scarce, open decisions use confidence descending; tie keeps AI output order
- AI may close before TP
- SL is mandatory; TP optional
- AI may tighten SL; may not widen original risk
- TP may be changed or removed while SL remains

## Session risk

Do not use equity delta. Use session trading history:

```text
realized trade PnL since session start + live unrealized PnL - reliably-accounted fees
```

Deposits/withdrawals/transfers do not affect this value and do not pause/restart the session.

A separate risk monitor triggers on total session TP/SL:
- persist lock first
- block new opens
- cancel pending orders
- flatten positions
- cancel residual orders
- verify flat from exchange truth
- lock survives process restart

## Legacy isolation

Only `decision_mode=ai_free` bypasses:
- Vergex directional enforcement
- NOFX generic trade throttle
- NOFX autonomous drawdown exit monitor
- generic prompt strategy prescriptions
- mandatory TP / mandatory 3:1 R/R
- equity-ratio sizing as primary sizing

All legacy NOFX modes remain unchanged.

## Backtest

True 7-day AI replay is Phase 2, not part of the first live-engine patch: 672 historical 15m cycles, no future data, same risk engine as live, one AI request per cycle containing all selected symbols.
