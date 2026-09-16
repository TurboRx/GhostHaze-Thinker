package battle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OpponentActivePoke struct {
	Ident           string
	Species         string
	Types           []string
	HPPercent       float64
	Status          string
	Boosts          map[string]int
	Ability         string
	Item            string
	Moves           []string
	LockedMove      string
	ConfirmedFaster bool
	ConfirmedSlower bool
	Terastallized   string
}

type OpponentBenchPoke struct {
	Species string
	Fainted bool
	Moves   []string
	Ability string
	Item    string
	Status  string
}

type Battle struct {
	mu sync.RWMutex

	Room                  string
	MyPlayerID            string
	OpponentID            string
	OpponentName          string
	Gametype              string
	Tier                  string
	Turn                  int
	OpponentActive        OpponentActivePoke
	OpponentTeam          []OpponentBenchPoke
	OpponentHazards       map[string]bool
	OpponentHazardLayers  map[string]int
	MyHazards             map[string]int
	MyBoosts              map[string]int
	MyVolatiles           map[string]bool
	OpponentVolatiles     map[string]bool
	Weather               string
	Terrain               string
	ConsecutiveProtects   int
	LastMoveUsed          string
	Ended                 bool
	Winner                string
	LastRQID              int
	Engine                BattleEngine
	LastActivity          time.Time
	TimerActive                bool
	FirstMoverThisTurn         string
	MyActiveTerastallized      string
	OpponentSwitchedThisTurn   bool
	OpponentSwitchHazardDamage bool
}

func NewBattle(room string, engine BattleEngine) *Battle {
	if engine == nil {
		engine = NewDefaultEngine()
	}
	return &Battle{
		Room:                 room,
		Gametype:             "singles",
		OpponentHazards:      make(map[string]bool),
		OpponentHazardLayers: make(map[string]int),
		MyHazards:            make(map[string]int),
		MyBoosts:             make(map[string]int),
		MyVolatiles:          make(map[string]bool),
		OpponentVolatiles:    make(map[string]bool),
		OpponentActive: OpponentActivePoke{
			HPPercent: 1.0,
			Boosts:    make(map[string]int),
		},
		Engine:       engine,
		LastActivity: time.Now(),
	}
}

// setantipredictability enables or disables anti-predictability on the battle engine.
func (b *Battle) SetAntiPredictability(enable bool) {
	if def, ok := b.Engine.(*DefaultEngine); ok && def != nil {
		def.SetAntiPredictability(enable)
	} else if mm, ok := b.Engine.(*MinimaxEngine); ok && mm != nil {
		mm.SetAntiPredictability(enable)
	}
}

func (b *Battle) OpponentHasHazard(hazard string) bool {
	return b.OpponentHazards[hazard]
}

