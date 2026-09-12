package battle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type OpponentActivePoke struct {
	Ident     string
	Species   string
	Types     []string
	HPPercent float64
	Status    string
	Boosts    map[string]int
}

type OpponentBenchPoke struct {
	Species string
	Fainted bool
}

type Battle struct {
	mu sync.RWMutex

	Room            string
	MyPlayerID      string
	OpponentID      string
	OpponentName    string
	Gametype        string
	Tier            string
	Turn            int
	OpponentActive  OpponentActivePoke
	OpponentTeam    []OpponentBenchPoke
	OpponentHazards map[string]bool
	Ended           bool
	Winner          string
	LastRQID        int
	Engine          BattleEngine
}

func NewBattle(room string, engine BattleEngine) *Battle {
	if engine == nil {
		engine = NewDefaultEngine()
	}
	return &Battle{
		Room:            room,
		Gametype:        "singles",
		OpponentHazards: make(map[string]bool),
		OpponentActive: OpponentActivePoke{
			HPPercent: 1.0,
			Boosts:    make(map[string]int),
		},
		Engine: engine,
	}
}

func (b *Battle) OpponentHasHazard(hazard string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.OpponentHazards[hazard]
}

func (b *Battle) OpponentAliveCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.OpponentTeam) == 0 {
		return 6
	}
	count := 0
	for _, p := range b.OpponentTeam {
		if !p.Fainted {
			count++
		}
	}
	return count
}

func (b *Battle) HandleLine(parts []string, myUsername string) (choice string, shouldSend bool) {
	if len(parts) == 0 {
		return "", false
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	msgType := parts[0]

	switch msgType {
	case "player":
		if len(parts) >= 3 {
			playerID := parts[1]
			username := parts[2]
			if toID(username) == toID(myUsername) || strings.EqualFold(strings.TrimSpace(username), strings.TrimSpace(myUsername)) {
				b.MyPlayerID = playerID
				if playerID == "p1" {
					b.OpponentID = "p2"
				} else {
					b.OpponentID = "p1"
				}
			} else {
				b.OpponentName = username
				if b.OpponentID == "" {
					b.OpponentID = playerID
				}
			}
		}

	case "tier":
		if len(parts) > 1 {
			b.Tier = parts[1]
		}

	case "gametype":
		if len(parts) > 1 {
			b.Gametype = parts[1]
		}

	case "turn":
		if len(parts) > 1 {
			if turnNum, err := strconv.Atoi(parts[1]); err == nil {
				b.Turn = turnNum
			}
		}

	case "poke":
		// team preview poke broadcast e.g. |poke|p2|Garchomp, L80, M|item
		if len(parts) >= 3 {
			playerID := parts[1]
			if b.OpponentID != "" && playerID == b.OpponentID {
				species, _, _ := ParsePokemonDetails(parts[2])
				b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
					Species: species,
				})
			}
		}

	case "switch", "drag":
		// e.g. |switch|p2a: Garchomp|Garchomp, L80, M|100/100
		if len(parts) >= 4 {
			ident := parts[1]
			isOpp := b.isOpponentIdent(ident)
			if isOpp {
				details := parts[2]
				condition := parts[3]
				species, _, _ := ParsePokemonDetails(details)
				hp := parseHPPercent(condition)
				status := parseStatus(condition)
				types := GetSpeciesTypes(species)

				b.OpponentActive = OpponentActivePoke{
					Ident:     ident,
					Species:   species,
					Types:     types,
					HPPercent: hp,
					Status:    status,
					Boosts:    make(map[string]int),
				}
			}
		}

	case "-damage", "-heal":
		// e.g. |-damage|p2a: Garchomp|45/100
		if len(parts) >= 3 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				condition := parts[2]
				b.OpponentActive.HPPercent = parseHPPercent(condition)
				b.OpponentActive.Status = parseStatus(condition)
			}
		}

	case "-status":
		// e.g. |-status|p2a: Garchomp|brn
		if len(parts) >= 3 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.Status = parts[2]
			}
		}

	case "-curestatus":
		if len(parts) >= 2 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.Status = ""
			}
		}

	case "-boost":
		if len(parts) >= 4 && b.isOpponentIdent(parts[1]) {
			stat := parts[2]
			if amount, err := strconv.Atoi(parts[3]); err == nil {
				if b.OpponentActive.Boosts == nil {
					b.OpponentActive.Boosts = make(map[string]int)
				}
				b.OpponentActive.Boosts[stat] += amount
			}
		}

	case "-unboost":
		if len(parts) >= 4 && b.isOpponentIdent(parts[1]) {
			stat := parts[2]
			if amount, err := strconv.Atoi(parts[3]); err == nil {
				if b.OpponentActive.Boosts == nil {
					b.OpponentActive.Boosts = make(map[string]int)
				}
				b.OpponentActive.Boosts[stat] -= amount
			}
		}

	case "-sidestart":
		// e.g. |-sidestart|p2: username|move: Stealth Rock
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			effect := strings.ToLower(parts[2])
			if strings.Contains(effect, "stealth rock") {
				b.OpponentHazards["stealthrock"] = true
			} else if strings.Contains(effect, "spikes") {
				b.OpponentHazards["spikes"] = true
			} else if strings.Contains(effect, "toxic spikes") {
				b.OpponentHazards["toxicspikes"] = true
			} else if strings.Contains(effect, "sticky web") {
				b.OpponentHazards["stickyweb"] = true
			}
		}

	case "-sideend":
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			effect := strings.ToLower(parts[2])
			if strings.Contains(effect, "stealth rock") {
				delete(b.OpponentHazards, "stealthrock")
			} else if strings.Contains(effect, "spikes") {
				delete(b.OpponentHazards, "spikes")
			} else if strings.Contains(effect, "toxic spikes") {
				delete(b.OpponentHazards, "toxicspikes")
			} else if strings.Contains(effect, "sticky web") {
				delete(b.OpponentHazards, "stickyweb")
			}
		}

	case "faint":
		if len(parts) >= 2 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.HPPercent = 0.0
				for i := range b.OpponentTeam {
					if strings.EqualFold(b.OpponentTeam[i].Species, b.OpponentActive.Species) {
						b.OpponentTeam[i].Fainted = true
						break
					}
				}
			}
		}

	case "win", "tie":
		b.Ended = true
		if len(parts) > 1 {
			b.Winner = parts[1]
		}
		return "", false

	case "error":
		if len(parts) > 1 && strings.Contains(strings.ToLower(parts[1]), "invalid choice") {
			if b.LastRQID > 0 {
				return fmt.Sprintf("/choose default|%d", b.LastRQID), true
			}
			return "/choose default", true
		}

	case "request":
		if len(parts) > 1 {
			reqJSON := strings.Join(parts[1:], "|")
			if reqJSON != "" {
				var req BattleRequest
				if err := json.Unmarshal([]byte(reqJSON), &req); err == nil {
					b.LastRQID = req.RQID
					if req.Wait {
						return "", false
					}
					dec := b.Engine.Decide(b, req)
					choiceStr := b.formatDecision(dec, req.RQID)
					if choiceStr != "" {
						return choiceStr, true
					}
				}
			}
		}
	}

	return "", false
}

