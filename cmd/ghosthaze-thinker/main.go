package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/internal/config"
	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown/battle"
)

func logWith(prefix, msg string) {
	ts := time.Now().UTC().Format(time.RFC3339)
	if prefix == "" {
		fmt.Printf("[%s] %s\n", ts, msg)
	} else {
		fmt.Printf("[%s] %s %s\n", ts, prefix, msg)
	}
}

func logInfo(format string, args ...any) {
	logWith("", fmt.Sprintf(format, args...))
}

func logWarn(format string, args ...any) {
	logWith("WARN:", fmt.Sprintf(format, args...))
}

func main() {
	fmt.Println()
	fmt.Println("  GhostHaze-Thinker")
	fmt.Println("  A Pokémon Showdown bot and client library in Go")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	bot := showdown.NewClient(*cfg)

	bot.OnConnect(func() {
		logInfo("WebSocket connection established.")
	})

	bot.OnDisconnect(func(err error) {
		logWarn("Connection closed (%v). Reconnecting in %v...", err, cfg.ReconnectDelay)
	})

	bot.OnLogin(func(username string, isGuest bool) {
		if isGuest {
			logWarn("Logged in as guest (%s).", username)
		} else {
			logInfo("Successfully logged in as %s", username)
		}
	})

	bot.OnRoomJoin(func(room, roomType string) {
		logInfo("Joined room: %s (%s)", room, roomType)
	})

	bot.OnRoomLeave(func(room string) {
		logInfo("Left room: %s", room)
	})

	bot.OnPopup(func(text string) {
		logWarn("Popup: %s", text)
	})

	bot.OnPM(func(msg showdown.PrivateMessage) {
		logInfo("PM from %s to %s: %s", msg.From, msg.To, msg.Text)
	})

	bot.OnChat(func(msg showdown.ChatMessage) {
		logInfo("[%s] %s: %s", msg.Room, msg.User, msg.Text)
	})

	bot.OnChallenge(func(from, format string) {
		logInfo("Received challenge from %s in format %s", from, format)
	})

	bot.OnBattleStart(func(b *battle.Battle) {
		logInfo("Battle started in room %s", b.Room)
	})

	bot.OnBattleEnd(func(b *battle.Battle, winner string) {
		logInfo("Battle ended in room %s. Winner: %s", b.Room, winner)
	})

	bot.HandleCommand("ping", func(room, user, args string) {
		_ = bot.Reply(room, user, "pong!")
	})

	bot.HandleCommand("battle", func(room, user, args string) {
		fmtName := strings.TrimSpace(args)
		if fmtName == "" {
			fmtName = "gen9randombattle"
		}
		if err := bot.ChallengeUser(user, fmtName); err != nil {
			_ = bot.Reply(room, user, fmt.Sprintf("Failed to send challenge: %v", err))
		} else {
			_ = bot.Reply(room, user, fmt.Sprintf("Challenge sent to %s in %s! Accept to battle.", user, fmtName))
		}
	})

	bot.HandleCommand("challenge", func(room, user, args string) {
		fmtName := strings.TrimSpace(args)
		if fmtName == "" {
			fmtName = "gen9randombattle"
		}
		if err := bot.ChallengeUser(user, fmtName); err != nil {
			_ = bot.Reply(room, user, fmt.Sprintf("Failed to send challenge: %v", err))
		} else {
			_ = bot.Reply(room, user, fmt.Sprintf("Challenge sent to %s in %s! Accept to battle.", user, fmtName))
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logInfo("Connecting to %s...", cfg.ServerURL)

	go func() {
		<-ctx.Done()
		fmt.Println()
		logInfo("Received shutdown signal. Disconnecting...")
		bot.Disconnect()
	}()

	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logWarn("Bot stopped with error: %v", err)
	}

	logInfo("Disconnected.")
}
