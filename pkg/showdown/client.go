package showdown

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown/battle"
	"github.com/gorilla/websocket"
)

type loginResponse struct {
	ActionSuccess bool   `json:"actionsuccess"`
	Assertion     string `json:"assertion"`
	CurUser       *struct {
		LoggedIn bool `json:"loggedin"`
	} `json:"curuser"`
}

type Client struct {
	config Config

	wsConn    *websocket.Conn
	writeMu   sync.Mutex
	stateMu   sync.RWMutex
	handlerMu sync.RWMutex
	battleMu  sync.RWMutex

	connected        bool
	connectedAt      time.Time
	stopped          bool
	loggedIn         bool
	intentionalClose bool
	currentRoom      string
	username         string
	lastChallstr     string
	reconnectWake    chan struct{}
	roomInIntro      map[string]bool
	roomUsers        map[string]map[string]string
	roomAway         map[string]map[string]bool
	roomTitles       map[string]string
	formats          []Format
	formatsMap       map[string]Format
	lastSend         time.Time

	battles       map[string]*battle.Battle
	battleEngine  battle.BattleEngine
	autoBattle    bool
	battleFormats []string
	battleTeam    string

	onConnect         []func()
	onDisconnect      []func(error)
	onLogin           []func(username string, isGuest bool)
	onChat            []ChatHandler
	onPM              []PMHandler
	onRoomJoin        []func(room, roomType string)
	onRoomLeave       []func(room string)
	onRoomRename      []func(oldRoom, newRoom, title string)
	onRoomJoinFailure []func(room, reason, message string)
	onFormats         []func([]Format)
	onPopup           []func(text string)
	onRawMessages     []MessageHandler
	onChallenge       []func(from, format string)
	onBattleStart     []func(b *battle.Battle)
	onBattleEnd       []func(b *battle.Battle, winner string)
	commands          map[string]CommandHandler
}

func NewClient(cfg Config) *Client {
	cfg.ApplyDefaults()

	return &Client{
		config:        cfg,
		commands:      make(map[string]CommandHandler),
		roomInIntro:   make(map[string]bool),
		roomUsers:     make(map[string]map[string]string),
		roomAway:      make(map[string]map[string]bool),
		roomTitles:    make(map[string]string),
		formatsMap:    make(map[string]Format),
		battles:       make(map[string]*battle.Battle),
		battleEngine:  battle.NewDefaultEngine(),
		autoBattle:    cfg.AutoBattle,
		battleFormats: append([]string{}, cfg.BattleFormats...),
		battleTeam:    cfg.BattleTeam,
	}
}

func (c *Client) OnConnect(fn func()) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onConnect = append(c.onConnect, fn)
}

func (c *Client) OnDisconnect(fn func(error)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onDisconnect = append(c.onDisconnect, fn)
}

func (c *Client) OnLogin(fn func(username string, isGuest bool)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onLogin = append(c.onLogin, fn)
}

func (c *Client) OnChat(fn ChatHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onChat = append(c.onChat, fn)
}

func (c *Client) OnPM(fn PMHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onPM = append(c.onPM, fn)
}

func (c *Client) OnRoomJoin(fn func(room, roomType string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onRoomJoin = append(c.onRoomJoin, fn)
}

func (c *Client) OnRoomLeave(fn func(room string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onRoomLeave = append(c.onRoomLeave, fn)
}

func (c *Client) OnRoomRename(fn func(oldRoom, newRoom, title string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onRoomRename = append(c.onRoomRename, fn)
}

func (c *Client) OnRoomJoinFailure(fn func(room, reason, message string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onRoomJoinFailure = append(c.onRoomJoinFailure, fn)
}

func (c *Client) OnFormats(fn func(formats []Format)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onFormats = append(c.onFormats, fn)
}

func (c *Client) OnPopup(fn func(text string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onPopup = append(c.onPopup, fn)
}

func (c *Client) OnRawMessage(fn MessageHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onRawMessages = append(c.onRawMessages, fn)
}

func (c *Client) OnChallenge(fn func(from, format string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onChallenge = append(c.onChallenge, fn)
}

func (c *Client) OnBattleStart(fn func(b *battle.Battle)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onBattleStart = append(c.onBattleStart, fn)
}

func (c *Client) OnBattleEnd(fn func(b *battle.Battle, winner string)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.onBattleEnd = append(c.onBattleEnd, fn)
}

func (c *Client) HandleCommand(cmd string, fn CommandHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	if c.commands == nil {
		c.commands = make(map[string]CommandHandler)
	}
	name := strings.TrimPrefix(cmd, c.config.CommandChar)
	c.commands[strings.ToLower(name)] = fn
}

func (c *Client) IsConnected() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.connected
}

func (c *Client) IsLoggedIn() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.loggedIn
}

func (c *Client) Send(message string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.wsConn == nil {
		return errors.New("cannot send: websocket is not connected")
	}

	if c.config.ThrottleDelay > 0 && !c.lastSend.IsZero() {
		elapsed := time.Since(c.lastSend)
		if elapsed < c.config.ThrottleDelay {
			time.Sleep(c.config.ThrottleDelay - elapsed)
		}
	}

	_ = c.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.wsConn.WriteMessage(websocket.TextMessage, []byte(message))
	if err == nil {
		c.lastSend = time.Now()
	}
	return err
}

func (c *Client) SendToRoom(room, message string) error {
	trimmedRoom := strings.TrimSpace(room)
	if trimmedRoom == "" {
		return errors.New("cannot send to room: room is empty")
	}
	lines := strings.Split(message, "\n")
	sent := 0
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			if sent > 0 && c.config.ThrottleDelay == 0 {
				time.Sleep(100 * time.Millisecond)
			}
			if err := c.Send(fmt.Sprintf("%s|%s", trimmedRoom, line)); err != nil {
				return err
			}
			sent++
		}
	}
	return nil
}

// sendroom sends a chat message to a specific room (alias for sendtoroom)
func (c *Client) SendRoom(room, message string) error {
	return c.SendToRoom(room, message)
}

func (c *Client) SendPM(targetUser, message string) error {
	target := CleanUsername(targetUser)
	if target == "" {
		return errors.New("cannot send PM: target user is empty")
	}
	lines := strings.Split(message, "\n")
	sent := 0
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			if sent > 0 && c.config.ThrottleDelay == 0 {
				time.Sleep(100 * time.Millisecond)
			}
			if err := c.Send(fmt.Sprintf("|/pm %s,%s", target, line)); err != nil {
				return err
			}
			sent++
		}
	}
	return nil
}

