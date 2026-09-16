package battle

import (
	"strconv"
	"strings"
)

type BattleRequest struct {
	Active      []RequestActive `json:"active"`
	Side        RequestSide     `json:"side"`
	RQID        int             `json:"rqid"`
	TeamPreview bool            `json:"teamPreview"`
	ForceSwitch []bool          `json:"forceSwitch"`
	Wait        bool            `json:"wait"`
	NoCancel    bool            `json:"noCancel"`
}

type RequestActive struct {
	Moves           []RequestMove `json:"moves"`
	CanMegaEvo      bool          `json:"canMegaEvo"`
	CanUltraBurst   bool          `json:"canUltraBurst"`
	CanDynamax      bool          `json:"canDynamax"`
	CanTerastallize string        `json:"canTerastallize"`
	Trapped         bool          `json:"trapped"`
	MaybeTrapped    bool          `json:"maybeTrapped"`
}

type RequestMove struct {
	Move     string `json:"move"`
	ID       string `json:"id"`
	PP       int    `json:"pp"`
	MaxPP    int    `json:"maxpp"`
	Target   string `json:"target"`
	Disabled any    `json:"disabled"`
}

func (m RequestMove) IsDisabled() bool {
	if m.Disabled == nil {
		return false
	}
	switch v := m.Disabled.(type) {
	case bool:
		return v
	case string:
		return v != ""
	default:
		return true
	}
}

type RequestSide struct {
	Name    string           `json:"name"`
	ID      string           `json:"id"`
	Pokemon []RequestPokemon `json:"pokemon"`
}

type RequestPokemon struct {
	Ident         string         `json:"ident"`
	Details       string         `json:"details"`
	Condition     string         `json:"condition"`
	Active        bool           `json:"active"`
	Stats         map[string]int `json:"stats"`
	Moves         []string       `json:"moves"`
	BaseAbility   string         `json:"baseAbility"`
	Item          string         `json:"item"`
	Pokeball      string         `json:"pokeball"`
	Ability       string         `json:"ability"`
	TeraType      string         `json:"teraType"`
	Terastallized string         `json:"terastallized"`
}

func (p RequestPokemon) IsFainted() bool {
	return strings.Contains(p.Condition, "fnt") || strings.HasPrefix(p.Condition, "0/") || p.Condition == "0"
}

func (p RequestPokemon) Species() string {
	species, _, _ := ParsePokemonDetails(p.Details)
	return species
}

func (p RequestPokemon) HPPercent() float64 {
	if p.IsFainted() {
		return 0.0
	}
	parts := strings.Split(p.Condition, " ")
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

func (p RequestPokemon) Status() string {
	parts := strings.Split(p.Condition, " ")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

type BattleDecisionType string

const (
	DecisionMove   BattleDecisionType = "move"
	DecisionSwitch BattleDecisionType = "switch"
	DecisionTeam   BattleDecisionType = "team"
	DecisionPass   BattleDecisionType = "pass"
)

type BattleDecision struct {
	Type         BattleDecisionType
	Slot         int // 1-indexed for moves [1..4] and bench switches [1..6]
	Terastallize bool
	Mega         bool
	TeamOrder    string
}

func ParsePokemonDetails(details string) (species string, level int, gender string) {
	parts := strings.Split(details, ",")
	species = strings.TrimSpace(parts[0])
	level = 100
	gender = "N"
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "L") {
			if lvl, err := strconv.Atoi(part[1:]); err == nil {
				level = lvl
			}
		} else if part == "M" || part == "F" {
			gender = part
		}
	}
	return
}
