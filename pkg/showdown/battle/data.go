package battle

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed data/moves.json
var embeddedMovesJSON []byte

//go:embed data/pokedex.json
var embeddedPokedexJSON []byte

//go:embed data/random_sets.json
var embeddedRandomSetsJSON []byte

//go:embed data/formats_data.json
var embeddedFormatsJSON []byte

//go:embed data/abilities.json
var embeddedAbilitiesJSON []byte

// pokedexentry represents the core attributes of a pokemon species.
type PokedexEntry struct {
	Name      string         `json:"name"`
	Types     []string       `json:"types"`
	BaseStats map[string]int `json:"baseStats"`
}

// randombattleroleset represents a role set in official random battles.
type RandomBattleRoleSet struct {
	Role      string   `json:"role"`
	Movepool  []string `json:"movepool"`
	Abilities []string `json:"abilities,omitempty"`
	TeraTypes []string `json:"teraTypes,omitempty"`
}

// randombattlespeciesdata represents the random battle parameters for a species.
type RandomBattleSpeciesData struct {
	Level int                   `json:"level"`
	Sets  []RandomBattleRoleSet `json:"sets"`
}

// abilitydata represents ability information.
type AbilityData struct {
	Name   string  `json:"name"`
	Rating float64 `json:"rating"`
}

// speciesformatdata represents competitive tier metadata.
type SpeciesFormatData struct {
	Tier        string `json:"tier"`
	DoublesTier string `json:"doublesTier,omitempty"`
}

var (
	embeddedMovesOnce      sync.Once
	embeddedMoves          map[string]MoveData
	embeddedPokedexOnce    sync.Once
	embeddedPokedex        map[string]PokedexEntry
	embeddedRandomSetsOnce sync.Once
	embeddedMultiGenSets   map[string]map[string]RandomBattleSpeciesData
	embeddedFormatsOnce    sync.Once
	embeddedFormats        map[string]SpeciesFormatData
	embeddedAbilitiesOnce  sync.Once
	embeddedAbilities      map[string]AbilityData
)

var abilityImmunities = map[string]string{
	// ground immunities
	"levitate":   "ground",
	"eartheater": "ground",

	// fire immunities
	"flashfire":     "fire",
	"wellbakedbody": "fire",

	// electric immunities
	"voltabsorb":   "electric",
	"lightningrod": "electric",
	"motordrive":   "electric",

	// water immunities
	"waterabsorb": "water",
	"stormdrain":  "water",
	"dryskin":     "water",

	// grass immunities
	"sapsipper": "grass",
}

// getembeddedmoves returns the parsed map of all known moves.
func getEmbeddedMoves() map[string]MoveData {
	embeddedMovesOnce.Do(func() {
		embeddedMoves = make(map[string]MoveData)
		if len(embeddedMovesJSON) > 0 {
			_ = json.Unmarshal(embeddedMovesJSON, &embeddedMoves)
		}
	})
	return embeddedMoves
}

// getembeddedpokedex returns the parsed map of all known pokemon species.
func getEmbeddedPokedex() map[string]PokedexEntry {
	embeddedPokedexOnce.Do(func() {
		embeddedPokedex = make(map[string]PokedexEntry)
		if len(embeddedPokedexJSON) > 0 {
			_ = json.Unmarshal(embeddedPokedexJSON, &embeddedPokedex)
		}
	})
	return embeddedPokedex
}

// getembeddedrandomsets returns the parsed map of official random battle sets across all generations.
func getEmbeddedRandomSets() map[string]map[string]RandomBattleSpeciesData {
	embeddedRandomSetsOnce.Do(func() {
		embeddedMultiGenSets = make(map[string]map[string]RandomBattleSpeciesData)
		if len(embeddedRandomSetsJSON) == 0 {
			return
		}

		// try parsing as multi-gen map {"gen1": ..., "gen9": ...}
		var multiGen map[string]map[string]RandomBattleSpeciesData
		if err := json.Unmarshal(embeddedRandomSetsJSON, &multiGen); err == nil && len(multiGen) > 0 {
			// verify if keys look like genn
			for k := range multiGen {
				if strings.HasPrefix(k, "gen") {
					embeddedMultiGenSets = multiGen
					return
				}
			}
		}

		// fallback: parse as flat gen9 map {"venusaur": ...}
		var flat map[string]RandomBattleSpeciesData
		if err := json.Unmarshal(embeddedRandomSetsJSON, &flat); err == nil {
			embeddedMultiGenSets["gen9"] = flat
		}
	})
	return embeddedMultiGenSets
}

