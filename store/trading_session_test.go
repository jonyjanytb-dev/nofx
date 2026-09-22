package store

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestTradingSessionLockSurvivesDBReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ss := NewTradingSessionStore(db)
	if err := ss.InitTables(); err != nil {
		t.Fatal(err)
	}
	s := &TradingSession{TraderID: "t1", ManagedCapitalUSDT: 100, MaxLossPerTradeUSDT: 5, MaxLeverage: 5, MaxPositions: 3}
	if err := ss.Create(s); err != nil {
		t.Fatal(err)
	}
	if err := ss.Lock(s.ID, TradingSessionSLLocked, "test"); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	db2, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewTradingSessionStore(db2).GetLatest("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != TradingSessionSLLocked {
		t.Fatalf("expected persisted lock, got %+v", got)
	}
}

func TestSessionRealizedPnLUsesTradesSinceStartOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pnl.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&TraderPosition{}); err != nil {
		t.Fatal(err)
	}
	rows := []TraderPosition{
		{TraderID: "t1", Symbol: "BTCUSDT", Side: "LONG", EntryPrice: 100, ExitPrice: 101, EntryTime: 100, ExitTime: 900, Status: "CLOSED", RealizedPnL: 50, Fee: 1},
		{TraderID: "t1", Symbol: "ETHUSDT", Side: "LONG", EntryPrice: 100, ExitPrice: 101, EntryTime: 1000, ExitTime: 1500, Status: "CLOSED", RealizedPnL: 7, Fee: 0.5},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	pnl, fees, err := NewPositionStore(db).SessionRealizedPnL("t1", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if pnl != 7 || fees != 0.5 {
		t.Fatalf("expected only post-session trades, got pnl=%v fees=%v", pnl, fees)
	}
}
