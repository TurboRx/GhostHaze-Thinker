package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/internal/config"
	"github.com/TurboRx/GhostHaze-Thinker/internal/web"
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
	getServerFlag := flag.String("get-server", "", "resolve showdown server connection details from a url or server id")
	flag.Parse()

	if *getServerFlag != "" {
		info, err := showdown.GetShowdownServer(*getServerFlag, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving server: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(info.String())
		return
	}

	fmt.Println()
	fmt.Println("  GhostHaze-Thinker")
	fmt.Println("  A Pokémon Showdown bot and client library in Go")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	bot := showdown.NewClient(*cfg.Config)

	// initialize native web control panel if enabled
	var webServer *web.Server
	if cfg.WebEnabled {
		var err error
		webServer, err = web.NewServer(bot, cfg.WebHost, cfg.WebPort, cfg.WebAdminPassword)
		if err != nil {
			logWarn("Failed to initialize web control panel: %v", err)
		} else {
			go func() {
				logInfo("Control panel active at http://%s:%d", cfg.WebHost, cfg.WebPort)
				if err := webServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logWarn("Web server error: %v", err)
				}
			}()
		}
	}

	bot.OnConnect(func() {
		logInfo("WebSocket connection established.")
		if webServer != nil {
			webServer.AddLog("system", "Showdown", "WebSocket connection established")
		}
	})

	bot.OnDisconnect(func(err error) {
		logWarn("Connection closed (%v). Reconnecting in %v...", err, cfg.ReconnectDelay)
		if webServer != nil {
			webServer.AddLog("system", "Showdown", fmt.Sprintf("Disconnected (%v)", err))
		}
	})

	bot.OnLogin(func(username string, isGuest bool) {
		if isGuest {
			logWarn("Logged in as guest (%s).", username)
		} else {
			logInfo("Successfully logged in as %s", username)
		}
		if webServer != nil {
			webServer.AddLog("system", "Auth", fmt.Sprintf("Logged in as %s (guest: %v)", username, isGuest))
		}
	})

	bot.OnRoomJoin(func(room, roomType string) {
		logInfo("Joined room: %s (%s)", room, roomType)
		if webServer != nil {
			webServer.AddLog("room", "Rooms", fmt.Sprintf("Joined %s (%s)", room, roomType))
		}
	})

	bot.OnRoomLeave(func(room string) {
		logInfo("Left room: %s", room)
		if webServer != nil {
			webServer.AddLog("room", "Rooms", fmt.Sprintf("Left %s", room))
		}
	})

	bot.OnPopup(func(text string) {
		logWarn("Popup: %s", text)
		if webServer != nil {
			webServer.AddLog("system", "Popup", text)
		}
	})

	bot.OnPM(func(msg showdown.PrivateMessage) {
		logInfo("PM from %s to %s: %s", msg.From, msg.To, msg.Text)
		if webServer != nil {
			webServer.AddLog("pm", msg.From, msg.Text)
		}
	})

	bot.OnChat(func(msg showdown.ChatMessage) {
		logInfo("[%s] %s: %s", msg.Room, msg.User, msg.Text)
	})

	bot.OnChallenge(func(from, format string) {
		logInfo("Received challenge from %s in format %s", from, format)
		if webServer != nil {
			webServer.AddLog("battle", "Challenge", fmt.Sprintf("From %s in %s", from, format))
		}
	})

	bot.OnBattleStart(func(b *battle.Battle) {
		logInfo("Battle started in room %s", b.Room)
		if webServer != nil {
			webServer.AddLog("battle", b.Room, "Battle started")
		}
	})

	bot.OnBattleEnd(func(b *battle.Battle, winner string) {
		logInfo("Battle ended in room %s. Winner: %s", b.Room, winner)
		if webServer != nil {
			webServer.AddLog("battle", b.Room, fmt.Sprintf("Battle ended. Winner: %s", winner))
		}
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

	bot.HandleCommand("forfeit", func(room, user, args string) {
		targetRoom := strings.TrimSpace(args)
		if targetRoom == "" && strings.HasPrefix(room, "battle-") {
			targetRoom = room
		}
		if targetRoom == "" {
			_ = bot.Reply(room, user, "Usage: forfeit <battle-room-id>")
			return
		}
		if err := bot.ForfeitBattle(targetRoom); err != nil {
			_ = bot.Reply(room, user, fmt.Sprintf("Failed to forfeit battle: %v", err))
		} else {
			_ = bot.Reply(room, user, fmt.Sprintf("Forfeited and left battle %s.", targetRoom))
		}
	})

	bot.HandleCommand("leave", func(room, user, args string) {
		targetRoom := strings.TrimSpace(args)
		if targetRoom == "" {
			targetRoom = room
		}
		if targetRoom == "" {
			_ = bot.Reply(room, user, "Usage: leave [room-id]")
			return
		}
		if strings.HasPrefix(targetRoom, "battle-") {
			_ = bot.LeaveBattle(targetRoom)
		} else {
			_ = bot.LeaveRoom(targetRoom)
		}
		_ = bot.Reply(room, user, fmt.Sprintf("Left room %s.", targetRoom))
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logInfo("Connecting to %s...", cfg.ServerURL)

	go func() {
		<-ctx.Done()
		fmt.Println()
		logInfo("Received shutdown signal. Disconnecting...")
		if webServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = webServer.Shutdown(shutdownCtx)
		}
		bot.Disconnect()
	}()

	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logWarn("Bot stopped with error: %v", err)
	}

	logInfo("Disconnected.")
}