func (b *Battle) isOpponentIdent(ident string) bool {
	if b.OpponentID != "" && strings.HasPrefix(ident, b.OpponentID) {
		return true
	}
	if b.MyPlayerID != "" && !strings.HasPrefix(ident, b.MyPlayerID) {
		return true
	}
	return strings.HasPrefix(ident, "p2")
}

func (b *Battle) formatDecision(dec BattleDecision, rqid int) string {
	switch dec.Type {
	case DecisionTeam:
		order := dec.TeamOrder
		if order == "" {
			order = "123456"
		}
		return fmt.Sprintf("/choose team %s|%d", order, rqid)

	case DecisionSwitch:
		slot := dec.Slot
		if slot <= 0 {
			slot = 1
		}
		return fmt.Sprintf("/choose switch %d|%d", slot, rqid)

	case DecisionMove:
		teraSuffix := ""
		if dec.Terastallize {
			teraSuffix = " terastallize"
		}
		slot := dec.Slot
		if slot <= 0 {
			slot = 1
		}
		return fmt.Sprintf("/choose move %d%s|%d", slot, teraSuffix, rqid)

	case DecisionPass:
		return fmt.Sprintf("/choose pass|%d", rqid)
	}
	return ""
}

func parseHPPercent(condition string) float64 {
	if strings.Contains(condition, "fnt") || strings.HasPrefix(condition, "0/") || condition == "0" {
		return 0.0
	}
	parts := strings.Split(condition, " ")
	if len(parts) == 0 {
		return 1.0
	}
	slash := strings.Split(parts[0], "/")
	if len(slash) == 2 {
		cur, err1 := strconv.ParseFloat(slash[0], 64)
		max, err2 := strconv.ParseFloat(slash[1], 64)
		if err1 == nil && err2 == nil && max > 0 {
			return cur / max
		}
	}
	return 1.0
}

func parseStatus(condition string) string {
	parts := strings.Split(condition, " ")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func toID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