// getembeddedformats returns tier format classifications.
func getEmbeddedFormats() map[string]SpeciesFormatData {
	embeddedFormatsOnce.Do(func() {
		embeddedFormats = make(map[string]SpeciesFormatData)
		if len(embeddedFormatsJSON) > 0 {
			_ = json.Unmarshal(embeddedFormatsJSON, &embeddedFormats)
		}
	})
	return embeddedFormats
}

// getembeddedabilities returns ability data.
func getEmbeddedAbilities() map[string]AbilityData {
	embeddedAbilitiesOnce.Do(func() {
		embeddedAbilities = make(map[string]AbilityData)
		if len(embeddedAbilitiesJSON) > 0 {
			_ = json.Unmarshal(embeddedAbilitiesJSON, &embeddedAbilities)
		}
	})
	return embeddedAbilities
}

// cleanid converts a string to lowercase and removes non-alphanumeric characters.
func cleanID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// getspeciesbasestats returns the base stats for a given species, or standard defaults if not found.
func GetSpeciesBaseStats(species string) map[string]int {
	pokedex := getEmbeddedPokedex()
	clean := cleanID(species)
	if entry, exists := pokedex[clean]; exists && entry.BaseStats != nil {
		return entry.BaseStats
	}
	return map[string]int{
		"hp":  80,
		"atk": 80,
		"def": 80,
		"spa": 80,
		"spd": 80,
		"spe": 80,
	}
}

// getspeciespokedexentry returns the full pokedex entry for a given species if known.
func GetSpeciesPokedexEntry(species string) (PokedexEntry, bool) {
	pokedex := getEmbeddedPokedex()
	clean := cleanID(species)
	entry, exists := pokedex[clean]
	return entry, exists
}

// getgenrandombattleset returns the official random battle sets for a given generation and species.
func GetGenRandomBattleSet(gen int, species string) (RandomBattleSpeciesData, bool) {
	allGens := getEmbeddedRandomSets()
	clean := cleanID(species)
	if gen <= 0 {
		gen = 9
	}

	genKey := fmt.Sprintf("gen%d", gen)
	if genSets, ok := allGens[genKey]; ok {
		if data, exists := genSets[clean]; exists {
			return data, true
		}
	}

	// fallback to gen9 if requested generation set is not found
	if gen != 9 {
		if gen9Sets, ok := allGens["gen9"]; ok {
			if data, exists := gen9Sets[clean]; exists {
				return data, true
			}
		}
	}
	return RandomBattleSpeciesData{}, false
}

// getrandombattleset returns the official random battle sets for a given species (defaulting to gen 9).
func GetRandomBattleSet(species string) (RandomBattleSpeciesData, bool) {
	return GetGenRandomBattleSet(9, species)
}

// getspeciestier returns the competitive tier (e.g. ou, uu, ubers) for a species.
func GetSpeciesTier(species string) string {
	formats := getEmbeddedFormats()
	clean := cleanID(species)
	if data, exists := formats[clean]; exists && data.Tier != "" {
		return data.Tier
	}
	return "OU"
}

// getabilitydata returns parsed ability information if available.
func GetAbilityData(ability string) (AbilityData, bool) {
	abilities := getEmbeddedAbilities()
	clean := cleanID(ability)
	data, exists := abilities[clean]
	return data, exists
}

// isabilityimmune returns true if the specified ability grants complete immunity to the given move type.
func IsAbilityImmune(ability, moveType string) bool {
	clean := cleanID(ability)
	immuneType, exists := abilityImmunities[clean]
	if !exists {
		return false
	}
	return strings.EqualFold(immuneType, moveType)
}

// getrandomspecies returns a random species name from the pokedex.
func GetRandomSpecies() string {
	pokedex := getEmbeddedPokedex()
	for _, entry := range pokedex {
		if entry.Name != "" {
			return entry.Name
		}
	}
	return "Pikachu"
}

// getrandommovename returns a random move name from moves data.
func GetRandomMoveName() string {
	moves := getEmbeddedMoves()
	for _, m := range moves {
		if m.Name != "" {
			return m.Name
		}
	}
	return "Thunderbolt"
}