func (c *Client) Reply(room, user, message string) error {
	if strings.TrimSpace(room) != "" {
		return c.SendToRoom(room, message)
	}
	return c.SendPM(user, message)
}

func (c *Client) SafeReply(room, user, message string) error {
	return c.Reply(room, user, EscapeChat(message))
}

func (c *Client) JoinRoom(room string) error {
	trimmed := strings.TrimSpace(room)
	if trimmed == "" {
		return errors.New("cannot join room: room name is empty")
	}
	return c.Send(fmt.Sprintf("|/join %s", trimmed))
}

// leaveroom requests leaving a room and vacates battle if applicable
func (c *Client) LeaveRoom(room string) error {
	trimmed := strings.TrimSpace(room)
	if trimmed == "" {
		return errors.New("cannot leave room: room name is empty")
	}
	roomID := ToRoomID(trimmed)
	if strings.HasPrefix(trimmed, "battle-") {
		_ = c.SendToRoom(trimmed, "/leavebattle")
		c.battleMu.Lock()
		delete(c.battles, roomID)
		c.battleMu.Unlock()
	}
	_ = c.SendToRoom(trimmed, "/leave")
	_ = c.Send(fmt.Sprintf("|/noreply /leave %s", trimmed))
	err := c.Send(fmt.Sprintf("|/leave %s", trimmed))

	c.stateMu.Lock()
	delete(c.roomInIntro, roomID)
	delete(c.roomUsers, roomID)
	delete(c.roomAway, roomID)
	delete(c.roomTitles, roomID)
	c.stateMu.Unlock()

	c.dispatchRoomLeave(trimmed)
	return err
}

// forfeitbattle sends forfeit to the battle room and vacates the room
func (c *Client) ForfeitBattle(room string) error {
	trimmed := strings.TrimSpace(room)
	if trimmed == "" {
		return errors.New("cannot forfeit: room is empty")
	}
	_ = c.SendToRoom(trimmed, "/forfeit")
	_ = c.SendToRoom(trimmed, "/leavebattle")
	err := c.LeaveRoom(trimmed)

	roomID := ToRoomID(trimmed)
	c.battleMu.Lock()
	delete(c.battles, roomID)
	c.battleMu.Unlock()

	c.stateMu.Lock()
	delete(c.roomUsers, roomID)
	delete(c.roomInIntro, roomID)
	delete(c.roomAway, roomID)
	delete(c.roomTitles, roomID)
	c.stateMu.Unlock()

	return err
}

// leavebattle vacates the battle player slot and leaves the battle room
func (c *Client) LeaveBattle(room string) error {
	trimmed := strings.TrimSpace(room)
	if trimmed == "" {
		return errors.New("cannot leave battle: room is empty")
	}
	_ = c.SendToRoom(trimmed, "/leavebattle")
	err := c.LeaveRoom(trimmed)

	roomID := ToRoomID(trimmed)
	c.battleMu.Lock()
	delete(c.battles, roomID)
	c.battleMu.Unlock()

	c.stateMu.Lock()
	delete(c.roomUsers, roomID)
	delete(c.roomInIntro, roomID)
	delete(c.roomAway, roomID)
	delete(c.roomTitles, roomID)
	c.stateMu.Unlock()

	return err
}

func (c *Client) SetAvatar(avatar string) error {
	trimmed := strings.TrimSpace(avatar)
	if trimmed == "" {
		return errors.New("cannot set avatar: avatar ID is empty")
	}
	return c.Send(fmt.Sprintf("|/avatar %s", trimmed))
}

func (c *Client) RoomUsers(room string) []string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	roomID := ToRoomID(room)
	usersMap, exists := c.roomUsers[roomID]
	if !exists {
		return nil
	}

	users := make([]string, 0, len(usersMap))
	for _, u := range usersMap {
		users = append(users, u)
	}
	return users
}

// username returns the active username
func (c *Client) Username() string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.username != "" {
		return c.username
	}
	return c.config.Username
}

