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
	t.Run("Standard PM", func(t *testing.T) {
		msg := RawMessage{
			Type:  "pm",
			Parts: []string{"brock", "ghosthaze thinker", "Rock on!"},
			Raw:   "|pm|brock|ghosthaze thinker|Rock on!",
		}

		pm, ok := ParsePrivateMessage(msg)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if pm.From != "brock" || pm.To != "ghosthaze thinker" || pm.Text != "Rock on!" || pm.IsHidden {
			t.Errorf("unexpected PM: %+v", pm)
		}
	})

	t.Run("Botmsg hidden PM", func(t *testing.T) {
		msg := RawMessage{
			Type:  "pm",
			Parts: []string{"~admin", "ghosthaze thinker", "/botmsg .ping test"},
			Raw:   "|pm|~admin|ghosthaze thinker|/botmsg .ping test",
		}

		pm, ok := ParsePrivateMessage(msg)
		if !ok {
			t.Fatal("expected ok to be true")
		}
		if pm.From != "~admin" || pm.To != "ghosthaze thinker" || pm.Text != ".ping test" || !pm.IsHidden {
			t.Errorf("unexpected hidden PM: %+v", pm)
		}
	})
}

func TestToRoomID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"botdevelopment", "botdevelopment"},
		{"Bot Development", "botdevelopment"},
		{"battle-gen9ou-12345", "battle-gen9ou-12345"},
		{"groupchat-botdevelopment-secret", "groupchat-botdevelopment-secret"},
		{"Lobby!", "lobby"},
		{"--room--", "--room--"},
	}

	for _, tt := range tests {
		if got := ToRoomID(tt.input); got != tt.expected {
			t.Errorf("ToRoomID(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
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

func TestEscapeChat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/wall hello", " /wall hello"},
		{"!dt pikachu", " !dt pikachu"},
		{"/kick user", " /kick user"},
		{"normal message", "normal message"},
		{"  /nested", "   /nested"},
	}

	for _, tt := range tests {
		if got := EscapeChat(tt.input); got != tt.expected {
			t.Errorf("EscapeChat(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestAwayStatusParsing(t *testing.T) {
	if !IsAway("+Alice@!") {
		t.Error("expected +Alice@! to be away")
	}
	if IsAway("+Alice") {
		t.Error("expected +Alice not to be away")
	}
	if CleanUsername("+Alice@!") != "Alice" {
		t.Errorf("expected 'Alice', got %q", CleanUsername("+Alice@!"))
	}

	chat, ok := ParseChatMessage(RawMessage{
		Room:  "botdevelopment",
		Type:  "c",
		Parts: []string{"+Alice@!", "hello"},
	})
	if !ok || !chat.Away || chat.CleanUser() != "Alice" {
		t.Errorf("unexpected chat away result: %+v", chat)
	}

	pm, ok := ParsePrivateMessage(RawMessage{
		Type:  "pm",
		Parts: []string{"Bob@!", "ghosthaze thinker", "ping"},
	})
	if !ok || !pm.Away || pm.CleanFrom() != "Bob" {
		t.Errorf("unexpected PM away result: %+v", pm)
	}

	uu, ok := ParseUserUpdate(RawMessage{
		Type:  "updateuser",
		Parts: []string{"ghosthaze thinker@!", "1", "169"},
	})
	if !ok || !uu.Away || uu.Username != "ghosthaze thinker" {
		t.Errorf("unexpected updateuser away result: %+v", uu)
	}
}

func TestParseFormats(t *testing.T) {
	msg := RawMessage{
		Type: "formats",
		Parts: []string{
			",[Gen 9] Singles",
			"gen9ou,1",
			"gen9ubers,1",
			",[Gen 9] Doubles",
			"gen9doublesou,2",
		},
	}

	formats := ParseFormats(msg)
	if len(formats) != 3 {
		t.Fatalf("expected 3 formats, got %d", len(formats))
	}

	if formats[0].ID != "gen9ou" || formats[0].Name != "gen9ou" || formats[0].Section != "[Gen 9] Singles" {
		t.Errorf("unexpected format 0: %+v", formats[0])
	}
	if formats[1].ID != "gen9ubers" || formats[1].Section != "[Gen 9] Singles" {
		t.Errorf("unexpected format 1: %+v", formats[1])
	}
	if formats[2].ID != "gen9doublesou" || formats[2].Section != "[Gen 9] Doubles" {
		t.Errorf("unexpected format 2: %+v", formats[2])
	}

	// test modern format stream with numeric column headers
	modernMsg := RawMessage{
		Type: "formats",
		Parts: []string{
			",1",
			"S/V Singles",
			"[Gen 9] Random Battle,4f",
			"[Gen 9] OU,e",
			",2",
			"Randomized Metas",
			"Battle Factory,4f",
		},
	}
	modernFormats := ParseFormats(modernMsg)
	if len(modernFormats) != 3 {
		t.Fatalf("expected 3 modern formats, got %d", len(modernFormats))
	}
	if modernFormats[0].ID != "gen9randombattle" || modernFormats[0].Name != "[Gen 9] Random Battle" || modernFormats[0].Section != "S/V Singles" {
		t.Errorf("unexpected modern format 0: %+v", modernFormats[0])
	}
	if modernFormats[1].ID != "gen9ou" || modernFormats[1].Name != "[Gen 9] OU" || modernFormats[1].Section != "S/V Singles" {
		t.Errorf("unexpected modern format 1: %+v", modernFormats[1])
	}
	if modernFormats[2].ID != "battlefactory" || modernFormats[2].Section != "Randomized Metas" {
		t.Errorf("unexpected modern format 2: %+v", modernFormats[2])
	}
}
