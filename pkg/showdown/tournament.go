package showdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tournamentclient exposes the network and team retrieval capabilities required by the tournament manager.
type TournamentClient interface {
	Send(message string) error
	SendToRoom(room, message string) error
	GetTeamForFormat(format string) string
	Username() string
	IsGuest() bool
}

// tournament captures the current lifecycle state of an active tournament in a chatroom.
type Tournament struct {
	Room         string    `json:"room"`
	Format       string    `json:"format"`
	Generator    string    `json:"generator"`
	PlayerCap    int       `json:"player_cap,omitempty"`
	IsStarted    bool      `json:"is_started"`
	IsJoined     bool      `json:"is_joined"`
	Challenges   []string  `json:"challenges,omitempty"`
	ChallengeBys []string  `json:"challenge_bys,omitempty"`
	Challenged   string    `json:"challenged,omitempty"`
	Challenging  string    `json:"challenging,omitempty"`
	CurrentMatch string    `json:"current_match,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type tournamentUpdatePayload struct {
	Format       *string   `json:"format"`
	Generator    *string   `json:"generator"`
	IsStarted    *bool     `json:"isStarted"`
	IsJoined     *bool     `json:"isJoined"`
	Challenges   *[]string `json:"challenges"`
	ChallengeBys *[]string `json:"challengeBys"`
	Challenged   *string   `json:"challenged"`
	Challenging  *string   `json:"challenging"`
}

// tournamentmanager coordinates tournament participation and match queueing across chatrooms.
type TournamentManager struct {
	mu             sync.RWMutex
	tournaments    map[string]*Tournament
	autoJoin       bool
	allowedFormats map[string]bool
	onUpdate       func(tour *Tournament)
}

// newtournamentmanager initializes a new tournament manager with optional format filters.
func NewTournamentManager(autoJoin bool, allowedFormats []string) *TournamentManager {
	fmts := make(map[string]bool)
	for _, f := range allowedFormats {
		clean := ToID(f)
		if clean != "" {
			fmts[clean] = true
		}
	}
	return &TournamentManager{
		tournaments:    make(map[string]*Tournament),
		autoJoin:       autoJoin,
		allowedFormats: fmts,
	}
}

// onupdate sets an optional callback triggered when tournament states change.
func (m *TournamentManager) OnUpdate(fn func(tour *Tournament)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onUpdate = fn
}

// setautojoin toggles automated tournament entry upon creation.
func (m *TournamentManager) SetAutoJoin(enable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoJoin = enable
}

// isautojoin returns true if automated tournament entry is enabled.
func (m *TournamentManager) IsAutoJoin() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.autoJoin
}

// setallowedformats updates the list of permitted tournament formats.
func (m *TournamentManager) SetAllowedFormats(formats []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedFormats = make(map[string]bool)
	for _, f := range formats {
		clean := ToID(f)
		if clean != "" {
			m.allowedFormats[clean] = true
		}
	}
}

// allowedformats returns the slice of permitted tournament format ids.
func (m *TournamentManager) AllowedFormats() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []string
	for k := range m.allowedFormats {
		res = append(res, k)
	}
	return res
}

func (m *TournamentManager) isFormatAllowedLocked(format string) bool {
	if len(m.allowedFormats) == 0 {
		return true
	}
	return m.allowedFormats[ToID(format)]
}

// isformatallowed returns true if the specified format is permitted for tournament play.
func (m *TournamentManager) IsFormatAllowed(format string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isFormatAllowedLocked(format)
}

// get returns the tournament in a specific chatroom if active.
func (m *TournamentManager) Get(room string) (*Tournament, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tour, ok := m.tournaments[ToRoomID(room)]
	if !ok || tour == nil {
		return nil, false
	}
	cp := *tour
	return &cp, true
}

// getall returns a list of all currently tracked tournaments.
func (m *TournamentManager) GetAll() []*Tournament {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*Tournament
	for _, t := range m.tournaments {
		if t != nil {
			cp := *t
			res = append(res, &cp)
		}
	}
	return res
}

// join sends join commands for an active tournament in the given chatroom.
func (m *TournamentManager) Join(client TournamentClient, room string) error {
	if client == nil {
		return errors.New("client is not available")
	}
	if client.IsGuest() {
		return errors.New("cannot join tournament as guest: registered account required")
	}
	cleanRoom := ToRoomID(room)
	m.mu.RLock()
	tour := m.tournaments[cleanRoom]
	m.mu.RUnlock()

	var format string
	if tour != nil {
		format = tour.Format
	}
	team := client.GetTeamForFormat(format)
	if team != "" {
		_ = client.Send(fmt.Sprintf("|/utm %s", team))
	} else {
		_ = client.Send("|/utm null")
	}
	return client.SendToRoom(room, "/tournament join")
}

// leave sends leave commands for an active tournament in the given chatroom.
func (m *TournamentManager) Leave(client TournamentClient, room string) error {
	if client == nil {
		return errors.New("client is not available")
	}
	return client.SendToRoom(room, "/tournament leave")
}

// handlemessage processes incoming showdown tournament lines for a chatroom.
func (m *TournamentManager) HandleMessage(client TournamentClient, room string, parts []string) {
	if len(parts) == 0 {
		return
	}
	cleanRoom := ToRoomID(room)
	subCmd := strings.ToLower(parts[0])

	switch subCmd {
	case "create":
		// e.g. |tournament|create|gen9randombattle|single elimination|32
		format := ""
		generator := ""
		playerCap := 0
		if len(parts) > 1 {
			format = parts[1]
		}
		if len(parts) > 2 {
			generator = parts[2]
		}
		if len(parts) > 3 {
			playerCap, _ = strconv.Atoi(parts[3])
		}

		tour := &Tournament{
			Room:      room,
			Format:    format,
			Generator: generator,
			PlayerCap: playerCap,
			CreatedAt: time.Now(),
		}

		m.mu.Lock()
		m.tournaments[cleanRoom] = tour
		autoJoin := m.autoJoin
		allowed := m.isFormatAllowedLocked(format)
		updateCb := m.onUpdate
		m.mu.Unlock()

		if updateCb != nil {
			updateCb(tour)
		}

		// automatically enter tournament if permitted format and registered account
		if autoJoin && allowed && client != nil && !client.IsGuest() {
			team := client.GetTeamForFormat(format)
			if team != "" {
				_ = client.Send(fmt.Sprintf("|/utm %s", team))
			} else {
				_ = client.Send("|/utm null")
			}
			_ = client.SendToRoom(room, "/tournament join")
		}

	case "join":
		// e.g. |tournament|join|username
		if len(parts) > 1 && client != nil {
			joinedUser := parts[1]
			if strings.EqualFold(CleanUsername(joinedUser), CleanUsername(client.Username())) {
				m.mu.Lock()
				if tour, ok := m.tournaments[cleanRoom]; ok && tour != nil {
					tour.IsJoined = true
				}
				m.mu.Unlock()
			}
		}

	case "leave", "disqualify":
		// e.g. |tournament|leave|username
		if len(parts) > 1 && client != nil {
			leftUser := parts[1]
			if strings.EqualFold(CleanUsername(leftUser), CleanUsername(client.Username())) {
				m.mu.Lock()
				if tour, ok := m.tournaments[cleanRoom]; ok && tour != nil {
					tour.IsJoined = false
				}
				m.mu.Unlock()
			}
		}

	case "start":
		m.mu.Lock()
		if tour, ok := m.tournaments[cleanRoom]; ok && tour != nil {
			tour.IsStarted = true
		}
		m.mu.Unlock()

	case "update":
		// e.g. |tournament|update|{"format":"gen9randombattle","isstarted":true,"challenges":["alice"]}
		if len(parts) < 2 {
			return
		}
		rawJSON := strings.Join(parts[1:], "|")
		var payload tournamentUpdatePayload
		if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
			return
		}

		m.mu.Lock()
		tour, ok := m.tournaments[cleanRoom]
		if !ok || tour == nil {
			tour = &Tournament{
				Room:      room,
				CreatedAt: time.Now(),
			}
			m.tournaments[cleanRoom] = tour
		}

		if payload.Format != nil {
			tour.Format = *payload.Format
		}
		if payload.Generator != nil {
			tour.Generator = *payload.Generator
		}
		if payload.IsStarted != nil {
			tour.IsStarted = *payload.IsStarted
		}
		if payload.IsJoined != nil {
			tour.IsJoined = *payload.IsJoined
		}
		if payload.Challenges != nil {
			tour.Challenges = *payload.Challenges
		}
		if payload.ChallengeBys != nil {
			tour.ChallengeBys = *payload.ChallengeBys
		}
		if payload.Challenged != nil {
			tour.Challenged = *payload.Challenged
		}
		if payload.Challenging != nil {
			tour.Challenging = *payload.Challenging
		}

		isJoined := tour.IsJoined
		isStarted := tour.IsStarted
		challenged := tour.Challenged
		challenges := tour.Challenges
		challenging := tour.Challenging
		format := tour.Format
		updateCb := m.onUpdate
		m.mu.Unlock()

		if updateCb != nil {
			updateCb(tour)
		}

		// manage automated match acceptance and challenge issuing
		if client != nil && isJoined && isStarted {
			team := client.GetTeamForFormat(format)
			if challenged != "" {
				// accept incoming challenge
				if team != "" {
					_ = client.Send(fmt.Sprintf("|/utm %s", team))
				} else {
					_ = client.Send("|/utm null")
				}
				_ = client.SendToRoom(room, "/tournament acceptchallenge")
			} else if len(challenges) > 0 && challenging == "" {
				// issue challenge to designated opponent
				opp := challenges[0]
				if team != "" {
					_ = client.Send(fmt.Sprintf("|/utm %s", team))
				} else {
					_ = client.Send("|/utm null")
				}
				_ = client.SendToRoom(room, fmt.Sprintf("/tournament challenge %s", opp))
			}
		}

	case "battlestart":
		// e.g. |tournament|battlestart|user1|user2|battleroomid
		if len(parts) >= 4 && client != nil {
			u1 := CleanUsername(parts[1])
			u2 := CleanUsername(parts[2])
			battleRoom := parts[3]
			myNick := CleanUsername(client.Username())

			if strings.EqualFold(u1, myNick) || strings.EqualFold(u2, myNick) {
				m.mu.Lock()
				if tour, ok := m.tournaments[cleanRoom]; ok && tour != nil {
					tour.CurrentMatch = battleRoom
				}
				m.mu.Unlock()
			}
		}

	case "battleend":
		// e.g. |tournament|battleend|user1|user2|result|score|status|battleroomid
		if len(parts) >= 3 && client != nil {
			u1 := CleanUsername(parts[1])
			u2 := CleanUsername(parts[2])
			myNick := CleanUsername(client.Username())

			if strings.EqualFold(u1, myNick) || strings.EqualFold(u2, myNick) {
				m.mu.Lock()
				if tour, ok := m.tournaments[cleanRoom]; ok && tour != nil {
					tour.CurrentMatch = ""
				}
				m.mu.Unlock()
			}
		}

	case "end", "forceend":
		m.mu.Lock()
		delete(m.tournaments, cleanRoom)
		m.mu.Unlock()
	}
}
