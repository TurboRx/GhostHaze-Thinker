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
	client := NewClient(Config{CommandChar: "."})

	var wg sync.WaitGroup
	var receivedRoom, receivedUser, receivedArgs string

	wg.Add(1)
	client.HandleCommand("ping", func(room, user, args string) {
		receivedRoom = room
		receivedUser = user
		receivedArgs = args
		wg.Done()
	})

	client.routeCommand("lobby", "alice", ".ping hello world")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if receivedRoom != "lobby" || receivedUser != "alice" || receivedArgs != "hello world" {
			t.Errorf("unexpected command execution: room=%s, user=%s, args=%s", receivedRoom, receivedUser, receivedArgs)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for command execution")
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
