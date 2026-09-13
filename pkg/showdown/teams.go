package showdown

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// battleteam represents a competitive team in the vault
type BattleTeam struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Format     string    `json:"format"`
	TeamRaw    string    `json:"team_raw"`
	TeamPacked string    `json:"team_packed"`
	Pokemon    []string  `json:"pokemon"`
	Active     bool      `json:"active"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// teamstore manages persistence and retrieval of battle teams
type TeamStore struct {
	mu       sync.RWMutex
	filePath string
	teams    map[string]BattleTeam
}

// newteamstore creates a new team store instance
func NewTeamStore(filePath string) *TeamStore {
	store := &TeamStore{
		filePath: filePath,
		teams:    make(map[string]BattleTeam),
	}
	if filePath != "" {
		_ = store.Load()
	}
	return store
}

// load reads teams from disk
func (s *TeamStore) Load() error {
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

	var list []BattleTeam
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.teams = make(map[string]BattleTeam)
	for _, t := range list {
		if t.ID != "" {
			s.teams[t.ID] = t
		}
	}
	return nil
}

// save writes teams to disk
func (s *TeamStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.filePath == "" {
		return nil
	}

	list := make([]BattleTeam, 0, len(s.teams))
	for _, t := range s.teams {
		list = append(list, t)
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

// list returns a copy of all teams
func (s *TeamStore) List() []BattleTeam {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]BattleTeam, 0, len(s.teams))
	for _, t := range s.teams {
		res = append(res, t)
	}
	return res
}

// get retrieves a team by id
func (s *TeamStore) Get(id string) (*BattleTeam, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.teams[id]
	if !ok {
		return nil, false
	}
	cpy := t
	return &cpy, true
}

// addorupdate stores a team
func (s *TeamStore) AddOrUpdate(team BattleTeam) error {
	if strings.TrimSpace(team.Name) == "" {
		return errors.New("team name cannot be blank")
	}
	if strings.TrimSpace(team.Format) == "" {
		return errors.New("format cannot be blank")
	}
	if strings.TrimSpace(team.TeamRaw) == "" && strings.TrimSpace(team.TeamPacked) == "" {
		return errors.New("team data cannot be blank")
	}

	pokes, packed, err := ParsePokepaste(team.TeamRaw)
	if err != nil {
		return fmt.Errorf("failed to parse team: %w", err)
	}

	s.mu.Lock()
	if team.ID == "" {
		team.ID = generateID()
	}
	team.Pokemon = pokes
	team.TeamPacked = packed
	team.UpdatedAt = time.Now()
	s.teams[team.ID] = team
	s.mu.Unlock()

	return s.Save()
}

// delete removes a team by id
func (s *TeamStore) Delete(id string) bool {
	s.mu.Lock()
	_, ok := s.teams[id]
	if ok {
		delete(s.teams, id)
	}
	s.mu.Unlock()

	if ok {
		_ = s.Save()
	}
	return ok
}

// toggle switches the active status of a team
func (s *TeamStore) Toggle(id string) (bool, error) {
	s.mu.Lock()
	t, ok := s.teams[id]
	if !ok {
		s.mu.Unlock()
		return false, errors.New("team not found")
	}
	t.Active = !t.Active
	t.UpdatedAt = time.Now()
	s.teams[id] = t
	newActive := t.Active
	s.mu.Unlock()

	_ = s.Save()
	return newActive, nil
}

// getteamforformat finds an active team matching the given format
func (s *TeamStore) GetTeamForFormat(format string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetID := ToID(format)
	var matches []string

	for _, t := range s.teams {
		if !t.Active {
			continue
		}
		if targetID != "" && ToID(t.Format) == targetID {
			matches = append(matches, t.TeamPacked)
		}
	}

	// if specific format has active teams, pick one (randomly if multiple)
	if len(matches) > 0 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(matches))))
		return matches[n.Int64()]
	}

	// fallback to any active team if no format-specific match exists
	var allActive []string
	for _, t := range s.teams {
		if t.Active && t.TeamPacked != "" {
			allActive = append(allActive, t.TeamPacked)
		}
	}
	if len(allActive) > 0 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(allActive))))
		return allActive[n.Int64()]
	}

	return ""
}

// parsepokepaste parses pokepaste export text into pokemon names and packed format
func ParsePokepaste(text string) ([]string, string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, "", errors.New("empty team text")
	}

	// check if text is already in showdown packed format
	if !strings.Contains(trimmed, "\n") && strings.Contains(trimmed, "|") {
		var pokes []string
		sets := strings.Split(trimmed, "]")
		for _, set := range sets {
			parts := strings.Split(set, "|")
			if len(parts) > 0 && parts[0] != "" {
				name := parts[0]
				if len(parts) > 1 && parts[1] != "" {
					name = parts[1]
				}
				pokes = append(pokes, name)
			}
		}
		return pokes, trimmed, nil
	}

	// parse standard showdown export / pokepaste format
	lines := strings.Split(trimmed, "\n")
	type pokeSet struct {
		name      string
		species   string
		gender    string
		item      string
		ability   string
		nature    string
		teraType  string
		level     string
		shiny     string
		evs       [6]string
		ivs       [6]string
		moves     []string
	}

	var teamSets []*pokeSet
	var current *pokeSet

	statMap := map[string]int{
		"hp": 0, "atk": 1, "def": 2, "spa": 3, "spd": 4, "spe": 5,
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || line == "---" {
			current = nil
			continue
		}

		if current == nil {
			current = &pokeSet{}
			teamSets = append(teamSets, current)

			// parse header line: [Nickname (Species)] [(Gender)] [@ Item]
			if idx := strings.LastIndex(line, " @ "); idx != -1 {
				current.item = strings.TrimSpace(line[idx+3:])
				line = strings.TrimSpace(line[:idx])
			}

			if strings.HasSuffix(line, " (M)") {
				current.gender = "M"
				line = strings.TrimSpace(strings.TrimSuffix(line, " (M)"))
			} else if strings.HasSuffix(line, " (F)") {
				current.gender = "F"
				line = strings.TrimSpace(strings.TrimSuffix(line, " (F)"))
			}

			if strings.HasSuffix(line, ")") {
				if parenIdx := strings.LastIndex(line, " ("); parenIdx != -1 {
					current.species = strings.TrimSuffix(line[parenIdx+2:], ")")
					current.name = strings.TrimSpace(line[:parenIdx])
				} else {
					current.species = line
					current.name = line
				}
			} else {
				current.species = line
				current.name = line
			}
			continue
		}

		// parse attributes
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "ability:"):
			current.ability = strings.TrimSpace(line[8:])
		case strings.HasPrefix(lower, "tera type:"):
			current.teraType = strings.TrimSpace(line[10:])
		case strings.HasPrefix(lower, "level:"):
			current.level = strings.TrimSpace(line[6:])
		case strings.HasPrefix(lower, "shiny:"):
			if strings.Contains(lower, "yes") {
				current.shiny = "S"
			}
		case strings.HasPrefix(lower, "evs:"):
			parts := strings.Split(line[4:], "/")
			for _, part := range parts {
				p := strings.TrimSpace(part)
				tokens := strings.Fields(p)
				if len(tokens) == 2 {
					val := tokens[0]
					stat := strings.ToLower(tokens[1])
					if idx, ok := statMap[stat]; ok {
						current.evs[idx] = val
					}
				}
			}
		case strings.HasPrefix(lower, "ivs:"):
			parts := strings.Split(line[4:], "/")
			for _, part := range parts {
				p := strings.TrimSpace(part)
				tokens := strings.Fields(p)
				if len(tokens) == 2 {
					val := tokens[0]
					stat := strings.ToLower(tokens[1])
					if idx, ok := statMap[stat]; ok {
						current.ivs[idx] = val
					}
				}
			}
		case strings.HasSuffix(lower, " nature"):
			current.nature = strings.TrimSpace(line[:len(line)-7])
		case strings.HasPrefix(line, "-"):
			move := strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if move != "" {
				current.moves = append(current.moves, move)
			}
		}
	}

	if len(teamSets) == 0 {
		return nil, "", errors.New("no pokemon found in team definition")
	}

	var pokes []string
	var packedSets []string

	for _, s := range teamSets {
		dispName := s.species
		if dispName == "" {
			dispName = s.name
		}
		if dispName != "" {
			pokes = append(pokes, dispName)
		}

		// showdown packed format:
		// name|species|item|ability|moves|nature|evs|gender|ivs|shiny|level|misc
		var sb strings.Builder

		// 1. name
		sb.WriteString(s.name)
		sb.WriteString("|")

		// 2. species (empty if identical to name)
		if ToID(s.name) != ToID(s.species) && s.species != "" {
			sb.WriteString(ToID(s.species))
		}
		sb.WriteString("|")

		// 3. item
		sb.WriteString(ToID(s.item))
		sb.WriteString("|")

		// 4. ability
		sb.WriteString(ToID(s.ability))
		sb.WriteString("|")

		// 5. moves
		var moveIDs []string
		for _, m := range s.moves {
			id := ToID(m)
			if id != "" {
				moveIDs = append(moveIDs, id)
			}
		}
		sb.WriteString(strings.Join(moveIDs, ","))
		sb.WriteString("|")

		// 6. nature
		sb.WriteString(s.nature)
		sb.WriteString("|")

		// 7. evs (hp,atk,def,spa,spd,spe)
		evsStr := strings.Join(s.evs[:], ",")
		if evsStr != ",,,,," {
			sb.WriteString(evsStr)
		}
		sb.WriteString("|")

		// 8. gender
		sb.WriteString(s.gender)
		sb.WriteString("|")

		// 9. ivs (hp,atk,def,spa,spd,spe)
		ivsStr := strings.Join(s.ivs[:], ",")
		if ivsStr != ",,,,," {
			sb.WriteString(ivsStr)
		}
		sb.WriteString("|")

		// 10. shiny
		sb.WriteString(s.shiny)
		sb.WriteString("|")

		// 11. level
		if s.level != "" && s.level != "100" {
			sb.WriteString(s.level)
		}
		sb.WriteString("|")

		// 12. misc: happiness,pokeball,hpType,gigantamax,dynamaxLevel,teraType
		if s.teraType != "" {
			sb.WriteString(",,,,," + s.teraType)
		}

		packedSets = append(packedSets, sb.String())
	}

	packed := strings.Join(packedSets, "]")
	return pokes, packed, nil
}

// generateid creates an 8-byte unique random hex id
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// inttostr is a safe integer to string conversion
func intToStr(i int) string {
	return strconv.Itoa(i)
}
