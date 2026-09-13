package showdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandStore_Operations(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "commands.json")

	store := NewCommandStore(filePath)
	if len(store.List()) != 0 {
		t.Fatalf("expected empty commands store")
	}

	err := store.AddOrUpdate(CustomCommand{
		Name:     "discord",
		Response: "Join our discord at https://discord.gg/example",
		MinRank:  "all",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("failed to add command: %v", err)
	}

	cmd, exists := store.Get("discord")
	if !exists {
		t.Fatal("expected discord command to exist")
	}
	if cmd.Response != "Join our discord at https://discord.gg/example" {
		t.Fatalf("unexpected response: %s", cmd.Response)
	}

	enabled, err := store.Toggle("discord")
	if err != nil {
		t.Fatalf("failed to toggle command: %v", err)
	}
	if enabled {
		t.Fatal("expected command to be disabled")
	}

	if !store.Delete("discord") {
		t.Fatal("expected delete to succeed")
	}
	if len(store.List()) != 0 {
		t.Fatal("expected 0 commands")
	}

	_ = os.Remove(filePath)
}

func TestRankLevel_And_CanExecute(t *testing.T) {
	if !CanExecute("+", "+") {
		t.Fatal("voice should be able to execute voice command")
	}
	if !CanExecute("@", "+") {
		t.Fatal("mod should be able to execute voice command")
	}
	if CanExecute(" ", "+") {
		t.Fatal("regular user should not be able to execute voice command")
	}
	if !CanExecute("~", "@") {
		t.Fatal("admin should be able to execute mod command")
	}
	if CanExecute("+", "@") {
		t.Fatal("voice should not be able to execute mod command")
	}
}

func TestFormatCommandResponse(t *testing.T) {
	tpl := "Hello {user}, I am {bot}! You said: {args}"
	out := FormatCommandResponse(tpl, "Alice", "MyBot", "foo bar")
	expected := "Hello Alice, I am MyBot! You said: foo bar"
	if out != expected {
		t.Fatalf("expected '%s', got '%s'", expected, out)
	}
}
