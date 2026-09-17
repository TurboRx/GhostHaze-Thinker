package battle

import (
	"strings"
)

// simulatedpokemon holds the minimal battle state needed for turn evaluation.
type SimulatedPokemon struct {
	Slot       int
	Species    string
	HPPercent  float64
	Status     string
	Boosts     map[string]int
	Item            string
	Ability         string
	Volatiles       map[string]bool
	Fainted         bool
	LockedMove      string
	Moves           []string
	ConfirmedFaster bool
	ConfirmedSlower bool
}

// simulatedstate represents the complete board state during minimax search.
type SimulatedState struct {
	OurActive           SimulatedPokemon
	OurBench            []SimulatedPokemon
	OppActive           SimulatedPokemon
	OppBench            []SimulatedPokemon
	Weather             string
	Terrain             string
	OurHazards          map[string]int
	OppHazards          map[string]int
	OurScreens          map[string]bool
	OppScreens          map[string]bool
	ConsecutiveProtects int
	Turn                int
	WinConSpecies       string
	SackFodder          map[string]bool
	PredictiveRate      float64
}

// evaluatebattlestate scores a simulated battle state from our perspective.
// a positive score favors our team; a negative score favors the opponent.
func EvaluateBattleState(s *SimulatedState) float64 {
	// 1. check terminal win / loss conditions
	ourAlive := 0
	if !s.OurActive.Fainted && s.OurActive.HPPercent > 0.001 {
		ourAlive++
	}
	for _, p := range s.OurBench {
		if !p.Fainted && p.HPPercent > 0.001 {
			ourAlive++
		}
	}

	oppAlive := 0
	if !s.OppActive.Fainted && s.OppActive.HPPercent > 0.001 {
		oppAlive++
	}
	for _, p := range s.OppBench {
		if !p.Fainted && p.HPPercent > 0.001 {
			oppAlive++
		}
	}

	if ourAlive == 0 {
		return -10000.0 - (s.OppActive.HPPercent * 100.0)
	}
	if oppAlive == 0 {
		// prefer clean victories where our team took minimal or zero damage
		return 10000.0 + (s.OurActive.HPPercent * 100.0)
	}

	score := 0.0

	// 2. pokemon count differential (+35 per alive advantage)
	score += float64(ourAlive-oppAlive) * 35.0

	winConSpecies := s.WinConSpecies
	sackFodder := s.SackFodder
	if winConSpecies == "" && sackFodder == nil {
		winConSpecies, sackFodder = IdentifyWinConditionAndSackFodder(s)
	}

	// 3. active pokemon hp evaluation with win condition preservation
	if !s.OurActive.Fainted {
		weight := 120.0
		switch {
		case s.OurActive.Species != "" && strings.EqualFold(s.OurActive.Species, winConSpecies):
			weight = 160.0 // strategic premium on preserving active win condition
		case sackFodder[s.OurActive.Species]:
			weight = 65.0 // low preservation priority for sack fodder
		}
		score += s.OurActive.HPPercent * weight
	}
	if !s.OppActive.Fainted {
		score -= s.OppActive.HPPercent * 120.0
	}

	// 4. bench pokemon hp evaluation with sweeper preservation and sack fodder discount
	for _, p := range s.OurBench {
		if !p.Fainted {
			weight := 80.0
			switch {
			case p.Species != "" && strings.EqualFold(p.Species, winConSpecies):
				weight = 125.0 // high preservation premium for bench win condition
			case sackFodder[p.Species]:
				weight = 35.0 // heavy discount for sacrifice fodder
			default:
				baseStats := GetSpeciesBaseStats(p.Species)
				if (baseStats["atk"] >= 115 || baseStats["spa"] >= 115) && baseStats["spe"] >= 80 {
					weight += 15.0
				}
			}
			score += p.HPPercent * weight
		}
	}
	for _, p := range s.OppBench {
		if !p.Fainted {
			weight := 80.0
			baseStats := GetSpeciesBaseStats(p.Species)
			if (baseStats["atk"] >= 115 || baseStats["spa"] >= 115) && baseStats["spe"] >= 80 {
				weight += 20.0
			}
			score -= p.HPPercent * weight
		}
	}

	// 5. stat boosts for active pokemon
	score += evaluateActiveBoosts(s.OurActive)
	score -= evaluateActiveBoosts(s.OppActive)

	// 6. major status penalties
	score += evaluateStatusPenalty(s.OurActive)
	for _, p := range s.OurBench {
		score += evaluateStatusPenalty(p) * 0.5
	}
	score -= evaluateStatusPenalty(s.OppActive)
	for _, p := range s.OppBench {
		score -= evaluateStatusPenalty(p) * 0.5
	}

	// 7. volatiles evaluation (substitute, leech seed, confusion)
	if s.OurActive.Volatiles != nil {
		if s.OurActive.Volatiles["substitute"] {
			score += 65.0
		}
		if s.OurActive.Volatiles["leechseed"] {
			score -= 30.0
		}
		if s.OurActive.Volatiles["confusion"] {
			score -= 15.0
		}
	}
	if s.OppActive.Volatiles != nil {
		if s.OppActive.Volatiles["substitute"] {
			score -= 65.0
		}
		if s.OppActive.Volatiles["leechseed"] {
			score += 30.0
		}
		if s.OppActive.Volatiles["confusion"] {
			score += 15.0
		}
	}

	// 8. entry hazard evaluation scaled by bench count
	ourBenchCount := 0.0
	for _, p := range s.OurBench {
		if !p.Fainted {
			ourBenchCount++
		}
	}
	oppBenchCount := 0.0
	for _, p := range s.OppBench {
		if !p.Fainted {
			oppBenchCount++
		}
	}
	// if bench count is unknown (early in battle), assume standard bench size of 5
	if oppBenchCount == 0 && len(s.OppBench) == 0 {
		oppBenchCount = 5.0
	}
	if ourBenchCount == 0 && len(s.OurBench) == 0 {
		ourBenchCount = 5.0
	}

	earlyHazardWeight := 0.0
	if s.Turn > 0 && s.Turn <= 3 {
		earlyHazardWeight = 45.0 // opening tempo weight for setting hazards on early turns
	}

	if s.OurHazards != nil {
		if s.OurHazards["stealthrock"] > 0 && ourBenchCount > 0 {
			score -= 65.0 + earlyHazardWeight + (ourBenchCount * 28.0)
		}
		if layers := s.OurHazards["spikes"]; layers > 0 && ourBenchCount > 0 {
			score -= float64(layers) * (22.0 + (ourBenchCount * 14.0))
		}
		if layers := s.OurHazards["toxicspikes"]; layers > 0 && ourBenchCount > 0 {
			score -= float64(layers) * (24.0 + (ourBenchCount * 14.0))
		}
		if s.OurHazards["stickyweb"] > 0 && ourBenchCount > 0 {
			score -= 40.0 + earlyHazardWeight + (ourBenchCount * 20.0)
		}
	}
	if s.OppHazards != nil {
		if s.OppHazards["stealthrock"] > 0 && oppBenchCount > 0 {
			score += 65.0 + earlyHazardWeight + (oppBenchCount * 28.0)
		}
		if layers := s.OppHazards["spikes"]; layers > 0 && oppBenchCount > 0 {
			score += float64(layers) * (22.0 + (oppBenchCount * 14.0))
		}
		if layers := s.OppHazards["toxicspikes"]; layers > 0 && oppBenchCount > 0 {
			score += float64(layers) * (24.0 + (oppBenchCount * 14.0))
		}
		if s.OppHazards["stickyweb"] > 0 && oppBenchCount > 0 {
			score += 40.0 + earlyHazardWeight + (oppBenchCount * 20.0)
		}
	}

	// 9. active speed advantage
	ourSpe := calculatePokemonSpeed(s.OurActive, s.Weather, s.Terrain, s.OurScreens != nil && s.OurScreens["tailwind"])
	oppSpe := calculatePokemonSpeed(s.OppActive, s.Weather, s.Terrain, s.OppScreens != nil && s.OppScreens["tailwind"])
	switch {
	case s.OppActive.ConfirmedFaster:
		score -= 18.0
	case s.OppActive.ConfirmedSlower:
		score += 18.0
	case ourSpe > oppSpe:
		score += 15.0
	case oppSpe > ourSpe:
		score -= 15.0
	}

	// 10. active screens evaluation
	if s.OurScreens != nil {
		if s.OurScreens["reflect"] {
			score += 40.0
		}
		if s.OurScreens["lightscreen"] {
			score += 40.0
		}
		if s.OurScreens["auroraveil"] {
			score += 70.0
		}
		if s.OurScreens["tailwind"] {
			score += 35.0
		}
	}
	if s.OppScreens != nil {
		if s.OppScreens["reflect"] {
			score -= 40.0
		}
		if s.OppScreens["lightscreen"] {
			score -= 40.0
		}
		if s.OppScreens["auroraveil"] {
			score -= 70.0
		}
		if s.OppScreens["tailwind"] {
			score -= 35.0
		}
	}

	return score
}

