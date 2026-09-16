package battle

import (
	"math"
	"math/rand/v2"
	"strings"
)

// simactiontype denotes whether an action is a move or a switch.
type SimActionType int

const (
	actionMove SimActionType = iota
	actionSwitch
)

// simaction models a discrete choice available to either player during a turn.
type SimAction struct {
	Type          SimActionType
	MoveSlot      int
	MoveID        string
	MoveData      MoveData
	Terastallize  bool
	SwitchSlot    int
	SwitchSpecies string
	SwitchIndex   int
}

// minimaxengine implements the battleengine interface using simultaneous turn simulation.
type MinimaxEngine struct{}

// newminimaxengine instantiates a new simultaneous minimax battle engine.
func NewMinimaxEngine() *MinimaxEngine {
	return &MinimaxEngine{}
}

// decide selects the optimal battle decision via game-theoretic minimax search.
func (e *MinimaxEngine) Decide(b *Battle, req BattleRequest) BattleDecision {
	if req.Wait {
		return BattleDecision{Type: DecisionPass}
	}

	// 1. team preview phase
	if req.TeamPreview {
		return e.decideTeamPreview(b, req)
	}

	// 2. forced switch phase
	if len(req.ForceSwitch) > 0 && req.ForceSwitch[0] {
		return e.decideForcedSwitch(b, req)
	}

	// 3. active turn phase with candidate moves and voluntary switches
	if len(req.Active) > 0 && len(req.Active[0].Moves) > 0 {
		return e.decideSimultaneousTurn(b, req)
	}

	// fallback to switch or pass
	return e.decideForcedSwitch(b, req)
}

// decodeteampreview selects the best lead pokemon against opponent's previewed roster.
func (e *MinimaxEngine) decideTeamPreview(b *Battle, req BattleRequest) BattleDecision {
	pokemonList := req.Side.Pokemon
	if len(pokemonList) == 0 {
		return BattleDecision{Type: DecisionTeam, TeamOrder: "123456"}
	}

	bestLeadIndex := 0
	bestScore := -99999.0

	for i, poke := range pokemonList {
		if poke.IsFainted() {
			continue
		}
		pokeTypes := GetSpeciesTypes(poke.Species())
		score := 0.0

		for _, oppPoke := range b.OpponentTeam {
			oppTypes := GetSpeciesTypes(oppPoke.Species)
			for _, pt := range pokeTypes {
				score += GetMultipleEffectiveness(pt, oppTypes...) * 25.0
			}
			for _, ot := range oppTypes {
				score -= GetMultipleEffectiveness(ot, pokeTypes...) * 25.0
			}
		}

		// factor in base speed tier of lead
		baseSpe := GetSpeciesBaseStats(poke.Species())["spe"]
		score += float64(baseSpe) * 0.1

		if score > bestScore {
			bestScore = score
			bestLeadIndex = i
		}
	}

	var order strings.Builder
	order.WriteString(string(rune('1' + bestLeadIndex)))
	for i := 0; i < len(pokemonList); i++ {
		if i != bestLeadIndex {
			order.WriteString(string(rune('1' + i)))
		}
	}

	return BattleDecision{
		Type:      DecisionTeam,
		TeamOrder: order.String(),
	}
}

// decidesforcedswitch chooses the best bench pokemon to enter following a faint.
func (e *MinimaxEngine) decideForcedSwitch(b *Battle, req BattleRequest) BattleDecision {
	pokemonList := req.Side.Pokemon
	if len(pokemonList) <= 1 {
		return BattleDecision{Type: DecisionPass}
	}

	state := buildSimulatedState(b, req)
	oppTypes := state.OppActive.Types()
	bestSlot := -1
	bestScore := -99999.0

	for i := 0; i < len(pokemonList); i++ {
		poke := pokemonList[i]
		if poke.IsFainted() || poke.Active {
			continue
		}

		benchTypes := GetSpeciesTypes(poke.Species())
		hpRatio := poke.HPPercent()

		// defensive resistance
		defMult := 1.0
		for _, ot := range oppTypes {
			defMult *= GetMultipleEffectiveness(ot, benchTypes...)
		}
		defScore := 2.0 / (defMult + 0.1)

		// entry hazard impact
		hazardPenalty := 0.0
		if state.OurHazards["stealthrock"] > 0 && !strings.EqualFold(poke.Item, "heavydutyboots") {
			hazardPenalty += 0.125 * GetMultipleEffectiveness("rock", benchTypes...) * 100.0
		}

		// offensive coverage
		offScore := 40.0
		for _, moveName := range poke.Moves {
			mData := GetMoveData(moveName)
			eff := GetMultipleEffectiveness(mData.Type, oppTypes...)
			stab := 1.0
			for _, bt := range benchTypes {
				if strings.EqualFold(mData.Type, bt) {
					stab = 1.5
					break
				}
			}
			dmgEst := float64(mData.BasePower) * eff * stab
			if dmgEst > offScore {
				offScore = dmgEst
			}
		}

		total := (defScore * 120.0) + offScore + (hpRatio * 80.0) - hazardPenalty
		if total > bestScore {
			bestScore = total
			bestSlot = i + 1
		}
	}

	if bestSlot <= 0 {
		for i := 0; i < len(pokemonList); i++ {
			if !pokemonList[i].IsFainted() && !pokemonList[i].Active {
				bestSlot = i + 1
				break
			}
		}
	}

	if bestSlot <= 0 {
		return BattleDecision{Type: DecisionPass}
	}

	return BattleDecision{
		Type: DecisionSwitch,
		Slot: bestSlot,
	}
}

