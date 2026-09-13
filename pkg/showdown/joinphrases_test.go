package showdown

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJoinPhraseStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "joinphrase_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "joinphrases.json")
	store := NewJoinPhraseStore(filePath)

	// save a new phrase
	jp, err := store.Save(JoinPhrase{
		Username: "AshKetchum",
		Room:     "lobby",
		Phrase:   "Welcome champion {user} to the {room}! {bot} salutes you!",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if jp.UserID != "ashketchum" {
		t.Errorf("expected user id 'ashketchum', got '%s'", jp.UserID)
	}

	now := time.Now()
	// test match
	msg, matched := store.Match("AshKetchum", "lobby", "GhostBot", now)
	if !matched {
		t.Fatalf("expected phrase to match")
	}
	expected := "Welcome champion AshKetchum to the lobby! GhostBot salutes you!"
	if msg != expected {
		t.Errorf("expected '%s', got '%s'", expected, msg)
	}

	// test cooldown (should not match immediately after)
	_, matched2 := store.Match("AshKetchum", "lobby", "GhostBot", now.Add(1*time.Minute))
	if matched2 {
		t.Errorf("expected phrase to be on cooldown")
	}

	// test after cooldown passes (5+ minutes)
	_, matched3 := store.Match("AshKetchum", "lobby", "GhostBot", now.Add(6*time.Minute))
	if !matched3 {
		t.Errorf("expected phrase to match after cooldown")
	}

	// test toggle
	enabled, err := store.Toggle(jp.ID)
	if err != nil || enabled {
		t.Errorf("expected phrase to be disabled")
	}

	// should not match when disabled
	_, matchedDisabled := store.Match("AshKetchum", "lobby", "GhostBot", now.Add(12*time.Minute))
	if matchedDisabled {
		t.Errorf("expected disabled phrase not to match")
	}

	// test delete
	if !store.Delete(jp.ID) {
		t.Errorf("expected delete to succeed")
	}
	if len(store.List()) != 0 {
		t.Errorf("expected 0 phrases after delete")
	}
}
