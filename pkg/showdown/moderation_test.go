package showdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModerationStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "moderation_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "moderation.json")
	store := NewModerationStore(filePath)

	cfg := ModerationConfig{
		Enabled:        true,
		BannedWords:    []string{"badword", "trollphrase"},
		MaxCapsPercent: 70,
		CapsMinLength:  8,
		Action:         "warn",
		CustomWarning:  "Please keep the chatroom friendly.",
		ExemptRanks:    "+, %, @, *, #, ~",
	}

	if err := store.SaveConfig(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	// test normal clean message
	violation, _, _ := store.CheckMessage("RegularUser", "Hello everyone, hope you are having a nice day!")
	if violation {
		t.Errorf("expected no violation for clean message")
	}

	// test banned word
	violation, action, reason := store.CheckMessage("RegularUser", "You are such a badword!")
	if !violation {
		t.Errorf("expected violation for banned word")
	}
	if action != "warn" {
		t.Errorf("expected action 'warn', got '%s'", action)
	}
	if reason == "" {
		t.Errorf("expected reason to be populated")
	}

	// test excessive caps
	violationCaps, _, _ := store.CheckMessage("RegularUser", "THIS IS AN ALL CAPS SHOUTING MESSAGE")
	if !violationCaps {
		t.Errorf("expected violation for excessive caps")
	}

	// test short caps below min length
	violationShort, _, _ := store.CheckMessage("RegularUser", "LOL")
	if violationShort {
		t.Errorf("expected short message to be allowed")
	}

	// test exempt user rank (e.g. +voice or @mod)
	violationExempt, _, _ := store.CheckMessage("@ModUser", "THIS IS AN ALL CAPS SHOUTING MESSAGE")
	if violationExempt {
		t.Errorf("expected exempt rank to bypass moderation")
	}

	violationVoice, _, _ := store.CheckMessage("+VoiceUser", "THIS IS AN ALL CAPS SHOUTING MESSAGE")
	if violationVoice {
		t.Errorf("expected +voice exempt rank to bypass moderation")
	}
}
