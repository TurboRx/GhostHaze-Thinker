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
	"strconv"
	"strings"
	"time"
)

const (
	smogonBaseURL       = "https://raw.githubusercontent.com/smogon/pokemon-showdown/master/data"
	smogonPokedexURL    = smogonBaseURL + "/pokedex.ts"
	smogonMovesURL      = smogonBaseURL + "/moves.ts"
	smogonRandomSetsURL = smogonBaseURL + "/random-battles/gen9/sets.json"
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

func updateRandomSets(showdownDir, destPath string) error {
	relPath := filepath.Join("random-battles", "gen9", "sets.json")
	data, err := readSource(showdownDir, relPath, smogonRandomSetsURL)
	if err != nil {
		return err
	}

	var rawSets map[string]struct {
		Level int `json:"level"`
		Sets  []struct {
			Role      string   `json:"role"`
			Movepool  []string `json:"movepool"`
			Abilities []string `json:"abilities"`
			TeraTypes []string `json:"teraTypes"`
		} `json:"sets"`
	}

	if err := json.Unmarshal(data, &rawSets); err != nil {
		return fmt.Errorf("failed to unmarshal random sets: %w", err)
	}

	processed := make(map[string]any, len(rawSets))
	for species, set := range rawSets {
		cleanID := cleanKey(species)
		processed[cleanID] = set
	}

	outData, err := json.Marshal(processed)
	if err != nil {
		return err
	}

	if err := os.WriteFile(destPath, outData, 0644); err != nil {
		return err
	}
	fmt.Printf("wrote %d random battle sets to %s\n", len(processed), destPath)
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
	randomSetsDest := filepath.Join(dataDir, "random_sets.json")

	if err := updateMoves(*showdownDir, movesDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating moves: %v\n", err)
		os.Exit(1)
	}

	if err := updatePokedex(*showdownDir, pokedexDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating pokedex: %v\n", err)
		os.Exit(1)
	}

	if err := updateRandomSets(*showdownDir, randomSetsDest); err != nil {
		fmt.Fprintf(os.Stderr, "error updating random sets: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("dataset update completed successfully from official smogon/pokemon-showdown.")
}