// evaluateactiveboosts calculates the net advantage of stat stages.
func evaluateActiveBoosts(p SimulatedPokemon) float64 {
	if p.Boosts == nil || p.Fainted {
		return 0.0
	}
	boostScore := 0.0

	// speed boost is crucial for turn priority
	speMult := statStageMultiplier(p.Boosts["spe"])
	boostScore += (speMult - 1.0) * 35.0

	// offensive boosts
	atkMult := statStageMultiplier(p.Boosts["atk"])
	boostScore += (atkMult - 1.0) * 30.0
	spaMult := statStageMultiplier(p.Boosts["spa"])
	boostScore += (spaMult - 1.0) * 30.0

	// defensive boosts
	defMult := statStageMultiplier(p.Boosts["def"])
	boostScore += (defMult - 1.0) * 15.0
	spdMult := statStageMultiplier(p.Boosts["spd"])
	boostScore += (spdMult - 1.0) * 15.0

	return boostScore
}

// evaluatestatuspenalty calculates the negative score penalty for status conditions.
func evaluateStatusPenalty(p SimulatedPokemon) float64 {
	if p.Status == "" || p.Fainted {
		return 0.0
	}
	switch p.Status {
	case "slp":
		return -35.0
	case "frz":
		return -40.0
	case "tox":
		return -30.0
	case "par":
		return -25.0
	case "psn":
		return -12.0
	case "brn":
		abilityClean := cleanID(p.Ability)
		if abilityClean == "guts" || abilityClean == "marvelscale" {
			return 15.0
		}
		return -25.0
	default:
		return -10.0
	}
}

