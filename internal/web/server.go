package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TurboRx/GhostHaze-Thinker/internal/config"
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

// backupconfig models the configuration state saved to a backup
type BackupConfig struct {
	ServerID        string   `json:"server_id"`
	ServerHost      string   `json:"server_host"`
	ServerPort      int      `json:"server_port"`
	ServerSSL       bool     `json:"server_ssl"`
	ServerURL       string   `json:"server_url"`
	LoginServer     string   `json:"login_server"`
	LoginURL        string   `json:"login_url"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	Avatar          string   `json:"avatar"`
	CommandChar     string   `json:"command_char"`
	Rooms           []string `json:"rooms"`
	AutoBattle      bool     `json:"auto_battle"`
	AutoLeaveBattle bool     `json:"auto_leave_battle"`
	MaxBattles      int      `json:"max_battles"`
	BattleStartMsg  string   `json:"battle_start_msg"`
	BattleWinMsg    string   `json:"battle_win_msg"`
	BattleLoseMsg   string   `json:"battle_lose_msg"`
	BattleFormats   []string `json:"battle_formats"`
	BattleTeam      string   `json:"battle_team"`
}

// backuppayload models a complete backup file
type BackupPayload struct {
	Signature string       `json:"signature"`
	Version   string       `json:"version"`
	Timestamp string       `json:"timestamp"`
	Config    BackupConfig `json:"config"`
}

const BackupSignature = "$GHOSTHAZE$CONFIG$BACKUP$v1"

// abusemonitor tracks failed attempts per client ip
type abuseMonitor struct {
	mu           sync.Mutex
	attempts     map[string][]time.Time
	lockedUntil  map[string]time.Time
	maxAttempts  int
	window       time.Duration
	lockDuration time.Duration
}

func newAbuseMonitor(maxAttempts int, window, lockDuration time.Duration) *abuseMonitor {
	return &abuseMonitor{
		attempts:     make(map[string][]time.Time),
		lockedUntil:  make(map[string]time.Time),
		maxAttempts:  maxAttempts,
		window:       window,
		lockDuration: lockDuration,
	}
}

func (m *abuseMonitor) isLocked(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.lockedUntil[ip]
	if ok {
		if time.Now().Before(until) {
			return true
		}
		delete(m.lockedUntil, ip)
		delete(m.attempts, ip)
	}
	return false
}

func (m *abuseMonitor) recordFailure(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var recent []time.Time
	for _, t := range m.attempts[ip] {
		if now.Sub(t) < m.window {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	m.attempts[ip] = recent
	if len(recent) >= m.maxAttempts {
		m.lockedUntil[ip] = now.Add(m.lockDuration)
		return true
	}
	return false
}

func (m *abuseMonitor) recordSuccess(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.attempts, ip)
	delete(m.lockedUntil, ip)
}

// server manages the web control panel http server
type Server struct {
	client        *showdown.Client
	host          string
	port          int
	startTime     time.Time
	httpServer    *http.Server
	template      *template.Template
	loginTemplate *template.Template
	logMu         sync.RWMutex
	logs          []LogEntry
	maxLogs       int
	adminPassword string
	sessionMu     sync.RWMutex
	sessions      map[string]time.Time
	abuseMon      *abuseMonitor
}

// newserver initializes a new control panel server
func NewServer(client *showdown.Client, host string, port int, adminPassword ...string) (*Server, error) {
	tmpl, err := template.ParseFS(webFS, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse web templates: %w", err)
	}

	loginTmpl, _ := template.ParseFS(webFS, "templates/login.html")

	if host == "" {
		host = "0.0.0.0"
	}
	if port <= 0 {
		port = 8080
	}

	pw := ""
	if len(adminPassword) > 0 {
		pw = adminPassword[0]
	}
	if pw == "" {
		pw = "admin"
	}

	s := &Server{
		client:        client,
		host:          host,
		port:          port,
		startTime:     time.Now(),
		template:      tmpl,
		loginTemplate: loginTmpl,
		maxLogs:       200,
		adminPassword: pw,
		sessions:      make(map[string]time.Time),
		abuseMon:      newAbuseMonitor(5, 15*time.Minute, 15*time.Minute),
	}

	return s, nil
}

// setadminpassword updates the panel admin password
func (s *Server) SetAdminPassword(pw string) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if pw == "" {
		pw = "admin"
	}
	s.adminPassword = pw
}

// isauthenabled returns whether admin password protection is turned on
func (s *Server) IsAuthEnabled() bool {
	return true
}

// isauthenticated checks if request contains a valid session cookie or token
func (s *Server) IsAuthenticated(r *http.Request) bool {
	if !s.IsAuthEnabled() {
		return true
	}

	// inspect session cookie
	cookie, err := r.Cookie("ghosthaze_session")
	if err == nil && cookie != nil && cookie.Value != "" {
		s.sessionMu.RLock()
		expires, ok := s.sessions[cookie.Value]
		s.sessionMu.RUnlock()
		if ok && time.Now().Before(expires) {
			return true
		}
	}

	// inspect authorization header
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		s.sessionMu.RLock()
		expires, ok := s.sessions[token]
		s.sessionMu.RUnlock()
		if ok && time.Now().Before(expires) {
			return true
		}
	}

	return false
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

// clearlogs empties the circular log buffer
func (s *Server) ClearLogs() {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.logs = []LogEntry{}
}

// formatduration formats duration into human readable string
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
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

// uptime returns a formatted string of server process uptime
func (s *Server) Uptime() string {
	return formatDuration(time.Since(s.startTime))
}

// authmiddleware guards routes with password authentication when enabled
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.IsAuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// allow static files, login page, and login authentication api
		if strings.HasPrefix(r.URL.Path, "/static/") ||
			r.URL.Path == "/login" ||
			r.URL.Path == "/api/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		if s.IsAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}

		// unauthenticated api request
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "authentication required",
			})
			return
		}

		// redirect browser to login page
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

// buildmux registers all http routes
func (s *Server) buildMux() (http.Handler, error) {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to open static files sub-filesystem: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	mux.HandleFunc("/login", s.handleLoginPage)
	mux.HandleFunc("/api/auth/login", s.handleAPILogin)
	mux.HandleFunc("/api/auth/logout", s.handleAPILogout)

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleAPIStatus)
	mux.HandleFunc("/api/logs", s.handleAPILogs)
	mux.HandleFunc("/api/logs/raw", s.handleAPILogsRaw)
	mux.HandleFunc("/api/logs/clear", s.handleAPILogsClear)
	mux.HandleFunc("/api/backup/download", s.handleAPIBackupDownload)
	mux.HandleFunc("/api/backup/restore", s.handleAPIBackupRestore)
	mux.HandleFunc("/api/rooms/join", s.handleAPIRoomsJoin)
	mux.HandleFunc("/api/rooms/leave", s.handleAPIRoomsLeave)
	mux.HandleFunc("/api/send", s.handleAPISend)
	mux.HandleFunc("/api/challenge", s.handleAPIChallenge)
	mux.HandleFunc("/api/formats", s.handleAPIFormats)
	mux.HandleFunc("/api/tools/get-server", s.handleAPIGetServer)
	mux.HandleFunc("/api/config/update", s.handleAPIConfigUpdate)
	mux.HandleFunc("/api/bot/stop", s.handleAPIBotStop)
	mux.HandleFunc("/api/bot/reconnect", s.handleAPIBotReconnect)
	mux.HandleFunc("/api/bot/avatar", s.handleAPIBotAvatar)
	mux.HandleFunc("/api/bot/login", s.handleAPIBotLogin)
	mux.HandleFunc("/api/battles/forfeit", s.handleAPIBattlesForfeit)
	mux.HandleFunc("/api/battles/leave", s.handleAPIBattlesLeave)

	return s.authMiddleware(mux), nil
}

// handleloginpage serves the login view
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.IsAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if s.loginTemplate != nil {
		_ = s.loginTemplate.Execute(w, map[string]any{
			"AuthRequired": s.IsAuthEnabled(),
		})
		return
	}
	fmt.Fprintf(w, "<!DOCTYPE html><html><body><h2>Admin Login</h2><form method='post' action='/api/auth/login'><input type='password' name='password'/><button type='submit'>Login</button></form></body></html>")
}

// handleapilogin processes panel credentials and sets session cookie
func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := clientIP(r)
	if s.abuseMon.isLocked(ip) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "Too many failed attempts. Account locked for 15 minutes.",
		})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Password == "" {
		req.Password = r.FormValue("password")
	}

	valid := false
	s.sessionMu.RLock()
	adminPass := s.adminPassword
	s.sessionMu.RUnlock()
	if adminPass == "" {
		adminPass = "admin"
	}

	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(adminPass)) == 1 {
		valid = true
	}

	if !valid {
		locked := s.abuseMon.recordFailure(ip)
		s.AddLog("system", "Auth", fmt.Sprintf("Failed login attempt from IP: %s", ip))
		if locked {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "Too many failed attempts. Account locked for 15 minutes.",
			})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid password",
		})
		return
	}

	s.abuseMon.recordSuccess(ip)
	tokenBytes := make([]byte, 32)
	_, _ = rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	s.sessionMu.Lock()
	s.sessions[token] = time.Now().Add(24 * time.Hour)
	s.sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "ghosthaze_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	s.AddLog("system", "Auth", fmt.Sprintf("Admin login successful from IP: %s", ip))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"token": token,
	})
}

// handleapilogout clears current session
func (s *Server) handleAPILogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("ghosthaze_session")
	if err == nil && cookie != nil {
		s.sessionMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionMu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ghosthaze_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

	ssl := true
	if cfg.ServerSSL != nil {
		ssl = *cfg.ServerSSL
	}

	connectedAt := s.client.ConnectedAt()
	contime := "Offline"
	if !connectedAt.IsZero() && s.client.IsConnected() {
		contime = formatDuration(time.Since(connectedAt))
	} else if s.client.IsStopped() {
		contime = "Stopped"
	}

	// filter chatrooms from battle rooms
	chatRooms := make([]string, 0)
	for _, r := range s.client.Rooms() {
		if !strings.HasPrefix(r, "battle-") {
			chatRooms = append(chatRooms, r)
		}
	}

	data := map[string]any{
		"Connected":          s.client.IsConnected(),
		"Stopped":            s.client.IsStopped(),
		"LoggedIn":           s.client.IsLoggedIn(),
		"Username":           s.client.Username(),
		"ServerID":           cfg.ServerID,
		"ServerHost":         cfg.ServerHost,
		"ServerPort":         cfg.ServerPort,
		"ServerSSL":          ssl,
		"Avatar":             cfg.Avatar,
		"CommandChar":        cfg.CommandChar,
		"Rooms":              chatRooms,
		"AllRooms":           s.client.Rooms(),
		"ChatRoomsCount":     len(chatRooms),
		"ActiveBattlesCount": len(battlesData),
		"ConfigRooms":        strings.Join(cfg.Rooms, ", "),
		"ActiveBattles":      battlesData,
		"AutoBattle":         s.client.AutoBattle(),
		"AutoLeaveBattle":    cfg.ShouldAutoLeaveBattle(),
		"MaxBattles":         cfg.MaxBattles,
		"BattleStartMsg":     cfg.BattleStartMsg,
		"BattleWinMsg":       cfg.BattleWinMsg,
		"BattleLoseMsg":      cfg.BattleLoseMsg,
		"BattleFormats":      strings.Join(s.client.BattleFormats(), ", "),
		"BattleTeam":         s.client.BattleTeam(),
		"Uptime":             s.Uptime(),
		"ConTime":            contime,
		"AuthRequired":       s.IsAuthEnabled(),
		"Authenticated":      s.IsAuthenticated(r),
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

	connectedAt := s.client.ConnectedAt()
	var connectedAtMs int64
	var uptimeSec int64
	var contimeStr string
	if !connectedAt.IsZero() && s.client.IsConnected() {
		connectedAtMs = connectedAt.UnixMilli()
		uptimeSec = int64(time.Since(connectedAt).Seconds())
		contimeStr = formatDuration(time.Since(connectedAt))
	} else if s.client.IsStopped() {
		contimeStr = "Stopped"
	} else {
		contimeStr = "Disconnected"
	}
	serverUptimeSec := int64(time.Since(s.startTime).Seconds())
	serverUptimeStr := formatDuration(time.Since(s.startTime))

	// count non-battle chatrooms
	chatRoomsCount := 0
	for _, r := range s.client.Rooms() {
		if !strings.HasPrefix(r, "battle-") {
			chatRoomsCount++
		}
	}

	ssl := true
	if cfg.ServerSSL != nil {
		ssl = *cfg.ServerSSL
	}

	resp := map[string]any{
		"connected":                 s.client.IsConnected(),
		"stopped":                   s.client.IsStopped(),
		"logged_in":                 s.client.IsLoggedIn(),
		"is_guest":                  s.client.IsGuest(),
		"username":                  s.client.Username(),
		"config_username":           cfg.Username,
		"server_id":                 cfg.ServerID,
		"server_host":               cfg.ServerHost,
		"server_port":               cfg.ServerPort,
		"server_ssl":                ssl,
		"server_url":                cfg.ServerURL,
		"avatar":                    cfg.Avatar,
		"command_char":              cfg.CommandChar,
		"auto_battle":               s.client.AutoBattle(),
		"auto_leave_battle":         cfg.ShouldAutoLeaveBattle(),
		"max_battles":               cfg.MaxBattles,
		"battle_start_msg":          cfg.BattleStartMsg,
		"battle_win_msg":            cfg.BattleWinMsg,
		"battle_lose_msg":           cfg.BattleLoseMsg,
		"battle_formats":            s.client.BattleFormats(),
		"battle_team":               s.client.BattleTeam(),
		"rooms":                     s.client.Rooms(),
		"config_rooms":              cfg.Rooms,
		"chat_rooms_count":          chatRoomsCount,
		"active_battles_count":      len(battlesData),
		"active_battles":            battlesData,
		"connected_at_ms":           connectedAtMs,
		"server_started_at_ms":      s.startTime.UnixMilli(),
		"uptime_seconds":            uptimeSec,
		"connection_uptime_seconds": uptimeSec,
		"server_uptime_seconds":     serverUptimeSec,
		"process_uptime_seconds":    serverUptimeSec,
		"contime":                   contimeStr,
		"uptime":                    serverUptimeStr,
		"auth_required":             s.IsAuthEnabled(),
		"authenticated":             s.IsAuthenticated(r),
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

// handleapilogsraw streams activity logs as plain text
func (s *Server) handleAPILogsRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logs := s.LogsList()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	var sb strings.Builder
	for i := len(logs) - 1; i >= 0; i-- {
		entry := logs[i]
		sb.WriteString(fmt.Sprintf("[%s] [%s] %s: %s\n", entry.Time, entry.Type, entry.Source, entry.Message))
	}

	if sb.Len() == 0 {
		sb.WriteString("No logs recorded yet.\n")
	}

	_, _ = w.Write([]byte(sb.String()))
}

// handleapilogsclear empties all stored activity events
func (s *Server) handleAPILogsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.ClearLogs()
	s.AddLog("system", "Activity Log", "Logs cleared by user")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleapibackupdownload generates a downloadable configuration backup json file
func (s *Server) handleAPIBackupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.client.ClientConfig()
	ssl := true
	if cfg.ServerSSL != nil {
		ssl = *cfg.ServerSSL
	}

	payload := BackupPayload{
		Signature: BackupSignature,
		Version:   "1.0.0",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Config: BackupConfig{
			ServerID:        cfg.ServerID,
			ServerHost:      cfg.ServerHost,
			ServerPort:      cfg.ServerPort,
			ServerSSL:       ssl,
			ServerURL:       cfg.ServerURL,
			LoginServer:     cfg.LoginServer,
			LoginURL:        cfg.LoginURL,
			Username:        cfg.Username,
			Password:        cfg.Password,
			Avatar:          cfg.Avatar,
			CommandChar:     cfg.CommandChar,
			Rooms:           cfg.Rooms,
			AutoBattle:      s.client.AutoBattle(),
			AutoLeaveBattle: cfg.ShouldAutoLeaveBattle(),
			MaxBattles:      cfg.MaxBattles,
			BattleStartMsg:  cfg.BattleStartMsg,
			BattleWinMsg:    cfg.BattleWinMsg,
			BattleLoseMsg:   cfg.BattleLoseMsg,
			BattleFormats:   s.client.BattleFormats(),
			BattleTeam:      s.client.BattleTeam(),
		},
	}

	fileName := fmt.Sprintf("ghosthaze_backup_%s.json", time.Now().Format("2006_01_02"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

// handleapibackuprestore parses and applies an uploaded configuration backup
func (s *Server) handleAPIBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rawData []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse uploaded file: " + err.Error()})
			return
		}
		file, _, err := r.FormFile("backupfile")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing backupfile in form upload"})
			return
		}
		defer file.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(file)
		rawData = buf.Bytes()
	} else {
		var err error
		rawData, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
			return
		}
	}

	var payload BackupPayload
	if err := json.Unmarshal(rawData, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid backup json format: " + err.Error()})
		return
	}

	if payload.Signature != BackupSignature {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid backup signature: unverified backup file"})
		return
	}

	bCfg := payload.Config
	s.client.UpdateConfig(func(c *showdown.Config) {
		if bCfg.ServerID != "" {
			c.ServerID = bCfg.ServerID
		}
		if bCfg.ServerHost != "" {
			c.ServerHost = bCfg.ServerHost
		}
		if bCfg.ServerPort > 0 {
			c.ServerPort = bCfg.ServerPort
		}
		c.ServerSSL = &bCfg.ServerSSL
		if bCfg.ServerURL != "" {
			c.ServerURL = bCfg.ServerURL
		}
		if bCfg.LoginServer != "" {
			c.LoginServer = bCfg.LoginServer
		}
		if bCfg.LoginURL != "" {
			c.LoginURL = bCfg.LoginURL
		}
		if bCfg.Username != "" {
			c.Username = bCfg.Username
		}
		if bCfg.Password != "" {
			c.Password = bCfg.Password
		}
		if bCfg.Avatar != "" {
			c.Avatar = bCfg.Avatar
		}
		if bCfg.CommandChar != "" {
			c.CommandChar = bCfg.CommandChar
		}
		if bCfg.Rooms != nil {
			c.Rooms = bCfg.Rooms
		}
		c.AutoBattle = bCfg.AutoBattle
		c.AutoLeaveBattle = &bCfg.AutoLeaveBattle
		if bCfg.MaxBattles > 0 {
			c.MaxBattles = bCfg.MaxBattles
		}
		c.BattleStartMsg = bCfg.BattleStartMsg
		c.BattleWinMsg = bCfg.BattleWinMsg
		c.BattleLoseMsg = bCfg.BattleLoseMsg
		c.BattleFormats = bCfg.BattleFormats
		c.BattleTeam = bCfg.BattleTeam
	})

	savedCfg := s.client.ClientConfig()
	_ = config.SaveEnvFile(".env", &savedCfg)

	s.AddLog("system", "Backup", fmt.Sprintf("Configuration restored successfully (timestamp: %s)", payload.Timestamp))

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Configuration restored successfully.",
	})
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

	// remove room from configured auto-join rooms if present and persist
	s.client.UpdateConfig(func(cfg *showdown.Config) {
		var updated []string
		for _, rm := range cfg.Rooms {
			if showdown.ToRoomID(rm) != showdown.ToRoomID(room) {
				updated = append(updated, rm)
			}
		}
		cfg.Rooms = updated
	})
	savedCfg := s.client.ClientConfig()
	_ = config.SaveEnvFile(".env", &savedCfg)

	s.AddLog("room", "Control Panel", "Left chatroom: "+room)
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

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is empty"})
		return
	}

	var err error
	if req.IsPM {
		err = s.client.SendPM(req.Target, msg)
		if err == nil {
			s.AddLog("pm", "Outbox -> "+req.Target, msg)
		}
	} else {
		err = s.client.SendRoom(req.Target, msg)
		if err == nil {
			s.AddLog("chat", "Outbox -> "+req.Target, msg)
		}
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleapichallenge sends a battle challenge
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is empty"})
		return
	}
	if format == "" {
		format = "gen9randombattle"
	}

	if err := s.client.ChallengeUser(user, format); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("battle", "Challenge", fmt.Sprintf("Challenged %s in %s", user, format))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "user": user, "format": format})
}

// handleapiformats returns the list of formats fetched from the showdown server
func (s *Server) handleAPIFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	formats := s.client.Formats()
	writeJSON(w, http.StatusOK, formats)
}

// handleapigetserver resolves showdown server parameters
func (s *Server) handleAPIGetServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing url or server id"})
		return
	}

	info, err := showdown.GetShowdownServer(strings.TrimSpace(req.URL), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	cfg := s.client.ClientConfig()
	loginServer := cfg.LoginServer
	if loginServer == "" {
		loginServer = showdown.DefaultLoginServer
	}

	resp := map[string]any{
		"id":        info.ID,
		"host":      info.Host,
		"port":      info.Port,
		"ssl":       info.HTTPS,
		"ws_url":    info.WebSocketURL(),
		"login_url": info.LoginActionURL(loginServer),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleapiconfigupdate updates bot runtime config and persists to env
func (s *Server) handleAPIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ServerID         string   `json:"server_id"`
		ServerHost       string   `json:"server_host"`
		ServerPort       int      `json:"server_port"`
		ServerSSL        *bool    `json:"server_ssl"`
		Username         string   `json:"username"`
		Password         string   `json:"password"`
		Avatar           string   `json:"avatar"`
		CommandChar      string   `json:"command_char"`
		Rooms            []string `json:"rooms"`
		AutoBattle       *bool    `json:"auto_battle"`
		AutoLeaveBattle  *bool    `json:"auto_leave_battle"`
		MaxBattles       int      `json:"max_battles"`
		BattleStartMsg   string   `json:"battle_start_msg"`
		BattleWinMsg     string   `json:"battle_win_msg"`
		BattleLoseMsg    string   `json:"battle_lose_msg"`
		BattleFormats    []string `json:"battle_formats"`
		BattleTeam       string   `json:"battle_team"`
		WebAdminPassword string   `json:"web_admin_password"`
		Reconnect        bool     `json:"reconnect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	s.client.UpdateConfig(func(cfg *showdown.Config) {
		if req.ServerID != "" {
			cfg.ServerID = req.ServerID
		}
		if req.ServerHost != "" {
			cfg.ServerHost = req.ServerHost
		}
		if req.ServerPort > 0 {
			cfg.ServerPort = req.ServerPort
		}
		if req.ServerSSL != nil {
			cfg.ServerSSL = req.ServerSSL
		}
		if req.Username != "" {
			cfg.Username = req.Username
		}
		if req.Password != "" {
			cfg.Password = req.Password
		}
		if req.Avatar != "" {
			cfg.Avatar = req.Avatar
		}
		if req.CommandChar != "" {
			cfg.CommandChar = req.CommandChar
		}
		if req.Rooms != nil {
			cfg.Rooms = req.Rooms
		}
		if req.AutoBattle != nil {
			cfg.AutoBattle = *req.AutoBattle
		}
		if req.AutoLeaveBattle != nil {
			cfg.AutoLeaveBattle = req.AutoLeaveBattle
		}
		if req.MaxBattles > 0 {
			cfg.MaxBattles = req.MaxBattles
		}
		if req.BattleStartMsg != "" {
			cfg.BattleStartMsg = req.BattleStartMsg
		}
		if req.BattleWinMsg != "" {
			cfg.BattleWinMsg = req.BattleWinMsg
		}
		if req.BattleLoseMsg != "" {
			cfg.BattleLoseMsg = req.BattleLoseMsg
		}
		if req.BattleFormats != nil {
			cfg.BattleFormats = req.BattleFormats
		}
		cfg.BattleTeam = req.BattleTeam
		cfg.ServerURL = ""
		cfg.ApplyDefaults()
	})

	if req.WebAdminPassword != "" {
		s.SetAdminPassword(req.WebAdminPassword)
		_ = os.Setenv("WEB_ADMIN_PASSWORD", req.WebAdminPassword)
	}

	// persist updated configuration to .env file
	savedCfg := s.client.ClientConfig()
	_ = config.SaveEnvFile(".env", &savedCfg)

	s.AddLog("system", "Control Panel", "Configuration updated")

	if req.Avatar != "" {
		_ = s.client.SetAvatar(req.Avatar)
	}

	if req.Reconnect {
		s.AddLog("system", "Control Panel", "Reconnecting bot with updated configuration...")
		s.client.Reconnect()
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "configuration saved"})
}

