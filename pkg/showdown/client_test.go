package showdown

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRoomUserTracking(t *testing.T) {
	client := NewClient(Config{Username: "ghosthaze thinker"})

	// initially empty
	if users := client.RoomUsers("botdevelopment"); users != nil {
		t.Errorf("expected nil room users before init, got %v", users)
	}
	if client.IsInRoom("botdevelopment", "alice") {
		t.Error("expected alice not to be in room")
	}

	// users list broadcast
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "users",
		Parts: []string{"3,+Alice,@Bob,Charlie"},
	})

	if !client.IsInRoom("botdevelopment", "alice") {
		t.Error("expected alice to be in room")
	}
	if !client.IsInRoom("botdevelopment", "+Alice") {
		t.Error("expected +Alice to match in room")
	}
	if !client.IsInRoom("botdevelopment", "Bob") {
		t.Error("expected Bob to be in room")
	}
	if !client.IsInRoom("botdevelopment", "charlie") {
		t.Error("expected charlie to be in room")
	}
	if client.IsInRoom("botdevelopment", "david") {
		t.Error("expected david not to be in room")
	}

	if rank := client.UserRankInRoom("botdevelopment", "alice"); rank != "+" {
		t.Errorf("expected rank '+', got %q", rank)
	}
	if rank := client.UserRankInRoom("botdevelopment", "bob"); rank != "@" {
		t.Errorf("expected rank '@', got %q", rank)
	}
	if rank := client.UserRankInRoom("botdevelopment", "charlie"); rank != "" {
		t.Errorf("expected rank '', got %q", rank)
	}

	users := client.RoomUsers("botdevelopment")
	if len(users) != 3 {
		t.Fatalf("expected 3 room users, got %d", len(users))
	}

	// user join
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "J",
		Parts: []string{"%David"},
	})
	if !client.IsInRoom("botdevelopment", "david") {
		t.Error("expected david to be in room after join")
	}
	if rank := client.UserRankInRoom("botdevelopment", "david"); rank != "%" {
		t.Errorf("expected rank '%%', got %q", rank)
	}

	// user leave
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "L",
		Parts: []string{"@Bob"},
	})
	if client.IsInRoom("botdevelopment", "bob") {
		t.Error("expected bob not to be in room after leave")
	}

	// user rename
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "N",
		Parts: []string{"+Charles", "charlie"},
	})
	if client.IsInRoom("botdevelopment", "charlie") {
		t.Error("expected old name charlie to be removed")
	}
	if !client.IsInRoom("botdevelopment", "charles") {
		t.Error("expected new name charles to be present")
	}
	if rank := client.UserRankInRoom("botdevelopment", "charles"); rank != "+" {
		t.Errorf("expected rank '+', got %q", rank)
	}

	// deinit room
	client.processMessage(RawMessage{
		Room: "botdevelopment",
		Type: "deinit",
	})
	if client.RoomUsers("botdevelopment") != nil {
		t.Error("expected room users to be cleared on deinit")
	}
	if client.IsInRoom("botdevelopment", "charles") {
		t.Error("expected room to be empty after deinit")
	}
}

func TestIntroBacklogSuppression(t *testing.T) {
	client := NewClient(Config{CommandChar: ".", Username: "ghosthaze thinker"})

	cmdTriggered := make(chan struct{}, 5)
	client.HandleCommand("ping", func(room, user, args string) {
		cmdTriggered <- struct{}{}
	})

	// room init starts intro backlog
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "init",
		Parts: []string{"chat"},
	})

	// backlog chat arrives while in intro
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "c",
		Parts: []string{"alice", ".ping"},
	})

	select {
	case <-cmdTriggered:
		t.Fatal("expected command from intro backlog to be suppressed")
	case <-time.After(50 * time.Millisecond):
	}

	// room intro backlog ends with :
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  ":",
		Parts: []string{"1710000000"},
	})

	// live chat arrives after intro
	client.processMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "c",
		Parts: []string{"alice", ".ping"},
	})

	select {
	case <-cmdTriggered:
		// success: command executed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected live command to execute after intro backlog ended")
	}
}

func TestReplyHelperRouting(t *testing.T) {
	client := NewClient(Config{})

	// empty room and empty user
	if err := client.Reply("", "", "hi"); err == nil {
		t.Error("expected error for empty target")
	}

	// empty room -> routes to SendPM
	errPM := client.Reply("", "alice", "hi")
	if errPM == nil || !strings.Contains(errPM.Error(), "websocket is not connected") {
		t.Errorf("expected websocket disconnected error from SendPM, got %v", errPM)
	}

	// non-empty room -> routes to SendToRoom
	errRoom := client.Reply("botdevelopment", "alice", "hi")
	if errRoom == nil || !strings.Contains(errRoom.Error(), "websocket is not connected") {
		t.Errorf("expected websocket disconnected error from SendToRoom, got %v", errRoom)
	}
}

func TestDualAuthentication(t *testing.T) {
	t.Run("Unregistered account GET assertion", func(t *testing.T) {
		var receivedMethod, receivedQuery string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedQuery = r.URL.RawQuery
			_, _ = w.Write([]byte("assertion_token_123"))
		}))
		defer ts.Close()

		client := NewClient(Config{
			Username:   "guest bot 99",
			Password:   "",
			LoginURL:   ts.URL,
			HTTPClient: ts.Client(),
		})

		client.authenticate("challstr_value")

		if receivedMethod != "GET" {
			t.Errorf("expected GET method, got %s", receivedMethod)
		}
		if !strings.Contains(receivedQuery, "act=getassertion") || !strings.Contains(receivedQuery, "userid=guestbot99") || !strings.Contains(receivedQuery, "challstr=challstr_value") {
			t.Errorf("unexpected query parameters: %s", receivedQuery)
		}
	})

	t.Run("Registered account POST login", func(t *testing.T) {
		var receivedMethod, receivedContentType, receivedBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedMethod = r.Method
			receivedContentType = r.Header.Get("Content-Type")
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			receivedBody = buf.String()
			_, _ = w.Write([]byte(`]{"actionsuccess":true,"assertion":"assertion_token_456"}`))
		}))
		defer ts.Close()

		client := NewClient(Config{
			Username:   "ghosthaze thinker",
			Password:   "topsecret",
			LoginURL:   ts.URL,
			HTTPClient: ts.Client(),
		})

		client.authenticate("challstr_value")

		if receivedMethod != "POST" {
			t.Errorf("expected POST method, got %s", receivedMethod)
		}
		if !strings.Contains(receivedContentType, "application/x-www-form-urlencoded") {
			t.Errorf("unexpected content type: %s", receivedContentType)
		}
		if !strings.Contains(receivedBody, "act=login") || !strings.Contains(receivedBody, "name=ghosthazethinker") || !strings.Contains(receivedBody, "pass=topsecret") || !strings.Contains(receivedBody, "challstr=challstr_value") {
			t.Errorf("unexpected POST body: %s", receivedBody)
		}
	})
}