// rooms returns a slice of currently joined room identifiers
func (c *Client) Rooms() []string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	rooms := make([]string, 0, len(c.roomUsers))
	for r := range c.roomUsers {
		if !strings.HasPrefix(r, "battle-") {
			rooms = append(rooms, r)
		}
	}
	return rooms
}

// clientconfig returns the current configuration
func (c *Client) ClientConfig() Config {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.config
}

// connectedat returns the timestamp of current connection establishment
func (c *Client) ConnectedAt() time.Time {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.connectedAt
}

// start resets the stopped flag and wakes the runner loop to reconnect immediately
func (c *Client) Start() {
	c.stateMu.Lock()
	c.stopped = false
	c.intentionalClose = false
	wake := c.reconnectWake
	c.stateMu.Unlock()

	if wake != nil {
		select {
		case <-wake:
		default:
			close(wake)
		}
	}
}

// isstopped returns whether the bot client is currently stopped
func (c *Client) IsStopped() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.stopped
}

// stop disconnects the bot and places it in a stopped state until start is called
func (c *Client) Stop() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.stateMu.Lock()
	c.stopped = true
	c.intentionalClose = true
	c.connected = false
	c.connectedAt = time.Time{}
	c.loggedIn = false
	conn := c.wsConn
	c.wsConn = nil
	if c.reconnectWake != nil {
		select {
		case <-c.reconnectWake:
		default:
			close(c.reconnectWake)
		}
	}
	c.stateMu.Unlock()

	if conn != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bot stopped"))
		_ = conn.Close()
	}
}

// reconnect forces closing the websocket connection to trigger automatic reconnect
func (c *Client) Reconnect() {
	c.Start()
	c.writeMu.Lock()
	conn := c.wsConn
	c.writeMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// updateconfig updates client configuration safely
func (c *Client) UpdateConfig(fn func(cfg *Config)) {
	c.stateMu.Lock()
	fn(&c.config)
	c.stateMu.Unlock()

	c.battleMu.Lock()
	c.autoBattle = c.config.AutoBattle
	c.battleFormats = append([]string{}, c.config.BattleFormats...)
	c.battleTeam = c.config.BattleTeam
	c.battleMu.Unlock()
}


func (c *Client) IsInRoom(room, user string) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	roomID := ToRoomID(room)
	usersMap, exists := c.roomUsers[roomID]
	if !exists {
		return false
	}
	_, found := usersMap[ToID(user)]
	return found
}

func (c *Client) UserRankInRoom(room, user string) string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	roomID := ToRoomID(room)
	usersMap, exists := c.roomUsers[roomID]
	if !exists {
		return ""
	}
	nameWithRank, found := usersMap[ToID(user)]
	if !found {
		return ""
	}
	return UserRank(nameWithRank)
}

func (c *Client) IsUserAway(room, user string) bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	roomID := ToRoomID(room)
	awayMap, exists := c.roomAway[roomID]
	if !exists {
		return false
	}
	return awayMap[ToID(user)]
}

func (c *Client) RoomTitle(room string) string {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	roomID := ToRoomID(room)
	return c.roomTitles[roomID]
}

func (c *Client) Formats() []Format {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	res := make([]Format, len(c.formats))
	copy(res, c.formats)
	return res
}

func (c *Client) Format(id string) (Format, bool) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	f, ok := c.formatsMap[ToID(id)]
	return f, ok
}

func (c *Client) SetAutoBattle(enabled bool) {
	c.battleMu.Lock()
	defer c.battleMu.Unlock()
	c.autoBattle = enabled
}

func (c *Client) AutoBattle() bool {
	c.battleMu.RLock()
	defer c.battleMu.RUnlock()
	return c.autoBattle
}

func (c *Client) SetBattleFormats(formats []string) {
	c.battleMu.Lock()
	defer c.battleMu.Unlock()
	c.battleFormats = append([]string{}, formats...)
}

func (c *Client) BattleFormats() []string {
	c.battleMu.RLock()
	defer c.battleMu.RUnlock()
	res := make([]string, len(c.battleFormats))
	copy(res, c.battleFormats)
	return res
}

func (c *Client) SetBattleTeam(team string) {
	c.battleMu.Lock()
	defer c.battleMu.Unlock()
	c.battleTeam = team
}

func (c *Client) BattleTeam() string {
	c.battleMu.RLock()
	defer c.battleMu.RUnlock()
	return c.battleTeam
}

func (c *Client) SetBattleEngine(engine battle.BattleEngine) {
	c.battleMu.Lock()
	defer c.battleMu.Unlock()
	if engine != nil {
		c.battleEngine = engine
	}
}

func (c *Client) Battle(room string) (*battle.Battle, bool) {
	c.battleMu.RLock()
	defer c.battleMu.RUnlock()
	b, ok := c.battles[ToRoomID(room)]
	return b, ok
}

func (c *Client) ActiveBattles() []*battle.Battle {
	c.battleMu.RLock()
	defer c.battleMu.RUnlock()
	res := make([]*battle.Battle, 0, len(c.battles))
	for _, b := range c.battles {
		if !b.IsEnded() {
			res = append(res, b)
		}
	}
	return res
}