// handleapibotstop disconnects and halts the bot
func (s *Server) handleAPIBotStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.client.Stop()
	s.AddLog("system", "Bot", "Bot connection stopped via control panel")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"stopped": true,
		"message": "Bot disconnected and stopped.",
	})
}

// handleapibotlogin authenticates the bot with username and password
func (s *Server) handleAPIBotLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Username) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or missing username"})
		return
	}

	if err := s.client.Login(req.Username, req.Password); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	savedCfg := s.client.ClientConfig()
	_ = config.SaveEnvFile(".env", &savedCfg)

	s.AddLog("system", "Control Panel", fmt.Sprintf("Login request submitted for user '%s'", req.Username))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "login initiated"})
}

// handleapibattlesforfeit forfeits an active battle and leaves the room
func (s *Server) handleAPIBattlesForfeit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Room) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or missing room"})
		return
	}

	trimmed := strings.TrimSpace(req.Room)
	if err := s.client.ForfeitBattle(trimmed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("battle", trimmed, "Forfeited and left battle")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "battle forfeited"})
}

// handleapibattlesleave leaves a battle room
func (s *Server) handleAPIBattlesLeave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Room) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or missing room"})
		return
	}

	trimmed := strings.TrimSpace(req.Room)
	if err := s.client.LeaveBattle(trimmed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("battle", trimmed, "Left battle room")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "left battle room"})
}

// handleapibotreconnect triggers a reconnection
func (s *Server) handleAPIBotReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.AddLog("system", "Control Panel", "Manual reconnect triggered")
	s.client.Reconnect()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"stopped": false,
		"message": "bot reconnecting",
	})
}

// handleapibotavatar sets avatar directly
func (s *Server) handleAPIBotAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Avatar string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}

	trimmed := strings.TrimSpace(req.Avatar)
	if trimmed == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar cannot be empty"})
		return
	}

	if err := s.client.SetAvatar(trimmed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", "Avatar updated to: "+trimmed)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "avatar": trimmed})
}

// clientip extracts client ip address from request headers or remoteaddr
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
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