func (b *Battle) OpponentAliveCount() int {
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
	b.LastActivity = time.Now()

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
				b.FirstMoverThisTurn = ""
			}
		}
		// deduce heavy-duty boots if opponent switched in with active stealth rock and took no damage
		if b.OpponentSwitchedThisTurn {
			hasRocks := b.OpponentHazardLayers["stealthrock"] > 0 || b.OpponentHazards["stealthrock"]
			if hasRocks && b.OpponentActive.Species != "" && b.OpponentActive.Item == "" {
				if cleanID(b.OpponentActive.Ability) != "magicguard" && !b.OpponentSwitchHazardDamage {
					b.OpponentActive.Item = "heavydutyboots"
					b.updateOpponentBenchItem(b.OpponentActive.Species, "heavydutyboots")
				}
			}
			b.OpponentSwitchedThisTurn = false
			b.OpponentSwitchHazardDamage = false
		}

	case "poke":
		// team preview poke broadcast e.g. |poke|p2|garchomp, l80, m|item
		if len(parts) >= 3 {
			playerID := parts[1]
			if b.OpponentID != "" && playerID == b.OpponentID {
				species, _, _ := ParsePokemonDetails(parts[2])
				b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
					Species: species,
				})
			}
		}

	case "switch", "drag", "replace":
		// e.g. |switch|p2a: garchomp|garchomp, l80, m|100/100
		// e.g. |replace|p2a: zoroark|zoroark, l80, m|100/100
		if len(parts) >= 4 {
			ident := parts[1]
			isOpp := b.isOpponentIdent(ident)
			if isOpp {
				b.OpponentSwitchedThisTurn = true
				b.OpponentSwitchHazardDamage = false
				// persist outgoing active pokemon's revealed data into bench list
				if b.OpponentActive.Species != "" {
					persisted := false
					for i := range b.OpponentTeam {
						if strings.EqualFold(b.OpponentTeam[i].Species, b.OpponentActive.Species) {
							b.OpponentTeam[i].Moves = b.OpponentActive.Moves
							b.OpponentTeam[i].Ability = b.OpponentActive.Ability
							b.OpponentTeam[i].Item = b.OpponentActive.Item
							b.OpponentTeam[i].Status = b.OpponentActive.Status
							persisted = true
							break
						}
					}
					if !persisted {
						b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
							Species: b.OpponentActive.Species,
							Moves:   b.OpponentActive.Moves,
							Ability: b.OpponentActive.Ability,
							Item:    b.OpponentActive.Item,
							Status:  b.OpponentActive.Status,
						})
					}
				}

				details := parts[2]
				condition := parts[3]
				species, _, _ := ParsePokemonDetails(details)
				hp := parseHPPercent(condition)
				status := parseStatus(condition)
				types := GetSpeciesTypes(species)

				// restore any previously known moves, ability, or item for incoming species
				var rememberedMoves []string
				rememberedAbility := ""
				rememberedItem := ""
				for _, p := range b.OpponentTeam {
					if strings.EqualFold(p.Species, species) {
						rememberedMoves = p.Moves
						rememberedAbility = p.Ability
						rememberedItem = p.Item
						if status == "" && p.Status != "" {
							status = p.Status
						}
						break
					}
				}

				b.OpponentActive = OpponentActivePoke{
					Ident:     ident,
					Species:   species,
					Types:     types,
					HPPercent: hp,
					Status:    status,
					Boosts:    make(map[string]int),
					Moves:     rememberedMoves,
					Ability:   rememberedAbility,
					Item:      rememberedItem,
				}
				b.OpponentVolatiles = make(map[string]bool)
			} else {
				// reset active boosts and volatiles on our switch
				b.MyBoosts = make(map[string]int)
				b.MyVolatiles = make(map[string]bool)
				b.ConsecutiveProtects = 0
			}
		}

	case "-damage", "-heal":
		// e.g. |-damage|p2a: garchomp|45/100|[from] item: life orb
		// e.g. |-heal|p2a: garchomp|51/100|[from] item: leftovers
		// e.g. |-damage|p1a: urshifu|84/100|[from] item: rocky helmet|[of] p2a: toxapex
		if len(parts) >= 3 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				condition := parts[2]
				b.OpponentActive.HPPercent = parseHPPercent(condition)
				b.OpponentActive.Status = parseStatus(condition)
			}
			// extract item if revealed via [from] item: ...
			for i := 3; i < len(parts); i++ {
				lowerPart := strings.ToLower(parts[i])
				if strings.HasPrefix(lowerPart, "[from] item:") {
					itemName := strings.TrimSpace(strings.TrimPrefix(lowerPart, "[from] item:"))
					itemClean := cleanID(itemName)
					holderIsOpponent := b.isOpponentIdent(ident)
					for j := 3; j < len(parts); j++ {
						if strings.HasPrefix(strings.ToLower(parts[j]), "[of] ") {
							ofIdent := strings.TrimSpace(strings.TrimPrefix(parts[j], "[of] "))
							holderIsOpponent = b.isOpponentIdent(ofIdent)
						}
					}
					if holderIsOpponent {
						b.OpponentActive.Item = itemClean
						b.updateOpponentBenchItem(b.OpponentActive.Species, itemClean)
						if (itemClean == "choicescarf" || itemClean == "choiceband" || itemClean == "choicespecs") && len(b.OpponentActive.Moves) > 0 {
							b.OpponentActive.LockedMove = b.OpponentActive.Moves[len(b.OpponentActive.Moves)-1]
						}
					}
				}
				if strings.Contains(lowerPart, "stealth rock") && b.isOpponentIdent(ident) {
					b.OpponentSwitchHazardDamage = true
				}
			}
		}

	case "-status":
		// e.g. |-status|p2a: garchomp|brn
		// e.g. |-status|p2a: gliscor|tox|[from] item: toxic orb
		if len(parts) >= 3 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.Status = parts[2]
				for i := 3; i < len(parts); i++ {
					lower := strings.ToLower(parts[i])
					if strings.HasPrefix(lower, "[from] item:") {
						it := cleanID(strings.TrimPrefix(lower, "[from] item:"))
						b.OpponentActive.Item = it
						b.updateOpponentBenchItem(b.OpponentActive.Species, it)
					}
				}
			}
		}

	case "-curestatus":
		if len(parts) >= 2 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.Status = ""
			}
		}

	case "-cureteam":
		// e.g. |-cureteam|p2a: blissey|[from] move: heal bell
		if len(parts) >= 2 {
			if b.isOpponentIdent(parts[1]) {
				b.OpponentActive.Status = ""
				for i := range b.OpponentTeam {
					b.OpponentTeam[i].Status = ""
				}
			}
		}

	case "-boost":
		if len(parts) >= 4 {
			stat := parts[2]
			if amount, err := strconv.Atoi(parts[3]); err == nil {
				if b.isOpponentIdent(parts[1]) {
					if b.OpponentActive.Boosts == nil {
						b.OpponentActive.Boosts = make(map[string]int)
					}
					b.OpponentActive.Boosts[stat] += amount
				} else {
					if b.MyBoosts == nil {
						b.MyBoosts = make(map[string]int)
					}
					b.MyBoosts[stat] += amount
				}
			}
		}

	case "-unboost":
		if len(parts) >= 4 {
			stat := parts[2]
			if amount, err := strconv.Atoi(parts[3]); err == nil {
				if b.isOpponentIdent(parts[1]) {
					if b.OpponentActive.Boosts == nil {
						b.OpponentActive.Boosts = make(map[string]int)
					}
					b.OpponentActive.Boosts[stat] -= amount
				} else {
					if b.MyBoosts == nil {
						b.MyBoosts = make(map[string]int)
					}
					b.MyBoosts[stat] -= amount
				}
			}
		}

	case "-setboost":
		// e.g. |-setboost|p2a: azumarill|atk|6
		if len(parts) >= 4 {
			stat := parts[2]
			if amount, err := strconv.Atoi(parts[3]); err == nil {
				if b.isOpponentIdent(parts[1]) {
					if b.OpponentActive.Boosts == nil {
						b.OpponentActive.Boosts = make(map[string]int)
					}
					b.OpponentActive.Boosts[stat] = amount
				} else {
					if b.MyBoosts == nil {
						b.MyBoosts = make(map[string]int)
					}
					b.MyBoosts[stat] = amount
				}
			}
		}

	case "-clearboost":
		if len(parts) >= 2 {
			if b.isOpponentIdent(parts[1]) {
				b.OpponentActive.Boosts = make(map[string]int)
			} else {
				b.MyBoosts = make(map[string]int)
			}
		}

	case "-clearallboost":
		b.OpponentActive.Boosts = make(map[string]int)
		b.MyBoosts = make(map[string]int)

	case "-clearnegativeboost":
		// e.g. |-clearnegativeboost|p2a: landorus|[from] item: white herb
		if len(parts) >= 2 {
			if b.isOpponentIdent(parts[1]) {
				for k, v := range b.OpponentActive.Boosts {
					if v < 0 {
						delete(b.OpponentActive.Boosts, k)
					}
				}
			} else {
				for k, v := range b.MyBoosts {
					if v < 0 {
						delete(b.MyBoosts, k)
					}
				}
			}
		}

	case "-sidestart":
		// e.g. |-sidestart|p2: username|move: stealth rock
		if len(parts) >= 3 {
			effect := strings.ToLower(parts[2])
			isOpp := b.isOpponentIdent(parts[1])
			hazard := ""
			switch {
			case strings.Contains(effect, "stealth rock"):
				hazard = "stealthrock"
			case strings.Contains(effect, "toxic spikes"):
				hazard = "toxicspikes"
			case strings.Contains(effect, "spikes"):
				hazard = "spikes"
			case strings.Contains(effect, "sticky web"):
				hazard = "stickyweb"
			}
			if hazard != "" {
				if isOpp {
					b.OpponentHazards[hazard] = true
					if b.OpponentHazardLayers == nil {
						b.OpponentHazardLayers = make(map[string]int)
					}
					b.OpponentHazardLayers[hazard]++
				} else {
					if b.MyHazards == nil {
						b.MyHazards = make(map[string]int)
					}
					b.MyHazards[hazard]++
				}
			}
		}

	case "-sideend":
		if len(parts) >= 3 {
			effect := strings.ToLower(parts[2])
			isOpp := b.isOpponentIdent(parts[1])
			hazard := ""
			switch {
			case strings.Contains(effect, "stealth rock"):
				hazard = "stealthrock"
			case strings.Contains(effect, "toxic spikes"):
				hazard = "toxicspikes"
			case strings.Contains(effect, "spikes"):
				hazard = "spikes"
			case strings.Contains(effect, "sticky web"):
				hazard = "stickyweb"
			}
			if hazard != "" {
				if isOpp {
					delete(b.OpponentHazards, hazard)
					if b.OpponentHazardLayers != nil {
						delete(b.OpponentHazardLayers, hazard)
					}
				} else if b.MyHazards != nil {
					delete(b.MyHazards, hazard)
				}
			}
		}

	case "-weather":
		// e.g. |-weather|raindance|[from] ability: drizzle
		if len(parts) >= 2 {
			w := cleanID(parts[1])
			if w == "none" || w == "clear" {
				b.Weather = ""
			} else {
				b.Weather = w
			}
		}

	case "-fieldstart":
		// e.g. |-fieldstart|move: electric terrain
		if len(parts) >= 2 {
			f := cleanID(parts[1])
			switch {
			case strings.Contains(f, "electric"):
				b.Terrain = "electricterrain"
			case strings.Contains(f, "grassy"):
				b.Terrain = "grassyterrain"
			case strings.Contains(f, "psychic"):
				b.Terrain = "psychicterrain"
			case strings.Contains(f, "misty"):
				b.Terrain = "mistyterrain"
			}
		}

	case "-fieldend":
		b.Terrain = ""

	case "-start":
		// e.g. |-start|p1a: garchomp|substitute
		if len(parts) >= 3 {
			isOpp := b.isOpponentIdent(parts[1])
			effect := cleanID(parts[2])
			if isOpp {
				if b.OpponentVolatiles == nil {
					b.OpponentVolatiles = make(map[string]bool)
				}
				b.OpponentVolatiles[effect] = true
			} else {
				if b.MyVolatiles == nil {
					b.MyVolatiles = make(map[string]bool)
				}
				b.MyVolatiles[effect] = true
			}
		}

	case "-end":
		if len(parts) >= 3 {
			isOpp := b.isOpponentIdent(parts[1])
			effect := cleanID(parts[2])
			if isOpp && b.OpponentVolatiles != nil {
				delete(b.OpponentVolatiles, effect)
			} else if !isOpp && b.MyVolatiles != nil {
				delete(b.MyVolatiles, effect)
			}
		}

	case "move":
		// e.g. |move|p2a: garchomp|earthquake|p1a: blastoise
		if len(parts) >= 3 {
			moveID := cleanID(parts[2])
			mData := GetMoveData(moveID)
			// deduce speed tier relationship when first neutral priority attack executes
			if b.FirstMoverThisTurn == "" && mData.Priority == 0 {
				b.FirstMoverThisTurn = parts[1]
				if b.isOpponentIdent(parts[1]) {
					b.OpponentActive.ConfirmedFaster = true
					b.OpponentActive.ConfirmedSlower = false
				} else {
					b.OpponentActive.ConfirmedFaster = false
					b.OpponentActive.ConfirmedSlower = true
				}
			}

			if b.isOpponentIdent(parts[1]) {
				exists := false
				for _, m := range b.OpponentActive.Moves {
					if m == moveID {
						exists = true
						break
					}
				}
				if !exists {
					b.OpponentActive.Moves = append(b.OpponentActive.Moves, moveID)
				}
				// lock opponent into move if holding a choice item
				itemClean := cleanID(b.OpponentActive.Item)
				if itemClean == "choicescarf" || itemClean == "choiceband" || itemClean == "choicespecs" {
					b.OpponentActive.LockedMove = moveID
				}
			} else {
				// our move
				b.LastMoveUsed = moveID
				if moveID == "protect" || moveID == "detect" || moveID == "spikyshield" || moveID == "banefulbunker" || moveID == "kingsshield" || moveID == "silktrap" || moveID == "burningbulwark" {
					b.ConsecutiveProtects++
				} else {
					b.ConsecutiveProtects = 0
				}
			}
		}

	case "-ability":
		// e.g. |-ability|p2a: rotom|levitate
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			ab := cleanID(parts[2])
			b.OpponentActive.Ability = ab
			b.updateOpponentBenchAbility(b.OpponentActive.Species, ab)
		}

	case "-item":
		// e.g. |-item|p2a: garchomp|leftovers
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			if len(parts) >= 4 && strings.Contains(strings.ToLower(parts[3]), "knock off") {
				b.OpponentActive.Item = ""
				b.OpponentActive.LockedMove = ""
				b.updateOpponentBenchItem(b.OpponentActive.Species, "")
			} else {
				itemClean := cleanID(parts[2])
				b.OpponentActive.Item = itemClean
				b.updateOpponentBenchItem(b.OpponentActive.Species, itemClean)
				if (itemClean == "choicescarf" || itemClean == "choiceband" || itemClean == "choicespecs") && len(b.OpponentActive.Moves) > 0 {
					b.OpponentActive.LockedMove = b.OpponentActive.Moves[len(b.OpponentActive.Moves)-1]
				}
			}
		}

	case "-enditem":
		// e.g. |-enditem|p2a: garchomp|choice band|[from] move: knock off
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			b.OpponentActive.Item = ""
			b.OpponentActive.LockedMove = ""
			b.updateOpponentBenchItem(b.OpponentActive.Species, "")
		}

	case "-activate":
		// e.g. |-activate|p1a: rotom|move: trick|[of] p2a: blissey
		// e.g. |-activate|p2a: iron valiant|ability: quark drive|[fromitem]
		if len(parts) >= 3 {
			ident := parts[1]
			effect := strings.ToLower(parts[2])
			if strings.Contains(effect, "trick") || strings.Contains(effect, "switcheroo") {
				b.OpponentActive.LockedMove = ""
			}
			if b.isOpponentIdent(ident) {
				if strings.HasPrefix(effect, "ability:") {
					ab := cleanID(strings.TrimPrefix(effect, "ability:"))
					b.OpponentActive.Ability = ab
					b.updateOpponentBenchAbility(b.OpponentActive.Species, ab)
				} else if strings.HasPrefix(effect, "item:") {
					it := cleanID(strings.TrimPrefix(effect, "item:"))
					b.OpponentActive.Item = it
					b.updateOpponentBenchItem(b.OpponentActive.Species, it)
				}
				for _, p := range parts[2:] {
					lowerP := strings.ToLower(p)
					if strings.Contains(lowerP, "[fromitem]") || strings.Contains(lowerP, "booster energy") {
						b.OpponentActive.Item = "boosterenergy"
						b.updateOpponentBenchItem(b.OpponentActive.Species, "boosterenergy")
					}
				}
			}
		}

	case "-terastallize":
		// e.g. |-terastallize|p2a: dragonite|normal
		if len(parts) >= 3 {
			ident := parts[1]
			teraType := strings.ToLower(cleanID(parts[2]))
			if b.isOpponentIdent(ident) {
				b.OpponentActive.Types = []string{teraType}
				b.OpponentActive.Terastallized = teraType
			} else {
				b.MyActiveTerastallized = teraType
			}
		}

	case "-formechange":
		// e.g. |-formechange|p2a: palafin|palafin-hero
		if len(parts) >= 3 && b.isOpponentIdent(parts[1]) {
			newSpecies, _, _ := ParsePokemonDetails(parts[2])
			if newSpecies != "" {
				b.OpponentActive.Species = newSpecies
				b.OpponentActive.Types = GetSpeciesTypes(newSpecies)
			}
		}

	case "faint":
		if len(parts) >= 2 {
			ident := parts[1]
			if b.isOpponentIdent(ident) {
				b.OpponentActive.HPPercent = 0.0
				found := false
				for i := range b.OpponentTeam {
					if strings.EqualFold(b.OpponentTeam[i].Species, b.OpponentActive.Species) {
						b.OpponentTeam[i].Fainted = true
						found = true
						break
					}
				}
				if !found && b.OpponentActive.Species != "" {
					b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
						Species: b.OpponentActive.Species,
						Fainted: true,
						Moves:   b.OpponentActive.Moves,
						Ability: b.OpponentActive.Ability,
						Item:    b.OpponentActive.Item,
					})
				}
			}
		}

	case "inactive":
		b.TimerActive = true

	case "inactiveoff":
		b.TimerActive = false

	case "win", "tie", "prematureend", "expire":
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
		if slot <= 1 {
			slot = 2
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
		maxHP, err2 := strconv.ParseFloat(slash[1], 64)
		if err1 == nil && err2 == nil && maxHP > 0 {
			return cur / maxHP
		}
	}
	return 1.0
}

