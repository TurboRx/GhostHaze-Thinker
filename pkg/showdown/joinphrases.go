package showdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// joinphrase represents a custom greeting dispatched when a user joins a chatroom
type JoinPhrase struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	UserID   string `json:"user_id"`
	Room     string `json:"room"`
	Phrase   string `json:"phrase"`
	Enabled  bool   `json:"enabled"`
}

// joinphrasestore manages user join phrases and greetings
type JoinPhraseStore struct {
	filePath  string
	mu        sync.RWMutex
	phrases   map[string]*JoinPhrase
	cooldowns map[string]time.Time
}

// newjoinphrasestore creates a new join phrase store with disk persistence
func NewJoinPhraseStore(filePath string) *JoinPhraseStore {
	store := &JoinPhraseStore{
		filePath:  filePath,
		phrases:   make(map[string]*JoinPhrase),
		cooldowns: make(map[string]time.Time),
	}
	_ = store.load()
	return store
}

// load reads phrases from the json file
func (s *JoinPhraseStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []JoinPhrase
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.phrases = make(map[string]*JoinPhrase)
	for i := range list {
		item := list[i]
		item.UserID = ToID(item.Username)
		s.phrases[item.ID] = &item
	}
	return nil
}

// savetofile writes phrases to the json file
func (s *JoinPhraseStore) saveToFile() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var list []JoinPhrase
	for _, p := range s.phrases {
		list = append(list, *p)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// list returns all configured join phrases
func (s *JoinPhraseStore) List() []JoinPhrase {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []JoinPhrase
	for _, p := range s.phrases {
		result = append(result, *p)
	}
	return result
}

// save adds or updates a join phrase
func (s *JoinPhraseStore) Save(jp JoinPhrase) (JoinPhrase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(jp.Username) == "" {
		return JoinPhrase{}, errors.New("username is required")
	}
	if strings.TrimSpace(jp.Phrase) == "" {
		return JoinPhrase{}, errors.New("greeting phrase is required")
	}

	if jp.ID == "" {
		jp.ID = fmt.Sprintf("jp_%d", time.Now().UnixNano())
	}
	jp.Username = CleanUsername(jp.Username)
	jp.UserID = ToID(jp.Username)
	jp.Room = ToRoomID(jp.Room)

	s.phrases[jp.ID] = &jp
	_ = s.saveToFile()
	return jp, nil
}

// delete removes a join phrase by id
func (s *JoinPhraseStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.phrases[id]; !exists {
		return false
	}
	delete(s.phrases, id)
	_ = s.saveToFile()
	return true
}

// toggle switches the enabled state of a join phrase
func (s *JoinPhraseStore) Toggle(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.phrases[id]
	if !exists {
		return false, errors.New("join phrase not found")
	}

	p.Enabled = !p.Enabled
	_ = s.saveToFile()
	return p.Enabled, nil
}

// match checks if there is an active join phrase for the joining user in the room
func (s *JoinPhraseStore) Match(username, room string, botName string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	userID := ToID(username)
	if userID == "" {
		return "", false
	}
	roomID := ToRoomID(room)

	// find matching phrase
	var matched *JoinPhrase
	for _, p := range s.phrases {
		if !p.Enabled {
			continue
		}
		if p.UserID == userID {
			// check room restriction
			if p.Room == "" || p.Room == roomID {
				matched = p
				break
			}
		}
	}

	if matched == nil {
		return "", false
	}

	// check cooldown (5 minutes cooldown per user and room)
	cdKey := fmt.Sprintf("%s:%s", userID, roomID)
	if last, ok := s.cooldowns[cdKey]; ok {
		if now.Sub(last) < 5*time.Minute {
			return "", false
		}
	}

	s.cooldowns[cdKey] = now

	// format phrase placeholders
	text := matched.Phrase
	text = strings.ReplaceAll(text, "{user}", CleanUsername(username))
	text = strings.ReplaceAll(text, "{bot}", CleanUsername(botName))
	text = strings.ReplaceAll(text, "{room}", room)

	return text, true
}
