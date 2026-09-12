package battle

import (
	"fmt"
	"strings"
)

type BattleEngine interface {
	Decide(b *Battle, req BattleRequest) BattleDecision
}

type DefaultEngine struct{}

func NewDefaultEngine() *DefaultEngine {
	return &DefaultEngine{}
}

func (e *DefaultEngine) Decide(b *Battle, req BattleRequest) BattleDecision {
	if req.Wait {
		return BattleDecision{Type: DecisionPass}
	}

	// 1. team preview phase
	if req.TeamPreview {
		return e.decideTeamPreview(b, req)
	}

	// 2. forced switch phase (after a faint or eject)
	if len(req.ForceSwitch) > 0 && req.ForceSwitch[0] {
		return e.decideSwitch(b, req, true)
	}

	// 3. regular turn (active moves or voluntary switch)
	if len(req.Active) > 0 && len(req.Active[0].Moves) > 0 {
		return e.decideActiveTurn(b, req)
	}

	// fallback to switch if available
	return e.decideSwitch(b, req, false)
}

func (e *DefaultEngine) decideTeamPreview(b *Battle, req BattleRequest) BattleDecision {
	pokemonList := req.Side.Pokemon
	if len(pokemonList) == 0 {
		return BattleDecision{Type: DecisionTeam, TeamOrder: "123456"}
	}

	bestLeadIndex := 0
	bestScore := -9999.0

	for i, poke := range pokemonList {
		if poke.IsFainted() {
			continue
		}
		pokeTypes := GetSpeciesTypes(poke.Species())
		score := 0.0

		// evaluate against opponent's previewed team
		for _, oppPoke := range b.OpponentTeam {
			oppTypes := GetSpeciesTypes(oppPoke.Species)
			for _, pt := range pokeTypes {
				score += GetMultipleEffectiveness(pt, oppTypes...)
			}
			for _, ot := range oppTypes {
				score -= GetMultipleEffectiveness(ot, pokeTypes...)
			}
		}

		if score > bestScore {
			bestScore = score
			bestLeadIndex = i
		}
	}

	// construct team order string (1-indexed) placing bestLeadIndex at position 1
	var order strings.Builder
	order.WriteString(fmt.Sprintf("%d", bestLeadIndex+1))
	for i := 0; i < len(pokemonList); i++ {
		if i != bestLeadIndex {
			order.WriteString(fmt.Sprintf("%d", i+1))
		}
	}

	return BattleDecision{
		Type:      DecisionTeam,
		TeamOrder: order.String(),
	}
}

func (e *DefaultEngine) decideSwitch(b *Battle, req BattleRequest, forced bool) BattleDecision {
	pokemonList := req.Side.Pokemon
	if len(pokemonList) <= 1 {
		return BattleDecision{Type: DecisionPass}
	}

	oppTypes := b.OpponentActive.Types
	if len(oppTypes) == 0 && b.OpponentActive.Species != "" {
		oppTypes = GetSpeciesTypes(b.OpponentActive.Species)
	}

	bestSlot := 0
	bestScore := -99999.0

	// bench pokemon start at index 1 (slot 2 in showdown)
	for i := 1; i < len(pokemonList); i++ {
		poke := pokemonList[i]
		if poke.IsFainted() || poke.Active {
			continue
		}

		benchTypes := GetSpeciesTypes(poke.Species())
		hpRatio := poke.HPPercent()

		// defensive resistance against opponent types
		defMultiplier := 1.0
		for _, ot := range oppTypes {
			defMultiplier *= GetMultipleEffectiveness(ot, benchTypes...)
		}
		defScore := 2.0 / (defMultiplier + 0.1)

		// offensive coverage with known moves
		offScore := 50.0
		for _, moveName := range poke.Moves {
			data := GetMoveData(moveName)
			eff := GetMultipleEffectiveness(data.Type, oppTypes...)
			stab := 1.0
			for _, bt := range benchTypes {
				if strings.EqualFold(data.Type, bt) {
					stab = 1.5
					break
				}
			}
			dmgEst := float64(data.BasePower) * eff * stab
			if dmgEst > offScore {
				offScore = dmgEst
			}
		}

		totalScore := (defScore * 120.0) + offScore + (hpRatio * 80.0)
		if totalScore > bestScore {
			bestScore = totalScore
			bestSlot = i + 1 // 1-indexed slot
		}
	}

	if bestSlot == 0 {
		// no valid bench pokemon found, fallback to slot 1 or pass
		if forced {
			return BattleDecision{Type: DecisionSwitch, Slot: 1}
		}
		return BattleDecision{Type: DecisionPass}
	}

	return BattleDecision{
		Type: DecisionSwitch,
		Slot: bestSlot,
	}
}

