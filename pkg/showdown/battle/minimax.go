package battle

import (
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
	bestSlot := 2
	bestScore := -99999.0

	for i := 1; i < len(pokemonList); i++ {
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

	bestScore := -999999.0
	bestAction := ourActions[0]

	for _, ourAct := range ourActions {
		worstScore := 999999.0
		totalScore := 0.0

		for _, oppAct := range oppActions {
			simResult := simulateTurn(state, ourAct, oppAct)
			score := EvaluateBattleState(simResult)

			if score < worstScore {
				worstScore = score
			}
			totalScore += score
		}

		avgScore := totalScore / float64(len(oppActions))
		// maximin score with weighted average expectation
		compositeScore := (0.75 * worstScore) + (0.25 * avgScore)

		if compositeScore > bestScore {
			bestScore = compositeScore
			bestAction = ourAct
		}
	}

	return actionToDecision(bestAction)
}

// buildsimulatedstate constructs a snapshot of the current battle state.
func buildSimulatedState(b *Battle, req BattleRequest) *SimulatedState {
	s := &SimulatedState{
		Weather:             b.Weather,
		Terrain:             b.Terrain,
		ConsecutiveProtects: b.ConsecutiveProtects,
		OurHazards:          make(map[string]int),
		OppHazards:          make(map[string]int),
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

	// our active pokemon
	if len(req.Side.Pokemon) > 0 {
		active := req.Side.Pokemon[0]
		s.OurActive = SimulatedPokemon{
			Species:   active.Species(),
			HPPercent: active.HPPercent(),
			Status:    active.Status(),
			Item:      active.Item,
			Ability:   active.Ability,
			Fainted:   active.IsFainted(),
			Boosts:    make(map[string]int),
			Volatiles: make(map[string]bool),
		}
		for k, v := range b.MyBoosts {
			s.OurActive.Boosts[k] = v
		}
		for k, v := range b.MyVolatiles {
			s.OurActive.Volatiles[k] = v
		}

		// bench pokemon
		for i := 1; i < len(req.Side.Pokemon); i++ {
			p := req.Side.Pokemon[i]
			s.OurBench = append(s.OurBench, SimulatedPokemon{
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
		Species:   b.OpponentActive.Species,
		HPPercent: b.OpponentActive.HPPercent,
		Status:    b.OpponentActive.Status,
		Item:      b.OpponentActive.Item,
		Ability:   b.OpponentActive.Ability,
		Fainted:   b.OpponentActive.HPPercent <= 0.001,
		Boosts:    make(map[string]int),
		Volatiles: make(map[string]bool),
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

			// terastallize option if available
			if activeReq.CanTerastallize != "" {
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

	// voluntary switches
	for i, benchPoke := range state.OurBench {
		if !benchPoke.Fainted && benchPoke.HPPercent > 0.05 {
			actions = append(actions, SimAction{
				Type:          actionSwitch,
				SwitchSlot:    i + 2, // showdown bench slot is 1-indexed starting at 2
				SwitchSpecies: benchPoke.Species,
				SwitchIndex:   i,
			})
		}
	}

	return actions
}

// generateopponentactions predicts opponent actions from revealed moves and random sets.
func generateOpponentActions(b *Battle, state *SimulatedState) []SimAction {
	var actions []SimAction
	seenMoves := make(map[string]bool)

	// 1. revealed moves
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

	// 2. deduce likely moves from official random battle sets if needed
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
		ourFirst = ourSpe >= oppSpe
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
	cp.Boosts = make(map[string]int)
	for k, v := range p.Boosts {
		cp.Boosts[k] = v
	}
	cp.Volatiles = make(map[string]bool)
	for k, v := range p.Volatiles {
		cp.Volatiles[k] = v
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
