package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
)

func TestWebServerEndpoints(t *testing.T) {
	t.Cleanup(func() {
		_ = os.RemoveAll("logs")
	})

	// initialize test client and server
	cfg := showdown.Config{
		ServerID:   "testserver",
		ServerHost: "dummyhost.psim.us",
		Username:   "testbot",
		Rooms:      []string{"botdevelopment"},
	}
	client := showdown.NewClient(cfg)

	srv, err := NewServer(client, "127.0.0.1", 8080)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux, err := srv.buildMux()
	if err != nil {
		t.Fatalf("failed to build mux: %v", err)
	}

	srv.sessionMu.Lock()
	srv.sessions["test-session"] = time.Now().Add(time.Hour)
	srv.sessionMu.Unlock()

	authReq := func(req *http.Request) *http.Request {
		req.AddCookie(&http.Cookie{Name: "ghosthaze_session", Value: "test-session"})
		return req
	}

	t.Run("index html renders successfully", func(t *testing.T) {
		req := authReq(httptest.NewRequest(http.MethodGet, "/", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !bytes.Contains([]byte(body), []byte("GhostHaze-Thinker")) {
			t.Errorf("expected body to contain GhostHaze-Thinker")
		}
	})

	t.Run("api status returns valid json", func(t *testing.T) {
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/status", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		var data map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
			t.Fatalf("failed to decode json: %v", err)
		}
		if data["server_id"] != "testserver" {
			t.Errorf("expected server_id testserver, got %v", data["server_id"])
		}
	})

	t.Run("static assets serve correctly", func(t *testing.T) {
		for _, path := range []string{"/static/style.css", "/static/app.js"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected 200 for %s, got %d", path, rr.Code)
			}
			if rr.Body.Len() == 0 {
				t.Errorf("expected non-empty body for %s", path)
			}
		}
	})

	t.Run("logs api and addlog", func(t *testing.T) {
		srv.AddLog("system", "TestUnit", "Test log message")
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/logs", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		var logs []LogEntry
		if err := json.Unmarshal(rr.Body.Bytes(), &logs); err != nil {
			t.Fatalf("failed to decode logs: %v", err)
		}
		if len(logs) == 0 {
			t.Errorf("expected at least one log entry")
		}
	})

	t.Run("get-server tool api fast path", func(t *testing.T) {
		body := bytes.NewBufferString(`{"url":"play.pokemonshowdown.com"}`)
		req := authReq(httptest.NewRequest(http.MethodPost, "/api/tools/get-server", body))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse response json: %v", err)
		}
		if res["id"] != "showdown" {
			t.Errorf("expected id showdown, got %v", res["id"])
		}
	})

	t.Run("not found for unknown path", func(t *testing.T) {
		req := authReq(httptest.NewRequest(http.MethodGet, "/unknown/path", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("formats api endpoint", func(t *testing.T) {
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/formats", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for formats api, got %d", rr.Code)
		}
	})

	t.Run("config update and avatar endpoints", func(t *testing.T) {
		configPayload := `{"server_id":"dummytest","server_host":"dummytest.psim.us","server_port":8000,"server_ssl":true,"command_char":"!","avatar":"123","auto_battle":true}`
		req := authReq(httptest.NewRequest(http.MethodPost, "/api/config/update", bytes.NewBufferString(configPayload)))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		// test avatar endpoint
		avatarPayload := `{"avatar":"169"}`
		avatarReq := authReq(httptest.NewRequest(http.MethodPost, "/api/bot/avatar", bytes.NewBufferString(avatarPayload)))
		avatarRR := httptest.NewRecorder()
		mux.ServeHTTP(avatarRR, avatarReq)

		// in disconnected mock state avatar returns 200 or 500 depending on conn
		if avatarRR.Code != http.StatusOK && avatarRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status: %d", avatarRR.Code)
		}

		// test reconnect endpoint
		reconnectReq := authReq(httptest.NewRequest(http.MethodPost, "/api/bot/reconnect", nil))
		reconnectRR := httptest.NewRecorder()
		mux.ServeHTTP(reconnectRR, reconnectReq)

		if reconnectRR.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", reconnectRR.Code)
		}

		// test battle forfeit and leave endpoints
		forfeitReq := authReq(httptest.NewRequest(http.MethodPost, "/api/battles/forfeit", bytes.NewBufferString(`{"room":"battle-gen9randombattle-9999"}`)))
		forfeitRR := httptest.NewRecorder()
		mux.ServeHTTP(forfeitRR, forfeitReq)
		if forfeitRR.Code != http.StatusOK && forfeitRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status for forfeit: %d", forfeitRR.Code)
		}

		leaveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/battles/leave", bytes.NewBufferString(`{"room":"battle-gen9randombattle-9999"}`)))
		leaveRR := httptest.NewRecorder()
		mux.ServeHTTP(leaveRR, leaveReq)
		if leaveRR.Code != http.StatusOK && leaveRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status for leave: %d", leaveRR.Code)
		}

		// test bot login endpoint
		loginPayload := `{"username":"testeruser","password":"mypassword"}`
		loginReq := authReq(httptest.NewRequest(http.MethodPost, "/api/bot/login", bytes.NewBufferString(loginPayload)))
		loginRR := httptest.NewRecorder()
		mux.ServeHTTP(loginRR, loginReq)
		if loginRR.Code != http.StatusOK {
			t.Errorf("expected 200 for bot login, got %d: %s", loginRR.Code, loginRR.Body.String())
		}
	})

	t.Run("raw logs endpoint serves plain text", func(t *testing.T) {
		srv.AddLog("chat", "lobby", "hello world from raw test")
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/logs/raw", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for raw logs, got %d", rr.Code)
		}
		contentType := rr.Header().Get("Content-Type")
		if !bytes.Contains([]byte(contentType), []byte("text/plain")) {
			t.Errorf("expected text/plain content type, got %s", contentType)
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte("hello world from raw test")) {
			t.Errorf("expected raw log content in response")
		}
	})

	t.Run("logs clear endpoint", func(t *testing.T) {
		srv.AddLog("chat", "lobby", "message to be cleared")
		req := authReq(httptest.NewRequest(http.MethodPost, "/api/logs/clear", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for clear logs, got %d", rr.Code)
		}

		logs := srv.LogsList()
		// should only contain the single clear log entry
		for _, l := range logs {
			if l.Message == "message to be cleared" {
				t.Errorf("expected 'message to be cleared' to have been deleted")
			}
		}
	})

	t.Run("bot stop endpoint", func(t *testing.T) {
		req := authReq(httptest.NewRequest(http.MethodPost, "/api/bot/stop", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for bot stop, got %d", rr.Code)
		}
		if !client.IsStopped() {
			t.Errorf("expected client to be stopped")
		}

		// resume with reconnect
		reconnectReq := authReq(httptest.NewRequest(http.MethodPost, "/api/bot/reconnect", nil))
		reconnectRR := httptest.NewRecorder()
		mux.ServeHTTP(reconnectRR, reconnectReq)
		if reconnectRR.Code != http.StatusOK {
			t.Errorf("expected 200 for bot reconnect, got %d", reconnectRR.Code)
		}
		if client.IsStopped() {
			t.Errorf("expected client to be resumed")
		}
	})

	t.Run("backup download and restore", func(t *testing.T) {
		// download backup
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/backup/download", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for backup download, got %d", rr.Code)
		}

		var payload BackupPayload
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to parse backup json: %v", err)
		}
		if payload.Signature != BackupSignature {
			t.Errorf("expected signature %s, got %s", BackupSignature, payload.Signature)
		}

		// modify payload and restore
		payload.Config.CommandChar = "$"
		payload.Config.Rooms = []string{"lobby"}
		modifiedBytes, _ := json.Marshal(payload)

		restoreReq := authReq(httptest.NewRequest(http.MethodPost, "/api/backup/restore", bytes.NewBuffer(modifiedBytes)))
		restoreReq.Header.Set("Content-Type", "application/json")
		restoreRR := httptest.NewRecorder()
		mux.ServeHTTP(restoreRR, restoreReq)

		if restoreRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for restore, got %d: %s", restoreRR.Code, restoreRR.Body.String())
		}

		activeCfg := client.ClientConfig()
		if activeCfg.CommandChar != "$" {
			t.Errorf("expected restored command char '$', got %s", activeCfg.CommandChar)
		}
	})

	t.Run("teams endpoints crud", func(t *testing.T) {
		// clean up any existing teams for test isolation
		for _, tm := range client.Teams().List() {
			client.Teams().Delete(tm.ID)
		}
		defer func() {
			for _, tm := range client.Teams().List() {
				client.Teams().Delete(tm.ID)
			}
		}()

		// save team
		teamJSON := `{"name":"OU Team","format":"gen9ou","team_raw":"Pikachu @ Light Ball\nAbility: Lightning Rod\n- Thunderbolt\n","active":true}`
		saveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/teams/save", bytes.NewBufferString(teamJSON)))
		saveReq.Header.Set("Content-Type", "application/json")
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)
		if saveRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for teams save, got %d: %s", saveRR.Code, saveRR.Body.String())
		}

		// list teams
		listReq := authReq(httptest.NewRequest(http.MethodGet, "/api/teams", nil))
		listRR := httptest.NewRecorder()
		mux.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for teams list, got %d", listRR.Code)
		}
		var teams []showdown.BattleTeam
		if err := json.Unmarshal(listRR.Body.Bytes(), &teams); err != nil || len(teams) != 1 {
			t.Fatalf("expected 1 team returned, got %v", teams)
		}
		teamID := teams[0].ID

		// toggle team
		toggleJSON := `{"id":"` + teamID + `"}`
		toggleReq := authReq(httptest.NewRequest(http.MethodPost, "/api/teams/toggle", bytes.NewBufferString(toggleJSON)))
		toggleReq.Header.Set("Content-Type", "application/json")
		toggleRR := httptest.NewRecorder()
		mux.ServeHTTP(toggleRR, toggleReq)
		if toggleRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for teams toggle, got %d", toggleRR.Code)
		}

		// import team via pokepaste
		mockPaste := "Garchomp @ Life Orb\nAbility: Rough Skin\n- Earthquake\n- Outrage\n"
		pokeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockPaste))
		}))
		defer pokeServer.Close()
		origClient := showdown.SetPokepasteHTTPClient
		_ = origClient
		showdown.SetPokepasteHTTPClient(pokeServer.Client())
		showdown.SetPokepasteBaseURL(pokeServer.URL)
		defer showdown.SetPokepasteBaseURL("")

		importJSON := `{"url":"https://pokepast.es/testimport","format":"gen9ou","name":"Imported Garchomp"}`
		importReq := authReq(httptest.NewRequest(http.MethodPost, "/api/teams/import", bytes.NewBufferString(importJSON)))
		importReq.Header.Set("Content-Type", "application/json")
		importRR := httptest.NewRecorder()
		mux.ServeHTTP(importRR, importReq)
		if importRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for teams import, got %d: %s", importRR.Code, importRR.Body.String())
		}

		// delete team
		delJSON := `{"id":"` + teamID + `"}`
		delReq := authReq(httptest.NewRequest(http.MethodPost, "/api/teams/delete", bytes.NewBufferString(delJSON)))
		delReq.Header.Set("Content-Type", "application/json")
		delRR := httptest.NewRecorder()
		mux.ServeHTTP(delRR, delReq)
		if delRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for teams delete, got %d", delRR.Code)
		}
	})

	t.Run("commands endpoints crud", func(t *testing.T) {
		// save command
		cmdJSON := `{"name":"rules","response":"Be respectful!","min_rank":"all","enabled":true}`
		saveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/commands/save", bytes.NewBufferString(cmdJSON)))
		saveReq.Header.Set("Content-Type", "application/json")
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)
		if saveRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for commands save, got %d: %s", saveRR.Code, saveRR.Body.String())
		}

		// list commands
		listReq := authReq(httptest.NewRequest(http.MethodGet, "/api/commands", nil))
		listRR := httptest.NewRecorder()
		mux.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for commands list, got %d", listRR.Code)
		}
		var cmds []showdown.CustomCommand
		if err := json.Unmarshal(listRR.Body.Bytes(), &cmds); err != nil || len(cmds) != 1 {
			t.Fatalf("expected 1 command returned, got %v", cmds)
		}

		// toggle command
		toggleJSON := `{"name":"rules"}`
		toggleReq := authReq(httptest.NewRequest(http.MethodPost, "/api/commands/toggle", bytes.NewBufferString(toggleJSON)))
		toggleReq.Header.Set("Content-Type", "application/json")
		toggleRR := httptest.NewRecorder()
		mux.ServeHTTP(toggleRR, toggleReq)
		if toggleRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for commands toggle, got %d", toggleRR.Code)
		}

		// delete command
		delJSON := `{"name":"rules"}`
		delReq := authReq(httptest.NewRequest(http.MethodPost, "/api/commands/delete", bytes.NewBufferString(delJSON)))
		delReq.Header.Set("Content-Type", "application/json")
		delRR := httptest.NewRecorder()
		mux.ServeHTTP(delRR, delReq)
		if delRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for commands delete, got %d", delRR.Code)
		}
	})

	t.Run("battle history endpoints", func(t *testing.T) {
		// record dummy battle directly into client history
		_ = client.History().Record(showdown.BattleRecord{
			BattleID:   "battle-gen9ou-test",
			Room:       "battle-gen9ou-test",
			Format:     "gen9ou",
			Opponent:   "UserBlue",
			Outcome:    "win",
			Turns:      12,
			FinishedAt: time.Now(),
		})

		// fetch history
		req := authReq(httptest.NewRequest(http.MethodGet, "/api/battles/history?limit=10", nil))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for battle history, got %d", rr.Code)
		}

		var payload struct {
			Records []showdown.BattleRecord `json:"records"`
			Stats   showdown.BattleStats    `json:"stats"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil || len(payload.Records) == 0 {
			t.Fatalf("expected records returned, got %v", payload)
		}

		// clear history
		clearReq := authReq(httptest.NewRequest(http.MethodPost, "/api/battles/history/clear", nil))
		clearRR := httptest.NewRecorder()
		mux.ServeHTTP(clearRR, clearReq)
		if clearRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for history clear, got %d", clearRR.Code)
		}
	})

	t.Run("timers endpoints crud and trigger", func(t *testing.T) {
		// save timer
		tJSON := `{"name":"Welcome","room":"lobby","interval_minutes":15,"message":"Welcome to the chatroom!","enabled":true}`
		saveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/timers/save", bytes.NewBufferString(tJSON)))
		saveReq.Header.Set("Content-Type", "application/json")
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)
		if saveRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for timer save, got %d: %s", saveRR.Code, saveRR.Body.String())
		}

		// list timers
		listReq := authReq(httptest.NewRequest(http.MethodGet, "/api/timers", nil))
		listRR := httptest.NewRecorder()
		mux.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for timers list, got %d", listRR.Code)
		}
		var listResp struct {
			Timers []showdown.ChatroomTimer `json:"timers"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil || len(listResp.Timers) != 1 {
			t.Fatalf("expected 1 timer, got %v", listResp.Timers)
		}
		timerID := listResp.Timers[0].ID

		// toggle timer
		togJSON := `{"id":"` + timerID + `"}`
		togReq := authReq(httptest.NewRequest(http.MethodPost, "/api/timers/toggle", bytes.NewBufferString(togJSON)))
		togReq.Header.Set("Content-Type", "application/json")
		togRR := httptest.NewRecorder()
		mux.ServeHTTP(togRR, togReq)
		if togRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for timer toggle, got %d", togRR.Code)
		}

		// delete timer
		delJSON := `{"id":"` + timerID + `"}`
		delReq := authReq(httptest.NewRequest(http.MethodPost, "/api/timers/delete", bytes.NewBufferString(delJSON)))
		delReq.Header.Set("Content-Type", "application/json")
		delRR := httptest.NewRecorder()
		mux.ServeHTTP(delRR, delReq)
		if delRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for timer delete, got %d", delRR.Code)
		}
	})

	t.Run("blacklist endpoints", func(t *testing.T) {
		// add user to blacklist
		blJSON := `{"username":"TrollMaster","reason":"Spamming commands"}`
		addReq := authReq(httptest.NewRequest(http.MethodPost, "/api/blacklist/add", bytes.NewBufferString(blJSON)))
		addReq.Header.Set("Content-Type", "application/json")
		addRR := httptest.NewRecorder()
		mux.ServeHTTP(addRR, addReq)
		if addRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for blacklist add, got %d: %s", addRR.Code, addRR.Body.String())
		}

		// list blacklist
		listReq := authReq(httptest.NewRequest(http.MethodGet, "/api/blacklist", nil))
		listRR := httptest.NewRecorder()
		mux.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for blacklist list, got %d", listRR.Code)
		}
		var blResp struct {
			Blacklist []showdown.BlacklistEntry `json:"blacklist"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &blResp); err != nil || len(blResp.Blacklist) != 1 {
			t.Fatalf("expected 1 blacklisted user, got %v", blResp.Blacklist)
		}

		// remove user from blacklist
		rmJSON := `{"username":"TrollMaster"}`
		rmReq := authReq(httptest.NewRequest(http.MethodPost, "/api/blacklist/remove", bytes.NewBufferString(rmJSON)))
		rmReq.Header.Set("Content-Type", "application/json")
		rmRR := httptest.NewRecorder()
		mux.ServeHTTP(rmRR, rmReq)
		if rmRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for blacklist remove, got %d", rmRR.Code)
		}
	})

	t.Run("joinphrases endpoints", func(t *testing.T) {
		// save join phrase
		jpJSON := `{"username":"PokemonUser","room":"lobby","phrase":"Welcome {user}!","enabled":true}`
		saveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/joinphrases/save", bytes.NewBufferString(jpJSON)))
		saveReq.Header.Set("Content-Type", "application/json")
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)
		if saveRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for join phrase save, got %d: %s", saveRR.Code, saveRR.Body.String())
		}

		// list join phrases
		listReq := authReq(httptest.NewRequest(http.MethodGet, "/api/joinphrases", nil))
		listRR := httptest.NewRecorder()
		mux.ServeHTTP(listRR, listReq)
		if listRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for join phrases list, got %d", listRR.Code)
		}
		var jpResp struct {
			JoinPhrases []showdown.JoinPhrase `json:"joinphrases"`
		}
		if err := json.Unmarshal(listRR.Body.Bytes(), &jpResp); err != nil || len(jpResp.JoinPhrases) != 1 {
			t.Fatalf("expected 1 join phrase, got %v", jpResp.JoinPhrases)
		}
		jpID := jpResp.JoinPhrases[0].ID

		// toggle join phrase
		togJSON := `{"id":"` + jpID + `"}`
		togReq := authReq(httptest.NewRequest(http.MethodPost, "/api/joinphrases/toggle", bytes.NewBufferString(togJSON)))
		togReq.Header.Set("Content-Type", "application/json")
		togRR := httptest.NewRecorder()
		mux.ServeHTTP(togRR, togReq)
		if togRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for join phrase toggle, got %d", togRR.Code)
		}

		// delete join phrase
		delJSON := `{"id":"` + jpID + `"}`
		delReq := authReq(httptest.NewRequest(http.MethodPost, "/api/joinphrases/delete", bytes.NewBufferString(delJSON)))
		delReq.Header.Set("Content-Type", "application/json")
		delRR := httptest.NewRecorder()
		mux.ServeHTTP(delRR, delReq)
		if delRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for join phrase delete, got %d", delRR.Code)
		}
	})

	t.Run("moderation endpoints", func(t *testing.T) {
		// get moderation config
		getReq := authReq(httptest.NewRequest(http.MethodGet, "/api/moderation", nil))
		getRR := httptest.NewRecorder()
		mux.ServeHTTP(getRR, getReq)
		if getRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for moderation get, got %d", getRR.Code)
		}

		// save moderation config
		cfgJSON := `{"enabled":true,"banned_words":["badphrase"],"max_caps_percent":75,"caps_min_length":12,"action":"warn","custom_warning":"No caps please"}`
		saveReq := authReq(httptest.NewRequest(http.MethodPost, "/api/moderation/save", bytes.NewBufferString(cfgJSON)))
		saveReq.Header.Set("Content-Type", "application/json")
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)
		if saveRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for moderation save, got %d: %s", saveRR.Code, saveRR.Body.String())
		}
	})

	t.Run("ladder endpoints", func(t *testing.T) {
		// get ladder status
		getReq := authReq(httptest.NewRequest(http.MethodGet, "/api/ladder/status", nil))
		getRR := httptest.NewRecorder()
		mux.ServeHTTP(getRR, getReq)
		if getRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for ladder status, got %d", getRR.Code)
		}

		// stop ladder
		stopReq := authReq(httptest.NewRequest(http.MethodPost, "/api/ladder/stop", nil))
		stopRR := httptest.NewRecorder()
		mux.ServeHTTP(stopRR, stopReq)
		if stopRR.Code != http.StatusOK {
			t.Fatalf("expected 200 for ladder stop, got %d", stopRR.Code)
		}
	})
}

func TestWebAuthenticationAndLockout(t *testing.T) {
	cfg := showdown.Config{
		ServerID:   "testserver",
		ServerHost: "dummyhost.psim.us",
		Username:   "testbot",
	}
	client := showdown.NewClient(cfg)

	// initialize server with admin password
	srv, err := NewServer(client, "127.0.0.1", 8080, "secretpassword123")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux, err := srv.buildMux()
	if err != nil {
		t.Fatalf("failed to build mux: %v", err)
	}

	t.Run("unauthenticated browser request redirects to login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Errorf("expected 302 redirect to /login, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login" {
			t.Errorf("expected Location /login, got %s", loc)
		}
	})

	t.Run("unauthenticated api request returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 unauthorized, got %d", rr.Code)
		}
	})

	t.Run("successful login sets session cookie and unlocks access", func(t *testing.T) {
		loginBody := `{"password":"secretpassword123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(loginBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for successful login, got %d: %s", rr.Code, rr.Body.String())
		}

		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == "ghosthaze_session" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil || sessionCookie.Value == "" {
			t.Fatalf("expected ghosthaze_session cookie to be set")
		}

		// authenticated request to index
		authReq := httptest.NewRequest(http.MethodGet, "/", nil)
		authReq.AddCookie(sessionCookie)
		authRR := httptest.NewRecorder()
		mux.ServeHTTP(authRR, authReq)

		if authRR.Code != http.StatusOK {
			t.Errorf("expected 200 for authenticated index, got %d", authRR.Code)
		}

		// logout invalidates session
		logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		logoutReq.AddCookie(sessionCookie)
		logoutRR := httptest.NewRecorder()
		mux.ServeHTTP(logoutRR, logoutReq)

		if logoutRR.Code != http.StatusOK {
			t.Errorf("expected 200 for logout, got %d", logoutRR.Code)
		}

		// after logout, index should redirect again
		afterReq := httptest.NewRequest(http.MethodGet, "/", nil)
		afterReq.AddCookie(sessionCookie)
		afterRR := httptest.NewRecorder()
		mux.ServeHTTP(afterRR, afterReq)
		if afterRR.Code != http.StatusFound {
			t.Errorf("expected 302 after logout, got %d", afterRR.Code)
		}
	})

	t.Run("brute force attempts trigger lockout", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			badReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"wrong"}`))
			badReq.Header.Set("Content-Type", "application/json")
			badReq.RemoteAddr = "192.0.2.100:1234"
			badRR := httptest.NewRecorder()
			mux.ServeHTTP(badRR, badReq)
			if i < 4 && badRR.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for attempt %d, got %d", i+1, badRR.Code)
			}
		}

		// 6th attempt should return 403 forbidden due to lockout
		lockReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"wrong"}`))
		lockReq.Header.Set("Content-Type", "application/json")
		lockReq.RemoteAddr = "192.0.2.100:1234"
		lockRR := httptest.NewRecorder()
		mux.ServeHTTP(lockRR, lockReq)

		if lockRR.Code != http.StatusForbidden {
			t.Errorf("expected 403 forbidden for locked ip, got %d", lockRR.Code)
		}
	})
}


