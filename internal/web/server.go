package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/pkg/showdown"
)

//go:embed templates/* static/*
var webFS embed.FS

// logentry represents an in-memory event entry
type LogEntry struct {
	Time    string `json:"time"`
	Type    string `json:"type"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// server manages the web control panel http server
type Server struct {
	client     *showdown.Client
	host       string
	port       int
	startTime  time.Time
	httpServer *http.Server
	template   *template.Template
	logMu      sync.RWMutex
	logs       []LogEntry
	maxLogs    int
}

// newserver initializes a new control panel server
func NewServer(client *showdown.Client, host string, port int) (*Server, error) {
	tmpl, err := template.ParseFS(webFS, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse web templates: %w", err)
	}

	if host == "" {
		host = "0.0.0.0"
	}
	if port <= 0 {
		port = 8080
	}

	s := &Server{
		client:    client,
		host:      host,
		port:      port,
		startTime: time.Now(),
		template:  tmpl,
		maxLogs:   200,
	}

	return s, nil
}

// addlog appends an event to the circular log buffer
func (s *Server) AddLog(entryType, source, msg string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Type:    entryType,
		Source:  source,
		Message: msg,
	}

	s.logs = append([]LogEntry{entry}, s.logs...)
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[:s.maxLogs]
	}
}

// logslist returns a copy of current logs
func (s *Server) LogsList() []LogEntry {
	s.logMu.RLock()
	defer s.logMu.RUnlock()

	res := make([]LogEntry, len(s.logs))
	copy(res, s.logs)
	return res
}

// uptime returns a formatted string of server uptime
func (s *Server) Uptime() string {
	d := time.Since(s.startTime).Round(time.Second)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// buildmux registers all http routes
func (s *Server) buildMux() (http.Handler, error) {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to open static files sub-filesystem: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/logs", s.handleAPILogs)
	mux.HandleFunc("/api/rooms/join", s.handleAPIRoomsJoin)
	mux.HandleFunc("/api/rooms/leave", s.handleAPIRoomsLeave)
	mux.HandleFunc("/api/send", s.handleAPISend)
	mux.HandleFunc("/api/challenge", s.handleAPIChallenge)
	mux.HandleFunc("/api/tools/get-server", s.handleAPIGetServer)

	return mux, nil
}

// handleindex renders the main control panel view
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	cfg := s.client.ClientConfig()
	activeBattles := s.client.ActiveBattles()

	battlesData := make([]map[string]any, 0, len(activeBattles))
	for _, b := range activeBattles {
		battlesData = append(battlesData, map[string]any{
			"room":     b.Room,
			"format":   b.Tier,
			"turn":     b.Turn,
			"opponent": b.OpponentName,
		})
	}

	data := map[string]any{
		"Connected":     s.client.IsConnected(),
		"LoggedIn":      s.client.IsLoggedIn(),
		"Username":      s.client.Username(),
		"ServerID":      cfg.ServerID,
		"ServerHost":    cfg.ServerHost,
		"CommandChar":   cfg.CommandChar,
		"Rooms":         s.client.Rooms(),
		"ActiveBattles": battlesData,
		"AutoBattle":    s.client.AutoBattle(),
		"Uptime":        s.Uptime(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.Execute(w, data); err != nil {
		http.Error(w, "failed to render template: "+err.Error(), http.StatusInternalServerError)
	}
}

// handleapistatus returns current bot status as json
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.client.ClientConfig()
	activeBattles := s.client.ActiveBattles()

	battlesData := make([]map[string]any, 0, len(activeBattles))
	for _, b := range activeBattles {
		battlesData = append(battlesData, map[string]any{
			"room":     b.Room,
			"format":   b.Tier,
			"turn":     b.Turn,
			"opponent": b.OpponentName,
		})
	}

	resp := map[string]any{
		"connected":      s.client.IsConnected(),
		"logged_in":      s.client.IsLoggedIn(),
		"username":       s.client.Username(),
		"server_id":      cfg.ServerID,
		"server_host":    cfg.ServerHost,
		"rooms":          s.client.Rooms(),
		"active_battles": battlesData,
		"uptime":         s.Uptime(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleapilogs returns stored activity events
func (s *Server) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.LogsList())
}

// handleapiroomsjoin joins a room
func (s *Server) handleAPIRoomsJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	room := strings.TrimSpace(req.Room)
	if room == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "room name is empty"})
		return
	}

	if err := s.client.JoinRoom(room); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("room", "Control Panel", "Requested join: "+room)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "room": room})
}

// handleapiroomsleave leaves a room
func (s *Server) handleAPIRoomsLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	room := strings.TrimSpace(req.Room)
	if room == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "room name is empty"})
		return
	}

	if err := s.client.LeaveRoom(room); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("room", "Control Panel", "Requested leave: "+room)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "room": room})
}

// handleapisend sends a message to room or user
func (s *Server) handleAPISend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target  string `json:"target"`
		Message string `json:"message"`
		IsPM    bool   `json:"is_pm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	target := strings.TrimSpace(req.Target)
	message := strings.TrimSpace(req.Message)
	if target == "" || message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target and message cannot be empty"})
		return
	}

	var err error
	if req.IsPM {
		err = s.client.SendPM(target, message)
		s.AddLog("pm", "Outbound PM", fmt.Sprintf("To %s: %s", target, message))
	} else {
		err = s.client.SendToRoom(target, message)
		s.AddLog("chat", "Outbound Room", fmt.Sprintf("[%s]: %s", target, message))
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleapichallenge issues a battle challenge
func (s *Server) handleAPIChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User   string `json:"user"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	user := strings.TrimSpace(req.User)
	format := strings.TrimSpace(req.Format)
	if user == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user cannot be empty"})
		return
	}
	if format == "" {
		format = "gen9randombattle"
	}

	if err := s.client.ChallengeUser(user, format); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("battle", "Control Panel", fmt.Sprintf("Challenge issued to %s in %s", user, format))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleapigetserver executes the get-server discovery tool
func (s *Server) handleAPIGetServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	target := strings.TrimSpace(req.URL)
	if target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target url cannot be empty"})
		return
	}

	info, err := showdown.GetShowdownServer(target, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]any{
		"host":          info.Host,
		"port":          info.Port,
		"id":            info.ID,
		"https":         info.HTTPS,
		"registered":    info.Registered,
		"websocket_url": info.WebSocketURL(),
		"login_url":     info.LoginActionURL(""),
	}

	writeJSON(w, http.StatusOK, resp)
}

// start launches the http listener
func (s *Server) Start() error {
	mux, err := s.buildMux()
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	s.AddLog("system", "Web Server", fmt.Sprintf("Control panel listening on http://%s", addr))

	err = s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// shutdown gracefully terminates the http server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// writejson serializes payload as json http response
func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