// decidesimultaneousturn builds the payoff matrix and selects the maximin action.
func (e *MinimaxEngine) decideSimultaneousTurn(b *Battle, req BattleRequest) BattleDecision {
	state := buildSimulatedState(b, req)
	ourActions := generateOurActions(b, req, state)
	if len(ourActions) == 0 {
		return e.decideForcedSwitch(b, req)
	}
	if len(ourActions) == 1 {
		return actionToDecision(ourActions[0])
	}

	oppActions := generateOpponentActions(b, state)
	if len(oppActions) == 0 {
		oppActions = generateFallbackOpponentActions(state)
	}

	// count surviving pokemon to trigger deep terminal solver in endgame (<= 2 alive per side)
	ourAlive := 0
	if !state.OurActive.Fainted && state.OurActive.HPPercent > 0.001 {
		ourAlive++
	}
	for _, p := range state.OurBench {
		if !p.Fainted && p.HPPercent > 0.001 {
			ourAlive++
		}
	}
	oppAlive := 0
	if !state.OppActive.Fainted && state.OppActive.HPPercent > 0.001 {
		oppAlive++
	}
	for _, p := range state.OppBench {
		if !p.Fainted && p.HPPercent > 0.001 {
			oppAlive++
		}
	}

	if ourAlive <= 2 && oppAlive <= 2 {
		maxDepth := 4
		if ourAlive == 1 && oppAlive == 1 {
			maxDepth = 6 // 1v1 terminal depth
		}
		return e.decideEndgameTerminal(state, ourActions, oppActions, maxDepth)
	}

	var scored []scoredAction
	bestScore := -999999.0
	bestAction := ourActions[0]

	for _, ourAct := range ourActions {
		worstScore := 999999.0
		totalScore := 0.0

		for _, oppAct := range oppActions {
			simResult := simulateTurn(state, ourAct, oppAct)
			var score float64
			if simResult.OurActive.Fainted || simResult.OppActive.Fainted {
				score = EvaluateBattleState(simResult)
			} else {
				// combine immediate turn 1 board state payoff with 2-ply lookahead
				score = (0.20 * EvaluateBattleState(simResult)) + (0.80 * evaluate2PlyLookahead(simResult))
			}

			if score < worstScore {
				worstScore = score
			}
			totalScore += score
		}

		avgScore := totalScore / float64(len(oppActions))
		// maximin score with weighted average expectation
		compositeScore := (0.75 * worstScore) + (0.25 * avgScore)

		scored = append(scored, scoredAction{action: ourAct, score: compositeScore})
		if compositeScore > bestScore {
			bestScore = compositeScore
			bestAction = ourAct
		}
	}

	selectedAction := selectMixedStrategy(scored, bestScore, bestAction)
	return actionToDecision(selectedAction)
}

// buildsimulatedstate constructs a snapshot of the current battle state.
func buildSimulatedState(b *Battle, req BattleRequest) *SimulatedState {
	s := &SimulatedState{
		Weather:             b.Weather,
		Terrain:             b.Terrain,
		ConsecutiveProtects: b.ConsecutiveProtects,
		OurHazards:          make(map[string]int),
		OppHazards:          make(map[string]int),
		Turn:                b.Turn,
	}

	for k, v := range b.MyHazards {
		s.OurHazards[k] = v
	}
	for k, v := range b.OpponentHazardLayers {
		s.OppHazards[k] = v
	}
	// copy boolean hazards if layers not yet populated
	for k, v := range b.OpponentHazards {
		if v && s.OppHazards[k] == 0 {
			s.OppHazards[k] = 1
		}
	}

	// our active and bench pokemon
	if len(req.Side.Pokemon) > 0 {
		activeIdx := 0
		for i, p := range req.Side.Pokemon {
			if p.Active {
				activeIdx = i
				break
			}
		}

		active := req.Side.Pokemon[activeIdx]
		var ourActiveMoves []string
		if len(req.Active) > 0 {
			for _, m := range req.Active[0].Moves {
				if !m.IsDisabled() && m.PP > 0 {
					ourActiveMoves = append(ourActiveMoves, cleanID(m.ID))
				}
			}
		}
		s.OurActive = SimulatedPokemon{
			Slot:      activeIdx + 1,
			Species:   active.Species(),
			HPPercent: active.HPPercent(),
			Status:    active.Status(),
			Item:      active.Item,
			Ability:   active.Ability,
			Fainted:   active.IsFainted(),
			Boosts:    make(map[string]int),
			Volatiles: make(map[string]bool),
			Moves:     ourActiveMoves,
		}
		for k, v := range b.MyBoosts {
			s.OurActive.Boosts[k] = v
		}
		for k, v := range b.MyVolatiles {
			s.OurActive.Volatiles[k] = v
		}

		// bench pokemon
		for i, p := range req.Side.Pokemon {
			if i == activeIdx {
				continue
			}
			s.OurBench = append(s.OurBench, SimulatedPokemon{
				Slot:      i + 1,
				Species:   p.Species(),
				HPPercent: p.HPPercent(),
				Status:    p.Status(),
				Item:      p.Item,
				Ability:   p.Ability,
				Fainted:   p.IsFainted(),
			})
		}
	}

	// opponent active pokemon
	s.OppActive = SimulatedPokemon{
		Species:         b.OpponentActive.Species,
		HPPercent:       b.OpponentActive.HPPercent,
		Status:          b.OpponentActive.Status,
		Item:            b.OpponentActive.Item,
		Ability:         b.OpponentActive.Ability,
		Fainted:         b.OpponentActive.HPPercent <= 0.001,
		Boosts:          make(map[string]int),
		Volatiles:       make(map[string]bool),
		LockedMove:      b.OpponentActive.LockedMove,
		Moves:           b.OpponentActive.Moves,
		ConfirmedFaster: b.OpponentActive.ConfirmedFaster,
		ConfirmedSlower: b.OpponentActive.ConfirmedSlower,
	}
	if s.OppActive.HPPercent == 0 && !s.OppActive.Fainted {
		s.OppActive.HPPercent = 1.0
	}
	for k, v := range b.OpponentActive.Boosts {
		s.OppActive.Boosts[k] = v
	}
	for k, v := range b.OpponentVolatiles {
		s.OppActive.Volatiles[k] = v
	}

	// opponent bench pokemon
	for _, p := range b.OpponentTeam {
		if !strings.EqualFold(p.Species, b.OpponentActive.Species) {
			s.OppBench = append(s.OppBench, SimulatedPokemon{
				Species:   p.Species,
				HPPercent: 1.0,
				Status:    p.Status,
				Fainted:   p.Fainted,
				Ability:   p.Ability,
				Item:      p.Item,
			})
		}
	}

	return s
}

// types returns species types for a simulated pokemon.
func (p SimulatedPokemon) Types() []string {
	if p.Species == "" {
		return []string{"normal"}
	}
	return GetSpeciesTypes(p.Species)
}

