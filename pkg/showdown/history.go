package showdown

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// battlerecord stores details of a completed battle
type BattleRecord struct {
	BattleID   string    `json:"battle_id"`
	Room       string    `json:"room"`
	Format     string    `json:"format"`
	Opponent   string    `json:"opponent"`
	Outcome    string    `json:"outcome"` // "win", "loss", "tie", "forfeit"
	Turns      int       `json:"turns"`
	ReplayURL  string    `json:"replay_url"`
	FinishedAt time.Time `json:"finished_at"`
}

// battlestats represents cumulative win/loss statistics
type BattleStats struct {
	TotalBattles int     `json:"total_battles"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Ties         int     `json:"ties"`
	WinRate      float64 `json:"win_rate"` // e.g. 75.0
}

// historystore manages persistent battle records
type HistoryStore struct {
	mu         sync.RWMutex
	filePath   string
	maxRecords int
	records    []BattleRecord
}

// newhistorystore creates a new battle history store
func NewHistoryStore(filePath string, maxRecords int) *HistoryStore {
	if maxRecords <= 0 {
		maxRecords = 100
	}
	store := &HistoryStore{
		filePath:   filePath,
		maxRecords: maxRecords,
		records:    make([]BattleRecord, 0),
	}
	if filePath != "" {
		_ = store.Load()
	}
	return store
}

// load reads battle history from disk
func (s *HistoryStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []BattleRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	if list == nil {
		s.records = make([]BattleRecord, 0)
	} else {
		s.records = list
	}
	return nil
}

// save writes battle history to disk
func (s *HistoryStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.filePath == "" {
		return nil
	}

	records := s.records
	if records == nil {
		records = make([]BattleRecord, 0)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// record adds a new battle result
func (s *HistoryStore) Record(rec BattleRecord) error {
	if rec.FinishedAt.IsZero() {
		rec.FinishedAt = time.Now()
	}
	if rec.ReplayURL == "" && rec.Room != "" {
		roomName := strings.TrimPrefix(rec.Room, "battle-")
		rec.ReplayURL = "https://replay.pokemonshowdown.com/" + roomName
	}

	s.mu.Lock()
	// prepend most recent
	s.records = append([]BattleRecord{rec}, s.records...)
	if len(s.records) > s.maxRecords {
		s.records = s.records[:s.maxRecords]
	}
	s.mu.Unlock()

	return s.Save()
}

// list returns the most recent battle records up to limit
func (s *HistoryStore) List(limit int) []BattleRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.records) {
		limit = len(s.records)
	}

	res := make([]BattleRecord, limit)
	copy(res, s.records[:limit])
	return res
}

// stats calculates win/loss records and win rate percentage
func (s *HistoryStore) Stats() BattleStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var st BattleStats
	st.TotalBattles = len(s.records)

	for _, r := range s.records {
		switch strings.ToLower(r.Outcome) {
		case "win":
			st.Wins++
		case "loss", "forfeit":
			st.Losses++
		case "tie":
			st.Ties++
		}
	}

	decided := st.Wins + st.Losses
	if decided > 0 {
		rate := (float64(st.Wins) / float64(decided)) * 100.0
		st.WinRate = math.Round(rate*10) / 10
	}

	return st
}

// clear resets history
func (s *HistoryStore) Clear() error {
	s.mu.Lock()
	s.records = make([]BattleRecord, 0)
	s.mu.Unlock()

	return s.Save()
}
