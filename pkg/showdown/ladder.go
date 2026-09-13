package showdown

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ladderstatus represents current ranked laddering statistics and state
type LadderStatus struct {
	Active        bool      `json:"active"`
	Format        string    `json:"format"`
	MaxBattles    int       `json:"max_battles"`
	BattlesPlayed int       `json:"battles_played"`
	Wins          int       `json:"wins"`
	Losses        int       `json:"losses"`
	Ties          int       `json:"ties"`
	Searching     bool      `json:"searching"`
	CurrentBattle string    `json:"current_battle"`
	StartedAt     time.Time `json:"started_at"`
}

// ladderclient interface exposes websocket messaging and team retrieval needed for laddering
type LadderClient interface {
	Send(message string) error
	GetTeamForFormat(format string) string
	IsGuest() bool
}

// laddercontroller manages automated ladder matchmaking and rotation
type LadderController struct {
	mu     sync.RWMutex
	status LadderStatus
	stopCh chan struct{}
}

// newladdercontroller initializes a new ladder controller
func NewLadderController() *LadderController {
	return &LadderController{}
}

// status returns a snapshot of the current laddering state
func (l *LadderController) Status() LadderStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.status
}

// start begins automated ranked ladder queuing
func (l *LadderController) Start(client LadderClient, format string, maxBattles int) error {
	if client == nil {
		return errors.New("client is not available")
	}
	if client.IsGuest() {
		return errors.New("cannot ladder as guest: registered account required")
	}
	if format == "" {
		format = "gen9randombattle"
	}
	if maxBattles < 0 {
		maxBattles = 0
	}

	l.mu.Lock()
	if l.status.Active {
		l.mu.Unlock()
		return errors.New("ladder bot is already active")
	}

	l.stopCh = make(chan struct{})
	l.status = LadderStatus{
		Active:        true,
		Format:        format,
		MaxBattles:    maxBattles,
		BattlesPlayed: 0,
		Wins:          0,
		Losses:        0,
		Ties:          0,
		Searching:     true,
		CurrentBattle: "",
		StartedAt:     time.Now(),
	}
	l.mu.Unlock()

	return l.dispatchSearch(client, format)
}

// stop halts automated ranked ladder queuing
func (l *LadderController) Stop(client LadderClient) error {
	l.mu.Lock()
	if !l.status.Active {
		l.mu.Unlock()
		return nil
	}

	l.status.Active = false
	l.status.Searching = false
	if l.stopCh != nil {
		close(l.stopCh)
		l.stopCh = nil
	}
	l.mu.Unlock()

	if client != nil {
		_ = client.Send("|/cancelsearch")
	}
	return nil
}

// dispatchsearch uploads active team and submits matchmaking search
func (l *LadderController) dispatchSearch(client LadderClient, format string) error {
	team := client.GetTeamForFormat(format)
	if team != "" {
		return client.Send(fmt.Sprintf("|/utm %s\n|/search %s", team, format))
	}
	return client.Send(fmt.Sprintf("|/utm null\n|/search %s", format))
}

// onsearchupdate notifies when pokemon showdown updates search queue
func (l *LadderController) OnSearchUpdate(searching bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status.Active {
		l.status.Searching = searching
	}
}

// onbattlestart records match start and resets searching flag
func (l *LadderController) OnBattleStart(battleID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status.Active {
		l.status.Searching = false
		l.status.CurrentBattle = battleID
	}
}

// onbattleend records match result and initiates next queue if target not met
func (l *LadderController) OnBattleEnd(client LadderClient, battleID string, outcome string) {
	l.mu.Lock()
	if !l.status.Active {
		l.mu.Unlock()
		return
	}

	l.status.BattlesPlayed++
	switch outcome {
	case "win":
		l.status.Wins++
	case "loss":
		l.status.Losses++
	default:
		l.status.Ties++
	}
	l.status.CurrentBattle = ""

	// check if target limit reached
	if l.status.MaxBattles > 0 && l.status.BattlesPlayed >= l.status.MaxBattles {
		l.status.Active = false
		l.status.Searching = false
		if l.stopCh != nil {
			close(l.stopCh)
			l.stopCh = nil
		}
		l.mu.Unlock()
		return
	}

	l.status.Searching = true
	format := l.status.Format
	stopCh := l.stopCh
	l.mu.Unlock()

	// schedule next match after a short cool-off delay
	go func() {
		select {
		case <-stopCh:
			return
		case <-time.After(4 * time.Second):
			l.mu.RLock()
			active := l.status.Active
			l.mu.RUnlock()
			if active && client != nil {
				_ = l.dispatchSearch(client, format)
			}
		}
	}()
}
