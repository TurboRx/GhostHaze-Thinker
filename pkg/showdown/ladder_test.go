package showdown

import (
	"strings"
	"testing"
	"time"
)

type mockLadderClient struct {
	sent     []string
	isGuest  bool
	teamFmt  string
	teamData string
}

func (m *mockLadderClient) Send(message string) error {
	m.sent = append(m.sent, message)
	return nil
}

func (m *mockLadderClient) GetTeamForFormat(format string) string {
	if format == m.teamFmt {
		return m.teamData
	}
	return ""
}

func (m *mockLadderClient) IsGuest() bool {
	return m.isGuest
}

func TestLadderController(t *testing.T) {
	client := &mockLadderClient{
		isGuest:  false,
		teamFmt:  "gen9ou",
		teamData: "Pikachu||LightBall|Static|Thunderbolt|||||",
	}

	ladder := NewLadderController()

	// guest check
	guestClient := &mockLadderClient{isGuest: true}
	if err := ladder.Start(guestClient, "gen9ou", 2); err == nil {
		t.Errorf("expected error when starting ladder as guest")
	}

	// start with format
	if err := ladder.Start(client, "gen9ou", 2); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	status := ladder.Status()
	if !status.Active || !status.Searching {
		t.Errorf("expected active and searching status")
	}
	if len(client.sent) == 0 || !strings.Contains(client.sent[0], "/search gen9ou") {
		t.Errorf("expected search command to be sent, got %+v", client.sent)
	}

	// simulate battle start
	ladder.OnBattleStart("battle-gen9ou-1")
	status = ladder.Status()
	if status.Searching || status.CurrentBattle != "battle-gen9ou-1" {
		t.Errorf("expected battle to be recorded, searching false")
	}

	// simulate battle 1 end (win)
	ladder.OnBattleEnd(client, "battle-gen9ou-1", "win")
	status = ladder.Status()
	if status.BattlesPlayed != 1 || status.Wins != 1 || !status.Active {
		t.Errorf("expected 1 played, 1 win, still active (max is 2)")
	}

	// wait for next match search dispatch (scheduled after 4s)
	// let's simulate battle 2
	ladder.OnBattleStart("battle-gen9ou-2")
	ladder.OnBattleEnd(client, "battle-gen9ou-2", "loss")

	status = ladder.Status()
	if status.BattlesPlayed != 2 || status.Losses != 1 || status.Active {
		t.Errorf("expected 2 played, 1 loss, inactive since max 2 reached")
	}

	// test manual stop when restarted
	_ = ladder.Start(client, "gen9randombattle", 0)
	time.Sleep(10 * time.Millisecond)
	if err := ladder.Stop(client); err != nil {
		t.Errorf("stop error: %v", err)
	}
	status = ladder.Status()
	if status.Active {
		t.Errorf("expected inactive after stop")
	}

	// test comma-separated multi-tier formats
	ladderMulti := NewLadderController()
	if err := ladderMulti.Start(client, "gen9randombattle, gen9ou, gen9ubers", 3); err != nil {
		t.Fatalf("multi-tier start failed: %v", err)
	}

	statusMulti := ladderMulti.Status()
	if len(statusMulti.Formats) != 3 || statusMulti.Formats[0] != "gen9randombattle" || statusMulti.Formats[1] != "gen9ou" || statusMulti.Formats[2] != "gen9ubers" {
		t.Errorf("expected 3 parsed formats, got %+v", statusMulti.Formats)
	}
	if statusMulti.CurrentFormat != "gen9randombattle" {
		t.Errorf("expected initial format gen9randombattle, got %s", statusMulti.CurrentFormat)
	}

	// finish battle 1 -> should rotate to gen9ou
	ladderMulti.OnBattleEnd(client, "battle-1", "win")
	statusMulti = ladderMulti.Status()
	if statusMulti.CurrentFormat != "gen9ou" {
		t.Errorf("expected rotated format gen9ou, got %s", statusMulti.CurrentFormat)
	}

	// finish battle 2 -> should rotate to gen9ubers
	ladderMulti.OnBattleEnd(client, "battle-2", "win")
	statusMulti = ladderMulti.Status()
	if statusMulti.CurrentFormat != "gen9ubers" {
		t.Errorf("expected rotated format gen9ubers, got %s", statusMulti.CurrentFormat)
	}
	_ = ladderMulti.Stop(client)
}