// generateouractions produces candidate move and switch options.
func generateOurActions(b *Battle, req BattleRequest, state *SimulatedState) []SimAction {
	var actions []SimAction

	// active moves
	if len(req.Active) > 0 {
		activeReq := req.Active[0]
		for i, m := range activeReq.Moves {
			if m.IsDisabled() || m.PP <= 0 {
				continue
			}
			mData := GetMoveData(m.ID)
			actions = append(actions, SimAction{
				Type:     actionMove,
				MoveSlot: i + 1,
				MoveID:   cleanID(m.ID),
				MoveData: mData,
			})

			// terastallize option if available and strategically justified
			if activeReq.CanTerastallize != "" && shouldConsiderTerastallize(activeReq, mData, state) {
				actions = append(actions, SimAction{
					Type:         actionMove,
					MoveSlot:     i + 1,
					MoveID:       cleanID(m.ID),
					MoveData:     mData,
					Terastallize: true,
				})
			}
		}
	}

	// voluntary switches (only if active pokemon is not trapped)
	isTrapped := len(req.Active) > 0 && (req.Active[0].Trapped || req.Active[0].MaybeTrapped)
	if !isTrapped {
		for i, benchPoke := range state.OurBench {
			if !benchPoke.Fainted && benchPoke.HPPercent > 0.05 {
				actions = append(actions, SimAction{
					Type:          actionSwitch,
					SwitchSlot:    benchPoke.Slot,
					SwitchSpecies: benchPoke.Species,
					SwitchIndex:   i,
				})
			}
		}
	}

	if len(actions) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveSlot: 1,
			MoveID:   "struggle",
			MoveData: GetMoveData("struggle"),
		})
	}

	return actions
}

// generateopponentactions predicts opponent actions from revealed moves and random sets.
func generateOpponentActions(b *Battle, state *SimulatedState) []SimAction {
	var actions []SimAction

	// 1. check if opponent is choice locked into a single move
	if state.OppActive.LockedMove != "" {
		clean := cleanID(state.OppActive.LockedMove)
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   clean,
			MoveData: GetMoveData(clean),
		})
	} else {
		seenMoves := make(map[string]bool)

		// revealed moves
		for _, m := range b.OpponentActive.Moves {
			clean := cleanID(m)
			if !seenMoves[clean] {
				seenMoves[clean] = true
				actions = append(actions, SimAction{
					Type:     actionMove,
					MoveID:   clean,
					MoveData: GetMoveData(clean),
				})
			}
		}

		// deduce likely moves from official random battle sets if needed
		if len(actions) < 4 && state.OppActive.Species != "" {
			if randData, found := GetGenRandomBattleSet(b.Generation(), state.OppActive.Species); found {
				for _, set := range randData.Sets {
					for _, m := range set.Movepool {
						clean := cleanID(m)
						if !seenMoves[clean] {
							seenMoves[clean] = true
							actions = append(actions, SimAction{
								Type:     actionMove,
								MoveID:   clean,
								MoveData: GetMoveData(clean),
							})
							if len(actions) >= 4 {
								break
							}
						}
					}
					if len(actions) >= 4 {
						break
					}
				}
			}
		}
	}

	// 3. opponent candidate switch if a favorable bench counter exists
	for i, benchPoke := range state.OppBench {
		if !benchPoke.Fainted && benchPoke.HPPercent > 0.3 {
			benchTypes := GetSpeciesTypes(benchPoke.Species)
			ourTypes := state.OurActive.Types()
			resistsUs := false
			for _, ot := range ourTypes {
				if GetMultipleEffectiveness(ot, benchTypes...) <= 0.5 {
					resistsUs = true
					break
				}
			}
			if resistsUs {
				actions = append(actions, SimAction{
					Type:          actionSwitch,
					SwitchSpecies: benchPoke.Species,
					SwitchIndex:   i,
				})
				break
			}
		}
	}

	return actions
}

// generatefallbackopponentactions creates generic stab attacks when no data is known.
func generateFallbackOpponentActions(state *SimulatedState) []SimAction {
	oppTypes := state.OppActive.Types()
	var actions []SimAction
	for _, t := range oppTypes {
		switch t {
		case "fire":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "flamethrower", MoveData: GetMoveData("flamethrower")})
		case "water":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "surf", MoveData: GetMoveData("surf")})
		case "grass":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "energyball", MoveData: GetMoveData("energyball")})
		case "electric":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "thunderbolt", MoveData: GetMoveData("thunderbolt")})
		case "ground":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "earthquake", MoveData: GetMoveData("earthquake")})
		case "fighting":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "closecombat", MoveData: GetMoveData("closecombat")})
		default:
			actions = append(actions, SimAction{Type: actionMove, MoveID: "bodyslam", MoveData: GetMoveData("bodyslam")})
		}
	}
	return actions
}

