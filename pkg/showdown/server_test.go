package showdown

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetShowdownServerFastPath(t *testing.T) {
	// test fast path for main showdown server
	for _, target := range []string{"play.pokemonshowdown.com", "https://play.pokemonshowdown.com", "showdown"} {
		info, err := GetShowdownServer(target, nil)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", target, err)
		}
		if info.Host != DefaultServerHost {
			t.Errorf("expected host %s, got %s", DefaultServerHost, info.Host)
		}
		if info.Port != DefaultServerPort {
			t.Errorf("expected port %d, got %d", DefaultServerPort, info.Port)
		}
		if info.ID != DefaultServerID {
			t.Errorf("expected id %s, got %s", DefaultServerID, info.ID)
		}
		if !info.HTTPS {
			t.Errorf("expected https to be true")
		}
		if !info.Registered {
			t.Errorf("expected registered to be true")
		}
		if info.WebSocketURL() != "wss://sim3.psim.us/showdown/websocket" {
			t.Errorf("unexpected websocket url: %s", info.WebSocketURL())
		}
		if info.LoginActionURL("") != "https://play.pokemonshowdown.com/api/login" {
			t.Errorf("unexpected login action url: %s", info.LoginActionURL(""))
		}
	}
}

func TestGetShowdownServerEmptyURL(t *testing.T) {
	// test empty input url
	_, err := GetShowdownServer("", nil)
	if err == nil {
		t.Errorf("expected error for empty server url")
	}
}

func TestGetShowdownServerWithEndpoint(t *testing.T) {
	// mock discovery server responding with dummy server metadata
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostQuery := r.URL.Query().Get("host")
		switch hostQuery {
		case "testserver.psim.us":
			response := `<!DOCTYPE html><script>var config = {"host":"dummyhost.psim.us","https":true,"id":"testserver","port":8001,"registered":true};</script>`
			_, _ = fmt.Fprint(w, response)
		case "dummyport443.psim.us":
			response := `<!DOCTYPE html><script>var config = {"host":"dummytest.psim.us","https":true,"id":"dummyport443","port":443,"registered":true};</script>`
			_, _ = fmt.Fprint(w, response)
		case "dummyhttp.psim.us":
			response := `<!DOCTYPE html><script>var config = {"host":"dummytest.psim.us","https":false,"id":"dummyhttp","port":80,"registered":false};</script>`
			_, _ = fmt.Fprint(w, response)
		case "dummyencoded.psim.us":
			response := `<!DOCTYPE html><script>var config = {"host":"127-0-0-1--8000","https":false,"id":"dummyencoded","port":0,"registered":false};</script>`
			_, _ = fmt.Fprint(w, response)
		case "dummylocalhost.psim.us":
			response := `<!DOCTYPE html><script>var config = {"host":"localhost-9000","https":false,"id":"dummylocalhost","port":0,"registered":false};</script>`
			_, _ = fmt.Fprint(w, response)
		case "malformed.psim.us":
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><script>var config = invalid json;</script>`)
		case "servererror.psim.us":
			http.Error(w, "internal error", http.StatusInternalServerError)
		default:
			_, _ = fmt.Fprint(w, `<!DOCTYPE html><script>var other = 1;</script>`)
		}
	}))
	defer ts.Close()

	client := ts.Client()

	t.Run("successful discovery for testserver", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("https://testserver.psim.us/", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Host != "dummyhost.psim.us" {
			t.Errorf("expected host dummyhost.psim.us, got %s", info.Host)
		}
		if info.Port != 8001 {
			t.Errorf("expected port 8001, got %d", info.Port)
		}
		if info.ID != "testserver" {
			t.Errorf("expected id testserver, got %s", info.ID)
		}
		if !info.HTTPS {
			t.Errorf("expected https to be true")
		}
		if !info.Registered {
			t.Errorf("expected registered to be true")
		}
		expectedWS := "wss://dummyhost.psim.us:8001/showdown/websocket"
		if info.WebSocketURL() != expectedWS {
			t.Errorf("expected websocket url %s, got %s", expectedWS, info.WebSocketURL())
		}
		expectedLogin := "https://play.pokemonshowdown.com/~~testserver/action.php"
		if info.LoginActionURL("") != expectedLogin {
			t.Errorf("expected login url %s, got %s", expectedLogin, info.LoginActionURL(""))
		}
	})

	t.Run("discovery with bare id testserver", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("testserver", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Host != "dummyhost.psim.us" {
			t.Errorf("expected host dummyhost.psim.us, got %s", info.Host)
		}
	})

	t.Run("discovery with standard port 443", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("dummyport443.psim.us", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedWS := "wss://dummytest.psim.us/showdown/websocket"
		if info.WebSocketURL() != expectedWS {
			t.Errorf("expected websocket url %s, got %s", expectedWS, info.WebSocketURL())
		}
	})

	t.Run("discovery with standard port 80 over http", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("dummyhttp.psim.us", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedWS := "ws://dummytest.psim.us/showdown/websocket"
		if info.WebSocketURL() != expectedWS {
			t.Errorf("expected websocket url %s, got %s", expectedWS, info.WebSocketURL())
		}
	})

	t.Run("discovery with encoded ip and port", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("dummyencoded.psim.us", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Host != "127.0.0.1" {
			t.Errorf("expected host 127.0.0.1, got %s", info.Host)
		}
		if info.Port != 8000 {
			t.Errorf("expected port 8000, got %d", info.Port)
		}
	})

	t.Run("discovery with encoded localhost", func(t *testing.T) {
		info, err := GetShowdownServerWithEndpoint("dummylocalhost.psim.us", client, ts.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Host != "localhost" {
			t.Errorf("expected host localhost, got %s", info.Host)
		}
		if info.Port != 9000 {
			t.Errorf("expected port 9000, got %d", info.Port)
		}
	})

	t.Run("error on malformed json", func(t *testing.T) {
		_, err := GetShowdownServerWithEndpoint("malformed.psim.us", client, ts.URL)
		if err == nil {
			t.Errorf("expected error for malformed json")
		}
	})

	t.Run("error on 500 response", func(t *testing.T) {
		_, err := GetShowdownServerWithEndpoint("servererror.psim.us", client, ts.URL)
		if err == nil {
			t.Errorf("expected error for 500 status code")
		}
	})

	t.Run("error on missing config block", func(t *testing.T) {
		_, err := GetShowdownServerWithEndpoint("missing.psim.us", client, ts.URL)
		if err == nil {
			t.Errorf("expected error for missing config block")
		}
	})
}

func TestServerInfoString(t *testing.T) {
	// test string formatting
	info := &ServerInfo{
		Host:       "dummytest.psim.us",
		Port:       8001,
		ID:         "dummytest",
		HTTPS:      true,
		Registered: true,
	}
	s := info.String()
	if s == "" {
		t.Errorf("expected non-empty string output")
	}
}
