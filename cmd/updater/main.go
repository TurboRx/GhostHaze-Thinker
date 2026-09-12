package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	smogonBaseURL      = "https://raw.githubusercontent.com/smogon/pokemon-showdown/master/data"
	smogonPokedexURL   = smogonBaseURL + "/pokedex.ts"
	smogonMovesURL     = smogonBaseURL + "/moves.ts"
	smogonFormatsURL   = smogonBaseURL + "/formats-data.ts"
	smogonAbilitiesURL = smogonBaseURL + "/abilities.ts"
)

var healingNames = map[string]bool{
	"recover": true, "roost": true, "softboiled": true, "slackoff": true, "milkdrink": true,
	"shoreup": true, "wish": true, "moonlight": true, "synthesis": true, "morningsun": true,
	"healorder": true, "lifedew": true, "junglehealing": true, "strengthsap": true,
}

var hazardNames = map[string]bool{
	"stealthrock": true, "spikes": true, "toxicspikes": true, "stickyweb": true,
	"ceaselessedge": true, "stoneaxe": true,
}

var statusNames = map[string]bool{
	"thunderwave": true, "willowisp": true, "toxic": true, "spore": true, "sleeppowder": true,
	"hypnosis": true, "glare": true, "stunspore": true, "poisonpowder": true, "darkvoid": true,
	"yawn": true, "nuzzle": true,
}

type moveOutput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Category  string `json:"category"`
	BasePower int    `json:"basePower"`
	Accuracy  int    `json:"accuracy"`
	Priority  int    `json:"priority"`
	IsHealing bool   `json:"isHealing"`
	IsHazard  bool   `json:"isHazard"`
	IsSetup   bool   `json:"isSetup"`
	IsStatus  bool   `json:"isStatus"`
}

type pokedexOutput struct {
	Name      string         `json:"name"`
	Types     []string       `json:"types"`
	BaseStats map[string]int `json:"baseStats"`
}

type speciesFormatOutput struct {
	Tier        string `json:"tier"`
	DoublesTier string `json:"doublesTier,omitempty"`
}

type abilityOutput struct {
	Name   string  `json:"name"`
	Rating float64 `json:"rating"`
}

type randomBattleRoleSet struct {
	Role      string   `json:"role"`
	Movepool  []string `json:"movepool"`
	Abilities []string `json:"abilities,omitempty"`
	TeraTypes []string `json:"teraTypes,omitempty"`
}

type randomBattleSpeciesOutput struct {
	Level int                   `json:"level"`
	Sets  []randomBattleRoleSet `json:"sets"`
}

