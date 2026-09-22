package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	TradingSessionRunning  = "RUNNING"
	TradingSessionTPLocked = "ACCOUNT_TP_LOCKED"
	TradingSessionSLLocked = "ACCOUNT_SL_LOCKED"
	TradingSessionStopped  = "STOPPED"
)

type TradingSession struct {
	ID                        uint64  `gorm:"primaryKey" json:"id"`
	TraderID                  string  `gorm:"column:trader_id;not null;index" json:"trader_id"`
	StartedAt                 int64   `gorm:"column:started_at;not null;index" json:"started_at"`
	EndedAt                   int64   `gorm:"column:ended_at;default:0" json:"ended_at"`
	Status                    string  `gorm:"column:status;not null;index" json:"status"`
	StartEquityUSDT           float64 `gorm:"column:start_equity_usdt;not null;default:0" json:"start_equity_usdt"`
	StartAvailableBalanceUSDT float64 `gorm:"column:start_available_balance_usdt;not null;default:0" json:"start_available_balance_usdt"`
	ManagedCapitalUSDT        float64 `gorm:"column:managed_capital_usdt;not null" json:"managed_capital_usdt"`
	MaxLossPerTradeUSDT       float64 `gorm:"column:max_loss_per_trade_usdt;not null" json:"max_loss_per_trade_usdt"`
	MaxLeverage               int     `gorm:"column:max_leverage;not null" json:"max_leverage"`
	MaxPositions              int     `gorm:"column:max_positions;not null" json:"max_positions"`
	AccountTakeProfitUSDT     float64 `gorm:"column:account_take_profit_usdt;default:0" json:"account_take_profit_usdt"`
	AccountStopLossUSDT       float64 `gorm:"column:account_stop_loss_usdt;default:0" json:"account_stop_loss_usdt"`
	LastRealizedPnLUSDT       float64 `gorm:"column:last_realized_pnl_usdt;default:0" json:"last_realized_pnl_usdt"`
	LastUnrealizedPnLUSDT     float64 `gorm:"column:last_unrealized_pnl_usdt;default:0" json:"last_unrealized_pnl_usdt"`
	LastTradingPnLUSDT        float64 `gorm:"column:last_trading_pnl_usdt;default:0" json:"last_trading_pnl_usdt"`
	TriggerReason             string  `gorm:"column:trigger_reason;default:''" json:"trigger_reason"`
	LockedAt                  int64   `gorm:"column:locked_at;default:0" json:"locked_at"`
	CreatedAt                 int64   `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt                 int64   `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (TradingSession) TableName() string { return "trading_sessions" }

type TradingSessionStore struct{ db *gorm.DB }

func NewTradingSessionStore(db *gorm.DB) *TradingSessionStore { return &TradingSessionStore{db: db} }
func (s *TradingSessionStore) InitTables() error              { return s.db.AutoMigrate(&TradingSession{}) }

func (s *TradingSessionStore) GetLatest(traderID string) (*TradingSession, error) {
	var v TradingSession
	err := s.db.Where("trader_id = ?", traderID).Order("started_at DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

func (s *TradingSessionStore) GetRunning(traderID string) (*TradingSession, error) {
	var v TradingSession
	err := s.db.Where("trader_id = ? AND status = ?", traderID, TradingSessionRunning).Order("started_at DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

func (s *TradingSessionStore) Create(v *TradingSession) error {
	now := time.Now().UTC().UnixMilli()
	if v.StartedAt == 0 {
		v.StartedAt = now
	}
	if v.Status == "" {
		v.Status = TradingSessionRunning
	}
	v.CreatedAt = now
	v.UpdatedAt = now
	return s.db.Create(v).Error
}

func (s *TradingSessionStore) UpdatePnL(id uint64, realized, unrealized float64) error {
	return s.db.Model(&TradingSession{}).Where("id = ?", id).Updates(map[string]any{
		"last_realized_pnl_usdt": realized, "last_unrealized_pnl_usdt": unrealized,
		"last_trading_pnl_usdt": realized + unrealized, "updated_at": time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *TradingSessionStore) Lock(id uint64, status, reason string) error {
	now := time.Now().UTC().UnixMilli()
	return s.db.Model(&TradingSession{}).Where("id = ? AND status = ?", id, TradingSessionRunning).Updates(map[string]any{
		"status": status, "trigger_reason": reason, "locked_at": now, "updated_at": now,
	}).Error
}

func (s *PositionStore) SessionRealizedPnL(traderID string, startedAtMs int64) (pnl, fees float64, err error) {
	var row struct {
		PnL  float64
		Fees float64
	}
	err = s.db.Model(&TraderPosition{}).
		Select("COALESCE(SUM(realized_pnl),0) AS pnl, COALESCE(SUM(fee),0) AS fees").
		Where("trader_id = ? AND status = ? AND exit_time >= ?", traderID, "CLOSED", startedAtMs).
		Scan(&row).Error
	return row.PnL, row.Fees, err
}
