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
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

// addlog appends an event to the circular log buffer and writes to persistent logs
func (s *Server) AddLog(entryType, source, msg string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()

	now := time.Now()
	entry := LogEntry{
		Time:    now.Format("15:04:05"),
		Type:    entryType,
		Source:  source,
		Message: msg,
	}

	s.logs = append([]LogEntry{entry}, s.logs...)
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[:s.maxLogs]
	}

	// append to disk daily security log matching showdown-chatbot format
	_ = os.MkdirAll("logs", 0755)
	dailyPath := fmt.Sprintf("logs/seclog_%s.log", now.Format("2006_01_02"))
	logLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", now.Format("2006-01-02 15:04:05"), entryType, source, msg)
	if f, err := os.OpenFile(dailyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		_, _ = f.WriteString(logLine)
		_ = f.Close()
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

	mux.HandleFunc("/api/teams", s.handleAPITeams)
	mux.HandleFunc("/api/teams/save", s.handleAPITeamsSave)
	mux.HandleFunc("/api/teams/delete", s.handleAPITeamsDelete)
	mux.HandleFunc("/api/teams/toggle", s.handleAPITeamsToggle)

	mux.HandleFunc("/api/commands", s.handleAPICommands)
	mux.HandleFunc("/api/commands/save", s.handleAPICommandsSave)
	mux.HandleFunc("/api/commands/delete", s.handleAPICommandsDelete)
	mux.HandleFunc("/api/commands/toggle", s.handleAPICommandsToggle)

	mux.HandleFunc("/api/battles/history", s.handleAPIBattlesHistory)
	mux.HandleFunc("/api/battles/history/clear", s.handleAPIBattlesHistoryClear)

	mux.HandleFunc("/api/timers", s.handleAPITimers)
	mux.HandleFunc("/api/timers/save", s.handleAPITimersSave)
	mux.HandleFunc("/api/timers/delete", s.handleAPITimersDelete)
	mux.HandleFunc("/api/timers/toggle", s.handleAPITimersToggle)
	mux.HandleFunc("/api/timers/trigger", s.handleAPITimersTrigger)

	mux.HandleFunc("/api/blacklist", s.handleAPIBlacklist)
	mux.HandleFunc("/api/blacklist/add", s.handleAPIBlacklistAdd)
	mux.HandleFunc("/api/blacklist/remove", s.handleAPIBlacklistRemove)

	mux.HandleFunc("/api/joinphrases", s.handleAPIJoinPhrases)
	mux.HandleFunc("/api/joinphrases/save", s.handleAPIJoinPhrasesSave)
	mux.HandleFunc("/api/joinphrases/delete", s.handleAPIJoinPhrasesDelete)
	mux.HandleFunc("/api/joinphrases/toggle", s.handleAPIJoinPhrasesToggle)

	mux.HandleFunc("/api/moderation", s.handleAPIModeration)
	mux.HandleFunc("/api/moderation/save", s.handleAPIModerationSave)

	mux.HandleFunc("/api/ladder/status", s.handleAPILadderStatus)
	mux.HandleFunc("/api/ladder/start", s.handleAPILadderStart)
	mux.HandleFunc("/api/ladder/stop", s.handleAPILadderStop)

	// auth actions
	mux.HandleFunc("/api/auth/change-password", s.handleAPIChangePassword)

	// chatroom status and keepalive
	mux.HandleFunc("/api/bot/status", s.handleAPIBotStatus)
	mux.HandleFunc("/api/bot/anti-afk", s.handleAPIAntiAFK)
	mux.HandleFunc("/api/rooms/join-official", s.handleAPIJoinOfficialRooms)
	mux.HandleFunc("/api/rooms/join-public", s.handleAPIJoinPublicRooms)
	mux.HandleFunc("/api/bot/hotpatch", s.handleAPIBotHotpatch)

	// admin and maintenance
	mux.HandleFunc("/api/admin/files", s.handleAPIAdminFiles)
	mux.HandleFunc("/api/admin/files/view", s.handleAPIAdminFileView)
	mux.HandleFunc("/api/admin/files/download", s.handleAPIAdminFileDownload)
	mux.HandleFunc("/api/admin/files/clear", s.handleAPIAdminFileClear)
	mux.HandleFunc("/api/admin/reload-data", s.handleAPIAdminReloadData)
	mux.HandleFunc("/api/admin/clear-cache", s.handleAPIAdminClearCache)
	mux.HandleFunc("/api/admin/clear-user-data", s.handleAPIAdminClearUserData)
	mux.HandleFunc("/api/admin/eval", s.handleAPIAdminEval)

	// user seen tracking
	mux.HandleFunc("/api/users/seen", s.handleAPIUsersSeen)
	mux.HandleFunc("/api/users/seen/clear", s.handleAPIUsersSeenClear)

	// command aliases
	mux.HandleFunc("/api/commands/aliases", s.handleAPICommandsAliases)
	mux.HandleFunc("/api/commands/aliases/save", s.handleAPICommandsAliasesSave)
	mux.HandleFunc("/api/commands/aliases/delete", s.handleAPICommandsAliasesDelete)

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
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.Password = r.FormValue("password")
	}
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

	var stats showdown.BattleStats
	if s.client.History() != nil {
		stats = s.client.History().Stats()
	}
	teamsCount := 0
	if s.client.Teams() != nil {
		teamsCount = len(s.client.Teams().List())
	}
	commandsCount := 0
	if s.client.DynamicCommands() != nil {
		commandsCount = len(s.client.DynamicCommands().List())
	}

	resp := map[string]any{
		"connected":                 s.client.IsConnected(),
		"stopped":                   s.client.IsStopped(),
		"logged_in":                 s.client.IsLoggedIn(),
		"is_guest":                  s.client.IsConnected() && s.client.IsGuest(),
		"username":                  func() string { if s.client.IsConnected() { return s.client.Username() } else { return cfg.Username } }(),
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
		"win_rate":                  stats.WinRate,
		"wins":                      stats.Wins,
		"losses":                    stats.Losses,
		"ties":                      stats.Ties,
		"total_battles":             stats.TotalBattles,
		"teams_count":               teamsCount,
		"commands_count":            commandsCount,
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

	if s.client.IsGuest() {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "The bot is currently connected as an anonymous Guest. Pokémon Showdown requires a registered bot account (username & password) to send battle challenges. Please login using the Bot Login Tool or enter credentials in Configuration.",
		})
		return
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
		s.client.Start()
		s.client.Reconnect()
	} else if req.Username != "" && req.Password != "" && !strings.HasPrefix(strings.ToLower(req.Username), "guest") {
		s.AddLog("system", "Control Panel", fmt.Sprintf("Initiating login for '%s' with updated credentials...", req.Username))
		s.client.Start()
		_ = s.client.Login(req.Username, req.Password)
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

	s.client.Start()
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

// handleapibotreconnect triggers a reconnection or starts a stopped bot
func (s *Server) handleAPIBotReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.client.ClientConfig()
	if strings.TrimSpace(cfg.Username) == "" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Username)), "guest") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Please configure your bot username and password in Configuration or Bot Login Tool first before connecting.",
		})
		return
	}

	s.AddLog("system", "Control Panel", "Bot connection initiated")
	s.client.Start()
	s.client.Reconnect()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"stopped": false,
		"message": "bot connecting",
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

