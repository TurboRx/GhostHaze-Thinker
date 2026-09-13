package showdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// chatroomtimer represents a scheduled recurring announcement in a chatroom
type ChatroomTimer struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Room            string    `json:"room"`
	IntervalMinutes int       `json:"interval_minutes"`
	Message         string    `json:"message"`
	Enabled         bool      `json:"enabled"`
	LastRun         time.Time `json:"last_run"`
}

// timersender defines the messaging capability needed to dispatch timer announcements
type TimerSender interface {
	SendRoomMessage(room, text string) error
}

// timerstore manages chatroom timer persistence and execution
type TimerStore struct {
	filePath string
	mu       sync.RWMutex
	timers   map[string]*ChatroomTimer
	stopChan chan struct{}
	running  bool
}

// newtimerstore initializes the timer store from disk
func NewTimerStore(filePath string) *TimerStore {
	store := &TimerStore{
		filePath: filePath,
		timers:   make(map[string]*ChatroomTimer),
	}
	_ = store.load()
	return store
}

// load reads timers from the json file
func (s *TimerStore) Load() error {
	return s.load()
}

func (s *TimerStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []ChatroomTimer
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.timers = make(map[string]*ChatroomTimer)
	for i := range list {
		item := list[i]
		s.timers[item.ID] = &item
	}
	return nil
}

// savetofile writes timers to the json file
func (s *TimerStore) saveToFile() error {
	if s.filePath == "" {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var list []ChatroomTimer
	for _, t := range s.timers {
		list = append(list, *t)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// list returns all configured timers
func (s *TimerStore) List() []ChatroomTimer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ChatroomTimer
	for _, t := range s.timers {
		result = append(result, *t)
	}
	return result
}

// get retrieves a timer by id
func (s *TimerStore) Get(id string) (ChatroomTimer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.timers[id]
	if !exists {
		return ChatroomTimer{}, false
	}
	return *t, true
}

// save adds or updates a timer
func (s *TimerStore) Save(timer ChatroomTimer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if timer.ID == "" {
		timer.ID = fmt.Sprintf("timer_%d", time.Now().UnixNano())
	}
	if timer.IntervalMinutes < 1 {
		timer.IntervalMinutes = 1
	}

	existing, exists := s.timers[timer.ID]
	if exists {
		timer.LastRun = existing.LastRun
	}

	s.timers[timer.ID] = &timer
	return s.saveToFile()
}

// delete removes a timer by id
func (s *TimerStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.timers[id]; !exists {
		return false
	}
	delete(s.timers, id)
	_ = s.saveToFile()
	return true
}

// toggle switches the enabled state of a timer
func (s *TimerStore) Toggle(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.timers[id]
	if !exists {
		return false, errors.New("timer not found")
	}

	t.Enabled = !t.Enabled
	_ = s.saveToFile()
	return t.Enabled, nil
}

// checkandrun evaluates all enabled timers and dispatches due messages
func (s *TimerStore) CheckAndRun(sender TimerSender, now time.Time) int {
	s.mu.Lock()
	var due []*ChatroomTimer
	for _, t := range s.timers {
		if !t.Enabled || t.Room == "" || t.Message == "" {
			continue
		}
		interval := time.Duration(t.IntervalMinutes) * time.Minute
		if t.LastRun.IsZero() || now.Sub(t.LastRun) >= interval {
			due = append(due, t)
		}
	}
	s.mu.Unlock()

	executed := 0
	for _, t := range due {
		if sender != nil {
			_ = sender.SendRoomMessage(t.Room, t.Message)
		}
		s.mu.Lock()
		if current, ok := s.timers[t.ID]; ok {
			current.LastRun = now
		}
		s.mu.Unlock()
		executed++
	}

	if executed > 0 {
		s.mu.Lock()
		_ = s.saveToFile()
		s.mu.Unlock()
	}

	return executed
}

// triggernow manually dispatches a timer message immediately
func (s *TimerStore) TriggerNow(id string, sender TimerSender) error {
	s.mu.Lock()
	t, exists := s.timers[id]
	if !exists {
		s.mu.Unlock()
		return errors.New("timer not found")
	}
	room := t.Room
	msg := t.Message
	t.LastRun = time.Now()
	_ = s.saveToFile()
	s.mu.Unlock()

	if sender == nil {
		return errors.New("sender not available")
	}
	return sender.SendRoomMessage(room, msg)
}

// start begins the background ticker loop
func (s *TimerStore) Start(sender TimerSender) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	stopCh := s.stopChan
	s.mu.Unlock()

	go func(ch <-chan struct{}) {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ch:
				return
			case now := <-ticker.C:
				s.CheckAndRun(sender, now)
			}
		}
	}(stopCh)
}

// stop halts the background ticker loop
func (s *TimerStore) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	s.running = false
	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = nil
	}
}
