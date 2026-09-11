package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TurboRx/turboot/pkg/showdown"
)

// Load reads optional .env files and environment variables, populating a showdown.Config.
func Load() (*showdown.Config, error) {
	// Attempt to load .env from current directory or parent directory
	_ = LoadEnvFile(".env")

	cfg := &showdown.Config{
		ServerURL:      getEnv("PS_SERVER_URL", showdown.DefaultServerURL),
		LoginURL:       getEnv("PS_LOGIN_URL", showdown.DefaultLoginURL),
		Username:       os.Getenv("PS_USERNAME"),
		Password:       os.Getenv("PS_PASSWORD"),
		Avatar:         os.Getenv("PS_AVATAR"),
		CommandChar:    getEnv("PS_COMMAND_CHAR", showdown.DefaultCommandChar),
		ReconnectDelay: showdown.DefaultReconnectDelay,
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

	cfg.ApplyDefaults()
	return cfg, nil
}

// LoadEnvFile reads a key=value formatted .env file and sets variables in the environment
// if they are not already set.
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

		// Strip surrounding single or double quotes
		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
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
