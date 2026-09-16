package showdown

import (
	"net/http"
	"strconv"
	"time"
)

const (
	DefaultServerID       = "showdown"
	DefaultServerHost     = "sim3.psim.us"
	DefaultServerPort     = 443
	DefaultServerURL      = "wss://sim3.psim.us/showdown/websocket"
	DefaultLoginServer    = "play.pokemonshowdown.com"
	DefaultLoginURL       = "https://play.pokemonshowdown.com/action.php"
	DefaultReconnectDelay = 10 * time.Second
	DefaultCommandChar    = "."
	DefaultThrottleDelay  = 100 * time.Millisecond
)

type Config struct {
	ServerID       string
	ServerHost     string
	ServerPort     int
	ServerSSL      *bool
	ServerURL      string
	LoginServer    string
	LoginURL       string
	Username       string
	Password       string
	Rooms          []string
	Avatar         string
	ReconnectDelay time.Duration
	ThrottleDelay  time.Duration
	CommandChar    string
	HTTPClient     *http.Client
	AutoBattle      bool
	AutoLeaveBattle *bool
	MaxBattles      int
	BattleStartMsg  string
	BattleWinMsg    string
	BattleLoseMsg   string
	BattleFormats     []string
	BattleTeam        string
	AutoTournaments   bool
	TournamentFormats []string
	AntiPredictability *bool
	LadderAutoStart    bool
	LadderFormat       string
	LadderMaxBattles   int
}

func (c *Config) ApplyDefaults() {
	if c.ServerID == "" {
		c.ServerID = DefaultServerID
	}

	// if server url is not explicitly specified, derive it from server id, host, port, ssl
	if c.ServerURL == "" {
		host := c.ServerHost
		port := c.ServerPort
		ssl := true
		if c.ServerSSL != nil {
			ssl = *c.ServerSSL
		}

		if host == "" {
			if c.ServerID != "" && c.ServerID != DefaultServerID {
				host = c.ServerID + ".psim.us"
			} else {
				host = DefaultServerHost
			}
		}

		protocol := "ws"
		if ssl {
			protocol = "wss"
		}

		if port > 0 && port != 80 && port != 443 {
			c.ServerURL = protocol + "://" + host + ":" + strconv.Itoa(port) + "/showdown/websocket"
		} else {
			c.ServerURL = protocol + "://" + host + "/showdown/websocket"
		}
	}

	// if login url is not explicitly specified, derive it from login server and server id
	if c.LoginURL == "" {
		loginHost := c.LoginServer
		if loginHost == "" {
			loginHost = DefaultLoginServer
		}
		if c.ServerID != "" && c.ServerID != DefaultServerID {
			c.LoginURL = "https://" + loginHost + "/~~" + c.ServerID + "/action.php"
		} else {
			c.LoginURL = "https://" + loginHost + "/action.php"
		}
	}

	if c.Rooms == nil {
		c.Rooms = []string{}
	}
	if c.BattleStartMsg == "" {
		c.BattleStartMsg = "Good luck, have fun!"
	}
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = DefaultReconnectDelay
	}
	if c.ThrottleDelay == 0 {
		c.ThrottleDelay = DefaultThrottleDelay
	} else if c.ThrottleDelay < 0 {
		c.ThrottleDelay = 0
	}
	if c.CommandChar == "" {
		c.CommandChar = DefaultCommandChar
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	if c.AutoLeaveBattle == nil {
		leave := true
		c.AutoLeaveBattle = &leave
	}
	if c.MaxBattles <= 0 {
		c.MaxBattles = 1
	}
	if c.AntiPredictability == nil {
		pred := true
		c.AntiPredictability = &pred
	}
	if c.LadderFormat == "" {
		c.LadderFormat = "gen9randombattle"
	}
	if c.LadderMaxBattles < 0 {
		c.LadderMaxBattles = 0
	}
}

// isantipredictability returns whether mixed-strategy anti-predictability is enabled
func (c *Config) IsAntiPredictability() bool {
	if c.AntiPredictability == nil {
		return true
	}
	return *c.AntiPredictability
}

// shouldautoleavebattle returns whether finished battles should be automatically left
func (c *Config) ShouldAutoLeaveBattle() bool {
	if c.AutoLeaveBattle == nil {
		return true
	}
	return *c.AutoLeaveBattle
}

// resolveserver queries the showdown crossdomain discovery service and configures server parameters
func (c *Config) ResolveServer(targetURL string) error {
	info, err := GetShowdownServer(targetURL, c.HTTPClient)
	if err != nil {
		return err
	}
	c.ServerID = info.ID
	c.ServerHost = info.Host
	c.ServerPort = info.Port
	c.ServerSSL = &info.HTTPS
	c.ServerURL = info.WebSocketURL()
	c.LoginURL = info.LoginActionURL(c.LoginServer)
	return nil
}