// simulateturn executes one simultaneous turn and returns the resulting board state.
func simulateTurn(initial *SimulatedState, ourAct, oppAct SimAction) *SimulatedState {
	s := cloneState(initial)
	s.Turn = initial.Turn + 1

	// handle switches first (priority +6)
	ourSwitched := ourAct.Type == actionSwitch
	oppSwitched := oppAct.Type == actionSwitch

	if ourSwitched && ourAct.SwitchIndex < len(s.OurBench) {
		incoming := s.OurBench[ourAct.SwitchIndex]
		s.OurBench[ourAct.SwitchIndex] = s.OurActive
		s.OurActive = incoming
		s.OurActive.Boosts = make(map[string]int)
		s.OurActive.Volatiles = make(map[string]bool)
		s.ConsecutiveProtects = 0
		applyEntryHazards(&s.OurActive, s.OurHazards)
	}

	if oppSwitched && oppAct.SwitchIndex < len(s.OppBench) {
		incoming := s.OppBench[oppAct.SwitchIndex]
		s.OppBench[oppAct.SwitchIndex] = s.OppActive
		s.OppActive = incoming
		s.OppActive.Boosts = make(map[string]int)
		s.OppActive.Volatiles = make(map[string]bool)
		applyEntryHazards(&s.OppActive, s.OppHazards)
	}

	// if both switched, no moves execute
	if ourSwitched && oppSwitched {
		applyEndOfTurnEffects(s)
		return s
	}

	// if only one switched, the other executes their move on incoming pokemon
	if ourSwitched {
		executeMove(s, oppAct, false)
		applyEndOfTurnEffects(s)
		return s
	}
	if oppSwitched {
		executeMove(s, ourAct, true)
		applyEndOfTurnEffects(s)
		return s
	}

	// both players chose moves: determine turn order
	ourPrio := ourAct.MoveData.Priority
	oppPrio := oppAct.MoveData.Priority

	// psychic terrain blocks priority against grounded targets
	ourPrioBlocked := false
	oppPrioBlocked := false
	if strings.Contains(strings.ToLower(s.Terrain), "psychic") {
		if ourPrio > 0 && IsGrounded(s.OppActive.Types(), s.OppActive.Ability, s.OppActive.Item) {
			ourPrioBlocked = true
		}
		if oppPrio > 0 && IsGrounded(s.OurActive.Types(), s.OurActive.Ability, s.OurActive.Item) {
			oppPrioBlocked = true
		}
	}

	ourSpe := calculatePokemonSpeed(s.OurActive, s.Weather, s.Terrain)
	oppSpe := calculatePokemonSpeed(s.OppActive, s.Weather, s.Terrain)

	ourFirst := false
	if ourPrio > oppPrio {
		ourFirst = true
	} else if ourPrio < oppPrio {
		ourFirst = false
	} else {
		if s.OppActive.ConfirmedFaster {
			ourFirst = false
		} else if s.OppActive.ConfirmedSlower {
			ourFirst = true
		} else {
			ourFirst = ourSpe >= oppSpe
		}
	}

	if ourFirst {
		if !ourPrioBlocked {
			executeMove(s, ourAct, true)
		}
		if !s.OppActive.Fainted && !oppPrioBlocked {
			executeMove(s, oppAct, false)
		}
	} else {
		if !oppPrioBlocked {
			executeMove(s, oppAct, false)
		}
		if !s.OurActive.Fainted && !ourPrioBlocked {
			executeMove(s, ourAct, true)
		}
	}

	applyEndOfTurnEffects(s)
	return s
}

// executemove processes damage, healing, status, or hazard effects of a move.
func executeMove(s *SimulatedState, act SimAction, isOurMove bool) {
	var attacker *SimulatedPokemon
	var defender *SimulatedPokemon
	var targetHazards map[string]int

	if isOurMove {
		attacker = &s.OurActive
		defender = &s.OppActive
		targetHazards = s.OppHazards
	} else {
		attacker = &s.OppActive
		defender = &s.OurActive
		targetHazards = s.OurHazards
	}

	if attacker.Fainted || defender.Fainted {
		return
	}

	mData := act.MoveData

	// protect logic
	if cleanID(mData.ID) == "protect" {
		if isOurMove {
			if s.ConsecutiveProtects == 0 {
				if attacker.Volatiles == nil {
					attacker.Volatiles = make(map[string]bool)
				}
				attacker.Volatiles["protected"] = true
				s.ConsecutiveProtects++
			}
		}
		return
	}

	// check if defender is protected
	if defender.Volatiles != nil && defender.Volatiles["protected"] {
		return
	}

	// healing moves
	if mData.IsHealing {
		attacker.HPPercent += 0.50
		if attacker.HPPercent > 1.0 {
			attacker.HPPercent = 1.0
		}
		return
	}

	// setup moves
	if mData.IsSetup {
		if attacker.Boosts == nil {
			attacker.Boosts = make(map[string]int)
		}
		cleanM := cleanID(mData.ID)
		switch cleanM {
		case "swordsdance":
			attacker.Boosts["atk"] += 2
		case "dragondance":
			attacker.Boosts["atk"]++
			attacker.Boosts["spe"]++
		case "calmmind":
			attacker.Boosts["spa"]++
			attacker.Boosts["spd"]++
		case "nastyplot":
			attacker.Boosts["spa"] += 2
		case "quiverdance":
			attacker.Boosts["spa"]++
			attacker.Boosts["spd"]++
			attacker.Boosts["spe"]++
		case "bulkup":
			attacker.Boosts["atk"]++
			attacker.Boosts["def"]++
		case "agility":
			attacker.Boosts["spe"] += 2
		}
		return
	}

	// hazard moves
	if mData.IsHazard {
		cleanM := cleanID(mData.ID)
		if cleanM == "stealthrock" || cleanM == "stickyweb" {
			targetHazards[cleanM] = 1
		} else {
			targetHazards[cleanM]++
		}
		return
	}

	// status infliction moves
	if mData.IsStatus {
		cleanM := cleanID(mData.ID)
		if defender.Status == "" {
			switch cleanM {
			case "thunderwave":
				for _, t := range defender.Types() {
					if t == "ground" || t == "electric" {
						return
					}
				}
				defender.Status = "par"
			case "willowisp":
				for _, t := range defender.Types() {
					if t == "fire" {
						return
					}
				}
				defender.Status = "brn"
			case "toxic":
				for _, t := range defender.Types() {
					if t == "steel" || t == "poison" {
						return
					}
				}
				defender.Status = "tox"
			case "spore", "sleeppowder":
				for _, t := range defender.Types() {
					if t == "grass" {
						return
					}
				}
				defender.Status = "slp"
			}
		}
		return
	}

	// attack moves: check ability immunity
	if IsAbilityImmune(defender.Ability, mData.Type) {
		return
	}

	// calculate type effectiveness
	typeEff := GetMultipleEffectiveness(mData.Type, defender.Types()...)
	if typeEff == 0.0 {
		return
	}

	// stab multiplier
	stab := 1.0
	for _, at := range attacker.Types() {
		if strings.EqualFold(mData.Type, at) {
			stab = 1.5
			break
		}
	}
	if act.Terastallize {
		stab = 2.0
	}

	// offensive and defensive stat stage application
	var atkBase, defBase int
	atkStats := GetSpeciesBaseStats(attacker.Species)
	defStats := GetSpeciesBaseStats(defender.Species)

	isPhysical := mData.Category == CategoryPhysical
	isPsyshockLike := cleanID(mData.ID) == "psyshock" || cleanID(mData.ID) == "psystrike" || cleanID(mData.ID) == "secretsword"

	if isPhysical {
		atkBase = atkStats["atk"]
		if attacker.Boosts != nil {
			atkBase = int(float64(atkBase) * statStageMultiplier(attacker.Boosts["atk"]))
		}
	} else {
		atkBase = atkStats["spa"]
		if attacker.Boosts != nil {
			atkBase = int(float64(atkBase) * statStageMultiplier(attacker.Boosts["spa"]))
		}
	}

	if isPhysical || isPsyshockLike {
		defBase = defStats["def"]
		if defender.Boosts != nil {
			defBase = int(float64(defBase) * statStageMultiplier(defender.Boosts["def"]))
		}
	} else {
		defBase = defStats["spd"]
		if defender.Boosts != nil {
			defBase = int(float64(defBase) * statStageMultiplier(defender.Boosts["spd"]))
		}
	}

	isBurned := attacker.Status == "brn"
	attackerGrounded := IsGrounded(attacker.Types(), attacker.Ability, attacker.Item)
	defenderGrounded := IsGrounded(defender.Types(), defender.Ability, defender.Item)

	dmgPoints := CalculateDamageWithWeatherAndTerrain(80, mData.BasePower, atkBase, defBase, stab, typeEff, isBurned, isPhysical, mData.Type, mData.ID, s.Weather, s.Terrain, attackerGrounded, defenderGrounded)

	// convert damage points to hp fraction (approx 250 avg max hp)
	hpLoss := dmgPoints / 250.0

	defender.HPPercent -= hpLoss
	if defender.HPPercent <= 0.001 {
		defender.HPPercent = 0.0
		defender.Fainted = true
	}
}

