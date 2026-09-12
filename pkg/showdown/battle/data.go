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

// pokedexentry represents the core attributes of a pokemon species.
type PokedexEntry struct {
	Name      string         `json:"name"`
	Types     []string       `json:"types"`
	BaseStats map[string]int `json:"baseStats"`
}

var (
	embeddedMovesOnce   sync.Once
	embeddedMoves       map[string]MoveData
	embeddedPokedexOnce sync.Once
	embeddedPokedex     map[string]PokedexEntry
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
