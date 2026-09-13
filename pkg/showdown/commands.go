package showdown

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// customcommand defines a dynamic user-configured command
type CustomCommand struct {
	Name      string    `json:"name"`
	Response  string    `json:"response"`
	MinRank   string    `json:"min_rank"`
	Rooms     []string  `json:"rooms"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// commandstore manages dynamic commands, aliases, and persistence
type CommandStore struct {
	mu       sync.RWMutex
	filePath string
	commands map[string]CustomCommand
	aliases  map[string]string
}

// newcommandstore creates a new dynamic command store
func NewCommandStore(filePath string) *CommandStore {
	store := &CommandStore{
		filePath: filePath,
		commands: make(map[string]CustomCommand),
		aliases:  make(map[string]string),
	}
	if filePath != "" {
		_ = store.Load()
	}
	return store
}

// load reads custom commands from disk
func (s *CommandStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var list []CustomCommand
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.commands = make(map[string]CustomCommand)
	for _, cmd := range list {
		cleanName := strings.ToLower(strings.TrimSpace(cmd.Name))
		if cleanName != "" {
			cmd.Name = cleanName
			s.commands[cleanName] = cmd
		}
	}
	return nil
}

// save writes custom commands to disk
func (s *CommandStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.filePath == "" {
		return nil
	}

	list := make([]CustomCommand, 0, len(s.commands))
	for _, cmd := range s.commands {
		list = append(list, cmd)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// list returns all configured dynamic commands
func (s *CommandStore) List() []CustomCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]CustomCommand, 0, len(s.commands))
	for _, cmd := range s.commands {
		res = append(res, cmd)
	}
	return res
}

// get returns a specific command by name or resolves an alias
func (s *CommandStore) Get(name string) (*CustomCommand, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clean := strings.ToLower(strings.TrimSpace(name))
	if target, ok := s.aliases[clean]; ok {
		clean = target
	}

	cmd, ok := s.commands[clean]
	if !ok {
		return nil, false
	}
	cpy := cmd
	return &cpy, true
}

// setalias maps a shortcut alias to a command name
func (s *CommandStore) SetAlias(alias, target string) {
	cleanAlias := strings.ToLower(strings.TrimSpace(alias))
	cleanAlias = strings.TrimPrefix(cleanAlias, ".")
	cleanAlias = strings.TrimPrefix(cleanAlias, "!")

	cleanTarget := strings.ToLower(strings.TrimSpace(target))
	cleanTarget = strings.TrimPrefix(cleanTarget, ".")
	cleanTarget = strings.TrimPrefix(cleanTarget, "!")

	if cleanAlias == "" || cleanTarget == "" {
		return
	}

	s.mu.Lock()
	s.aliases[cleanAlias] = cleanTarget
	s.mu.Unlock()
}

// getalias retrieves the target command for an alias
func (s *CommandStore) GetAlias(alias string) (string, bool) {
	cleanAlias := strings.ToLower(strings.TrimSpace(alias))
	cleanAlias = strings.TrimPrefix(cleanAlias, ".")
	cleanAlias = strings.TrimPrefix(cleanAlias, "!")

	s.mu.RLock()
	defer s.mu.RUnlock()
	target, ok := s.aliases[cleanAlias]
	return target, ok
}

// deletealias removes an existing alias
func (s *CommandStore) DeleteAlias(alias string) {
	cleanAlias := strings.ToLower(strings.TrimSpace(alias))
	cleanAlias = strings.TrimPrefix(cleanAlias, ".")
	cleanAlias = strings.TrimPrefix(cleanAlias, "!")

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.aliases, cleanAlias)
}

// listaliases returns a copy of all configured command aliases
func (s *CommandStore) ListAliases() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cpy := make(map[string]string, len(s.aliases))
	for k, v := range s.aliases {
		cpy[k] = v
	}
	return cpy
}

// addorupdate creates or edits a custom command
func (s *CommandStore) AddOrUpdate(cmd CustomCommand) error {
	cleanName := strings.ToLower(strings.TrimSpace(cmd.Name))
	cleanName = strings.TrimPrefix(cleanName, ".")
	cleanName = strings.TrimPrefix(cleanName, "!")
	if cleanName == "" {
		return errors.New("command name cannot be blank")
	}

	if strings.TrimSpace(cmd.Response) == "" {
		return errors.New("command response cannot be blank")
	}

	minRank := strings.TrimSpace(cmd.MinRank)
	if minRank == "" {
		minRank = "all"
	}

	var cleanRooms []string
	for _, r := range cmd.Rooms {
		tr := ToRoomID(r)
		if tr != "" {
			cleanRooms = append(cleanRooms, tr)
		}
	}

	s.mu.Lock()
	cmd.Name = cleanName
	cmd.MinRank = minRank
	cmd.Rooms = cleanRooms
	cmd.UpdatedAt = time.Now()
	s.commands[cleanName] = cmd
	s.mu.Unlock()

	return s.Save()
}

// delete removes a custom command
func (s *CommandStore) Delete(name string) bool {
	cleanName := strings.ToLower(strings.TrimSpace(name))
	s.mu.Lock()
	_, ok := s.commands[cleanName]
	if ok {
		delete(s.commands, cleanName)
	}
	s.mu.Unlock()

	if ok {
		_ = s.Save()
	}
	return ok
}

// toggle switches the enabled status of a command
func (s *CommandStore) Toggle(name string) (bool, error) {
	cleanName := strings.ToLower(strings.TrimSpace(name))
	s.mu.Lock()
	cmd, ok := s.commands[cleanName]
	if !ok {
		s.mu.Unlock()
		return false, errors.New("command not found")
	}
	cmd.Enabled = !cmd.Enabled
	cmd.UpdatedAt = time.Now()
	s.commands[cleanName] = cmd
	newEnabled := cmd.Enabled
	s.mu.Unlock()

	_ = s.Save()
	return newEnabled, nil
}

// ranklevel maps rank characters to numeric authority levels
func RankLevel(rank string) int {
	trimmed := strings.TrimSpace(rank)
	if len(trimmed) == 0 || trimmed == "all" || trimmed == "regular" {
		return 0
	}
	switch trimmed[0] {
	case '+':
		return 1 // voice
	case '%':
		return 2 // driver
	case '@':
		return 3 // mod
	case '*':
		return 3 // bot
	case '&':
		return 4 // leader
	case '#':
		return 5 // room owner
	case '~':
		return 6 // admin
	default:
		return 0
	}
}

// canexecute checks if user rank satisfies the command's minimum rank requirement
func CanExecute(userRank, requiredRank string) bool {
	return RankLevel(userRank) >= RankLevel(requiredRank)
}

// formatcommandresponse replaces placeholders with dynamic context
func FormatCommandResponse(template, user, bot, args string) string {
	res := template
	res = strings.ReplaceAll(res, "{user}", user)
	res = strings.ReplaceAll(res, "%USER%", user)
	res = strings.ReplaceAll(res, "{bot}", bot)
	res = strings.ReplaceAll(res, "%BOT%", bot)
	res = strings.ReplaceAll(res, "{args}", args)
	res = strings.ReplaceAll(res, "%ARGS%", args)
	return res
}