// applyentryhazards handles damage and status on switch-in.
func applyEntryHazards(p *SimulatedPokemon, hazards map[string]int) {
	if strings.EqualFold(p.Item, "heavydutyboots") || hazards == nil {
		return
	}

	types := p.Types()
	grounded := IsGrounded(types, p.Ability, p.Item)

	// stealth rock
	if hazards["stealthrock"] > 0 {
		rockEff := GetMultipleEffectiveness("rock", types...)
		p.HPPercent -= 0.125 * rockEff
	}

	// spikes (grounded only)
	if grounded {
		if layers := hazards["spikes"]; layers > 0 {
			switch layers {
			case 1:
				p.HPPercent -= 0.125
			case 2:
				p.HPPercent -= 0.1875
			default:
				p.HPPercent -= 0.25
			}
		}

		// toxic spikes (grounded only)
		if layers := hazards["toxicspikes"]; layers > 0 {
			isPoison := false
			isSteel := false
			for _, t := range types {
				if t == "poison" {
					isPoison = true
				}
				if t == "steel" {
					isSteel = true
				}
			}
			if isPoison {
				delete(hazards, "toxicspikes") // absorb toxic spikes
			} else if !isSteel && p.Status == "" {
				if layers >= 2 {
					p.Status = "tox"
				} else {
					p.Status = "psn"
				}
			}
		}

		// sticky web (grounded only)
		if hazards["stickyweb"] > 0 {
			if p.Boosts == nil {
				p.Boosts = make(map[string]int)
			}
			p.Boosts["spe"]--
		}
	}

	if p.HPPercent <= 0.001 {
		p.HPPercent = 0.0
		p.Fainted = true
	}
}

// applyendofturneffects applies burn, poison, weather chip, and leftovers recovery.
func applyEndOfTurnEffects(s *SimulatedState) {
	applyMonEndOfTurn(&s.OurActive, s.Weather, s.Terrain)
	applyMonEndOfTurn(&s.OppActive, s.Weather, s.Terrain)
}

// applymonendofturn calculates per-pokemon end-of-turn adjustments.
func applyMonEndOfTurn(p *SimulatedPokemon, weather, terrain string) {
	if p.Fainted {
		return
	}

	// status chip
	switch p.Status {
	case "brn", "psn":
		p.HPPercent -= 0.0625
	case "tox":
		p.HPPercent -= 0.125
	}

	// leftovers recovery
	if cleanID(p.Item) == "leftovers" {
		p.HPPercent += 0.0625
		if p.HPPercent > 1.0 {
			p.HPPercent = 1.0
		}
	}

	// grassy terrain recovery
	if strings.Contains(strings.ToLower(terrain), "grassy") && IsGrounded(p.Types(), p.Ability, p.Item) {
		p.HPPercent += 0.0625
		if p.HPPercent > 1.0 {
			p.HPPercent = 1.0
		}
	}

	if p.HPPercent <= 0.001 {
		p.HPPercent = 0.0
		p.Fainted = true
	}
}

// clonestate creates a deep copy of simulatedstate for branch exploration.
func cloneState(orig *SimulatedState) *SimulatedState {
	clone := &SimulatedState{
		OurActive:           clonePokemon(orig.OurActive),
		OppActive:           clonePokemon(orig.OppActive),
		Weather:             orig.Weather,
		Terrain:             orig.Terrain,
		ConsecutiveProtects: orig.ConsecutiveProtects,
		OurHazards:          make(map[string]int),
		OppHazards:          make(map[string]int),
		Turn:                orig.Turn,
	}

	for k, v := range orig.OurHazards {
		clone.OurHazards[k] = v
	}
	for k, v := range orig.OppHazards {
		clone.OppHazards[k] = v
	}

	clone.OurBench = make([]SimulatedPokemon, len(orig.OurBench))
	for i, p := range orig.OurBench {
		clone.OurBench[i] = clonePokemon(p)
	}

	clone.OppBench = make([]SimulatedPokemon, len(orig.OppBench))
	for i, p := range orig.OppBench {
		clone.OppBench[i] = clonePokemon(p)
	}

	return clone
}

// clonepokemon duplicates a pokemon's mutable attributes.
func clonePokemon(p SimulatedPokemon) SimulatedPokemon {
	cp := p
	if len(p.Boosts) > 0 {
		cp.Boosts = make(map[string]int, len(p.Boosts))
		for k, v := range p.Boosts {
			cp.Boosts[k] = v
		}
	} else {
		cp.Boosts = nil
	}
	if len(p.Volatiles) > 0 {
		cp.Volatiles = make(map[string]bool, len(p.Volatiles))
		for k, v := range p.Volatiles {
			cp.Volatiles[k] = v
		}
	} else {
		cp.Volatiles = nil
	}
	if len(p.Moves) > 0 {
		cp.Moves = make([]string, len(p.Moves))
		copy(cp.Moves, p.Moves)
	}
	return cp
}