func cleanKey(s string) string {
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

func readSource(showdownDir, relPath, rawURL string) ([]byte, error) {
	if showdownDir != "" {
		localPath := filepath.Join(showdownDir, "data", relPath)
		if _, err := os.Stat(localPath); err == nil {
			fmt.Printf("reading %s from local checkout %s...\n", relPath, localPath)
			return os.ReadFile(localPath)
		}
	}

	fmt.Printf("fetching %s from %s...\n", relPath, rawURL)
	client := &http.Client{Timeout: 45 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GhostHaze-Thinker-Showdown-Updater)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status %d from %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

func updateMoves(showdownDir, destPath string) error {
	data, err := readSource(showdownDir, "moves.ts", smogonMovesURL)
	if err != nil {
		return err
	}

	content := string(data)
	blockRegex := regexp.MustCompile(`(?m)^\t("?([a-z0-9]+)"?):\s*\{\s*\n([\s\S]*?)\n\t\},`)
	nameRegex := regexp.MustCompile(`name:\s*"([^"]+)"`)
	catRegex := regexp.MustCompile(`category:\s*"([^"]+)"`)
	typeRegex := regexp.MustCompile(`type:\s*"([^"]+)"`)
	bpRegex := regexp.MustCompile(`basePower:\s*(\d+)`)
	accRegex := regexp.MustCompile(`accuracy:\s*(\d+|true|false)`)
	priRegex := regexp.MustCompile(`priority:\s*(-?\d+)`)
	sideConditionRegex := regexp.MustCompile(`sideCondition:\s*"([^"]+)"`)
	boostsRegex := regexp.MustCompile(`boosts:\s*\{([^}]+)\}`)
	statusRegex := regexp.MustCompile(`status:\s*"([^"]+)"`)

	matches := blockRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no move blocks found in smogon moves.ts")
	}

	processed := make(map[string]moveOutput, len(matches))
	for _, m := range matches {
		rawID := m[2]
		block := m[3]
		cleanID := cleanKey(rawID)

		nameMatch := nameRegex.FindStringSubmatch(block)
		name := rawID
		if len(nameMatch) > 1 {
			name = nameMatch[1]
		}

		cat := "Physical"
		if catMatch := catRegex.FindStringSubmatch(block); len(catMatch) > 1 {
			cat = strings.Title(strings.ToLower(catMatch[1]))
		}

		moveType := "normal"
		if typeMatch := typeRegex.FindStringSubmatch(block); len(typeMatch) > 1 {
			moveType = strings.ToLower(typeMatch[1])
		}

		basePower := 0
		if bpMatch := bpRegex.FindStringSubmatch(block); len(bpMatch) > 1 {
			basePower, _ = strconv.Atoi(bpMatch[1])
		}

		accuracy := 100
		if accMatch := accRegex.FindStringSubmatch(block); len(accMatch) > 1 {
			val := accMatch[1]
			if val == "true" {
				accuracy = 100
			} else if val == "false" {
				accuracy = 0
			} else {
				if num, err := strconv.Atoi(val); err == nil {
					accuracy = num
				}
			}
		}

		priority := 0
		if priMatch := priRegex.FindStringSubmatch(block); len(priMatch) > 1 {
			priority, _ = strconv.Atoi(priMatch[1])
		}

		isHealing := strings.Contains(block, "heal: 1") ||
			strings.Contains(block, "heal:") ||
			strings.Contains(block, "drain:") ||
			healingNames[cleanID]

		sideCond := ""
		if scMatch := sideConditionRegex.FindStringSubmatch(block); len(scMatch) > 1 {
			sideCond = strings.ToLower(scMatch[1])
		}
		isHazard := hazardNames[sideCond] || hazardNames[cleanID]

		isSetup := false
		if cat == "Status" {
			if boostsMatch := boostsRegex.FindStringSubmatch(block); len(boostsMatch) > 1 {
				if strings.Contains(boostsMatch[1], ": 1") ||
					strings.Contains(boostsMatch[1], ": 2") ||
					strings.Contains(boostsMatch[1], ": 3") {
					isSetup = true
				}
			}
		}

		isStatus := statusRegex.MatchString(block) || statusNames[cleanID]

		processed[cleanID] = moveOutput{
			ID:        cleanID,
			Name:      name,
			Type:      moveType,
			Category:  cat,
			BasePower: basePower,
			Accuracy:  accuracy,
			Priority:  priority,
			IsHealing: isHealing,
			IsHazard:  isHazard,
			IsSetup:   isSetup,
			IsStatus:  isStatus,
		}
	}

	outData, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d moves to %s\n", len(processed), destPath)
	return nil
}

func updatePokedex(showdownDir, destPath string) error {
	data, err := readSource(showdownDir, "pokedex.ts", smogonPokedexURL)
	if err != nil {
		return err
	}

	content := string(data)
	blockRegex := regexp.MustCompile(`(?m)^\t([a-z0-9]+):\s*\{\s*\n([\s\S]*?)\n\t\},`)
	nameRegex := regexp.MustCompile(`name:\s*"([^"]+)"`)
	typesRegex := regexp.MustCompile(`types:\s*\[([^\]]+)\]`)
	statsRegex := regexp.MustCompile(`baseStats:\s*\{\s*hp:\s*(\d+),\s*atk:\s*(\d+),\s*def:\s*(\d+),\s*spa:\s*(\d+),\s*spd:\s*(\d+),\s*spe:\s*(\d+)\s*\}`)

	matches := blockRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no pokemon blocks found in smogon pokedex.ts")
	}

	processed := make(map[string]pokedexOutput, len(matches))
	for _, m := range matches {
		rawID := m[1]
		block := m[2]
		cleanID := cleanKey(rawID)

		nameMatch := nameRegex.FindStringSubmatch(block)
		name := rawID
		if len(nameMatch) > 1 {
			name = nameMatch[1]
		}

		types := []string{"normal"}
		if typesMatch := typesRegex.FindStringSubmatch(block); len(typesMatch) > 1 {
			rawTypes := strings.Split(typesMatch[1], ",")
			types = make([]string, 0, len(rawTypes))
			for _, rt := range rawTypes {
				cleaned := strings.ToLower(strings.Trim(strings.TrimSpace(rt), `"`))
				if cleaned != "" {
					types = append(types, cleaned)
				}
			}
			if len(types) == 0 {
				types = []string{"normal"}
			}
		}

		baseStats := map[string]int{
			"hp": 80, "atk": 80, "def": 80, "spa": 80, "spd": 80, "spe": 80,
		}
		if statsMatch := statsRegex.FindStringSubmatch(block); len(statsMatch) > 6 {
			hp, _ := strconv.Atoi(statsMatch[1])
			atk, _ := strconv.Atoi(statsMatch[2])
			def, _ := strconv.Atoi(statsMatch[3])
			spa, _ := strconv.Atoi(statsMatch[4])
			spd, _ := strconv.Atoi(statsMatch[5])
			spe, _ := strconv.Atoi(statsMatch[6])
			baseStats = map[string]int{
				"hp": hp, "atk": atk, "def": def, "spa": spa, "spd": spd, "spe": spe,
			}
		}

		processed[cleanID] = pokedexOutput{
			Name:      name,
			Types:     types,
			BaseStats: baseStats,
		}
	}

	outData, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d pokemon species to %s\n", len(processed), destPath)
	return nil
}

