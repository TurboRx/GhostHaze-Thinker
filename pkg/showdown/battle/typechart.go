package battle

import (
	"strings"
)

// type effectiveness multipliers
const (
	EffImmune   = 0.0
	EffResist2  = 0.25
	EffResist   = 0.5
	EffNeutral  = 1.0
	EffSuper    = 2.0
	EffSuper2   = 4.0
)

// typechart maps [defendingtype][attackingtype] -> effectiveness multiplier
var typeChart = map[string]map[string]float64{
	"bug": {
		"fire": 2.0, "flying": 2.0, "rock": 2.0,
		"fighting": 0.5, "ground": 0.5, "grass": 0.5,
	},
	"dark": {
		"fighting": 2.0, "bug": 2.0, "fairy": 2.0,
		"ghost": 0.5, "dark": 0.5,
		"psychic": 0.0,
	},
	"dragon": {
		"ice": 2.0, "dragon": 2.0, "fairy": 2.0,
		"fire": 0.5, "water": 0.5, "grass": 0.5, "electric": 0.5,
	},
	"electric": {
		"ground": 2.0,
		"electric": 0.5, "flying": 0.5, "steel": 0.5,
	},
	"fairy": {
		"poison": 2.0, "steel": 2.0,
		"fighting": 0.5, "bug": 0.5, "dark": 0.5,
		"dragon": 0.0,
	},
	"fighting": {
		"flying": 2.0, "psychic": 2.0, "fairy": 2.0,
		"bug": 0.5, "rock": 0.5, "dark": 0.5,
	},
	"fire": {
		"water": 2.0, "ground": 2.0, "rock": 2.0,
		"fire": 0.5, "grass": 0.5, "ice": 0.5, "bug": 0.5, "steel": 0.5, "fairy": 0.5,
	},
	"flying": {
		"electric": 2.0, "ice": 2.0, "rock": 2.0,
		"grass": 0.5, "fighting": 0.5, "bug": 0.5,
		"ground": 0.0,
	},
	"ghost": {
		"ghost": 2.0, "dark": 2.0,
		"poison": 0.5, "bug": 0.5,
		"normal": 0.0, "fighting": 0.0,
	},
	"grass": {
		"fire": 2.0, "ice": 2.0, "poison": 2.0, "flying": 2.0, "bug": 2.0,
		"water": 0.5, "grass": 0.5, "electric": 0.5, "ground": 0.5,
	},
	"ground": {
		"water": 2.0, "grass": 2.0, "ice": 2.0,
		"poison": 0.5, "rock": 0.5,
		"electric": 0.0,
	},
	"ice": {
		"fire": 2.0, "fighting": 2.0, "rock": 2.0, "steel": 2.0,
		"ice": 0.5,
	},
	"normal": {
		"fighting": 2.0,
		"ghost": 0.0,
	},
	"poison": {
		"ground": 2.0, "psychic": 2.0,
		"grass": 0.5, "fighting": 0.5, "poison": 0.5, "bug": 0.5, "fairy": 0.5,
	},
	"psychic": {
		"bug": 2.0, "ghost": 2.0, "dark": 2.0,
		"fighting": 0.5, "psychic": 0.5,
	},
	"rock": {
		"water": 2.0, "grass": 2.0, "fighting": 2.0, "ground": 2.0, "steel": 2.0,
		"normal": 0.5, "fire": 0.5, "poison": 0.5, "flying": 0.5,
	},
	"steel": {
		"fire": 2.0, "fighting": 2.0, "ground": 2.0,
		"normal": 0.5, "grass": 0.5, "ice": 0.5, "flying": 0.5, "psychic": 0.5, "bug": 0.5, "rock": 0.5, "dragon": 0.5, "steel": 0.5, "fairy": 0.5,
		"poison": 0.0,
	},
	"water": {
		"electric": 2.0, "grass": 2.0,
		"fire": 0.5, "water": 0.5, "ice": 0.5, "steel": 0.5,
	},
}

// geteffectiveness returns the damage multiplier of an attacking type against a defending type.
func GetEffectiveness(attackType, defendType string) float64 {
	atk := strings.ToLower(strings.TrimSpace(attackType))
	def := strings.ToLower(strings.TrimSpace(defendType))

	if defChart, exists := typeChart[def]; exists {
		if mult, found := defChart[atk]; found {
			return mult
		}
	}
	return EffNeutral
}

// getmultipleeffectiveness returns the combined damage multiplier against up to two defending types.
func GetMultipleEffectiveness(attackType string, defTypes ...string) float64 {
	total := 1.0
	for _, dt := range defTypes {
		if dt == "" {
			continue
		}
		eff := GetEffectiveness(attackType, dt)
		if eff == 0.0 {
			return 0.0
		}
		total *= eff
	}
	return total
}

