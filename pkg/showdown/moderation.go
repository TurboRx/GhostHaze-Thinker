package showdown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// moderationconfig defines rules for automated chatroom moderation
type ModerationConfig struct {
	Enabled        bool     `json:"enabled"`
	BannedWords    []string `json:"banned_words"`
	MaxCapsPercent int      `json:"max_caps_percent"`
	CapsMinLength  int      `json:"caps_min_length"`
	Action         string   `json:"action"` // "warn", "mute", "message"
	CustomWarning  string   `json:"custom_warning"`
	ExemptRanks    string   `json:"exempt_ranks"` // "+, %, @, *, #, ~"
}

// defaultmoderationconfig provides sensible defaults
func DefaultModerationConfig() ModerationConfig {
	return ModerationConfig{
		Enabled:        false,
		BannedWords:    []string{},
		MaxCapsPercent: 70,
		CapsMinLength:  10,
		Action:         "warn",
		CustomWarning:  "Please refrain from profanity, spam, or excessive caps.",
		ExemptRanks:    "+, %, @, *, #, ~",
	}
}

// moderationstore manages moderation rules and enforcement
type ModerationStore struct {
	filePath string
	mu       sync.RWMutex
	config   ModerationConfig
}

// newmoderationstore creates a new moderation store
func NewModerationStore(filePath string) *ModerationStore {
	store := &ModerationStore{
		filePath: filePath,
		config:   DefaultModerationConfig(),
	}
	_ = store.load()
	return store
}

// load reads moderation configuration from disk
func (s *ModerationStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg ModerationConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.MaxCapsPercent <= 0 {
		cfg.MaxCapsPercent = 70
	}
	if cfg.CapsMinLength <= 0 {
		cfg.CapsMinLength = 10
	}
	if cfg.Action == "" {
		cfg.Action = "warn"
	}
	if cfg.ExemptRanks == "" {
		cfg.ExemptRanks = "+, %, @, *, #, ~"
	}
	if cfg.BannedWords == nil {
		cfg.BannedWords = []string{}
	}

	s.config = cfg
	return nil
}

// savetofile writes moderation configuration to disk
func (s *ModerationStore) saveToFile() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// getconfig returns a copy of current moderation configuration
func (s *ModerationStore) GetConfig() ModerationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// saveconfig updates moderation configuration
func (s *ModerationStore) SaveConfig(cfg ModerationConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.MaxCapsPercent <= 0 {
		cfg.MaxCapsPercent = 70
	}
	if cfg.CapsMinLength <= 0 {
		cfg.CapsMinLength = 10
	}
	if cfg.Action == "" {
		cfg.Action = "warn"
	}
	if cfg.ExemptRanks == "" {
		cfg.ExemptRanks = "+, %, @, *, #, ~"
	}
	if cfg.BannedWords == nil {
		cfg.BannedWords = []string{}
	}

	s.config = cfg
	return s.saveToFile()
}

// isrankexempt verifies whether a user rank is present in the comma-separated or raw exempt list
func isRankExempt(userRank, exemptRanks string) bool {
	if userRank == "" || exemptRanks == "" {
		return false
	}
	tokens := strings.Split(exemptRanks, ",")
	for _, tok := range tokens {
		t := strings.TrimSpace(tok)
		if t == "" {
			continue
		}
		if t == userRank || strings.Contains(t, userRank) {
			return true
		}
	}
	return false
}

// checkmessage inspects a chatroom message for moderation infractions
func (s *ModerationStore) CheckMessage(user, text string) (bool, string, string) {
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	if !cfg.Enabled {
		return false, "", ""
	}

	// exempt ranked users
	userRank := UserRank(user)
	if userRank != "" && isRankExempt(userRank, cfg.ExemptRanks) {
		return false, "", ""
	}

	cleanMsg := strings.TrimSpace(text)
	lowerMsg := strings.ToLower(cleanMsg)

	// check banned words
	for _, word := range cfg.BannedWords {
		cleanWord := strings.ToLower(strings.TrimSpace(word))
		if cleanWord == "" {
			continue
		}
		if strings.Contains(lowerMsg, cleanWord) {
			return true, cfg.Action, "Banned word / phrase detected"
		}
	}

	// check excessive caps
	totalLetters := 0
	upperLetters := 0
	for _, r := range cleanMsg {
		if unicode.IsLetter(r) {
			totalLetters++
			if unicode.IsUpper(r) {
				upperLetters++
			}
		}
	}

	if totalLetters >= cfg.CapsMinLength {
		capsPercent := (upperLetters * 100) / totalLetters
		if capsPercent >= cfg.MaxCapsPercent {
			return true, cfg.Action, "Excessive caps detected"
		}
	}

	return false, "", ""
}
