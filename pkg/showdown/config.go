package showdown

import (
	"net/http"
	"time"
)

const (
	DefaultServerURL      = "wss://sim3.psim.us/showdown/websocket"
	DefaultLoginURL       = "https://play.pokemonshowdown.com/api/login"
	DefaultRoom           = "botdevelopment"
	DefaultReconnectDelay = 10 * time.Second
	DefaultCommandChar    = "."
)

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

func (c *Config) ApplyDefaults() {
	if c.ServerURL == "" {
		c.ServerURL = DefaultServerURL
	}
	if c.LoginURL == "" {
		c.LoginURL = DefaultLoginURL
	}
	if len(c.Rooms) == 0 {
		c.Rooms = []string{DefaultRoom}
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