func parseStatus(condition string) string {
	parts := strings.Split(condition, " ")
	if len(parts) > 1 {
		status := parts[1]
		if status == "fnt" {
			return ""
		}
		return status
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

// generation returns the pokemon generation number (1-9) parsed from tier, defaulting to 9
func (b *Battle) Generation() int {
	tier := strings.ToLower(b.Tier)
	for g := 1; g <= 9; g++ {
		prefix := fmt.Sprintf("[gen %d]", g)
		if strings.Contains(tier, prefix) || strings.Contains(tier, fmt.Sprintf("gen %d", g)) || strings.Contains(tier, fmt.Sprintf("gen%d", g)) {
			return g
		}
	}
	return 9
}

// israndombattle returns true if the battle format is a random battle variant
func (b *Battle) IsRandomBattle() bool {
	return strings.Contains(strings.ToLower(b.Tier), "random")
}

// isended returns whether the battle has concluded
func (b *Battle) IsEnded() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Ended
}

// winnername returns the recorded winner of the battle
func (b *Battle) WinnerName() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Winner
}

// lastactivitytime returns timestamp of last activity in battle
func (b *Battle) LastActivityTime() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.LastActivity
}

// istimeractive returns whether the battle timer has been activated
func (b *Battle) IsTimerActive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.TimerActive
}

// settimeractive updates the timer active status
func (b *Battle) SetTimerActive(active bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.TimerActive = active
}

// setended marks the battle as ended
func (b *Battle) SetEnded(ended bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Ended = ended
}

// updateopponentbenchitem records revealed or deduced item for an opponent species
func (b *Battle) updateOpponentBenchItem(species, item string) {
	if species == "" {
		return
	}
	for i := range b.OpponentTeam {
		if strings.EqualFold(b.OpponentTeam[i].Species, species) {
			b.OpponentTeam[i].Item = item
			return
		}
	}
	b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
		Species: species,
		Item:    item,
	})
}

// updateopponentbenchability records revealed or deduced ability for an opponent species
func (b *Battle) updateOpponentBenchAbility(species, ability string) {
	if species == "" {
		return
	}
	for i := range b.OpponentTeam {
		if strings.EqualFold(b.OpponentTeam[i].Species, species) {
			b.OpponentTeam[i].Ability = ability
			return
		}
	}
	b.OpponentTeam = append(b.OpponentTeam, OpponentBenchPoke{
		Species: species,
		Ability: ability,
	})
}