// actiontodecision formats a simaction into a battledecision.
func actionToDecision(act SimAction) BattleDecision {
	switch act.Type {
	case actionSwitch:
		return BattleDecision{
			Type: DecisionSwitch,
			Slot: act.SwitchSlot,
		}
	case actionMove:
		return BattleDecision{
			Type:         DecisionMove,
			Slot:         act.MoveSlot,
			Terastallize: act.Terastallize,
		}
	default:
		return BattleDecision{Type: DecisionPass}
	}
}

// evaluate2plylookahead evaluates follow-up turn options to score multi-turn setups and 2hkos.
func evaluate2PlyLookahead(s *SimulatedState) float64 {
	// if either side has fainted or is in terminal state, return board evaluation directly
	if s.OurActive.Fainted || s.OppActive.Fainted {
		return EvaluateBattleState(s)
	}

	ourActs := generateTurn2OurActions(s)
	if len(ourActs) == 0 {
		return EvaluateBattleState(s)
	}

	oppActs := generateTurn2OppActions(s)
	if len(oppActs) == 0 {
		return EvaluateBattleState(s)
	}

	bestScore2 := -999999.0
	for _, ourAct := range ourActs {
		worstScore2 := 999999.0
		totalScore2 := 0.0

		for _, oppAct := range oppActs {
			simResult2 := simulateTurn(s, ourAct, oppAct)
			var score2 float64
			if simResult2.OurActive.Fainted || simResult2.OppActive.Fainted {
				score2 = EvaluateBattleState(simResult2)
			} else if shouldExtendToTurn3(ourAct, oppAct, simResult2) {
				score2 = evaluate3PlyQuiescence(simResult2)
			} else {
				score2 = EvaluateBattleState(simResult2)
			}

			if score2 < worstScore2 {
				worstScore2 = score2
			}
			totalScore2 += score2
		}

		avgScore2 := totalScore2 / float64(len(oppActs))
		compositeScore2 := (0.75 * worstScore2) + (0.25 * avgScore2)
		if compositeScore2 > bestScore2 {
			bestScore2 = compositeScore2
		}
	}

	// apply depth discount so immediate turn 1 kos are strictly preferred over delayed turn 2 kos
	return bestScore2 * 0.90
}

// generateturn2ouractions selects the strongest candidate follow-up moves for turn 2.
func generateTurn2OurActions(s *SimulatedState) []SimAction {
	var actions []SimAction
	if len(s.OurActive.Moves) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   "bodyslam",
			MoveData: GetMoveData("bodyslam"),
		})
		return actions
	}

	for _, m := range s.OurActive.Moves {
		mData := GetMoveData(m)
		// skip hazard moves on turn 2 if already placed
		if mData.IsHazard && s.OppHazards[cleanID(m)] > 0 {
			continue
		}
		// skip repeat setup if already boosted >= 2
		if mData.IsSetup && (s.OurActive.Boosts["atk"] >= 2 || s.OurActive.Boosts["spa"] >= 2) {
			continue
		}
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   cleanID(m),
			MoveData: mData,
		})
		if len(actions) >= 3 {
			break
		}
	}

	if len(actions) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   s.OurActive.Moves[0],
			MoveData: GetMoveData(s.OurActive.Moves[0]),
		})
	}

	return actions
}

// generateturn2oppactions selects the opponent's candidate follow-up moves for turn 2.
func generateTurn2OppActions(s *SimulatedState) []SimAction {
	var actions []SimAction
	if s.OppActive.LockedMove != "" {
		clean := cleanID(s.OppActive.LockedMove)
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   clean,
			MoveData: GetMoveData(clean),
		})
		return actions
	}

	if len(s.OppActive.Moves) > 0 {
		for _, m := range s.OppActive.Moves {
			clean := cleanID(m)
			actions = append(actions, SimAction{
				Type:     actionMove,
				MoveID:   clean,
				MoveData: GetMoveData(clean),
			})
			if len(actions) >= 2 {
				break
			}
		}
		return actions
	}

	// fallback attacks based on opponent types
	oppTypes := s.OppActive.Types()
	for _, t := range oppTypes {
		switch t {
		case "fire":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "flamethrower", MoveData: GetMoveData("flamethrower")})
		case "water":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "surf", MoveData: GetMoveData("surf")})
		case "electric":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "thunderbolt", MoveData: GetMoveData("thunderbolt")})
		case "grass":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "energyball", MoveData: GetMoveData("energyball")})
		default:
			actions = append(actions, SimAction{Type: actionMove, MoveID: "bodyslam", MoveData: GetMoveData("bodyslam")})
		}
		if len(actions) >= 2 {
			break
		}
	}

	return actions
}

// shouldextendtoturn3 determines if a simulated turn 2 state is tactically volatile.
func shouldExtendToTurn3(ourAct, oppAct SimAction, s *SimulatedState) bool {
	// tactical setup moves (swords dance, dragon dance, calm mind, nasty plot, quiver dance)
	// require 3-ply resolution to properly evaluate whether stat boosts convert into decisive knockouts
	return ourAct.MoveData.IsSetup || oppAct.MoveData.IsSetup
}

// evaluate3plyquiescence evaluates a 3rd ply along critical tactical lines (setups and lethal exchanges).
func evaluate3PlyQuiescence(s *SimulatedState) float64 {
	if s.OurActive.Fainted || s.OppActive.Fainted {
		return EvaluateBattleState(s)
	}

	ourActs3 := generateTurn3OurActions(s)
	if len(ourActs3) == 0 {
		return EvaluateBattleState(s)
	}

	oppActs3 := generateTurn3OppActions(s)
	if len(oppActs3) == 0 {
		return EvaluateBattleState(s)
	}

	bestScore3 := -999999.0
	for _, ourAct := range ourActs3 {
		worstScore3 := 999999.0
		totalScore3 := 0.0

		for _, oppAct := range oppActs3 {
			simResult3 := simulateTurn(s, ourAct, oppAct)
			score3 := EvaluateBattleState(simResult3)

			if score3 < worstScore3 {
				worstScore3 = score3
			}
			totalScore3 += score3
		}

		avgScore3 := totalScore3 / float64(len(oppActs3))
		compositeScore3 := (0.75 * worstScore3) + (0.25 * avgScore3)
		if compositeScore3 > bestScore3 {
			bestScore3 = compositeScore3
		}
	}

	// 3-ply depth discount: compounding with turn 2 produces a 0.81 discount factor at depth 3
	return bestScore3 * 0.90
}

