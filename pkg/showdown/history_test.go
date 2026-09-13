package showdown

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryStore_Operations(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "history.json")

	store := NewHistoryStore(filePath, 5)
	if len(store.List(10)) != 0 {
		t.Fatalf("expected empty history")
	}

	err := store.Record(BattleRecord{
		BattleID:   "battle-gen9ou-1",
		Room:       "battle-gen9ou-1",
		Format:     "gen9ou",
		Opponent:   "UserBlue",
		Outcome:    "win",
		Turns:      14,
		FinishedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to record battle: %v", err)
	}

	_ = store.Record(BattleRecord{
		BattleID:   "battle-gen9ou-2",
		Room:       "battle-gen9ou-2",
		Format:     "gen9ou",
		Opponent:   "UserRed",
		Outcome:    "loss",
		Turns:      22,
		FinishedAt: time.Now(),
	})

	_ = store.Record(BattleRecord{
		BattleID:   "battle-gen9ou-3",
		Room:       "battle-gen9ou-3",
		Format:     "gen9ou",
		Opponent:   "UserGreen",
		Outcome:    "win",
		Turns:      18,
		FinishedAt: time.Now(),
	})

	list := store.List(10)
	if len(list) != 3 {
		t.Fatalf("expected 3 battles, got %d", len(list))
	}

	// most recent should be first
	if list[0].Opponent != "UserGreen" {
		t.Fatalf("expected most recent to be UserGreen, got %s", list[0].Opponent)
	}

	stats := store.Stats()
	if stats.TotalBattles != 3 {
		t.Fatalf("expected 3 total battles, got %d", stats.TotalBattles)
	}
	if stats.Wins != 2 {
		t.Fatalf("expected 2 wins, got %d", stats.Wins)
	}
	if stats.Losses != 1 {
		t.Fatalf("expected 1 loss, got %d", stats.Losses)
	}
	// 2 wins out of 3 = 66.7%
	if stats.WinRate < 66.0 || stats.WinRate > 67.0 {
		t.Fatalf("expected ~66.7%% win rate, got %f", stats.WinRate)
	}

	// clear
	if err := store.Clear(); err != nil {
		t.Fatalf("failed to clear: %v", err)
	}
	if len(store.List(10)) != 0 {
		t.Fatal("expected 0 battles after clear")
	}

	_ = os.Remove(filePath)
}
