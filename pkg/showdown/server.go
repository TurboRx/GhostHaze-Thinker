package showdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// serverinfo holds connection metadata for a pokemon showdown server
type ServerInfo struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	ID         string `json:"id"`
	HTTPS      bool   `json:"https"`
	Registered bool   `json:"registered"`
}

// websocketurl constructs the websocket endpoint for this server
func (s *ServerInfo) WebSocketURL() string {
	protocol := "ws"
	if s.HTTPS {
		protocol = "wss"
	}
	if s.Port > 0 && s.Port != 80 && s.Port != 443 {
		return fmt.Sprintf("%s://%s:%d/showdown/websocket", protocol, s.Host, s.Port)
	}
	return fmt.Sprintf("%s://%s/showdown/websocket", protocol, s.Host)
}

// loginactionurl returns the authentication url for this server
func (s *ServerInfo) LoginActionURL(loginServer string) string {
	if loginServer == "" {
		loginServer = DefaultLoginServer
	}
	if s.ID != "" && s.ID != DefaultServerID {
		return fmt.Sprintf("https://%s/~~%s/action.php", loginServer, s.ID)
	}
	return fmt.Sprintf("https://%s/api/login", loginServer)
}

// string returns a formatted summary of the server details
func (s *ServerInfo) String() string {
	secure := "NO"
	if s.HTTPS {
		secure = "YES"
	}
	return fmt.Sprintf("Server: %s\nPort: %d\nServer-ID: %s\nSecure connection (TLS): %s\nWebSocket URL: %s",
		s.Host, s.Port, s.ID, secure, s.WebSocketURL())
}

// getshowdownserver resolves connection details for a pokemon showdown server url
func GetShowdownServer(targetURL string, client *http.Client) (*ServerInfo, error) {
	return GetShowdownServerWithEndpoint(targetURL, client, "https://play.pokemonshowdown.com/crossdomain.php")
}

// getshowdownserverwithendpoint resolves server details using a specified crossdomain endpoint
func GetShowdownServerWithEndpoint(targetURL string, client *http.Client, endpoint string) (*ServerInfo, error) {
	clean := strings.TrimSpace(targetURL)
	if clean == "" {
		return nil, errors.New("empty server url")
	}

	// strip protocol if present
	if strings.Contains(clean, "://") {
		if parsed, err := url.Parse(clean); err == nil && parsed.Host != "" {
			clean = parsed.Host
		}
	}

	// strip path, query, and trailing slash
	if idx := strings.IndexAny(clean, "/?#"); idx != -1 {
		clean = clean[:idx]
	}
	clean = strings.TrimSuffix(clean, "/")

	// fast path for the official main server
	if clean == "play.pokemonshowdown.com" || clean == DefaultServerID {
		return &ServerInfo{
			Host:       DefaultServerHost,
			Port:       DefaultServerPort,
			ID:         DefaultServerID,
			HTTPS:      true,
			Registered: true,
		}, nil
	}

	// append default domain suffix if bare server id was passed
	if !strings.Contains(clean, ".") && !strings.Contains(clean, ":") && clean != "localhost" {
		clean += ".psim.us"
	}

	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	reqURL := fmt.Sprintf("%s?host=%s&path=&protocol=https:", endpoint, url.QueryEscape(clean))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PokemonShowdownBot)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact server discovery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server discovery returned status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery response: %w", err)
	}

	jsonStr, err := extractConfigJSON(string(bodyBytes))
	if err != nil {
		return nil, err
	}

	var info ServerInfo
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		return nil, fmt.Errorf("failed to parse discovery json: %w", err)
	}

	if info.Host == "" {
		return nil, errors.New("discovered server host is empty")
	}

	info.Host, info.Port = parseShowdownHost(info.Host, info.Port)
	if info.Port == 0 {
		if info.HTTPS {
			info.Port = 443
		} else {
			info.Port = 80
		}
	}
	if info.ID == "" {
		// fallback to subdomain prefix if server id is omitted
		parts := strings.Split(clean, ".")
		info.ID = parts[0]
	}

	return &info, nil
}

// extractconfigjson finds and extracts the config json object from the response body
func extractConfigJSON(body string) (string, error) {
	search := "var config = "
	idx := strings.Index(body, search)
	if idx == -1 {
		return "", errors.New("failed to find server configuration in response")
	}
	remaining := body[idx+len(search):]
	semiIdx := strings.Index(remaining, ";")
	if semiIdx == -1 {
		return "", errors.New("malformed server configuration in response")
	}
	raw := strings.TrimSpace(remaining[:semiIdx])
	if raw == "" || raw == "null" || raw == "undefined" {
		return "", errors.New("server configuration is empty or null")
	}
	return raw, nil
}

// parseshowdownhost decodes encoded ip and port representations used by pokemon showdown
func parseShowdownHost(host string, currentPort int) (string, int) {
	if host == "" {
		return host, currentPort
	}
	// handle localhost-port pattern
	if strings.HasPrefix(host, "localhost-") && !strings.HasPrefix(host, "localhost--") {
		suffix := strings.TrimPrefix(host, "localhost-")
		if _, err := strconv.Atoi(suffix); err == nil {
			host = "localhost--" + suffix
		}
	}
	if !strings.Contains(host, ".") {
		// decode encoded ip addresses
		h := host
		h = strings.ReplaceAll(h, "----", "::")
		h = strings.ReplaceAll(h, "---", "=")
		h = strings.ReplaceAll(h, "--", ":")
		h = strings.ReplaceAll(h, "-", ".")
		h = strings.ReplaceAll(h, "=", "-")
		host = h
	}
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		portStr := host[colonIdx+1:]
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			return host[:colonIdx], p
		}
	}
	return host, currentPort
}
