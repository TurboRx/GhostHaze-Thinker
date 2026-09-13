package showdown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// blacklistentry represents a blocked user who is denied interaction with the bot
type BlacklistEntry struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Reason   string    `json:"reason"`
	AddedAt  time.Time `json:"added_at"`
}

// blackliststore manages user blacklist persistence and lookups
type BlacklistStore struct {
	filePath string
	mu       sync.RWMutex
	entries  map[string]BlacklistEntry
}

// newblackliststore creates a new blacklist store with disk persistence
func NewBlacklistStore(filePath string) *BlacklistStore {
	store := &BlacklistStore{
		filePath: filePath,
		entries:  make(map[string]BlacklistEntry),
	}
	_ = store.load()
	return store
}

// load reads blacklist entries from json
func (s *BlacklistStore) Load() error {
	return s.load()
}

func (s *BlacklistStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []BlacklistEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.entries = make(map[string]BlacklistEntry)
	for _, entry := range list {
		id := ToID(entry.UserID)
		if id == "" {
			id = ToID(entry.Username)
		}
		if id != "" {
			entry.UserID = id
			s.entries[id] = entry
		}
	}
	return nil
}

// savetofile writes blacklist entries to disk
func (s *BlacklistStore) saveToFile() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var list []BlacklistEntry
	for _, entry := range s.entries {
		list = append(list, entry)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// add inserts or updates a blacklisted user
func (s *BlacklistStore) Add(username, reason string) BlacklistEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := ToID(username)
	cleanName := CleanUsername(username)
	if cleanName == "" {
		cleanName = username
	}
	if reason == "" {
		reason = "No reason specified"
	}

	entry := BlacklistEntry{
		UserID:   id,
		Username: cleanName,
		Reason:   reason,
		AddedAt:  time.Now(),
	}

	s.entries[id] = entry
	_ = s.saveToFile()
	return entry
}

// remove deletes a user from the blacklist
func (s *BlacklistStore) Remove(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := ToID(username)
	if _, exists := s.entries[id]; !exists {
		return false
	}

	delete(s.entries, id)
	_ = s.saveToFile()
	return true
}

// isblacklisted checks whether a user id is currently blacklisted
func (s *BlacklistStore) IsBlacklisted(username string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := ToID(username)
	if id == "" {
		return false
	}
	_, exists := s.entries[id]
	return exists
}

// list returns all current blacklist entries
func (s *BlacklistStore) List() []BlacklistEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []BlacklistEntry
	for _, entry := range s.entries {
		result = append(result, entry)
	}
	return result
}
