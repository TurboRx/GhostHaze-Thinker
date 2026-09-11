package showdown

import (
	"net/http"
	"time"
)

const (
	// DefaultServerURL is the official Pokémon Showdown websocket endpoint.
	DefaultServerURL = "wss://sim3.psim.us/showdown/websocket"

	// DefaultLoginURL is the official Pokémon Showdown HTTP login API.
	DefaultLoginURL = "https://play.pokemonshowdown.com/api/login"

	// DefaultReconnectDelay is the backoff wait duration before attempting to reconnect.
	DefaultReconnectDelay = 10 * time.Second

	// DefaultCommandChar is the prefix symbol indicating a bot command.
	DefaultCommandChar = "."
)

// Config holds configuration parameters for connecting and authenticating with Pokémon Showdown.
type Config struct {
	ServerURL      string
	LoginURL       string
	Username       string
	Password       string
	Rooms          []string
	Avatar         string
	ReconnectDelay time.Duration
	CommandChar    string
	HTTPClient     *http.Client
}

// ApplyDefaults fills in zero-value configuration fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.ServerURL == "" {
		c.ServerURL = DefaultServerURL
	}
	if c.LoginURL == "" {
		c.LoginURL = DefaultLoginURL
	}
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = DefaultReconnectDelay
	}
	if c.CommandChar == "" {
		c.CommandChar = DefaultCommandChar
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
}
