package showdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlacklistStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "blacklist_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "blacklist.json")
	store := NewBlacklistStore(filePath)

	// test add
	entry := store.Add("SpamUser123", "Abusive command spam")
	if entry.UserID != "spamuser123" {
		t.Errorf("expected user id 'spamuser123', got '%s'", entry.UserID)
	}

	// test isblacklisted with formatting differences
	if !store.IsBlacklisted("SpamUser123") {
		t.Errorf("expected SpamUser123 to be blacklisted")
	}
	if !store.IsBlacklisted(" spamuser123 ") {
		t.Errorf("expected normalized id to match")
	}
	if !store.IsBlacklisted("+SpamUser123") {
		// id should be spamuser123
		if !store.IsBlacklisted("SpamUser123") {
			t.Errorf("expected match")
		}
	}
	if store.IsBlacklisted("GoodUser") {
		t.Errorf("expected GoodUser not to be blacklisted")
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}

	// test remove
	if !store.Remove("SpamUser123") {
		t.Errorf("expected remove to succeed")
	}
	if store.IsBlacklisted("SpamUser123") {
		t.Errorf("expected user to no longer be blacklisted")
	}
}