// generateturn3ouractions selects candidate attacking moves for 3-ply tactical resolution.
func generateTurn3OurActions(s *SimulatedState) []SimAction {
	var actions []SimAction
	if len(s.OurActive.Moves) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   "bodyslam",
			MoveData: GetMoveData("bodyslam"),
		})
		return actions
	}

	for _, m := range s.OurActive.Moves {
		mData := GetMoveData(m)
		// skip non-damaging status and hazard moves at depth 3 quiescence
		if mData.Category == CategoryStatus && !mData.IsHealing {
			continue
		}
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   cleanID(m),
			MoveData: mData,
		})
		if len(actions) >= 2 {
			break
		}
	}

	if len(actions) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   s.OurActive.Moves[0],
			MoveData: GetMoveData(s.OurActive.Moves[0]),
		})
	}

	return actions
}

// generateturn3oppactions selects candidate opponent attacks for 3-ply tactical resolution.
func generateTurn3OppActions(s *SimulatedState) []SimAction {
	var actions []SimAction
	if s.OppActive.LockedMove != "" {
		clean := cleanID(s.OppActive.LockedMove)
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   clean,
			MoveData: GetMoveData(clean),
		})
		return actions
	}

	if len(s.OppActive.Moves) > 0 {
		for _, m := range s.OppActive.Moves {
			clean := cleanID(m)
			mData := GetMoveData(clean)
			if mData.Category == CategoryStatus && !mData.IsHealing {
				continue
			}
			actions = append(actions, SimAction{
				Type:     actionMove,
				MoveID:   clean,
				MoveData: mData,
			})
			if len(actions) >= 2 {
				break
			}
		}
		if len(actions) > 0 {
			return actions
		}
	}

	// fallback attacks based on opponent types
	oppTypes := s.OppActive.Types()
	for _, t := range oppTypes {
		switch t {
		case "fire":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "flamethrower", MoveData: GetMoveData("flamethrower")})
		case "water":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "surf", MoveData: GetMoveData("surf")})
		case "electric":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "thunderbolt", MoveData: GetMoveData("thunderbolt")})
		case "grass":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "energyball", MoveData: GetMoveData("energyball")})
		default:
			actions = append(actions, SimAction{Type: actionMove, MoveID: "bodyslam", MoveData: GetMoveData("bodyslam")})
		}
		if len(actions) >= 2 {
			break
		}
	}

	return actions
}

// decideendgameterminal solves endgame positions (<= 2 pokemon per side) up to 6 plies deep.
func (e *MinimaxEngine) decideEndgameTerminal(state *SimulatedState, ourActions, oppActions []SimAction, maxDepth int) BattleDecision {
	bestScore := -999999.0
	bestAction := ourActions[0]

	for _, ourAct := range ourActions {
		worstScore := 999999.0
		totalScore := 0.0

		for _, oppAct := range oppActions {
			simResult := simulateTurn(state, ourAct, oppAct)
			score := searchEndgame(simResult, 2, maxDepth)

			if score < worstScore {
				worstScore = score
			}
			totalScore += score
		}

		avgScore := totalScore / float64(len(oppActions))
		compositeScore := (0.80 * worstScore) + (0.20 * avgScore)

		if compositeScore > bestScore {
			bestScore = compositeScore
			bestAction = ourAct
		}
	}

	return actionToDecision(bestAction)
}

// searchendgame performs deep recursive minimax search to find forced terminal win/loss lines.
func searchEndgame(s *SimulatedState, depth int, maxDepth int) float64 {
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
		// definitive loss: prefer lines that delay loss as long as possible
		return -10000.0 - (s.OppActive.HPPercent * 100.0) + float64(depth)*50.0
	}
	if oppAlive == 0 {
		// definitive victory: strictly prefer faster knockouts
		return 10000.0 + (s.OurActive.HPPercent * 100.0) - float64(depth)*50.0
	}

	if depth >= maxDepth {
		discount := 1.0
		for i := 1; i < depth; i++ {
			discount *= 0.90
		}
		return EvaluateBattleState(s) * discount
	}

	ourActs := generateEndgameOurActions(s)
	if len(ourActs) == 0 {
		discount := 1.0
		for i := 1; i < depth; i++ {
			discount *= 0.90
		}
		return EvaluateBattleState(s) * discount
	}

	oppActs := generateEndgameOppActions(s)
	if len(oppActs) == 0 {
		discount := 1.0
		for i := 1; i < depth; i++ {
			discount *= 0.90
		}
		return EvaluateBattleState(s) * discount
	}

	bestScore := -999999.0
	for _, ourAct := range ourActs {
		worstScore := 999999.0
		totalScore := 0.0

		for _, oppAct := range oppActs {
			simResult := simulateTurn(s, ourAct, oppAct)
			score := searchEndgame(simResult, depth+1, maxDepth)

			if score < worstScore {
				worstScore = score
			}
			totalScore += score
		}

		avgScore := totalScore / float64(len(oppActs))
		compositeScore := (0.80 * worstScore) + (0.20 * avgScore)
		if compositeScore > bestScore {
			bestScore = compositeScore
		}
	}

	return bestScore
}