// handleapiteams returns all configured battle teams
func (s *Server) handleAPITeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.client.Teams() == nil {
		writeJSON(w, http.StatusOK, []showdown.BattleTeam{})
		return
	}
	writeJSON(w, http.StatusOK, s.client.Teams().List())
}

// handleapiteamssave creates or edits a battle team
func (s *Server) handleAPITeamsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var team showdown.BattleTeam
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if s.client.Teams() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "team vault is not initialized"})
		return
	}

	if err := s.client.Teams().AddOrUpdate(team); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Saved battle team '%s' for format %s", team.Name, team.Format))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapiteamsdelete removes a battle team
func (s *Server) handleAPITeamsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "team id is required"})
		return
	}

	if s.client.Teams() == nil || !s.client.Teams().Delete(req.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "team not found"})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Deleted battle team %s", req.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapiteamstoggle toggles active state of a battle team
func (s *Server) handleAPITeamsToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "team id is required"})
		return
	}

	if s.client.Teams() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "team vault is not initialized"})
		return
	}

	active, err := s.client.Teams().Toggle(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": active})
}

// handleapicommands returns all dynamic custom commands
func (s *Server) handleAPICommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusOK, []showdown.CustomCommand{})
		return
	}
	writeJSON(w, http.StatusOK, s.client.DynamicCommands().List())
}