// speciestypes maps standard pokemon species names (lowercased) to their primary and secondary types.
var speciesTypes = map[string][]string{
	// gen 9 meta
	"koraidon":      {"fighting", "dragon"},
	"miraidon":      {"electric", "dragon"},
	"fluttermane":   {"ghost", "fairy"},
	"greattusk":     {"ground", "fighting"},
	"ironvaliant":   {"fairy", "fighting"},
	"ironbundle":    {"ice", "water"},
	"irontreads":    {"ground", "steel"},
	"ironmoth":      {"fire", "poison"},
	"ironhands":     {"fighting", "electric"},
	"roaringmoon":   {"dragon", "dark"},
	"tinglu":        {"dark", "ground"},
	"chienpao":      {"dark", "ice"},
	"chiyu":         {"dark", "fire"},
	"wochien":       {"dark", "grass"},
	"gholdengo":     {"steel", "ghost"},
	"kingambit":     {"dark", "steel"},
	"meowscarada":   {"grass", "dark"},
	"skeledirge":    {"fire", "ghost"},
	"quaquaval":     {"water", "fighting"},
	"annihilape":    {"fighting", "ghost"},
	"baxcalibur":    {"dragon", "ice"},
	"dondozo":       {"water"},
	"tatsugiri":     {"dragon", "water"},
	"ogerpon":       {"grass"},
	"ogerponwellspring": {"grass", "water"},
	"ogerponhearthflame": {"grass", "fire"},
	"ogerponcornerstone": {"grass", "rock"},
	"ursalunabloodmoon": {"ground", "normal"},
	"archaludon":    {"steel", "dragon"},
	"walkingwake":   {"water", "dragon"},
	"gougingfire":   {"fire", "dragon"},
	"ragingbolt":    {"electric", "dragon"},

	// classics & staples
	"pikachu":       {"electric"},
	"raichu":        {"electric"},
	"charizard":     {"fire", "flying"},
	"blastoise":     {"water"},
	"venusaur":      {"grass", "poison"},
	"gengar":        {"ghost", "poison"},
	"dragonite":     {"dragon", "flying"},
	"mewtwo":        {"psychic"},
	"mew":           {"psychic"},
	"tyranitar":     {"rock", "dark"},
	"zapdos":        {"electric", "flying"},
	"moltres":       {"fire", "flying"},
	"articuno":      {"ice", "flying"},
	"gyarados":      {"water", "flying"},
	"alakazam":      {"psychic"},
	"machamp":       {"fighting"},
	"snorlax":       {"normal"},
	"scizor":        {"bug", "steel"},
	"skarmory":      {"steel", "flying"},
	"blissey":       {"normal"},
	"swampert":      {"water", "ground"},
	"salamence":     {"dragon", "flying"},
	"metagross":     {"steel", "psychic"},
	"kyogre":        {"water"},
	"groudon":       {"ground"},
	"rayquaza":      {"dragon", "flying"},
	"jirachi":       {"steel", "psychic"},
	"garchomp":      {"dragon", "ground"},
	"lucario":       {"fighting", "steel"},
	"togekiss":      {"fairy", "flying"},
	"heatran":       {"fire", "steel"},
	"rotom":         {"electric", "ghost"},
	"rotomwash":     {"electric", "water"},
	"rotomheat":     {"electric", "fire"},
	"rotomfrost":    {"electric", "ice"},
	"rotomfan":      {"electric", "flying"},
	"rotommow":      {"electric", "grass"},
	"weavile":       {"dark", "ice"},
	"gliscor":       {"ground", "flying"},
	"dialga":        {"steel", "dragon"},
	"palkia":        {"water", "dragon"},
	"giratina":      {"ghost", "dragon"},
	"darkrai":       {"dark"},
	"excadrill":     {"ground", "steel"},
	"ferrothorn":    {"grass", "steel"},
	"volcarona":     {"bug", "fire"},
	"landorus":      {"ground", "flying"},
	"landorustherian": {"ground", "flying"},
	"thundurus":     {"electric", "flying"},
	"tornadus":      {"flying"},
	"greninja":      {"water", "dark"},
	"aegislash":     {"steel", "ghost"},
	"talonflame":    {"fire", "flying"},
	"clefable":      {"fairy"},
	"toxapex":       {"poison", "water"},
	"mimikyu":       {"ghost", "fairy"},
	"corviknight":   {"flying", "steel"},
	"dragapult":     {"dragon", "ghost"},
	"rillaboom":     {"grass"},
	"cinderace":     {"fire"},
	"urshifu":       {"fighting", "dark"},
	"urshifurapidstrike": {"fighting", "water"},
	"grimmsnarl":    {"dark", "fairy"},
	"hatterene":     {"psychic", "fairy"},
	"zacian":        {"fairy", "steel"},
	"zamazenta":     {"fighting", "steel"},
	"calyrex":       {"psychic", "grass"},
	"calyrexshadow": {"psychic", "ghost"},
	"calyrexice":    {"psychic", "ice"},
}

// getspeciestypes returns the known types of a species, or default normal if unknown.
func GetSpeciesTypes(species string) []string {
	clean := cleanID(species)
	// check embedded pokedex dataset first
	if entry, exists := getEmbeddedPokedex()[clean]; exists && len(entry.Types) > 0 {
		return entry.Types
	}
	// fallback to manual hardcoded definitions
	if types, exists := speciesTypes[clean]; exists {
		return types
	}
	return []string{"normal"}
}
