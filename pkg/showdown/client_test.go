package showdown

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("expected server url %s, got %s", DefaultServerURL, cfg.ServerURL)
	}
	if cfg.LoginURL != DefaultLoginURL {
		t.Errorf("expected login url %s, got %s", DefaultLoginURL, cfg.LoginURL)
	}
	if len(cfg.Rooms) != 1 || cfg.Rooms[0] != DefaultRoom {
		t.Errorf("expected rooms [%s], got %v", DefaultRoom, cfg.Rooms)
	}
	if cfg.ReconnectDelay != DefaultReconnectDelay {
		t.Errorf("expected reconnect delay %v, got %v", DefaultReconnectDelay, cfg.ReconnectDelay)
	}
	if cfg.CommandChar != DefaultCommandChar {
		t.Errorf("expected command char %s, got %s", DefaultCommandChar, cfg.CommandChar)
	}
	if cfg.HTTPClient == nil {
		t.Error("expected non-nil HTTPClient")
	}
}

func TestRouteCommand(t *testing.T) {
	client := NewClient(Config{CommandChar: ".", Username: "ghosthaze thinker"})

	var wg sync.WaitGroup
	var receivedRoom, receivedUser, receivedArgs string

	wg.Add(1)
	client.HandleCommand("ping", func(room, user, args string) {
		receivedRoom = room
		receivedUser = user
		receivedArgs = args
		wg.Done()
	})

	// test with rank symbol (+alice)
	client.routeCommand("botdevelopment", "+alice", ".ping hello world")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if receivedRoom != "botdevelopment" || receivedUser != "alice" || receivedArgs != "hello world" {
			t.Errorf("unexpected command execution: room=%s, user=%s, args=%s", receivedRoom, receivedUser, receivedArgs)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for command execution")
	}

	// test self-command guard (bot must ignore its own commands)
	selfChan := make(chan struct{}, 1)
	client.HandleCommand("self", func(room, user, args string) {
		selfChan <- struct{}{}
	})
	client.routeCommand("botdevelopment", "*ghosthaze thinker", ".self")
	select {
	case <-selfChan:
		t.Error("expected self-command to be ignored")
	case <-time.After(50 * time.Millisecond):
	}

	// test command with spaces after prefix
	echoChan := make(chan string, 1)
	client.HandleCommand("echo", func(room, user, args string) {
		echoChan <- args
	})
	client.routeCommand("botdevelopment", "bob", ".   echo   foo bar  ")
	select {
	case echoArgs := <-echoChan:
		if echoArgs != "foo bar" {
			t.Errorf("expected echoArgs 'foo bar', got '%s'", echoArgs)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for echo command")
	}
}

func TestUsernameAndIDHelpers(t *testing.T) {
	if CleanUsername("+Alice") != "Alice" {
		t.Errorf("expected 'Alice', got '%s'", CleanUsername("+Alice"))
	}
	if CleanUsername("@Bob") != "Bob" {
		t.Errorf("expected 'Bob', got '%s'", CleanUsername("@Bob"))
	}
	if CleanUsername("@+Charlie") != "Charlie" {
		t.Errorf("expected 'Charlie', got '%s'", CleanUsername("@+Charlie"))
	}
	if CleanUsername(" Dave") != "Dave" {
		t.Errorf("expected 'Dave', got '%s'", CleanUsername(" Dave"))
	}
	if UserRank("+Alice") != "+" {
		t.Errorf("expected '+', got '%s'", UserRank("+Alice"))
	}
	if UserRank("Normal") != "" {
		t.Errorf("expected '', got '%s'", UserRank("Normal"))
	}
	if ToID("Bot Development 123!") != "botdevelopment123" {
		t.Errorf("expected 'botdevelopment123', got '%s'", ToID("Bot Development 123!"))
	}
}

func TestValidationGuards(t *testing.T) {
	client := NewClient(Config{})
	if err := client.SendPM("", "hello"); err == nil {
		t.Error("expected error for empty PM target")
	}
	if err := client.SendToRoom("", "hello"); err == nil {
		t.Error("expected error for empty room")
	}
	if err := client.JoinRoom(""); err == nil {
		t.Error("expected error for empty join room")
	}
	if err := client.LeaveRoom(""); err == nil {
		t.Error("expected error for empty leave room")
	}
	if err := client.SetAvatar(""); err == nil {
		t.Error("expected error for empty avatar")
	}
}

func TestLoginResponseJSON(t *testing.T) {
	t.Run("Valid assertion with bracket prefix", func(t *testing.T) {
		raw := []byte(`]{"actionsuccess":true,"assertion":"valid_assertion_token"}`)
		clean := bytes.TrimPrefix(raw, []byte("]"))

		var resp loginResponse
		if err := json.Unmarshal(clean, &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if !resp.ActionSuccess || resp.Assertion != "valid_assertion_token" {
			t.Errorf("unexpected unmarshal result: %+v", resp)
		}
	})

	t.Run("Error assertion", func(t *testing.T) {
		raw := []byte(`]{"assertion":";;wrong password"}`)
		clean := bytes.TrimPrefix(raw, []byte("]"))

		var resp loginResponse
		if err := json.Unmarshal(clean, &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if resp.Assertion != ";;wrong password" {
			t.Errorf("unexpected assertion: %s", resp.Assertion)
		}
	})
}
