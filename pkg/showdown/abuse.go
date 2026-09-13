package showdown

import (
	"sync"
	"time"
)

// abusemonitor tracks command rate limits per user to prevent flooding
type AbuseMonitor struct {
	mu           sync.Mutex
	maxCommands  int
	window       time.Duration
	muteDuration time.Duration
	history      map[string][]time.Time
	lockedUntil  map[string]time.Time
}

// newabusemonitor initializes an abuse monitor with default rate limits
func NewAbuseMonitor(maxCommands int, window, muteDuration time.Duration) *AbuseMonitor {
	if maxCommands <= 0 {
		maxCommands = 4
	}
	if window <= 0 {
		window = 10 * time.Second
	}
	if muteDuration <= 0 {
		muteDuration = 30 * time.Second
	}

	return &AbuseMonitor{
		maxCommands:  maxCommands,
		window:       window,
		muteDuration: muteDuration,
		history:      make(map[string][]time.Time),
		lockedUntil:  make(map[string]time.Time),
	}
}

// isabusing checks whether a user has exceeded rate limits and locks them if abusing
func (m *AbuseMonitor) IsAbusing(username string) bool {
	id := ToID(username)
	if id == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// check if currently locked out
	if until, locked := m.lockedUntil[id]; locked {
		if now.Before(until) {
			return true
		}
		delete(m.lockedUntil, id)
	}

	// filter past timestamps within sliding window
	cutoff := now.Add(-m.window)
	timestamps := m.history[id]
	valid := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// prune stale keys periodically if map grows large
	if len(m.history) > 500 {
		for uid, times := range m.history {
			if len(times) == 0 || times[len(times)-1].Before(cutoff) {
				delete(m.history, uid)
			}
		}
	}

	// append current invocation
	valid = append(valid, now)
	m.history[id] = valid

	// verify threshold
	if len(valid) > m.maxCommands {
		m.lockedUntil[id] = now.Add(m.muteDuration)
		return true
	}

	return false
}

// reset clears lockout and history for a given user
func (m *AbuseMonitor) Reset(username string) {
	id := ToID(username)
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.history, id)
	delete(m.lockedUntil, id)
}
