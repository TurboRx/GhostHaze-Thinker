package showdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeenStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "seen_test")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "seen.json")
	store := NewSeenStore(filePath)

	// verify initially empty
	if _, found := store.Get("AshKetchum"); found {
		t.Errorf("expected not found for new user")
	}

	// record user activity
	store.Record("AshKetchum", "lobby", "Pikachu, I choose you!")

	entry, found := store.Get("ashketchum")
	if !found {
		t.Fatalf("expected user to be found")
	}
	if entry.Username != "AshKetchum" || entry.Room != "lobby" {
		t.Errorf("unexpected entry data: %+v", entry)
	}

	// verify getall
	all := store.GetAll(10)
	if len(all) != 1 {
		t.Errorf("expected 1 entry in getall, got %d", len(all))
	}

	// verify persistence across reload
	reloaded := NewSeenStore(filePath)
	if _, ok := reloaded.Get("ashketchum"); !ok {
		t.Errorf("expected entry to persist across reload")
	}

	// verify clear
	if err := store.Clear(); err != nil {
		t.Errorf("clear failed: %v", err)
	}
	if len(store.GetAll(10)) != 0 {
		t.Errorf("expected 0 entries after clear")
	}
}