func (e *DefaultEngine) decideActiveTurn(b *Battle, req BattleRequest) BattleDecision {
	activeOpt := req.Active[0]
	moves := activeOpt.Moves
	if len(moves) == 0 {
		return e.decideSwitch(b, req, true)
	}

	activePoke := req.Side.Pokemon[0]
	activeTypes := GetSpeciesTypes(activePoke.Species())
	activeHP := activePoke.HPPercent()

	oppTypes := b.OpponentActive.Types
	if len(oppTypes) == 0 && b.OpponentActive.Species != "" {
		oppTypes = GetSpeciesTypes(b.OpponentActive.Species)
	}

	bestSlot := 1
	bestScore := -99999.0
	shouldTera := false

	// speed calculation
	mySpe := 80
	if activePoke.Stats != nil && activePoke.Stats["spe"] > 0 {
		mySpe = activePoke.Stats["spe"]
	} else {
		mySpe = GetSpeciesBaseStats(activePoke.Species())["spe"]
	}
	if strings.Contains(activePoke.Status(), "par") {
		mySpe = mySpe / 2
	}

	oppSpe := GetSpeciesBaseStats(b.OpponentActive.Species)["spe"]
	if b.OpponentActive.Boosts != nil {
		stage := b.OpponentActive.Boosts["spe"]
		if stage > 0 {
			oppSpe = int(float64(oppSpe) * float64(2+stage) / 2.0)
		} else if stage < 0 {
			oppSpe = int(float64(oppSpe) * 2.0 / float64(2-stage))
		}
	}
	if strings.Contains(b.OpponentActive.Status, "par") {
		oppSpe = oppSpe / 2
	}

	isFaster := mySpe >= oppSpe

	for i, m := range moves {
		slot := i + 1
		if m.IsDisabled() || m.PP <= 0 {
			continue
		}

		data := GetMoveData(m.ID)
		score := 0.0

		// status moves evaluation
		if data.Category == CategoryStatus {
			score = e.scoreStatusMove(b, data, activeHP)
		} else {
			// attack moves evaluation
			eff := GetMultipleEffectiveness(data.Type, oppTypes...)

			// ability immunity check
			if b.OpponentActive.Ability != "" {
				if IsAbilityImmune(b.OpponentActive.Ability, data.Type) {
					eff = 0.0
				}
			} else if b.OpponentActive.Species != "" {
				if randData, found := GetGenRandomBattleSet(b.Generation(), b.OpponentActive.Species); found {
					for _, set := range randData.Sets {
						if len(set.Abilities) == 1 && IsAbilityImmune(set.Abilities[0], data.Type) {
							eff = 0.0
							break
						}
					}
				}
			}

			if eff == 0.0 {
				// immune: completely unviable
				score = -5000.0
			} else {
				stab := 1.0
				for _, at := range activeTypes {
					if strings.EqualFold(data.Type, at) {
						stab = 1.5
						break
					}
				}

				level := 80
				atk := 100
				def := 100
				if activePoke.Stats != nil {
					if data.Category == CategoryPhysical {
						atk = activePoke.Stats["atk"]
					} else {
						atk = activePoke.Stats["spa"]
					}
				}

				// evaluate target defense using known opponent base stats and stage boosts
				oppBaseStats := GetSpeciesBaseStats(b.OpponentActive.Species)
				isPsyshockLike := cleanID(data.ID) == "psyshock" || cleanID(data.ID) == "psystrike" || cleanID(data.ID) == "secretsword"
				if data.Category == CategoryPhysical || isPsyshockLike {
					def = oppBaseStats["def"]
					if b.OpponentActive.Boosts != nil {
						stage := b.OpponentActive.Boosts["def"]
						if stage > 0 {
							def = int(float64(def) * float64(2+stage) / 2.0)
						} else if stage < 0 {
							def = int(float64(def) * 2.0 / float64(2-stage))
						}
					}
				} else {
					def = oppBaseStats["spd"]
					if b.OpponentActive.Boosts != nil {
						stage := b.OpponentActive.Boosts["spd"]
						if stage > 0 {
							def = int(float64(def) * float64(2+stage) / 2.0)
						} else if stage < 0 {
							def = int(float64(def) * 2.0 / float64(2-stage))
						}
					}
				}
				if def <= 0 {
					def = 80
				}

				isBurned := strings.Contains(activePoke.Status(), "brn")
				isPhysical := data.Category == CategoryPhysical
				dmg := CalculateDamage(level, data.BasePower, atk, def, stab, eff, isBurned, isPhysical)

				score = dmg

				// lethal ko bonus
				oppHP := b.OpponentActive.HPPercent
				if oppHP <= 0 {
					oppHP = 1.0
				}
				// approx opponent max hp in points ~ 250
				approxOppPoints := oppHP * 250.0
				if dmg >= approxOppPoints {
					if isFaster {
						score += 3000.0 // faster lethal ko secures clean kill without taking damage
					} else {
						score += 1800.0
					}
				}

				// priority finisher bonus
				if data.Priority > 0 {
					if dmg >= approxOppPoints {
						score += 3500.0 // priority secures ko before opponent can move
					} else if !isFaster && activeHP < 0.35 {
						score += 800.0 // priority chip when outsped and in danger of fainting
					}
				}

				// super-effective bonus
				if eff >= 2.0 {
					score += 400.0
				}

				// accuracy factor
				if data.Accuracy > 0 && data.Accuracy < 100 {
					score *= float64(data.Accuracy) / 100.0
				}

				// terastallize opportunity check
				if activeOpt.CanTerastallize != "" && !shouldTera {
					teraType := strings.ToLower(activeOpt.CanTerastallize)
					if strings.EqualFold(data.Type, teraType) && eff >= 2.0 {
						shouldTera = true
						score += 600.0
					}
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestSlot = slot
		}
	}

	// check if voluntary switch is clearly better than our best move
	if bestScore < 80.0 && activeHP < 0.35 && len(req.Side.Pokemon) > 1 {
		switchDecision := e.decideSwitch(b, req, false)
		if switchDecision.Type == DecisionSwitch && switchDecision.Slot > 1 {
			return switchDecision
		}
	}

	return BattleDecision{
		Type:         DecisionMove,
		Slot:         bestSlot,
		Terastallize: shouldTera,
	}
}

func (e *DefaultEngine) scoreStatusMove(b *Battle, data MoveData, activeHP float64) float64 {
	// recovery moves
	if data.IsHealing {
		if activeHP < 0.40 {
			return 1800.0 // critical heal
		}
		if activeHP < 0.65 {
			return 950.0
		}
		return -800.0 // wasted turn when healthy
	}

	// entry hazards
	if data.IsHazard {
		if b.Turn <= 3 && !b.OpponentHasHazard(data.ID) && b.OpponentAliveCount() > 1 {
			return 1100.0
		}
		return -1000.0
	}

	// status infliction
	if data.IsStatus {
		if b.OpponentActive.Status != "" {
			return -1200.0 // already statused
		}
		// immunity checks
		for _, ot := range b.OpponentActive.Types {
			if data.ID == "thunderwave" && (ot == "electric" || ot == "ground") {
				return -1200.0
			}
			if data.ID == "willowisp" && ot == "fire" {
				return -1200.0
			}
			if data.ID == "toxic" && (ot == "poison" || ot == "steel") {
				return -1200.0
			}
		}
		return 1200.0 // great opening or crippling move
	}

	// setup moves
	if data.IsSetup {
		if activeHP > 0.65 {
			return 1050.0
		}
		if activeHP < 0.35 {
			return -600.0
		}
		return 300.0
	}

	// protect
	if data.ID == "protect" {
		if b.OpponentActive.Status == "tox" || b.OpponentActive.Status == "brn" {
			return 850.0
		}
		return 200.0
	}

	return 250.0
}
