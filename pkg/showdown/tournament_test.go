package showdown

import (
	"strings"
	"testing"
)

type mockTournamentClient struct {
	sent       []string
	roomSent   map[string][]string
	username   string
	isGuest    bool
	teams      map[string]string
}

func newMockTournamentClient(username string, isGuest bool) *mockTournamentClient {
	return &mockTournamentClient{
		username: username,
		isGuest:  isGuest,
		roomSent: make(map[string][]string),
		teams:    make(map[string]string),
	}
}

func (m *mockTournamentClient) Send(message string) error {
	m.sent = append(m.sent, message)
	return nil
}

func (m *mockTournamentClient) SendToRoom(room, message string) error {
	m.roomSent[room] = append(m.roomSent[room], message)
	return nil
}

func (m *mockTournamentClient) GetTeamForFormat(format string) string {
	return m.teams[format]
}

func (m *mockTournamentClient) Username() string {
	return m.username
}

func (m *mockTournamentClient) IsGuest() bool {
	return m.isGuest
}

func TestTournamentManager_AutoJoinLifecycle(t *testing.T) {
	client := newMockTournamentClient("GhostHazeBot", false)
	client.teams["gen9ou"] = "mew||lifeorb||surf,psychic|timid|||||,"

	mgr := NewTournamentManager(true, []string{"gen9ou", "gen9randombattle"})

	// 1. tournament created in room lobby
	mgr.HandleMessage(client, "lobby", []string{"create", "gen9ou", "Single Elimination", "16"})

	tour, ok := mgr.Get("lobby")
	if !ok || tour == nil {
		t.Fatalf("expected tournament to be tracked in lobby")
	}
	if tour.Format != "gen9ou" {
		t.Fatalf("expected format gen9ou, got %s", tour.Format)
	}

	// verify team uploaded and /tournament join sent
	if len(client.sent) == 0 || !strings.Contains(client.sent[0], "mew||lifeorb") {
		t.Fatalf("expected team to be sent via utm before joining, got: %v", client.sent)
	}
	lobbyMsgs := client.roomSent["lobby"]
	if len(lobbyMsgs) == 0 || lobbyMsgs[0] != "/tournament join" {
		t.Fatalf("expected /tournament join to be sent to lobby, got: %v", lobbyMsgs)
	}

	// 2. bot joins
	mgr.HandleMessage(client, "lobby", []string{"join", "GhostHazeBot"})
	tour, _ = mgr.Get("lobby")
	if !tour.IsJoined {
		t.Fatalf("expected isJoined to be true")
	}

	// 3. tournament starts
	mgr.HandleMessage(client, "lobby", []string{"start", "8"})
	tour, _ = mgr.Get("lobby")
	if !tour.IsStarted {
		t.Fatalf("expected isStarted to be true")
	}

	// 4. opponent challenges us
	client.sent = nil
	client.roomSent["lobby"] = nil
	mgr.HandleMessage(client, "lobby", []string{"update", `{"challenged":"RivalTrainer"}`})

	tour, _ = mgr.Get("lobby")
	if tour.Challenged != "RivalTrainer" {
		t.Fatalf("expected challenged to be RivalTrainer, got %s", tour.Challenged)
	}
	if len(client.roomSent["lobby"]) == 0 || client.roomSent["lobby"][0] != "/tournament acceptchallenge" {
		t.Fatalf("expected /tournament acceptchallenge, got: %v", client.roomSent["lobby"])
	}

	// 5. match starts
	mgr.HandleMessage(client, "lobby", []string{"battlestart", "GhostHazeBot", "RivalTrainer", "battle-gen9ou-1001"})
	tour, _ = mgr.Get("lobby")
	if tour.CurrentMatch != "battle-gen9ou-1001" {
		t.Fatalf("expected current match battle-gen9ou-1001, got %s", tour.CurrentMatch)
	}

	// 6. match ends
	mgr.HandleMessage(client, "lobby", []string{"battleend", "GhostHazeBot", "RivalTrainer", "win", "2-0", "success", "battle-gen9ou-1001"})
	tour, _ = mgr.Get("lobby")
	if tour.CurrentMatch != "" {
		t.Fatalf("expected current match to be cleared, got %s", tour.CurrentMatch)
	}

	// 7. tournament ends
	mgr.HandleMessage(client, "lobby", []string{"end", `{"results":[]}`})
	if _, exists := mgr.Get("lobby"); exists {
		t.Fatalf("expected tournament to be removed on end")
	}
}

func TestTournamentManager_AutoChallengeDesignatedOpponent(t *testing.T) {
	client := newMockTournamentClient("GhostHazeBot", false)
	mgr := NewTournamentManager(true, nil)

	// create and join
	mgr.HandleMessage(client, "tournaments", []string{"create", "gen9randombattle", "Single Elimination"})
	mgr.HandleMessage(client, "tournaments", []string{"join", "GhostHazeBot"})
	mgr.HandleMessage(client, "tournaments", []string{"start", "4"})

	client.roomSent["tournaments"] = nil
	// update with challenges
	mgr.HandleMessage(client, "tournaments", []string{"update", `{"challenges":["TargetOpponent"]}`})

	msgs := client.roomSent["tournaments"]
	if len(msgs) == 0 || msgs[0] != "/tournament challenge TargetOpponent" {
		t.Fatalf("expected /tournament challenge TargetOpponent, got: %v", msgs)
	}
}

func TestTournamentManager_ManualJoinAndLeave(t *testing.T) {
	client := newMockTournamentClient("GhostHazeBot", false)
	mgr := NewTournamentManager(false, nil)

	if err := mgr.Join(client, "tournaments"); err != nil {
		t.Fatalf("unexpected join error: %v", err)
	}
	if len(client.roomSent["tournaments"]) == 0 || client.roomSent["tournaments"][0] != "/tournament join" {
		t.Fatalf("expected /tournament join, got: %v", client.roomSent["tournaments"])
	}

	if err := mgr.Leave(client, "tournaments"); err != nil {
		t.Fatalf("unexpected leave error: %v", err)
	}
	if len(client.roomSent["tournaments"]) < 2 || client.roomSent["tournaments"][1] != "/tournament leave" {
		t.Fatalf("expected /tournament leave, got: %v", client.roomSent["tournaments"])
	}
}