// handleapicommandssave adds or updates a custom command
func (s *Server) handleAPICommandsSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cmd showdown.CustomCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commands store is not initialized"})
		return
	}

	if err := s.client.DynamicCommands().AddOrUpdate(cmd); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Saved custom command '.%s'", cmd.Name))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapicommandsdelete removes a custom command
func (s *Server) handleAPICommandsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command name is required"})
		return
	}

	if s.client.DynamicCommands() == nil || !s.client.DynamicCommands().Delete(req.Name) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "command not found"})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Deleted custom command '.%s'", req.Name))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapicommandstoggle toggles enabled state of a custom command
func (s *Server) handleAPICommandsToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command name is required"})
		return
	}

	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commands store is not initialized"})
		return
	}

	enabled, err := s.client.DynamicCommands().Toggle(req.Name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

// handleapibattleshistory returns match history records and statistics
func (s *Server) handleAPIBattlesHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	var records []showdown.BattleRecord
	var stats showdown.BattleStats
	if s.client.History() != nil {
		records = s.client.History().List(limit)
		stats = s.client.History().Stats()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"records": records,
		"stats":   stats,
	})
}

// handleapibattleshistoryclear resets battle history
func (s *Server) handleAPIBattlesHistoryClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.History() != nil {
		_ = s.client.History().Clear()
	}

	s.AddLog("system", "Control Panel", "Cleared battle match history")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapitimers lists all chatroom timers
func (s *Server) handleAPITimers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var timers []showdown.ChatroomTimer
	if s.client.Timers() != nil {
		timers = s.client.Timers().List()
	}
	writeJSON(w, http.StatusOK, map[string]any{"timers": timers})
}

// handleapitimerssave creates or updates a chatroom timer
func (s *Server) handleAPITimersSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req showdown.ChatroomTimer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Room == "" || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chatroom and message are required"})
		return
	}

	if req.IntervalMinutes < 1 {
		req.IntervalMinutes = 5
	}

	if s.client.Timers() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "timers store not initialized"})
		return
	}

	if err := s.client.Timers().Save(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Saved chatroom timer for '%s' (interval: %dm)", req.Room, req.IntervalMinutes))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapitimersdelete removes a chatroom timer
func (s *Server) handleAPITimersDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timer id is required"})
		return
	}

	if s.client.Timers() == nil || !s.client.Timers().Delete(req.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "timer not found"})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Deleted chatroom timer '%s'", req.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapitimerstoggle toggles enabled state of a timer
func (s *Server) handleAPITimersToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timer id is required"})
		return
	}

	if s.client.Timers() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "timers store not initialized"})
		return
	}

	enabled, err := s.client.Timers().Toggle(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

// handleapitimerstrigger dispatches a timer announcement immediately
func (s *Server) handleAPITimersTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timer id is required"})
		return
	}

	if s.client.Timers() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "timers store not initialized"})
		return
	}

	if err := s.client.Timers().TriggerNow(req.ID, s.client); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Triggered chatroom timer '%s' immediately", req.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "sent"})
}

// handleapiblacklist returns blacklisted users
func (s *Server) handleAPIBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var list []showdown.BlacklistEntry
	if s.client.Blacklist() != nil {
		list = s.client.Blacklist().List()
	}
	writeJSON(w, http.StatusOK, map[string]any{"blacklist": list})
}

// handleapiblacklistadd adds a user to the blacklist
func (s *Server) handleAPIBlacklistAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Username) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	if s.client.Blacklist() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "blacklist store not initialized"})
		return
	}

	entry := s.client.Blacklist().Add(req.Username, req.Reason)
	s.AddLog("system", "Control Panel", fmt.Sprintf("Added '%s' to blacklist: %s", entry.Username, entry.Reason))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": entry})
}

