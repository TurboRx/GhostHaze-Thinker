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

func TestCustomSideServerConfig(t *testing.T) {
	// test dummy side server configuration
	os.Setenv("PS_SERVER_ID", "testserver")
	os.Setenv("PS_SERVER_HOST", "dummyhost.psim.us")
	os.Setenv("PS_SERVER_PORT", "8000")
	defer func() {
		os.Unsetenv("PS_SERVER_ID")
		os.Unsetenv("PS_SERVER_HOST")
		os.Unsetenv("PS_SERVER_PORT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.ServerID != "testserver" {
		t.Errorf("expected ServerID testserver, got %s", cfg.ServerID)
	}
	if cfg.ServerHost != "dummyhost.psim.us" {
		t.Errorf("expected ServerHost dummyhost.psim.us, got %s", cfg.ServerHost)
	}
	if cfg.ServerPort != 8000 {
		t.Errorf("expected ServerPort 8000, got %d", cfg.ServerPort)
	}
	expectedWS := "wss://dummyhost.psim.us:8000/showdown/websocket"
	if cfg.ServerURL != expectedWS {
		t.Errorf("expected ServerURL %s, got %s", expectedWS, cfg.ServerURL)
	}
	expectedLogin := "https://play.pokemonshowdown.com/~~testserver/action.php"
	if cfg.LoginURL != expectedLogin {
		t.Errorf("expected LoginURL %s, got %s", expectedLogin, cfg.LoginURL)
	}
}