// statstagemultiplier converts an integer stage [-6..+6] to an effective multiplier.
func statStageMultiplier(stage int) float64 {
	if stage > 6 {
		stage = 6
	} else if stage < -6 {
		stage = -6
	}
	if stage >= 0 {
		return float64(2+stage) / 2.0
	}
	return 2.0 / float64(2-stage)
}

// calculatepokemonspeed computes effective speed considering base stat, stage, tailwind, paralysis, and weather.
func calculatePokemonSpeed(p SimulatedPokemon, weather, terrain string, hasTailwind bool) int {
	if p.Fainted {
		return 0
	}
	baseStats := GetSpeciesBaseStats(p.Species)
	spe := baseStats["spe"]

	// stat stage multiplier
	if p.Boosts != nil {
		stage := p.Boosts["spe"]
		spe = int(float64(spe) * statStageMultiplier(stage))
	}

	// tailwind doubles speed
	if hasTailwind {
		spe *= 2
	}

	// paralysis halves speed
	if p.Status == "par" {
		spe /= 2
	}

	// choice scarf boosts speed by 1.5x
	itemClean := cleanID(p.Item)
	if itemClean == "choicescarf" {
		spe = int(float64(spe) * 1.5)
	}

	// weather ability boosts
	abilityClean := cleanID(p.Ability)
	weatherClean := strings.ToLower(weather)
	switch {
	case strings.Contains(weatherClean, "rain") && abilityClean == "swiftswim":
		spe *= 2
	case strings.Contains(weatherClean, "sun") && abilityClean == "chlorophyll":
		spe *= 2
	case strings.Contains(weatherClean, "sand") && abilityClean == "sandrush":
		spe *= 2
	case (strings.Contains(weatherClean, "snow") || strings.Contains(weatherClean, "hail")) && abilityClean == "slushrush":
		spe *= 2
	}

	// electric terrain surge surfer
	if strings.Contains(strings.ToLower(terrain), "electric") && abilityClean == "surgesurfer" {
		spe *= 2
	}

	return spe
}