// handleapiblacklistremove removes a user from the blacklist
func (s *Server) handleAPIBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Username) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	if s.client.Blacklist() == nil || !s.client.Blacklist().Remove(req.Username) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found in blacklist"})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Removed '%s' from blacklist", req.Username))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapijoinphrases returns all configured join phrases
func (s *Server) handleAPIJoinPhrases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var phrases []showdown.JoinPhrase
	if s.client.JoinPhrases() != nil {
		phrases = s.client.JoinPhrases().List()
	}
	writeJSON(w, http.StatusOK, map[string]any{"joinphrases": phrases})
}

// handleapijoinphrasessave creates or updates a join phrase
func (s *Server) handleAPIJoinPhrasesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req showdown.JoinPhrase
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if s.client.JoinPhrases() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "join phrases store not initialized"})
		return
	}

	saved, err := s.client.JoinPhrases().Save(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Saved join phrase for user '%s'", saved.Username))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": saved})
}

// handleapijoinphrasesdelete removes a join phrase
func (s *Server) handleAPIJoinPhrasesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phrase id is required"})
		return
	}

	if s.client.JoinPhrases() == nil || !s.client.JoinPhrases().Delete(req.ID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "phrase not found"})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Deleted join phrase '%s'", req.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "ok"})
}

// handleapijoinphrasestoggle toggles enabled state of a join phrase
func (s *Server) handleAPIJoinPhrasesToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phrase id is required"})
		return
	}

	if s.client.JoinPhrases() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "join phrases store not initialized"})
		return
	}

	enabled, err := s.client.JoinPhrases().Toggle(req.ID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

// handleapimoderation retrieves chatroom moderation rules
func (s *Server) handleAPIModeration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg showdown.ModerationConfig
	if s.client.Moderation() != nil {
		cfg = s.client.Moderation().GetConfig()
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

// handleapimoderationsave updates chatroom moderation rules
func (s *Server) handleAPIModerationSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg showdown.ModerationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if s.client.Moderation() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "moderation store not initialized"})
		return
	}

	if err := s.client.Moderation().SaveConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", "Updated chatroom automated moderation rules")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
}

// handleapiladderstatus returns the active state and metrics of the ladder bot
func (s *Server) handleAPILadderStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var status showdown.LadderStatus
	if s.client.Ladder() != nil {
		status = s.client.Ladder().Status()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ladder": status})
}

