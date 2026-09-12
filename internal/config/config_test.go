package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEnvFile(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")

	content := `
PS_USERNAME="ghosthaze thinker"
PS_PASSWORD='SecretPassword'
PS_ROOMS=botdevelopment # target room
PS_RECONNECT_DELAY_MS=5000
PS_COMMAND_CHAR=!
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test .env file: %v", err)
	}

	os.Unsetenv("PS_USERNAME")
	os.Unsetenv("PS_PASSWORD")
	os.Unsetenv("PS_ROOMS")
	os.Unsetenv("PS_RECONNECT_DELAY_MS")
	os.Unsetenv("PS_COMMAND_CHAR")

	if err := LoadEnvFile(envPath); err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}

	if val := os.Getenv("PS_USERNAME"); val != "ghosthaze thinker" {
		t.Errorf("expected PS_USERNAME to be 'ghosthaze thinker', got '%s'", val)
	}
	if val := os.Getenv("PS_PASSWORD"); val != "SecretPassword" {
		t.Errorf("expected PS_PASSWORD to be 'SecretPassword', got '%s'", val)
	}
	if val := os.Getenv("PS_COMMAND_CHAR"); val != "!" {
		t.Errorf("expected PS_COMMAND_CHAR to be '!', got '%s'", val)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Username != "ghosthaze thinker" {
		t.Errorf("expected Username 'ghosthaze thinker', got '%s'", cfg.Username)
	}
	if cfg.ReconnectDelay != 5*time.Second {
		t.Errorf("expected ReconnectDelay 5s, got %v", cfg.ReconnectDelay)
	}
	if len(cfg.Rooms) != 1 || cfg.Rooms[0] != "botdevelopment" {
		t.Errorf("unexpected rooms: %v", cfg.Rooms)
	}
	if cfg.CommandChar != "!" {
		t.Errorf("expected CommandChar '!', got '%s'", cfg.CommandChar)
	}
}
