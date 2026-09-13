package showdown

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// seenentry records when and where a user was last active
type SeenEntry struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Room        string    `json:"room"`
	LastMessage string    `json:"last_message"`
	LastSeen    time.Time `json:"last_seen"`
}

// seenstore maintains user activity records across chatrooms
type SeenStore struct {
	filePath string
	mu       sync.RWMutex
	entries  map[string]SeenEntry
}

// newseenstore initializes a seen tracker with optional persistence
func NewSeenStore(filePath string) *SeenStore {
	store := &SeenStore{
		filePath: filePath,
		entries:  make(map[string]SeenEntry),
	}
	_ = store.load()
	return store
}

// load reads stored seen entries from disk
func (s *SeenStore) load() error {
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

	var list []SeenEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.entries = make(map[string]SeenEntry)
	for _, e := range list {
		id := ToID(e.UserID)
		if id == "" {
			id = ToID(e.Username)
		}
		if id != "" {
			s.entries[id] = e
		}
	}
	return nil
}

// savetofile persists seen records to disk
func (s *SeenStore) saveToFile() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	list := make([]SeenEntry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// record logs activity for a user in a chatroom
func (s *SeenStore) Record(username, room, message string) {
	cleanName := CleanUsername(username)
	id := ToID(cleanName)
	if id == "" {
		return
	}

	// truncate very long message snippets
	snippet := strings.TrimSpace(message)
	if len(snippet) > 120 {
		snippet = snippet[:117] + "..."
	}

	s.mu.Lock()
	s.entries[id] = SeenEntry{
		UserID:      id,
		Username:    cleanName,
		Room:        room,
		LastMessage: snippet,
		LastSeen:    time.Now(),
	}
	s.mu.Unlock()

	_ = s.saveToFile()
}

// get retrieves the last seen record for a given username
func (s *SeenStore) Get(username string) (SeenEntry, bool) {
	id := ToID(username)
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[id]
	return entry, ok
}

// getall returns all seen records sorted by most recent first
func (s *SeenStore) GetAll(limit int) []SeenEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]SeenEntry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].LastSeen.After(list[j].LastSeen)
	})

	if limit > 0 && len(list) > limit {
		return list[:limit]
	}
	return list
}

// clear removes all recorded user activity
func (s *SeenStore) Clear() error {
	s.mu.Lock()
	s.entries = make(map[string]SeenEntry)
	s.mu.Unlock()
	return s.saveToFile()
}