func updateFormatsData(showdownDir, destPath string) error {
	data, err := readSource(showdownDir, "formats-data.ts", smogonFormatsURL)
	if err != nil {
		return err
	}

	content := string(data)
	blockRegex := regexp.MustCompile(`(?m)^\t([a-z0-9]+):\s*\{\s*\n([\s\S]*?)\n\t\},`)
	tierRegex := regexp.MustCompile(`tier:\s*"([^"]+)"`)
	doublesTierRegex := regexp.MustCompile(`doublesTier:\s*"([^"]+)"`)

	matches := blockRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no formats blocks found in formats-data.ts")
	}

	processed := make(map[string]speciesFormatOutput, len(matches))
	for _, m := range matches {
		rawID := m[1]
		block := m[2]
		cleanID := cleanKey(rawID)

		tier := "OU"
		if tierMatch := tierRegex.FindStringSubmatch(block); len(tierMatch) > 1 {
			tier = tierMatch[1]
		}

		doublesTier := ""
		if dtMatch := doublesTierRegex.FindStringSubmatch(block); len(dtMatch) > 1 {
			doublesTier = dtMatch[1]
		}

		processed[cleanID] = speciesFormatOutput{
			Tier:        tier,
			DoublesTier: doublesTier,
		}
	}

	outData, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d species formats tiers to %s\n", len(processed), destPath)
	return nil
}

func updateAbilities(showdownDir, destPath string) error {
	data, err := readSource(showdownDir, "abilities.ts", smogonAbilitiesURL)
	if err != nil {
		return err
	}

	content := string(data)
	blockRegex := regexp.MustCompile(`(?m)^\t([a-z0-9]+):\s*\{\s*\n([\s\S]*?)\n\t\},`)
	nameRegex := regexp.MustCompile(`name:\s*"([^"]+)"`)
	ratingRegex := regexp.MustCompile(`rating:\s*(-?\d+(\.\d+)?)`)

	matches := blockRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return fmt.Errorf("no ability blocks found in abilities.ts")
	}

	processed := make(map[string]abilityOutput, len(matches))
	for _, m := range matches {
		rawID := m[1]
		block := m[2]
		cleanID := cleanKey(rawID)

		name := rawID
		if nm := nameRegex.FindStringSubmatch(block); len(nm) > 1 {
			name = nm[1]
		}

		rating := 1.0
		if rm := ratingRegex.FindStringSubmatch(block); len(rm) > 1 {
			if parsed, err := strconv.ParseFloat(rm[1], 64); err == nil {
				rating = parsed
			}
		}

		processed[cleanID] = abilityOutput{
			Name:   name,
			Rating: rating,
		}
	}

	outData, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d abilities to %s\n", len(processed), destPath)
	return nil
}

