package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
)

func Load() (*showdown.Config, error) {
	_ = LoadEnvFile(".env")

	cfg := &showdown.Config{
		ServerID:       strings.TrimSpace(os.Getenv("PS_SERVER_ID")),
		ServerHost:     strings.TrimSpace(os.Getenv("PS_SERVER_HOST")),
		ServerURL:      strings.TrimSpace(os.Getenv("PS_SERVER_URL")),
		LoginServer:    strings.TrimSpace(os.Getenv("PS_LOGIN_SERVER")),
		LoginURL:       strings.TrimSpace(os.Getenv("PS_LOGIN_URL")),
		Username:       strings.TrimSpace(os.Getenv("PS_USERNAME")),
		Password:       os.Getenv("PS_PASSWORD"),
		Avatar:         strings.TrimSpace(os.Getenv("PS_AVATAR")),
		CommandChar:    getEnv("PS_COMMAND_CHAR", showdown.DefaultCommandChar),
		ReconnectDelay: showdown.DefaultReconnectDelay,
	}

	if portRaw := strings.TrimSpace(os.Getenv("PS_SERVER_PORT")); portRaw != "" {
		if p, err := strconv.Atoi(portRaw); err == nil && p > 0 {
			cfg.ServerPort = p
		}
	}

	if sslRaw := strings.TrimSpace(os.Getenv("PS_SERVER_SSL")); sslRaw != "" {
		ssl := sslRaw == "true" || sslRaw == "1"
		cfg.ServerSSL = &ssl
	}

	if roomsRaw := os.Getenv("PS_ROOMS"); roomsRaw != "" {
		for _, r := range strings.Split(roomsRaw, ",") {
			trimmed := strings.TrimSpace(r)
			if trimmed != "" {
				cfg.Rooms = append(cfg.Rooms, trimmed)
			}
		}
	}

	if delayRaw := os.Getenv("PS_RECONNECT_DELAY_MS"); delayRaw != "" {
		if ms, err := strconv.ParseInt(strings.TrimSpace(delayRaw), 10, 64); err == nil && ms > 0 {
			cfg.ReconnectDelay = time.Duration(ms) * time.Millisecond
		}
	}

	if autoBattleRaw := os.Getenv("PS_AUTO_BATTLE"); autoBattleRaw != "" {
		cfg.AutoBattle = autoBattleRaw == "true" || autoBattleRaw == "1"
	}

	if formatsRaw := os.Getenv("PS_BATTLE_FORMATS"); formatsRaw != "" {
		for _, f := range strings.Split(formatsRaw, ",") {
			trimmed := strings.TrimSpace(f)
			if trimmed != "" {
				cfg.BattleFormats = append(cfg.BattleFormats, trimmed)
			}
		}
	}

	cfg.BattleTeam = os.Getenv("PS_BATTLE_TEAM")

	// if a web url was supplied or auto-discovery was requested, resolve server parameters
	autoDiscover := os.Getenv("PS_DISCOVER_SERVER") == "true" || os.Getenv("PS_DISCOVER_SERVER") == "1"
	if strings.HasPrefix(cfg.ServerURL, "http://") || strings.HasPrefix(cfg.ServerURL, "https://") || autoDiscover {
		target := cfg.ServerURL
		if target == "" {
			target = cfg.ServerID
		}
		if target != "" {
			_ = cfg.ResolveServer(target)
		}
	}

	cfg.ApplyDefaults()
	return cfg, nil
}


func LoadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		// parse quoted value or strip unquoted inline comment
		if strings.HasPrefix(val, "\"") {
			if lastIdx := strings.LastIndex(val, "\""); lastIdx > 0 {
				val = val[1:lastIdx]
			}
		} else if strings.HasPrefix(val, "'") {
			if lastIdx := strings.LastIndex(val, "'"); lastIdx > 0 {
				val = val[1:lastIdx]
			}
		} else if idx := strings.Index(val, "#"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}

	return scanner.Err()
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