func (c *Client) AcceptChallenge(user string) error {
	cleanUser := ToID(user)
	if cleanUser == "" {
		return errors.New("cannot accept challenge: username is empty")
	}
	c.battleMu.RLock()
	team := c.battleTeam
	c.battleMu.RUnlock()

	if team != "" {
		return c.Send(fmt.Sprintf("|/utm %s\n|/accept %s", team, cleanUser))
	}
	return c.Send(fmt.Sprintf("|/accept %s", cleanUser))
}

func (c *Client) RejectChallenge(user string) error {
	cleanUser := ToID(user)
	if cleanUser == "" {
		return errors.New("cannot reject challenge: username is empty")
	}
	return c.Send(fmt.Sprintf("|/reject %s", cleanUser))
}

func (c *Client) ChallengeUser(user, format string) error {
	cleanUser := ToID(user)
	if cleanUser == "" {
		return errors.New("cannot challenge: username is empty")
	}
	cleanFmt := strings.TrimSpace(format)
	if cleanFmt == "" {
		cleanFmt = "gen9randombattle"
	}
	c.battleMu.RLock()
	team := c.battleTeam
	c.battleMu.RUnlock()

	if team != "" {
		return c.Send(fmt.Sprintf("|/utm %s\n|/challenge %s, %s", team, cleanUser, cleanFmt))
	}
	return c.Send(fmt.Sprintf("|/challenge %s, %s", cleanUser, cleanFmt))
}

func (c *Client) CancelChallengeTo(user string) error {
	cleanUser := ToID(user)
	if cleanUser == "" {
		return errors.New("cannot cancel challenge: username is empty")
	}
	return c.Send(fmt.Sprintf("|/cancelchallenge %s", cleanUser))
}

func (c *Client) Run(ctx context.Context) error {
	c.stateMu.Lock()
	c.intentionalClose = false
	c.stateMu.Unlock()

	for {
		select {
		case <-ctx.Done():
			c.Disconnect()
			return ctx.Err()
		default:
		}

		c.stateMu.RLock()
		isStopped := c.stopped
		isClosed := c.intentionalClose && !isStopped
		c.stateMu.RUnlock()
		if isClosed {
			return nil
		}
		if isStopped {
			c.stateMu.Lock()
			wake := make(chan struct{})
			c.reconnectWake = wake
			c.stateMu.Unlock()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-wake:
				continue
			}
		}

		err := c.connectAndListen(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		c.stateMu.RLock()
		isStopped = c.stopped
		isClosed = c.intentionalClose && !isStopped
		c.stateMu.RUnlock()
		if isClosed {
			return nil
		}
		if isStopped {
			continue
		}

		if err != nil {
			c.dispatchDisconnect(err)
		} else {
			c.dispatchDisconnect(errors.New("connection closed cleanly"))
		}

		// wait reconnect delay before retrying
		c.stateMu.Lock()
		wake := make(chan struct{})
		c.reconnectWake = wake
		c.stateMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wake:
			c.stateMu.RLock()
			closed := c.intentionalClose && !c.stopped
			c.stateMu.RUnlock()
			if closed {
				return nil
			}
			continue
		case <-time.After(c.config.ReconnectDelay):
		}
	}
}

func (c *Client) Disconnect() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.stateMu.Lock()
	c.intentionalClose = true
	c.connected = false
	c.connectedAt = time.Time{}
	c.loggedIn = false
	conn := c.wsConn
	c.wsConn = nil
	if c.reconnectWake != nil {
		select {
		case <-c.reconnectWake:
		default:
			close(c.reconnectWake)
		}
	}
	c.stateMu.Unlock()

	if conn != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "disconnecting"))
		_ = conn.Close()
	}
}

func (c *Client) connectAndListen(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.DialContext(ctx, c.config.ServerURL, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	c.writeMu.Lock()
	c.stateMu.Lock()
	c.wsConn = conn
	c.connected = true
	c.connectedAt = time.Now()
	c.currentRoom = ""
	c.stateMu.Unlock()
	c.writeMu.Unlock()

	c.dispatchConnect()

	done := make(chan struct{})
	defer close(done)

	// close connection if context is cancelled
	go func() {
		select {
		case <-ctx.Done():
			c.Disconnect()
		case <-done:
		}
	}()

	// ping server periodically to keep connection alive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-pingTicker.C:
				c.writeMu.Lock()
				currentConn := c.wsConn
				if currentConn != nil {
					_ = currentConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				}
				c.writeMu.Unlock()
				c.cleanupStaleBattles()
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		_, messageBytes, readErr := conn.ReadMessage()
		if readErr != nil {
			c.writeMu.Lock()
			c.stateMu.Lock()
			c.connected = false
			c.connectedAt = time.Time{}
			c.loggedIn = false
			c.wsConn = nil
			c.stateMu.Unlock()
			c.writeMu.Unlock()
			return readErr
		}

		c.handleRawPayload(string(messageBytes))
	}
}

func (c *Client) handleRawPayload(payload string) {
	c.stateMu.Lock()
	messages, newRoom := ParseRawStream(payload, c.currentRoom)
	c.currentRoom = newRoom
	c.stateMu.Unlock()

	for _, rawMsg := range messages {
		c.dispatchRawMessage(rawMsg)
		c.processMessage(rawMsg)
	}
}