// generateendgameouractions selects candidate actions for endgame recursive search.
func generateEndgameOurActions(s *SimulatedState) []SimAction {
	var actions []SimAction

	// if active pokemon fainted, only viable actions are switching in a surviving bench pokemon
	if s.OurActive.Fainted {
		for i, p := range s.OurBench {
			if !p.Fainted && p.HPPercent > 0.001 {
				actions = append(actions, SimAction{
					Type:          actionSwitch,
					SwitchSlot:    i + 2,
					SwitchSpecies: p.Species,
					SwitchIndex:   i,
				})
			}
		}
		return actions
	}

	// candidate moves for active pokemon
	if len(s.OurActive.Moves) == 0 {
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   "bodyslam",
			MoveData: GetMoveData("bodyslam"),
		})
		return actions
	}

	for _, m := range s.OurActive.Moves {
		mData := GetMoveData(m)
		// skip repeated entry hazard placement in endgame
		if mData.IsHazard && s.OppHazards[cleanID(m)] > 0 {
			continue
		}
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   cleanID(m),
			MoveData: mData,
		})
		if len(actions) >= 3 {
			break
		}
	}

	// voluntary switch in 2v1 / 2v2 endgame if bench pokemon is healthy
	for i, benchPoke := range s.OurBench {
		if !benchPoke.Fainted && benchPoke.HPPercent > 0.40 {
			actions = append(actions, SimAction{
				Type:          actionSwitch,
				SwitchSlot:    benchPoke.Slot,
				SwitchSpecies: benchPoke.Species,
				SwitchIndex:   i,
			})
			break
		}
	}

	if len(actions) == 0 {
		moveID := "struggle"
		if len(s.OurActive.Moves) > 0 {
			moveID = s.OurActive.Moves[0]
		}
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   moveID,
			MoveData: GetMoveData(moveID),
		})
	}

	return actions
}

// generateendgameoppactions selects candidate opponent actions for endgame recursive search.
func generateEndgameOppActions(s *SimulatedState) []SimAction {
	var actions []SimAction

	// if opponent active fainted, switch in surviving bench pokemon
	if s.OppActive.Fainted {
		for i, p := range s.OppBench {
			if !p.Fainted && p.HPPercent > 0.001 {
				actions = append(actions, SimAction{
					Type:          actionSwitch,
					SwitchSpecies: p.Species,
					SwitchIndex:   i,
				})
			}
		}
		return actions
	}

	// locked into choice item move
	if s.OppActive.LockedMove != "" {
		clean := cleanID(s.OppActive.LockedMove)
		actions = append(actions, SimAction{
			Type:     actionMove,
			MoveID:   clean,
			MoveData: GetMoveData(clean),
		})
		return actions
	}

	if len(s.OppActive.Moves) > 0 {
		for _, m := range s.OppActive.Moves {
			clean := cleanID(m)
			actions = append(actions, SimAction{
				Type:     actionMove,
				MoveID:   clean,
				MoveData: GetMoveData(clean),
			})
			if len(actions) >= 3 {
				break
			}
		}
		return actions
	}

	// fallback attacks based on opponent types
	oppTypes := s.OppActive.Types()
	for _, t := range oppTypes {
		switch t {
		case "fire":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "flamethrower", MoveData: GetMoveData("flamethrower")})
		case "water":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "surf", MoveData: GetMoveData("surf")})
		case "electric":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "thunderbolt", MoveData: GetMoveData("thunderbolt")})
		case "grass":
			actions = append(actions, SimAction{Type: actionMove, MoveID: "energyball", MoveData: GetMoveData("energyball")})
		default:
			actions = append(actions, SimAction{Type: actionMove, MoveID: "bodyslam", MoveData: GetMoveData("bodyslam")})
		}
		if len(actions) >= 2 {
			break
		}
	}

	return actions
}

type scoredAction struct {
	action SimAction
	score  float64
}

// selectmixedstrategy chooses among top-scoring candidate actions using a game-theoretic probability distribution.
// when a move is decisively superior (gap >= 1.5), it is selected deterministically.
// when candidate actions are in a close standoff (gap < 1.5), it samples proportionally to remain unexploitable.
func selectMixedStrategy(scored []scoredAction, bestScore float64, fallback SimAction) SimAction {
	if len(scored) <= 1 {
		return fallback
	}

	var contenders []scoredAction
	for _, sa := range scored {
		if sa.score >= bestScore-1.5 {
			contenders = append(contenders, sa)
		}
	}

	if len(contenders) <= 1 {
		return fallback
	}

	temp := 1.0
	var weights []float64
	totalWeight := 0.0
	for _, c := range contenders {
		w := math.Exp((c.score - bestScore) / temp)
		weights = append(weights, w)
		totalWeight += w
	}

	if totalWeight <= 0 {
		return fallback
	}

	r := rand.Float64() * totalWeight
	cum := 0.0
	for i, w := range weights {
		cum += w
		if r <= cum {
			return contenders[i].action
		}
	}

	return fallback
}

// shouldconsiderterastallize implements resource economy to prevent burning a one-time tera prematurely.
func shouldConsiderTerastallize(activeReq RequestActive, mData MoveData, state *SimulatedState) bool {
	// 1. never burn tera on non-setup status moves (e.g. thunder wave, toxic, hazards)
	if mData.Category == CategoryStatus && !mData.IsSetup {
		return false
	}

	// count opponent alive pokemon
	oppAlive := 0
	if !state.OppActive.Fainted && state.OppActive.HPPercent > 0.001 {
		oppAlive++
	}
	for _, p := range state.OppBench {
		if !p.Fainted && p.HPPercent > 0.001 {
			oppAlive++
		}
	}

	// 2. low hp restriction: do not waste one-time tera on a dying pokemon (< 35% hp)
	// unless it is an endgame turn where securing a knockout wins the entire match
	if state.OurActive.HPPercent < 0.35 {
		return oppAlive <= 1
	}

	// 3. offensive synergy: move type matches tera type (2.0x stab boost)
	teraType := cleanID(activeReq.CanTerastallize)
	if strings.EqualFold(mData.Type, teraType) {
		return true
	}

	// 4. stat-boosted sweeper timing: pokemon has offensive boosts and is sweeping
	if state.OurActive.Boosts != nil && (state.OurActive.Boosts["atk"] >= 1 || state.OurActive.Boosts["spa"] >= 1) {
		return true
	}

	// 5. designated primary sweeper preservation
	baseStats := GetSpeciesBaseStats(state.OurActive.Species)
	if (baseStats["atk"] >= 115 || baseStats["spa"] >= 115) && baseStats["spe"] >= 80 {
		return true
	}

	// 6. defensive flip: if opponent's active has super-effective coverage against our current typing
	for _, oppMove := range state.OppActive.Moves {
		oppMData := GetMoveData(oppMove)
		currentEff := GetMultipleEffectiveness(oppMData.Type, state.OurActive.Types()...)
		teraEff := GetMultipleEffectiveness(oppMData.Type, teraType)
		if currentEff >= 2.0 && teraEff <= 1.0 {
			return true // defensive tera sheds fatal weakness into resistance/neutral
		}
	}

	return false
}