// handleapiladderstart initiates automated ranked ladder matchmaking
func (s *Server) handleAPILadderStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Format     string `json:"format"`
		MaxBattles int    `json:"max_battles"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Format == "" {
		req.Format = "gen9randombattle"
	}

	if s.client.Ladder() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ladder bot not initialized"})
		return
	}

	if err := s.client.Ladder().Start(s.client, req.Format, req.MaxBattles); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", fmt.Sprintf("Started ranked ladder matchmaking in '%s'", req.Format))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ladder": s.client.Ladder().Status()})
}

// handleapiladderstop cancels automated ranked ladder matchmaking
func (s *Server) handleAPILadderStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.Ladder() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ladder bot not initialized"})
		return
	}

	if err := s.client.Ladder().Stop(s.client); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Control Panel", "Stopped ranked ladder matchmaking")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ladder": s.client.Ladder().Status()})
}

// handleapichangepassword updates the web panel admin password
func (s *Server) handleAPIChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	s.sessionMu.RLock()
	currentPass := s.adminPassword
	s.sessionMu.RUnlock()
	if currentPass == "" {
		currentPass = "admin"
	}

	if subtle.ConstantTimeCompare([]byte(req.OldPassword), []byte(currentPass)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Current admin password does not match"})
		return
	}

	trimmedNew := strings.TrimSpace(req.NewPassword)
	if trimmedNew == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New password cannot be blank"})
		return
	}

	s.SetAdminPassword(trimmedNew)
	_ = os.Setenv("WEB_ADMIN_PASSWORD", trimmedNew)

	savedCfg := s.client.ClientConfig()
	_ = config.SaveEnvFile(".env", &savedCfg)

	s.AddLog("system", "Auth", "Admin password successfully changed")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Password updated successfully"})
}

// handleapibotstatus reads or sets the bot custom status message
func (s *Server) handleAPIBotStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": s.client.StatusMessage(),
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if err := s.client.SetStatus(req.Status); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Chatrooms", fmt.Sprintf("Bot status updated to: %s", req.Status))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": req.Status})
}

// handleapiantiafk gets or toggles anti-afk keepalive ticker
func (s *Server) handleAPIAntiAFK(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]bool{
			"anti_afk": s.client.AntiAFK(),
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	s.client.SetAntiAFK(req.Enabled)
	s.AddLog("system", "Chatrooms", fmt.Sprintf("Anti-AFK keepalive set to: %v", req.Enabled))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "anti_afk": req.Enabled})
}

// handleapijoinofficialrooms instructs bot to join all official chatrooms
func (s *Server) handleAPIJoinOfficialRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.client.JoinOfficialRooms(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Chatrooms", "Joining all official chatrooms")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Joined official chatrooms"})
}

// handleapijoinpublicrooms instructs bot to join all public chatrooms
func (s *Server) handleAPIJoinPublicRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.client.JoinPublicRooms(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Chatrooms", "Joining all public chatrooms")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Joined public chatrooms"})
}

// adminfileinfo represents a file entry in logs or data explorer
type AdminFileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  string `json:"size"`
	Bytes int64  `json:"bytes"`
	Date  string `json:"date"`
	IsLog bool   `json:"is_log"`
}

// formatbytesize formats byte count into human-readable string
func formatByteSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// sanitizeadminpath verifies path is safe inside data or logs
func sanitizeAdminPath(p string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(p))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", errors.New("invalid path traversal")
	}
	if !strings.HasPrefix(clean, "logs/") && !strings.HasPrefix(clean, "data/") && clean != "logs" && clean != "data" {
		return "", errors.New("access denied outside logs or data directory")
	}
	return clean, nil
}

// handleapiadminfiles lists security logs matching showdown-chatbot seclog
func (s *Server) handleAPIAdminFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_ = os.MkdirAll("logs", 0755)

	var result []AdminFileInfo
	entries, err := os.ReadDir("logs")
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			relPath := filepath.Join("logs", e.Name())

			// calculate size in kb matching showdown-chatbot
			kb := float64(info.Size()) / 1024.0
			sizeStr := fmt.Sprintf("%.2f KB", kb)

			// parse human-readable date matching showdown-chatbot (e.g. September 13, 2026)
			dateStr := info.ModTime().Format("January 02, 2006")
			parts := strings.Split(strings.TrimSuffix(e.Name(), ".log"), "_")
			if len(parts) == 4 && parts[0] == "seclog" {
				if t, err := time.Parse("2006_01_02", parts[1]+"_"+parts[2]+"_"+parts[3]); err == nil {
					dateStr = t.Format("January 02, 2006")
				}
			}

			result = append(result, AdminFileInfo{
				Name:  e.Name(),
				Path:  relPath,
				Size:  sizeStr,
				Bytes: info.Size(),
				Date:  dateStr,
				IsLog: true,
			})
		}
	}

	// sort most recent first matching showdown-chatbot
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name > result[j].Name
	})

	writeJSON(w, http.StatusOK, result)
}

// handleapiadminfileview displays file contents
func (s *Server) handleAPIAdminFileView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParam := r.URL.Query().Get("file")
	cleanPath, err := sanitizeAdminPath(pathParam)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    cleanPath,
		"content": string(data),
		"size":    len(data),
	})
}

// handleapiadminfiledownload streams the file for download
func (s *Server) handleAPIAdminFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParam := r.URL.Query().Get("file")
	cleanPath, err := sanitizeAdminPath(pathParam)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(cleanPath)))
	http.ServeFile(w, r, cleanPath)
}

// handleapiadminfileclear empties the selected file
func (s *Server) handleAPIAdminFileClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.File == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file parameter required"})
		return
	}

	cleanPath, err := sanitizeAdminPath(req.File)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var emptyContent []byte
	if strings.HasSuffix(cleanPath, ".json") {
		emptyContent = []byte("[]\n")
	} else {
		emptyContent = []byte("")
	}

	if err := os.WriteFile(cleanPath, emptyContent, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.AddLog("system", "Admin", fmt.Sprintf("Cleared file: %s", cleanPath))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "file cleared successfully"})
}

// handleapiadminreloaddata reloads teams, commands, blacklist, joinphrases, timers from disk
func (s *Server) handleAPIAdminReloadData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.DynamicCommands() != nil {
		_ = s.client.DynamicCommands().Load()
	}
	if s.client.Teams() != nil {
		_ = s.client.Teams().Load()
	}
	if s.client.Blacklist() != nil {
		_ = s.client.Blacklist().Load()
	}
	if s.client.JoinPhrases() != nil {
		_ = s.client.JoinPhrases().Load()
	}
	if s.client.Timers() != nil {
		_ = s.client.Timers().Load()
	}

	s.AddLog("system", "Admin", "All data files reloaded successfully from disk")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Data reloaded successfully"})
}

// handleapibothotpatch executes live hotpatch of commands and data stores
func (s *Server) handleAPIBotHotpatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.DynamicCommands() != nil {
		_ = s.client.DynamicCommands().Load()
	}
	if s.client.Teams() != nil {
		_ = s.client.Teams().Load()
	}
	if s.client.Blacklist() != nil {
		_ = s.client.Blacklist().Load()
	}
	if s.client.JoinPhrases() != nil {
		_ = s.client.JoinPhrases().Load()
	}
	if s.client.Timers() != nil {
		_ = s.client.Timers().Load()
	}

	s.AddLog("system", "Hotpatch", "Bot hotpatch complete: dynamic commands and database stores reloaded")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Bot data and dynamic commands successfully hotpatched"})
}

// handleapiadminclearcache empties runtime memory caches
func (s *Server) handleAPIAdminClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.ClearLogs()
	if s.client.History() != nil {
		_ = s.client.History().Clear()
	}

	s.AddLog("system", "Admin", "Runtime caches and battle history cleared")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Caches cleared successfully"})
}

// handleapiadminclearuserdata clears seen users tracking database
func (s *Server) handleAPIAdminClearUserData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.Seen() != nil {
		s.client.Seen().Clear()
	}

	s.AddLog("system", "Admin", "User seen tracking data cleared")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "User data cleared successfully"})
}

// handleapiadmineval executes javascript code safely using node runtime
func (s *Server) handleAPIAdminEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "javascript code is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	nodePath, err := exec.LookPath("node")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"output":  "node.js runtime not found on host",
		})
		return
	}

	cmd := exec.CommandContext(ctx, nodePath, "-e", req.Code)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	output := outBuf.String()
	if errBuf.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += errBuf.String()
	}
	if runErr != nil && output == "" {
		output = runErr.Error()
	}

	s.AddLog("system", "Eval", "Executed JavaScript evaluation")
	writeJSON(w, http.StatusOK, map[string]any{
		"success": runErr == nil,
		"output":  output,
	})
}

// handleapiusersseen returns recently seen users or lookup for a single user
func (s *Server) handleAPIUsersSeen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userParam := strings.TrimSpace(r.URL.Query().Get("user"))
	if s.client.Seen() == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	if userParam != "" {
		if entry, ok := s.client.Seen().Get(userParam); ok {
			writeJSON(w, http.StatusOK, entry)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found in seen database"})
		return
	}

	writeJSON(w, http.StatusOK, s.client.Seen().GetAll(100))
}

// handleapiusersseenclear wipes seen store
func (s *Server) handleAPIUsersSeenClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.Seen() != nil {
		s.client.Seen().Clear()
	}

	s.AddLog("system", "Chatrooms", "Seen database cleared")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Seen database cleared"})
}

// handleapicommandsaliases returns list of command aliases
func (s *Server) handleAPICommandsAliases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}

	writeJSON(w, http.StatusOK, s.client.DynamicCommands().ListAliases())
}

// handleapicommandsaliasessave creates or updates an alias
func (s *Server) handleAPICommandsAliasesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Alias  string `json:"alias"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commands store not initialized"})
		return
	}

	s.client.DynamicCommands().SetAlias(req.Alias, req.Target)
	s.AddLog("system", "Commands", fmt.Sprintf("Added command alias: %s -> %s", req.Alias, req.Target))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "alias saved successfully"})
}

// handleapicommandsaliasesdelete removes an alias
func (s *Server) handleAPICommandsAliasesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Alias) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "alias parameter is required"})
		return
	}

	if s.client.DynamicCommands() == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commands store not initialized"})
		return
	}

	s.client.DynamicCommands().DeleteAlias(req.Alias)
	s.AddLog("system", "Commands", fmt.Sprintf("Removed command alias: %s", req.Alias))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "alias deleted successfully"})
}

