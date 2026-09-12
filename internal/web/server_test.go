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
}