func (c *Client) processMessage(msg RawMessage) {
	if strings.HasPrefix(msg.Room, "battle-") {
		c.handleBattleMessage(msg)
	}

	switch msg.Type {
	case "challstr":
		if challstr, ok := ParseChallstr(msg); ok {
			c.stateMu.Lock()
			c.lastChallstr = challstr
			c.stateMu.Unlock()
			go c.authenticate(challstr)
		}

	case "updateuser":
		if update, ok := ParseUserUpdate(msg); ok {
			c.stateMu.Lock()
			wasLoggedIn := c.loggedIn
			c.loggedIn = !update.IsGuest
			c.username = CleanUsername(update.Username)
			c.stateMu.Unlock()

			c.dispatchLogin(update.Username, update.IsGuest)

			// only trigger post-login room joins when transitioning to logged in
			if !update.IsGuest && !wasLoggedIn {
				c.onPostLogin()
			}
		}

	case ":":
		// end of room intro backlog
		c.stateMu.Lock()
		c.roomInIntro[ToRoomID(msg.Room)] = false
		c.stateMu.Unlock()

	case "formats":
		formats := ParseFormats(msg)
		if len(formats) > 0 {
			c.stateMu.Lock()
			c.formats = formats
			c.formatsMap = make(map[string]Format, len(formats))
			for _, f := range formats {
				c.formatsMap[f.ID] = f
			}
			c.stateMu.Unlock()
			c.dispatchFormats(formats)
		}

	case "title":
		if len(msg.Parts) > 0 {
			c.stateMu.Lock()
			c.roomTitles[ToRoomID(msg.Room)] = msg.Parts[0]
			c.stateMu.Unlock()
		}

	case "noinit":
		if len(msg.Parts) > 0 {
			action := msg.Parts[0]
			if action == "rename" && len(msg.Parts) >= 2 {
				newRoom := msg.Parts[1]
				title := ""
				if len(msg.Parts) > 2 {
					title = msg.Parts[2]
				}
				c.handleRoomRename(msg.Room, newRoom, title)
			} else {
				reason := action
				details := ""
				if len(msg.Parts) > 1 {
					details = strings.Join(msg.Parts[1:], "|")
				}
				c.dispatchRoomJoinFailure(msg.Room, reason, details)
			}
		}

	case "init":
		roomType := ""
		if len(msg.Parts) > 0 {
			roomType = msg.Parts[0]
		}
		c.stateMu.Lock()
		c.roomInIntro[ToRoomID(msg.Room)] = true
		c.stateMu.Unlock()
		c.dispatchRoomJoin(msg.Room, roomType)

	case "deinit":
		c.stateMu.Lock()
		delete(c.roomInIntro, ToRoomID(msg.Room))
		delete(c.roomUsers, ToRoomID(msg.Room))
		delete(c.roomAway, ToRoomID(msg.Room))
		delete(c.roomTitles, ToRoomID(msg.Room))
		c.stateMu.Unlock()
		c.dispatchRoomLeave(msg.Room)

	case "users":
		if len(msg.Parts) > 0 {
			c.handleUsersList(msg.Room, msg.Parts[0])
		}

	case "J", "j":
		if len(msg.Parts) > 0 {
			c.handleUserJoin(msg.Room, msg.Parts[0])
		}

	case "L", "l":
		if len(msg.Parts) > 0 {
			c.handleUserLeave(msg.Room, msg.Parts[0])
		}

	case "N", "n":
		if len(msg.Parts) >= 2 {
			c.handleUserRename(msg.Room, msg.Parts[0], msg.Parts[1])
		}

	case "popup":
		c.dispatchPopup(strings.Join(msg.Parts, "|"))

	case "c", "chat", "c:":
		if chat, ok := ParseChatMessage(msg); ok {
			c.stateMu.RLock()
			isIntro := c.roomInIntro[ToRoomID(chat.Room)]
			c.stateMu.RUnlock()
			chat.IsIntro = isIntro

			c.dispatchChat(chat)
			if !isIntro {
				c.routeCommand(chat.Room, chat.User, chat.Text)
			}
		}

	case "pm":
		if pm, ok := ParsePrivateMessage(msg); ok {
			c.dispatchPM(pm)
			c.routeCommand("", pm.From, pm.Text)
		}

	case "updatechallenges":
		if len(msg.Parts) > 0 {
			var cu challengesUpdate
			rawJSON := strings.Join(msg.Parts, "|")
			if err := json.Unmarshal([]byte(rawJSON), &cu); err == nil {
				c.handleChallengesUpdate(cu)
			}
		}
	}
}

type challengesUpdate struct {
	ChallengesFrom map[string]string `json:"challengesFrom"`
	ChallengeTo    *struct {
		To     string `json:"to"`
		Format string `json:"format"`
	} `json:"challengeTo"`
}

