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

	connected        bool
	loggedIn         bool
	intentionalClose bool
	currentRoom      string

	onConnect     []func()
	onDisconnect  []func(error)
	onLogin       []func(username string, isGuest bool)
	onChat        []ChatHandler
	onPM          []PMHandler
	onRoomJoin    []func(room, roomType string)
	onRoomLeave   []func(room string)
	onPopup       []func(text string)
	onRawMessages []MessageHandler
	commands      map[string]CommandHandler
}

func NewClient(cfg Config) *Client {
	cfg.ApplyDefaults()

	return &Client{
		config:   cfg,
		commands: make(map[string]CommandHandler),
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

	return c.wsConn.WriteMessage(websocket.TextMessage, []byte(message))
}

func (c *Client) SendToRoom(room, message string) error {
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			if err := c.Send(fmt.Sprintf("%s|%s", room, line)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) SendPM(targetUser, message string) error {
	target := CleanUsername(targetUser)
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			if err := c.Send(fmt.Sprintf("|/pm %s,%s", target, line)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) JoinRoom(room string) error {
	return c.Send(fmt.Sprintf("|/join %s", room))
}

func (c *Client) LeaveRoom(room string) error {
	return c.Send(fmt.Sprintf("|/leave %s", room))
}

func (c *Client) SetAvatar(avatar string) error {
	return c.Send(fmt.Sprintf("|/avatar %s", avatar))
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
		closed := c.intentionalClose
		c.stateMu.RUnlock()
		if closed {
			return nil
		}

		err := c.connectAndListen(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		c.stateMu.RLock()
		closed = c.intentionalClose
		c.stateMu.RUnlock()
		if closed {
			return nil
		}

		if err != nil {
			c.dispatchDisconnect(err)
		} else {
			c.dispatchDisconnect(errors.New("connection closed cleanly"))
		}

		// wait reconnect delay before retrying
		select {
		case <-ctx.Done():
			return ctx.Err()
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
	c.loggedIn = false
	conn := c.wsConn
	c.wsConn = nil
	c.stateMu.Unlock()

	if conn != nil {
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

	c.writeMu.Lock()
	c.stateMu.Lock()
	c.wsConn = conn
	c.connected = true
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
	switch msg.Type {
	case "challstr":
		if challstr, ok := ParseChallstr(msg); ok {
			go c.authenticate(challstr)
		}

	case "updateuser":
		if update, ok := ParseUserUpdate(msg); ok {
			c.stateMu.Lock()
			wasLoggedIn := c.loggedIn
			c.loggedIn = !update.IsGuest
			c.stateMu.Unlock()

			c.dispatchLogin(update.Username, update.IsGuest)

			// only trigger post-login room joins when transitioning to logged in
			if !update.IsGuest && !wasLoggedIn {
				c.onPostLogin()
			}
		}

	case "init":
		roomType := ""
		if len(msg.Parts) > 0 {
			roomType = msg.Parts[0]
		}
		c.dispatchRoomJoin(msg.Room, roomType)

	case "deinit":
		c.dispatchRoomLeave(msg.Room)

	case "popup":
		c.dispatchPopup(strings.Join(msg.Parts, "|"))

	case "c", "chat", "c:":
		if chat, ok := ParseChatMessage(msg); ok {
			c.dispatchChat(chat)
			c.routeCommand(chat.Room, chat.User, chat.Text)
		}

	case "pm":
		if pm, ok := ParsePrivateMessage(msg); ok {
			c.dispatchPM(pm)
			c.routeCommand("", pm.From, pm.Text)
		}
	}
}

func (c *Client) authenticate(challstr string) {
	if c.config.Username == "" {
		return
	}

	form := url.Values{}
	form.Set("name", c.config.Username)
	form.Set("pass", c.config.Password)
	form.Set("challstr", challstr)

	req, err := http.NewRequest("POST", c.config.LoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		log.Printf("Failed to create login request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		log.Printf("Login request error: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read login response: %v", err)
		return
	}

	// showdown returns a ']' prefix before the json payload
	body = bytes.TrimPrefix(body, []byte("]"))

	var result loginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("Failed to decode login JSON: %v (raw: %s)", err, string(body))
		return
	}

	if result.Assertion == "" {
		log.Printf("Login failed — empty assertion received")
		return
	}

	// assertions starting with ';;' indicate server rejection
	if strings.HasPrefix(result.Assertion, ";;") {
		log.Printf("Login assertion rejected by server: %s", result.Assertion[2:])
		return
	}

	_ = c.Send(fmt.Sprintf("|/trn %s,0,%s", c.config.Username, result.Assertion))
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
	// ignore commands sent by the bot itself
	if c.config.Username != "" && ToID(user) == ToID(c.config.Username) {
		return
	}

	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, c.config.CommandChar) {
		return
	}

	cmdStr := strings.TrimPrefix(trimmed, c.config.CommandChar)
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return
	}

	cmdName := strings.ToLower(fields[0])
	args := strings.TrimSpace(strings.TrimPrefix(cmdStr, fields[0]))

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