func updateRandomSets(showdownDir, destPath string) error {
	allGens := make(map[string]map[string]randomBattleSpeciesOutput)

	// gen 1 parsing
	gen1Rel := filepath.Join("random-battles", "gen1", "data.json")
	gen1URL := smogonBaseURL + "/random-battles/gen1/data.json"
	if gen1Data, err := readSource(showdownDir, gen1Rel, gen1URL); err == nil {
		var rawGen1 map[string]struct {
			Level          int      `json:"level"`
			Moves          []string `json:"moves"`
			EssentialMoves []string `json:"essentialMoves"`
			ExclusiveMoves []string `json:"exclusiveMoves"`
			ComboMoves     []string `json:"comboMoves"`
		}
		if err := json.Unmarshal(gen1Data, &rawGen1); err == nil {
			gen1Processed := make(map[string]randomBattleSpeciesOutput, len(rawGen1))
			for species, val := range rawGen1 {
				cleanID := cleanKey(species)
				moveMap := make(map[string]bool)
				for _, m := range val.Moves {
					moveMap[m] = true
				}
				for _, m := range val.EssentialMoves {
					moveMap[m] = true
				}
				for _, m := range val.ExclusiveMoves {
					moveMap[m] = true
				}
				for _, m := range val.ComboMoves {
					moveMap[m] = true
				}
				var allMoves []string
				for m := range moveMap {
					allMoves = append(allMoves, m)
				}
				sort.Strings(allMoves)

				gen1Processed[cleanID] = randomBattleSpeciesOutput{
					Level: val.Level,
					Sets: []randomBattleRoleSet{
						{
							Role:     "All",
							Movepool: allMoves,
						},
					},
				}
			}
			allGens["gen1"] = gen1Processed
			fmt.Printf("loaded %d gen1 random sets\n", len(gen1Processed))
		}
	}

	// gen 2 through gen 9 parsing
	for g := 2; g <= 9; g++ {
		genKey := fmt.Sprintf("gen%d", g)
		relPath := filepath.Join("random-battles", genKey, "sets.json")
		rawURL := fmt.Sprintf("%s/random-battles/%s/sets.json", smogonBaseURL, genKey)

		data, err := readSource(showdownDir, relPath, rawURL)
		if err != nil {
			fmt.Printf("warning: could not load %s sets: %v\n", genKey, err)
			continue
		}

		var rawSets map[string]randomBattleSpeciesOutput
		if err := json.Unmarshal(data, &rawSets); err != nil {
			fmt.Printf("warning: failed to unmarshal %s sets: %v\n", genKey, err)
			continue
		}

		processed := make(map[string]randomBattleSpeciesOutput, len(rawSets))
		for species, set := range rawSets {
			cleanID := cleanKey(species)
			processed[cleanID] = set
		}
		allGens[genKey] = processed
		fmt.Printf("loaded %d %s random sets\n", len(processed), genKey)
	}

	outData, err := json.Marshal(allGens)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote multi-generation random sets (%d generations) to %s\n", len(allGens), destPath)
	return nil
}

func main() {
	showdownDir := flag.String("showdown-dir", "", "path to smogon/pokemon-showdown checkout")
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting cwd: %v\n", err)
		os.Exit(1)
	}

	dataDir := filepath.Join(cwd, "pkg", "showdown", "battle", "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		dataDir = filepath.Join(cwd, "..", "..", "pkg", "showdown", "battle", "data")
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating data directory: %v\n", err)
		os.Exit(1)
	}

	movesDest := filepath.Join(dataDir, "moves.json")
	pokedexDest := filepath.Join(dataDir, "pokedex.json")
	formatsDest := filepath.Join(dataDir, "formats_data.json")
	abilitiesDest := filepath.Join(dataDir, "abilities.json")
	randomSetsDest := filepath.Join(dataDir, "random_sets.json")

	if err := updateMoves(*showdownDir, movesDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating moves: %v\n", err)
		os.Exit(1)
	}

	if err := updatePokedex(*showdownDir, pokedexDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating pokedex: %v\n", err)
		os.Exit(1)
	}

	if err := updateFormatsData(*showdownDir, formatsDest); err != nil {
		fmt.Fprintf(os.Stderr, "warning: updating formats data: %v\n", err)
	}

	if err := updateAbilities(*showdownDir, abilitiesDest); err != nil {
		fmt.Fprintf(os.Stderr, "warning: updating abilities: %v\n", err)
	}

	if err := updateRandomSets(*showdownDir, randomSetsDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating random sets: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("dataset update completed successfully from official smogon/pokemon-showdown.")
}