// identifywinconditionandsackfodder determines our primary sweeper and candidate sacrifice pokemon.
func IdentifyWinConditionAndSackFodder(s *SimulatedState) (string, map[string]bool) {
	sackFodder := make(map[string]bool)
	if s == nil {
		return "", sackFodder
	}

	// collect all remaining opponent threats
	var oppRemaining []SimulatedPokemon
	if !s.OppActive.Fainted && s.OppActive.HPPercent > 0.001 && s.OppActive.Species != "" {
		oppRemaining = append(oppRemaining, s.OppActive)
	}
	for _, op := range s.OppBench {
		if !op.Fainted && op.HPPercent > 0.001 && op.Species != "" {
			oppRemaining = append(oppRemaining, op)
		}
	}

	if len(oppRemaining) == 0 {
		return "", sackFodder
	}

	// collect all our surviving candidates
	var ourCandidates []SimulatedPokemon
	if !s.OurActive.Fainted && s.OurActive.HPPercent > 0.001 && s.OurActive.Species != "" {
		ourCandidates = append(ourCandidates, s.OurActive)
	}
	for _, p := range s.OurBench {
		if !p.Fainted && p.HPPercent > 0.001 && p.Species != "" {
			ourCandidates = append(ourCandidates, p)
		}
	}

	bestWinScore := -999.0
	bestWinSpecies := ""
	scores := make(map[string]float64)

	for _, p := range ourCandidates {
		baseStats := GetSpeciesBaseStats(p.Species)
		atk := float64(baseStats["atk"])
		spa := float64(baseStats["spa"])
		spe := float64(baseStats["spe"])
		bestOffense := atk
		if spa > bestOffense {
			bestOffense = spa
		}

		// base sweeper score from stats
		score := (bestOffense * 0.45) + (spe * 0.40)

		// check setup moves
		hasSetup := false
		for _, m := range p.Moves {
			mClean := cleanID(m)
			switch mClean {
			case "swordsdance", "dragondance", "nastyplot", "quiverdance", "calmmind", "bulkup", "agility", "autotomize", "rockpolish", "tidyup", "shiftgear", "shellsmash", "victorydance":
				hasSetup = true
			}
		}
		if hasSetup {
			score += 35.0
		}

		// check offensive sweeping items
		itemClean := cleanID(p.Item)
		switch itemClean {
		case "boosterenergy", "lifeorb", "choicescarf", "choiceband", "choicespecs":
			score += 20.0
		}

		// evaluate matchup against each remaining opponent
		pTypes := p.Types()
		for _, opp := range oppRemaining {
			oppTypes := opp.Types()
			oppStats := GetSpeciesBaseStats(opp.Species)
			oppSpe := float64(oppStats["spe"])

			maxEff := 0.0
			for _, m := range p.Moves {
				mData := GetMoveData(m)
				if mData.Category != "status" && mData.BasePower > 0 {
					eff := GetMultipleEffectiveness(mData.Type, oppTypes...)
					if eff > maxEff {
						maxEff = eff
					}
				}
			}
			if len(p.Moves) == 0 || maxEff == 0.0 {
				for _, pt := range pTypes {
					eff := GetMultipleEffectiveness(pt, oppTypes...)
					if eff > maxEff {
						maxEff = eff
					}
				}
			}

			switch {
			case maxEff >= 2.0:
				score += 25.0
			case maxEff >= 1.0:
				score += 10.0
			case maxEff <= 0.5:
				score -= 20.0
			}

			if spe > oppSpe {
				score += 12.0
			} else {
				score -= 10.0
			}
		}

		// scale by remaining health: injured pokemon cannot reliably sweep
		score *= p.HPPercent
		scores[p.Species] = score

		if score > bestWinScore {
			bestWinScore = score
			bestWinSpecies = p.Species
		}
	}

	winConSpecies := ""
	if bestWinScore >= 75.0 {
		winConSpecies = bestWinSpecies
	}

	// identify sack fodder: low hp or minimal impact against remaining opponent team
	for _, p := range ourCandidates {
		if p.Species == winConSpecies {
			continue
		}
		if p.HPPercent <= 0.25 || scores[p.Species] <= 35.0 {
			sackFodder[p.Species] = true
		}
	}

	return winConSpecies, sackFodder
}
