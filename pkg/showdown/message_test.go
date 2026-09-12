package showdown

import (
	"testing"
)

func TestParseRawStream(t *testing.T) {
	raw := ">botdevelopment\n|init|chat\n|c|alice|Hello everyone!\n|c:|1710000000|bob|testing 123\n|pm|charlie|ghosthaze thinker|hi bot"

	messages, finalRoom := ParseRawStream(raw, "")

	if finalRoom != "botdevelopment" {
		t.Fatalf("expected final room 'botdevelopment', got '%s'", finalRoom)
	}

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	if messages[0].Room != "botdevelopment" || messages[0].Type != "init" {
		t.Errorf("unexpected message 0: %+v", messages[0])
	}
	if messages[1].Room != "botdevelopment" || messages[1].Type != "c" {
		t.Errorf("unexpected message 1: %+v", messages[1])
	}
	if messages[2].Room != "botdevelopment" || messages[2].Type != "c:" {
		t.Errorf("unexpected message 2: %+v", messages[2])
	}
	if messages[3].Type != "pm" {
		t.Errorf("unexpected message 3: %+v", messages[3])
	}
}

func TestParseChatMessage(t *testing.T) {
	t.Run("Standard chat", func(t *testing.T) {
		msg := RawMessage{
			Room:  "botdevelopment",
			Type:  "c",
			Parts: []string{"ash", "Let's battle! | ready"},
			Raw:   "|c|ash|Let's battle! | ready",
		}

		chat, ok := ParseChatMessage(msg)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if chat.User != "ash" {
			t.Errorf("expected user 'ash', got '%s'", chat.User)
		}
		if chat.Text != "Let's battle! | ready" {
			t.Errorf("expected text 'Let's battle! | ready', got '%s'", chat.Text)
		}
		if chat.Room != "botdevelopment" {
			t.Errorf("expected room 'botdevelopment', got '%s'", chat.Room)
		}
	})

	t.Run("Timestamped chat", func(t *testing.T) {
		msg := RawMessage{
			Room:  "botdevelopment",
			Type:  "c:",
			Parts: []string{"1700000000", "gary", "Smell ya later!"},
			Raw:   "|c:|1700000000|gary|Smell ya later!",
		}

		chat, ok := ParseChatMessage(msg)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if chat.User != "gary" {
			t.Errorf("expected user 'gary', got '%s'", chat.User)
		}
		if chat.Text != "Smell ya later!" {
			t.Errorf("expected text 'Smell ya later!', got '%s'", chat.Text)
		}
		if chat.Timestamp.Unix() != 1700000000 {
			t.Errorf("expected timestamp %d, got %d", 1700000000, chat.Timestamp.Unix())
		}
		if chat.Room != "botdevelopment" {
			t.Errorf("expected room 'botdevelopment', got '%s'", chat.Room)
		}
	})
}

func TestParsePrivateMessage(t *testing.T) {
	msg := RawMessage{
		Type:  "pm",
		Parts: []string{"brock", "ghosthaze thinker", "Rock on!"},
		Raw:   "|pm|brock|ghosthaze thinker|Rock on!",
	}

	pm, ok := ParsePrivateMessage(msg)
	if !ok {
		t.Fatal("expected ok to be true")
	}
	if pm.From != "brock" || pm.To != "ghosthaze thinker" || pm.Text != "Rock on!" {
		t.Errorf("unexpected PM: %+v", pm)
	}
}

func TestParseUserUpdate(t *testing.T) {
	t.Run("Guest login", func(t *testing.T) {
		msg := RawMessage{
			Type:  "updateuser",
			Parts: []string{" Guest 1234 ", "0", "1"},
		}
		u, ok := ParseUserUpdate(msg)
		if !ok || !u.IsGuest || u.Username != "Guest 1234" {
			t.Errorf("unexpected guest update: %+v", u)
		}
	})

	t.Run("Registered user login", func(t *testing.T) {
		msg := RawMessage{
			Type:  "updateuser",
			Parts: []string{"ghosthaze thinker", "1", "169"},
		}
		u, ok := ParseUserUpdate(msg)
		if !ok || u.IsGuest || u.Username != "ghosthaze thinker" || u.Avatar != "169" {
			t.Errorf("unexpected user update: %+v", u)
		}
	})
}

func TestParseChallstr(t *testing.T) {
	msg := RawMessage{
		Type:  "challstr",
		Parts: []string{"4", "d89f81a74d42c38d8d"},
	}
	challstr, ok := ParseChallstr(msg)
	if !ok || challstr != "4|d89f81a74d42c38d8d" {
		t.Errorf("unexpected challstr: %s", challstr)
	}
}