func (c *Client) handleChallengesUpdate(cu challengesUpdate) {
	c.battleMu.RLock()
	auto := c.autoBattle
	allowedFormats := append([]string{}, c.battleFormats...)
	maxBattles := c.config.MaxBattles
	if maxBattles <= 0 {
		maxBattles = 1
	}
	currentBattles := 0
	for _, b := range c.battles {
		if !b.IsEnded() {
			currentBattles++
		}
	}
	c.battleMu.RUnlock()

	for from, format := range cu.ChallengesFrom {
		cleanFrom := CleanUsername(from)
		c.dispatchChallenge(cleanFrom, format)

		if auto {
			if currentBattles >= maxBattles {
				continue
			}
			allowed := false
			if len(allowedFormats) == 0 {
				allowed = true
			} else {
				normFmt := ToID(format)
				for _, af := range allowedFormats {
					if ToID(af) == normFmt {
						allowed = true
						break
					}
				}
			}
			if allowed {
				_ = c.AcceptChallenge(cleanFrom)
				currentBattles++
			}
		}
	}
}

func (c *Client) handleBattleMessage(msg RawMessage) {
	room := msg.Room
	if !strings.HasPrefix(room, "battle-") {
		return
	}

	roomID := ToRoomID(room)
	c.battleMu.Lock()
	b, exists := c.battles[roomID]
	if !exists {
		b = battle.NewBattle(room, c.battleEngine)
		c.battles[roomID] = b
		c.battleMu.Unlock()
		c.dispatchBattleStart(b)
		// enable timer to prevent stalling
		_ = c.SendToRoom(room, "/timer on")
	} else {
		c.battleMu.Unlock()
	}

	// ensure timer is activated if not started by turn 1
	if msg.Type == "turn" && len(msg.Parts) > 0 && msg.Parts[0] == "1" {
		if !b.IsTimerActive() {
			_ = c.SendToRoom(room, "/timer on")
		}
	}

	c.stateMu.RLock()
	myNick := c.username
	c.stateMu.RUnlock()

	tokens := append([]string{msg.Type}, msg.Parts...)
	choice, shouldSend := b.HandleLine(tokens, myNick)
	if shouldSend && choice != "" {
		_ = c.SendToRoom(room, choice)
	}

	if msg.Type == "win" || msg.Type == "tie" || msg.Type == "prematureend" || msg.Type == "expire" {
		b.SetEnded(true)
		winner := ""
		if len(msg.Parts) > 0 {
			winner = msg.Parts[0]
		}
		c.dispatchBattleEnd(b, winner)

		c.stateMu.RLock()
		autoLeave := c.config.ShouldAutoLeaveBattle()
		winMsg := c.config.BattleWinMsg
		loseMsg := c.config.BattleLoseMsg
		c.stateMu.RUnlock()

		if autoLeave {
			go c.autoLeaveBattleAfterDelay(room, winner, myNick, winMsg, loseMsg)
		}
	} else if msg.Type == "deinit" {
		c.battleMu.Lock()
		delete(c.battles, roomID)
		c.battleMu.Unlock()
	}
}

// autoleavebattleafterdelay handles optional win/lose messages and cleanly leaves the battle room
func (c *Client) autoLeaveBattleAfterDelay(room, winner, myNick, winMsg, loseMsg string) {
	// optional win or lose message before leaving
	cleanWinner := ToID(winner)
	cleanNick := ToID(myNick)
	if cleanWinner != "" && cleanNick != "" {
		if cleanWinner == cleanNick {
			if winMsg != "" {
				_ = c.SendToRoom(room, winMsg)
			}
		} else {
			if loseMsg != "" {
				_ = c.SendToRoom(room, loseMsg)
			}
		}
	}

	// brief delay to allow victory banner and chat messages to appear cleanly
	time.Sleep(1500 * time.Millisecond)

	_ = c.SendToRoom(room, "/leavebattle")
	_ = c.LeaveRoom(room)

	// clean up battle state
	roomID := ToRoomID(room)
	c.battleMu.Lock()
	delete(c.battles, roomID)
	c.battleMu.Unlock()

	c.stateMu.Lock()
	delete(c.roomUsers, roomID)
	delete(c.roomInIntro, roomID)
	delete(c.roomAway, roomID)
	delete(c.roomTitles, roomID)
	c.stateMu.Unlock()
}

func (c *Client) handleUsersList(room, userListStr string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	roomID := ToRoomID(room)
	if c.roomUsers[roomID] == nil {
		c.roomUsers[roomID] = make(map[string]string)
	}
	if c.roomAway[roomID] == nil {
		c.roomAway[roomID] = make(map[string]bool)
	}

	parts := strings.Split(userListStr, ",")
	if len(parts) <= 1 {
		return
	}

	for _, user := range parts[1:] {
		trimmed := strings.TrimSpace(user)
		if trimmed != "" {
			id := ToID(trimmed)
			c.roomUsers[roomID][id] = trimmed
			c.roomAway[roomID][id] = IsAway(trimmed)
		}
	}
}

func (c *Client) handleUserJoin(room, user string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	roomID := ToRoomID(room)
	if c.roomUsers[roomID] == nil {
		c.roomUsers[roomID] = make(map[string]string)
	}
	if c.roomAway[roomID] == nil {
		c.roomAway[roomID] = make(map[string]bool)
	}
	trimmed := strings.TrimSpace(user)
	if trimmed != "" {
		id := ToID(trimmed)
		c.roomUsers[roomID][id] = trimmed
		c.roomAway[roomID][id] = IsAway(trimmed)
	}
}

