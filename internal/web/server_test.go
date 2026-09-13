package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
)

func TestWebServerEndpoints(t *testing.T) {
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

	t.Run("index html renders successfully", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
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
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
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
		req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
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
		req := httptest.NewRequest(http.MethodPost, "/api/tools/get-server", body)
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
		req := httptest.NewRequest(http.MethodGet, "/unknown/path", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("config update and avatar endpoints", func(t *testing.T) {
		configPayload := `{"server_id":"dummytest","server_host":"dummytest.psim.us","server_port":8000,"server_ssl":true,"command_char":"!","avatar":"123","auto_battle":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/config/update", bytes.NewBufferString(configPayload))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		// test avatar endpoint
		avatarPayload := `{"avatar":"169"}`
		avatarReq := httptest.NewRequest(http.MethodPost, "/api/bot/avatar", bytes.NewBufferString(avatarPayload))
		avatarRR := httptest.NewRecorder()
		mux.ServeHTTP(avatarRR, avatarReq)

		// in disconnected mock state avatar returns 200 or 500 depending on conn
		if avatarRR.Code != http.StatusOK && avatarRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status: %d", avatarRR.Code)
		}

		// test reconnect endpoint
		reconnectReq := httptest.NewRequest(http.MethodPost, "/api/bot/reconnect", nil)
		reconnectRR := httptest.NewRecorder()
		mux.ServeHTTP(reconnectRR, reconnectReq)

		if reconnectRR.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", reconnectRR.Code)
		}

		// test battle forfeit and leave endpoints
		forfeitReq := httptest.NewRequest(http.MethodPost, "/api/battles/forfeit", bytes.NewBufferString(`{"room":"battle-gen9randombattle-9999"}`))
		forfeitRR := httptest.NewRecorder()
		mux.ServeHTTP(forfeitRR, forfeitReq)
		if forfeitRR.Code != http.StatusOK && forfeitRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status for forfeit: %d", forfeitRR.Code)
		}

		leaveReq := httptest.NewRequest(http.MethodPost, "/api/battles/leave", bytes.NewBufferString(`{"room":"battle-gen9randombattle-9999"}`))
		leaveRR := httptest.NewRecorder()
		mux.ServeHTTP(leaveRR, leaveReq)
		if leaveRR.Code != http.StatusOK && leaveRR.Code != http.StatusInternalServerError {
			t.Errorf("unexpected status for leave: %d", leaveRR.Code)
		}

		// test bot login endpoint
		loginPayload := `{"username":"testeruser","password":"mypassword"}`
		loginReq := httptest.NewRequest(http.MethodPost, "/api/bot/login", bytes.NewBufferString(loginPayload))
		loginRR := httptest.NewRecorder()
		mux.ServeHTTP(loginRR, loginReq)
		if loginRR.Code != http.StatusOK {
			t.Errorf("expected 200 for bot login, got %d: %s", loginRR.Code, loginRR.Body.String())
		}
	})

	t.Run("raw logs endpoint serves plain text", func(t *testing.T) {
		srv.AddLog("chat", "lobby", "hello world from raw test")
		req := httptest.NewRequest(http.MethodGet, "/api/logs/raw", nil)
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
		req := httptest.NewRequest(http.MethodPost, "/api/logs/clear", nil)
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
		req := httptest.NewRequest(http.MethodPost, "/api/bot/stop", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for bot stop, got %d", rr.Code)
		}
		if !client.IsStopped() {
			t.Errorf("expected client to be stopped")
		}

		// resume with reconnect
		reconnectReq := httptest.NewRequest(http.MethodPost, "/api/bot/reconnect", nil)
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
		req := httptest.NewRequest(http.MethodGet, "/api/backup/download", nil)
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

		restoreReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore", bytes.NewBuffer(modifiedBytes))
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


