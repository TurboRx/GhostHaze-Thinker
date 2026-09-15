package web

import (
	"bytes"
	"html/template"
	"io/fs"
	"testing"
)

func TestHTMLTemplatesSyntaxAndExecution(t *testing.T) {
	// verify template files exist in embedded filesystem
	entries, err := fs.ReadDir(webFS, "templates")
	if err != nil {
		t.Fatalf("failed to read templates directory from embedded fs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no template files found in embedded filesystem")
	}

	// parse templates to ensure zero syntax errors
	indexTmpl, err := template.ParseFS(webFS, "templates/index.html")
	if err != nil {
		t.Fatalf("failed to parse index.html template: %v", err)
	}

	loginTmpl, err := template.ParseFS(webFS, "templates/login.html")
	if err != nil {
		t.Fatalf("failed to parse login.html template: %v", err)
	}

	t.Run("login template renders cleanly", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Error": "",
		}
		if err := loginTmpl.Execute(&buf, data); err != nil {
			t.Fatalf("failed to execute login.html template: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("GhostHaze-Thinker")) {
			t.Errorf("login template missing expected brand title")
		}
	})

	t.Run("index template renders cleanly with complete data", func(t *testing.T) {
		var buf bytes.Buffer
		mockData := map[string]any{
			"Connected":          true,
			"Stopped":            false,
			"LoggedIn":           true,
			"Username":           "GhostBot",
			"ServerID":           "showdown",
			"ServerHost":         "sim3.psim.us",
			"ServerPort":         443,
			"ServerSSL":          true,
			"Avatar":             "gengar",
			"CommandChar":        ".",
			"Rooms":              []string{"lobby", "tournaments"},
			"AllRooms":           []string{"lobby", "tournaments", "battle-gen9ou-1"},
			"ChatRoomsCount":     2,
			"ActiveBattlesCount": 1,
			"ConfigRooms":        "lobby, tournaments",
			"ActiveBattles": []map[string]any{
				{
					"room":     "battle-gen9randombattle-12345",
					"format":   "gen9randombattle",
					"turn":     12,
					"opponent": "OpponentUser",
				},
			},
			"AutoBattle":      true,
			"AutoLeaveBattle": true,
			"MaxBattles":      3,
			"BattleStartMsg":  "Good luck, have fun!",
			"BattleWinMsg":    "Good game!",
			"BattleLoseMsg":   "Well played!",
			"BattleFormats":   "gen9randombattle, gen9ou",
			"BattleTeam":      "default",
			"StatusMessage":   "Testing control panel",
			"Uptime":          "2h 45m",
			"ConTime":         "1h 10m",
			"AuthRequired":    true,
			"Authenticated":   true,
		}

		if err := indexTmpl.Execute(&buf, mockData); err != nil {
			t.Fatalf("failed to execute index.html template: %v", err)
		}
		output := buf.String()
		if !bytes.Contains([]byte(output), []byte("GhostHaze-Thinker")) {
			t.Errorf("index template missing expected brand title")
		}
		if !bytes.Contains([]byte(output), []byte("status-bar")) {
			t.Errorf("index template missing status-bar footer element")
		}
	})
}