func (c *Client) handleUserLeave(room, user string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	roomID := ToRoomID(room)
	if c.roomUsers[roomID] != nil {
		id := ToID(user)
		delete(c.roomUsers[roomID], id)
		delete(c.roomAway[roomID], id)
	}
}

func (c *Client) handleUserRename(room, newUser, oldID string) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	roomID := ToRoomID(room)
	if c.roomUsers[roomID] == nil {
		c.roomUsers[roomID] = make(map[string]string)
	}
	if c.roomAway[roomID] == nil {
		c.roomAway[roomID] = make(map[string]bool)
	}
	delete(c.roomUsers[roomID], ToID(oldID))
	delete(c.roomAway[roomID], ToID(oldID))

	trimmed := strings.TrimSpace(newUser)
	if trimmed != "" {
		id := ToID(trimmed)
		c.roomUsers[roomID][id] = trimmed
		c.roomAway[roomID][id] = IsAway(trimmed)
	}
}

func (c *Client) handleRoomRename(oldRoom, newRoom, title string) {
	oldID := ToRoomID(oldRoom)
	newID := ToRoomID(newRoom)

	c.stateMu.Lock()
	if users, ok := c.roomUsers[oldID]; ok {
		c.roomUsers[newID] = users
		delete(c.roomUsers, oldID)
	}
	if away, ok := c.roomAway[oldID]; ok {
		c.roomAway[newID] = away
		delete(c.roomAway, oldID)
	}
	if intro, ok := c.roomInIntro[oldID]; ok {
		c.roomInIntro[newID] = intro
		delete(c.roomInIntro, oldID)
	}
	delete(c.roomTitles, oldID)
	if title != "" {
		c.roomTitles[newID] = title
	}
	c.stateMu.Unlock()

	c.dispatchRoomRename(oldRoom, newRoom, title)
}

func (c *Client) authenticate(challstr string) {
	if c.config.Username == "" {
		return
	}

	var req *http.Request
	var err error

	if c.config.Password == "" {
		// unregistered account assertion (GET request)
		params := url.Values{}
		params.Set("act", "getassertion")
		params.Set("userid", ToID(c.config.Username))
		params.Set("challstr", challstr)

		reqURL := c.config.LoginURL
		if strings.Contains(reqURL, "?") {
			reqURL += "&" + params.Encode()
		} else {
			reqURL += "?" + params.Encode()
		}

		req, err = http.NewRequest("GET", reqURL, nil)
	} else {
		// registered account login (POST request)
		form := url.Values{}
		form.Set("act", "login")
		form.Set("name", ToID(c.config.Username))
		form.Set("pass", c.config.Password)
		form.Set("challstr", challstr)

		req, err = http.NewRequest("POST", c.config.LoginURL, strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	if err != nil {
		log.Printf("Failed to create login request: %v", err)
		return
	}

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		log.Printf("Login request error: %v", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read login response: %v", err)
		return
	}
	bodyStr := string(bodyBytes)

	var assertion string
	if c.config.Password == "" {
		if bodyStr == ";" {
			log.Printf("Login failed — nickname '%s' is registered but no password was provided", c.config.Username)
			return
		}
		if strings.Contains(strings.ToLower(bodyStr), "heavy load") {
			log.Printf("Login failed — Showdown login server is under heavy load; please retry later")
			return
		}
		if strings.HasPrefix(bodyStr, ";;") {
			log.Printf("Login assertion rejected: %s", bodyStr[2:])
			return
		}
		assertion = strings.TrimSpace(bodyStr)
	} else {
		cleanBody := bytes.TrimPrefix(bodyBytes, []byte("]"))
		lowerBody := strings.ToLower(string(cleanBody))
		if strings.Contains(lowerBody, "wrong password") {
			log.Printf("Login failed — wrong password for user '%s'", c.config.Username)
			return
		}
		if strings.Contains(lowerBody, "heavy load") {
			log.Printf("Login failed — Showdown login server is under heavy load; please retry later")
			return
		}
		var result loginResponse
		if err := json.Unmarshal(cleanBody, &result); err != nil {
			log.Printf("Failed to decode login JSON: %v (raw: %s)", err, bodyStr)
			return
		}
		if strings.HasPrefix(result.Assertion, ";;") {
			log.Printf("Login assertion rejected by server: %s", result.Assertion[2:])
			return
		}
		assertion = result.Assertion
	}

	if assertion == "" {
		log.Printf("Login failed — empty assertion received")
		return
	}

	_ = c.Send(fmt.Sprintf("|/trn %s,0,%s", c.config.Username, assertion))
}

func (c *Client) onPostLogin() {
	if c.config.Avatar != "" {
		_ = c.SetAvatar(c.config.Avatar)
	}

	for _, room := range c.config.Rooms {
		room = strings.TrimSpace(room)
		if room != "" {
			_ = c.JoinRoom(room)
		}
	}
}

func (c *Client) routeCommand(room, user, text string) {
	c.stateMu.RLock()
	currentBotNick := c.username
	c.stateMu.RUnlock()

	// ignore commands sent by the bot itself
	userID := ToID(user)
	if userID != "" {
		if c.config.Username != "" && userID == ToID(c.config.Username) {
			return
		}
		if currentBotNick != "" && userID == ToID(currentBotNick) {
			return
		}
	}

	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, c.config.CommandChar) {
		return
	}

	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, c.config.CommandChar))
	if payload == "" {
		return
	}

	var cmdName, args string
	if idx := strings.IndexAny(payload, " \t"); idx != -1 {
		cmdName = strings.ToLower(payload[:idx])
		args = strings.TrimSpace(payload[idx+1:])
	} else {
		cmdName = strings.ToLower(payload)
		args = ""
	}

	c.handlerMu.RLock()
	handler, exists := c.commands[cmdName]
	c.handlerMu.RUnlock()

	if exists && handler != nil {
		cleanUser := CleanUsername(user)
		go handler(room, cleanUser, args)
	}
}

