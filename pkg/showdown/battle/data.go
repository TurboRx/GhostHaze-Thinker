package battle

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed data/moves.json
var embeddedMovesJSON []byte

//go:embed data/pokedex.json
var embeddedPokedexJSON []byte

//go:embed data/random_sets.json
var embeddedRandomSetsJSON []byte

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
	Abilities []string `json:"abilities"`
	TeraTypes []string `json:"teraTypes"`
}

// randombattlespeciesdata represents the random battle parameters for a species.
type RandomBattleSpeciesData struct {
	Level int                   `json:"level"`
	Sets  []RandomBattleRoleSet `json:"sets"`
}

var (
	embeddedMovesOnce      sync.Once
	embeddedMoves          map[string]MoveData
	embeddedPokedexOnce    sync.Once
	embeddedPokedex        map[string]PokedexEntry
	embeddedRandomSetsOnce sync.Once
	embeddedRandomSets     map[string]RandomBattleSpeciesData
)

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

// getembeddedrandomsets returns the parsed map of official random battle sets.
func getEmbeddedRandomSets() map[string]RandomBattleSpeciesData {
	embeddedRandomSetsOnce.Do(func() {
		embeddedRandomSets = make(map[string]RandomBattleSpeciesData)
		if len(embeddedRandomSetsJSON) > 0 {
			_ = json.Unmarshal(embeddedRandomSetsJSON, &embeddedRandomSets)
		}
	})
	return embeddedRandomSets
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

// getrandombattleset returns the official random battle sets for a given species.
func GetRandomBattleSet(species string) (RandomBattleSpeciesData, bool) {
	sets := getEmbeddedRandomSets()
	clean := cleanID(species)
	data, exists := sets[clean]
	return data, exists
}