func (c *Client) dispatchConnect() {
	c.handlerMu.RLock()
	handlers := append([]func(){}, c.onConnect...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h()
	}
}

func (c *Client) dispatchDisconnect(err error) {
	c.handlerMu.RLock()
	handlers := append([]func(error){}, c.onDisconnect...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(err)
	}
}

func (c *Client) dispatchLogin(username string, isGuest bool) {
	c.handlerMu.RLock()
	handlers := append([]func(string, bool){}, c.onLogin...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(username, isGuest)
	}
}

func (c *Client) dispatchChat(msg ChatMessage) {
	c.handlerMu.RLock()
	handlers := append([]ChatHandler{}, c.onChat...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(msg)
	}
}

func (c *Client) dispatchPM(msg PrivateMessage) {
	c.handlerMu.RLock()
	handlers := append([]PMHandler{}, c.onPM...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(msg)
	}
}

func (c *Client) dispatchRoomJoin(room, roomType string) {
	c.handlerMu.RLock()
	handlers := append([]func(string, string){}, c.onRoomJoin...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(room, roomType)
	}
}

func (c *Client) dispatchRoomLeave(room string) {
	c.handlerMu.RLock()
	handlers := append([]func(string){}, c.onRoomLeave...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(room)
	}
}

func (c *Client) dispatchRoomRename(oldRoom, newRoom, title string) {
	c.handlerMu.RLock()
	handlers := append([]func(string, string, string){}, c.onRoomRename...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(oldRoom, newRoom, title)
	}
}

func (c *Client) dispatchRoomJoinFailure(room, reason, message string) {
	c.handlerMu.RLock()
	handlers := append([]func(string, string, string){}, c.onRoomJoinFailure...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(room, reason, message)
	}
}

func (c *Client) dispatchFormats(formats []Format) {
	c.handlerMu.RLock()
	handlers := append([]func([]Format){}, c.onFormats...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(formats)
	}
}

func (c *Client) dispatchPopup(text string) {
	c.handlerMu.RLock()
	handlers := append([]func(string){}, c.onPopup...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(text)
	}
}

func (c *Client) dispatchRawMessage(msg RawMessage) {
	c.handlerMu.RLock()
	handlers := append([]MessageHandler{}, c.onRawMessages...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(msg)
	}
}

func (c *Client) dispatchChallenge(from, format string) {
	c.handlerMu.RLock()
	handlers := append([]func(string, string){}, c.onChallenge...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(from, format)
	}
}

func (c *Client) dispatchBattleStart(b *battle.Battle) {
	c.stateMu.RLock()
	greeting := c.config.BattleStartMsg
	c.stateMu.RUnlock()
	if greeting != "" {
		_ = c.SendRoom(b.Room, greeting)
	}

	c.handlerMu.RLock()
	handlers := append([]func(*battle.Battle){}, c.onBattleStart...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(b)
	}
}

func (c *Client) dispatchBattleEnd(b *battle.Battle, winner string) {
	c.handlerMu.RLock()
	handlers := append([]func(*battle.Battle, string){}, c.onBattleEnd...)
	c.handlerMu.RUnlock()

	for _, h := range handlers {
		go h(b, winner)
	}
}

// cleanupstalebattles removes battles that have ended or been abandoned without activity
func (c *Client) cleanupStaleBattles() {
	now := time.Now()
	var forfeitRooms []string
	var leaveRooms []string

	c.battleMu.RLock()
	for _, b := range c.battles {
		if b.IsEnded() {
			if now.Sub(b.LastActivityTime()) > 10*time.Second {
				leaveRooms = append(leaveRooms, b.Room)
			}
		} else {
			if now.Sub(b.LastActivityTime()) > 2*time.Minute {
				forfeitRooms = append(forfeitRooms, b.Room)
			}
		}
	}
	c.battleMu.RUnlock()

	for _, room := range forfeitRooms {
		_ = c.ForfeitBattle(room)
	}
	for _, room := range leaveRooms {
		_ = c.LeaveRoom(room)
	}
}

// login authenticates the client using the specified credentials
func (c *Client) Login(username, password string) error {
	cleanUser := strings.TrimSpace(username)
	if cleanUser == "" {
		return errors.New("cannot login: username is empty")
	}

	c.stateMu.Lock()
	c.config.Username = cleanUser
	c.config.Password = password
	challstr := c.lastChallstr
	connected := c.connected
	c.stateMu.Unlock()

	if !connected {
		c.Reconnect()
		return nil
	}

	if challstr != "" {
		go c.authenticate(challstr)
		return nil
	}

	c.Reconnect()
	return nil
}
