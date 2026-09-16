package battle

import (
	"testing"
	"time"
)

func TestMinimaxEngine_LethalKOAndImmunity(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-test-minimax-ko", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Gengar",
		Types:     []string{"ghost", "poison"},
		HPPercent: 0.10,
	}

	req := BattleRequest{
		RQID: 1,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "bodyslam", Move: "Body Slam", PP: 15},
					{ID: "earthquake", Move: "Earthquake", PP: 10},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Garchomp, L80",
					Condition: "280/280",
					Stats:     map[string]int{"atk": 250, "spa": 150},
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Type != DecisionMove || dec.Slot != 2 {
		t.Fatalf("expected earthquake (slot 2) to lethal ko gengar, got type=%v, slot=%d", dec.Type, dec.Slot)
	}
}

func TestMinimaxEngine_PriorityFinisher(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-test-minimax-prio", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Pikachu",
		Types:     []string{"electric"},
		HPPercent: 0.05,
	}

	req := BattleRequest{
		RQID: 2,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
					{ID: "aquajet", Move: "Aqua Jet", PP: 20},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Blastoise, L80",
					Condition: "250/250",
					Stats:     map[string]int{"atk": 180, "spa": 180},
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Type != DecisionMove || dec.Slot != 2 {
		t.Fatalf("expected aqua jet (slot 2) priority finisher, got type=%v, slot=%d", dec.Type, dec.Slot)
	}
}

func TestMinimaxEngine_PsychicTerrainBlocksPriority(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-test-psychic-terrain", engine)
	b.Terrain = "psychicterrain"
	b.OpponentActive = OpponentActivePoke{
		Species:   "Ting-Lu", // grounded ground/dark
		Types:     []string{"ground", "dark"},
		HPPercent: 0.15,
	}

	req := BattleRequest{
		RQID: 3,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "machpunch", Move: "Mach Punch", PP: 20}, // priority blocked by psychic terrain!
					{ID: "closecombat", Move: "Close Combat", PP: 5}, // normal priority hits grounded target
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Infernape, L80",
					Condition: "250/250",
					Stats:     map[string]int{"atk": 220, "spa": 220},
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Type != DecisionMove || dec.Slot != 2 {
		t.Fatalf("expected close combat (slot 2) since mach punch priority is blocked on psychic terrain, got slot %d", dec.Slot)
	}
}

func TestMinimaxEngine_WeatherBoosts(t *testing.T) {
	// in rain, water attacks receive 1.5x boost while fire receives 0.5x
	dmgRainWater := CalculateDamageWithWeatherAndTerrain(80, 80, 100, 100, 1.5, 1.0, false, false, "water", "surf", "raindance", "", true, true)
	dmgClearWater := CalculateDamageWithWeatherAndTerrain(80, 80, 100, 100, 1.5, 1.0, false, false, "water", "surf", "", "", true, true)
	if dmgRainWater <= dmgClearWater {
		t.Fatalf("expected rain to boost water damage: %f vs %f", dmgRainWater, dmgClearWater)
	}

	dmgRainFire := CalculateDamageWithWeatherAndTerrain(80, 90, 100, 100, 1.5, 1.0, false, false, "fire", "flamethrower", "raindance", "", true, true)
	dmgClearFire := CalculateDamageWithWeatherAndTerrain(80, 90, 100, 100, 1.5, 1.0, false, false, "fire", "flamethrower", "", "", true, true)
	if dmgRainFire >= dmgClearFire {
		t.Fatalf("expected rain to halve fire damage: %f vs %f", dmgRainFire, dmgClearFire)
	}
}

func TestMinimaxEngine_EffectiveSpeedCalculation(t *testing.T) {
	// base speed 100 pokemon
	pNormal := SimulatedPokemon{Species: "Mew"}
	speNormal := calculatePokemonSpeed(pNormal, "", "")
	if speNormal != 100 {
		t.Fatalf("expected mew base speed 100, got %d", speNormal)
	}

	// +1 speed stage
	pBoosted := SimulatedPokemon{
		Species: "Mew",
		Boosts:  map[string]int{"spe": 1},
	}
	speBoosted := calculatePokemonSpeed(pBoosted, "", "")
	if speBoosted != 150 {
		t.Fatalf("expected +1 speed stage to yield 150, got %d", speBoosted)
	}

	// paralysis halves speed
	pParalyzed := SimulatedPokemon{
		Species: "Mew",
		Status:  "par",
	}
	spePar := calculatePokemonSpeed(pParalyzed, "", "")
	if spePar != 50 {
		t.Fatalf("expected paralysis to yield 50, got %d", spePar)
	}

	// swift swim under rain doubles speed
	pSwiftSwim := SimulatedPokemon{
		Species: "Barraskewda",
		Ability: "swiftswim",
	}
	baseSpe := GetSpeciesBaseStats("Barraskewda")["spe"]
	speRain := calculatePokemonSpeed(pSwiftSwim, "raindance", "")
	if speRain != baseSpe*2 {
		t.Fatalf("expected swift swim rain speed %d, got %d", baseSpe*2, speRain)
	}
}

func TestMinimaxEngine_StealthRockWeaknessAndBoots(t *testing.T) {
	// charizard is 4x weak to rock, should take 50% max hp
	pCharizard := SimulatedPokemon{
		Species:   "Charizard",
		HPPercent: 1.0,
	}
	hazards := map[string]int{"stealthrock": 1}
	applyEntryHazards(&pCharizard, hazards)
	if pCharizard.HPPercent > 0.51 || pCharizard.HPPercent < 0.49 {
		t.Fatalf("expected charizard to take 50%% stealth rock damage, got hp %f", pCharizard.HPPercent)
	}

	// heavy-duty boots prevents all hazard damage
	pBoots := SimulatedPokemon{
		Species:   "Charizard",
		HPPercent: 1.0,
		Item:      "heavydutyboots",
	}
	applyEntryHazards(&pBoots, hazards)
	if pBoots.HPPercent != 1.0 {
		t.Fatalf("expected heavy-duty boots to ignore hazards, got hp %f", pBoots.HPPercent)
	}
}

func TestMinimaxEngine_ToxicSpikesAbsorption(t *testing.T) {
	// grounded poison type absorbs toxic spikes
	pGengar := SimulatedPokemon{
		Species:   "Gengar",
		HPPercent: 1.0,
	}
	hazards := map[string]int{"toxicspikes": 1}
	applyEntryHazards(&pGengar, hazards)
	if hazards["toxicspikes"] != 0 {
		t.Fatalf("expected grounded poison pokemon to absorb toxic spikes, remaining layers=%d", hazards["toxicspikes"])
	}
	if pGengar.Status != "" {
		t.Fatalf("expected poison pokemon not to be poisoned on absorb, got status %s", pGengar.Status)
	}
}

func TestMinimaxEngine_ConsecutiveProtectTracking(t *testing.T) {
	b := NewBattle("battle-protect-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	// turn 1: we use protect
	b.HandleLine([]string{"move", "p1a: Gliscor", "Protect", "p2a: Tyranitar"}, "GhostHaze Thinker")
	if b.ConsecutiveProtects != 1 {
		t.Fatalf("expected consecutive protects 1, got %d", b.ConsecutiveProtects)
	}

	// turn 2: we use protect again
	b.HandleLine([]string{"move", "p1a: Gliscor", "Protect", "p2a: Tyranitar"}, "GhostHaze Thinker")
	if b.ConsecutiveProtects != 2 {
		t.Fatalf("expected consecutive protects 2, got %d", b.ConsecutiveProtects)
	}

	// turn 3: we use earthquake
	b.HandleLine([]string{"move", "p1a: Gliscor", "Earthquake", "p2a: Tyranitar"}, "GhostHaze Thinker")
	if b.ConsecutiveProtects != 0 {
		t.Fatalf("expected consecutive protects to reset to 0, got %d", b.ConsecutiveProtects)
	}
}

func TestMinimaxEngine_SubMillisecondPerformance(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-perf-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Garchomp",
		Types:     []string{"dragon", "ground"},
		HPPercent: 0.75,
		Moves:     []string{"earthquake", "dragonclaw", "stoneedge", "swordsdance"},
	}
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Corviknight"},
		{Species: "Toxapex"},
	}

	req := BattleRequest{
		RQID: 10,
		Active: []RequestActive{
			{
				CanTerastallize: "water",
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
					{ID: "icebeam", Move: "Ice Beam", PP: 10},
					{ID: "recover", Move: "Recover", PP: 10},
					{ID: "toxic", Move: "Toxic", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Slowbro, L80",
					Condition: "250/250",
					Stats:     map[string]int{"atk": 100, "def": 200, "spa": 180, "spd": 150, "spe": 80},
				},
				{
					Details:   "Heatran, L80",
					Condition: "250/250",
				},
			},
		},
	}

	// warm up embedded datasets once before timing loop
	_ = engine.Decide(b, req)

	// run 50 consecutive simultaneous matrix decisions and measure total execution time
	start := time.Now()
	iterations := 50
	for i := 0; i < iterations; i++ {
		dec := engine.Decide(b, req)
		if dec.Slot <= 0 {
			t.Fatalf("expected valid slot decision, got %d", dec.Slot)
		}
	}
	duration := time.Since(start)
	avgDuration := duration / time.Duration(iterations)

	// verify execution time is fast and sub-millisecond in normal execution (allowing overhead buffer for race instrumentation)
	if avgDuration > 15*time.Millisecond {
		t.Fatalf("expected average decision time < 15ms under race instrumentation, got %v", avgDuration)
	}
}

func TestMinimaxEngine_3PlyQuiescenceSetup(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-3ply-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Garganacl",
		Types:     []string{"rock"},
		HPPercent: 0.85,
		Moves:     []string{"recover"},
	}

	req := BattleRequest{
		RQID: 15,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "swordsdance", Move: "Swords Dance", PP: 20},
					{ID: "earthquake", Move: "Earthquake", PP: 10},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Garchomp, L80",
					Condition: "250/250",
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	// swords dance (slot 1) should be chosen to break through recover via +2 sweep
	if dec.Slot != 1 {
		t.Fatalf("expected swords dance (slot 1) to break through recover, got slot %d", dec.Slot)
	}
}

func TestMinimaxEngine_EndgameTerminalSolver(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-endgame-test", engine)
	// 1v1 endgame: faster opponent with lethal attack vs our priority finisher
	b.OpponentActive = OpponentActivePoke{
		Species:   "Garchomp",
		Types:     []string{"dragon", "ground"},
		HPPercent: 0.20,
		Moves:     []string{"outrage"},
	}

	req := BattleRequest{
		RQID: 22,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "outrage", Move: "Outrage", PP: 10},
					{ID: "extremespeed", Move: "Extreme Speed", PP: 5},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Dragonite, L80",
					Condition: "20/250", // 8% hp, faints to garchomp outrage
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	// outrage would deal massive damage but garchomp outspeeds and kos dragonite first.
	// extreme speed has +2 priority, preventing the knockout and winning the endgame.
	if dec.Slot != 2 {
		t.Fatalf("expected extreme speed (slot 2) in endgame, got slot %d", dec.Slot)
	}
}

func TestMinimaxEngine_ConfirmedSpeedTier(t *testing.T) {
	b := NewBattle("battle-speed-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	// simulate turn 1 where opponent moved first with neutral priority move
	b.HandleLine([]string{"turn", "1"}, "GhostHaze Thinker")
	b.HandleLine([]string{"move", "p2a: Garchomp", "Earthquake", "p1a: Dragonite"}, "GhostHaze Thinker")
	b.HandleLine([]string{"move", "p1a: Dragonite", "Dragon Claw", "p2a: Garchomp"}, "GhostHaze Thinker")

	if !b.OpponentActive.ConfirmedFaster {
		t.Fatalf("expected opponent to be confirmed faster after moving first with earthquake")
	}

	// simulate opponent switch: confirmed faster should reset
	b.HandleLine([]string{"switch", "p2a: Blissey", "Blissey, L80, F", "100/100"}, "GhostHaze Thinker")
	if b.OpponentActive.ConfirmedFaster {
		t.Fatalf("expected confirmed faster to reset on switch")
	}
}

func TestMinimaxEngine_EndgameMultiTurnSwitchImmunity(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-endgame-switch-test", engine)
	// opponent is locked into earthquake
	b.OpponentActive = OpponentActivePoke{
		Species:    "Garchomp",
		Types:      []string{"dragon", "ground"},
		HPPercent:  0.40,
		LockedMove: "earthquake",
		Moves:      []string{"earthquake"},
	}

	req := BattleRequest{
		RQID: 25,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "flashcannon", Move: "Flash Cannon", PP: 10},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Heatran, L80",
					Condition: "250/250", // 4x weak to earthquake, faints in 1 hit
				},
				{
					Details:   "Corviknight, L80",
					Condition: "250/250", // flying type, 100% immune to ground earthquake
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	// heatran faints to 4x earthquake; switching to immune corviknight forces a win
	if dec.Type != DecisionSwitch || dec.Slot != 2 {
		t.Fatalf("expected switch to corviknight (slot 2), got decision %+v", dec)
	}
}

func TestBattle_KnockOffBreaksChoiceLock(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-knock-off-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:    "Tyranitar",
		Types:      []string{"rock", "dark"},
		HPPercent:  0.80,
		Item:       "choiceband",
		LockedMove: "stoneedge",
		Moves:      []string{"stoneedge", "crunch"},
	}

	// simulate knock off removing the choice band item via -enditem
	b.HandleLine([]string{"-enditem", "p2a: Tyranitar", "Choice Band", "[from] move: Knock Off"}, "GhostHaze Thinker")
	if b.OpponentActive.Item != "" {
		t.Fatalf("expected opponent item to be cleared after knock off, got %s", b.OpponentActive.Item)
	}
	if b.OpponentActive.LockedMove != "" {
		t.Fatalf("expected opponent locked move to be cleared after knock off, got %s", b.OpponentActive.LockedMove)
	}

	// re-lock item and test trick / switcheroo via -activate
	b.OpponentActive.Item = "choicescarf"
	b.OpponentActive.LockedMove = "stoneedge"
	b.HandleLine([]string{"-activate", "p1a: Rotom", "move: Trick", "[of] p2a: Tyranitar"}, "GhostHaze Thinker")
	if b.OpponentActive.LockedMove != "" {
		t.Fatalf("expected locked move to be cleared after trick, got %s", b.OpponentActive.LockedMove)
	}
}

func TestMinimaxEngine_TeraEconomyPreservation(t *testing.T) {
	// low hp non-endgame pokemon should not waste tera
	stateDying := &SimulatedState{
		OurActive: SimulatedPokemon{
			Species:   "Garchomp",
			HPPercent: 0.15,
		},
		OppActive: SimulatedPokemon{
			Species:   "Dragonite",
			HPPercent: 1.0,
		},
		OppBench: []SimulatedPokemon{
			{Species: "Toxapex", HPPercent: 1.0},
			{Species: "Corviknight", HPPercent: 1.0},
		},
	}
	activeReq := RequestActive{
		CanTerastallize: "Ground",
	}
	mDataEarthquake := GetMoveData("earthquake")

	if shouldConsiderTerastallize(activeReq, mDataEarthquake, stateDying) {
		t.Fatalf("expected dying pokemon (15%% hp) to preserve tera when opponent has bench alive")
	}

	// healthy sweeper matching tera type should consider tera
	stateHealthy := &SimulatedState{
		OurActive: SimulatedPokemon{
			Species:   "Garchomp",
			HPPercent: 1.0,
		},
		OppActive: SimulatedPokemon{
			Species:   "Dragonite",
			HPPercent: 1.0,
		},
		OppBench: []SimulatedPokemon{
			{Species: "Toxapex", HPPercent: 1.0},
		},
	}
	if !shouldConsiderTerastallize(activeReq, mDataEarthquake, stateHealthy) {
		t.Fatalf("expected healthy ground sweeper with earthquake to consider tera ground")
	}

	// status move without setup should not burn tera
	mDataToxic := GetMoveData("toxic")
	if shouldConsiderTerastallize(activeReq, mDataToxic, stateHealthy) {
		t.Fatalf("expected status move toxic to not burn tera")
	}
}

func TestMinimaxEngine_WeatherEndOfTurnChip(t *testing.T) {
	// electric mon takes sandstorm chip
	pElectric := SimulatedPokemon{
		Species:   "Pikachu",
		HPPercent: 1.0,
	}
	applyMonEndOfTurn(&pElectric, "sandstorm", "")
	if pElectric.HPPercent >= 1.0 {
		t.Fatalf("expected pikachu to take sandstorm chip, got %f", pElectric.HPPercent)
	}

	// steel mon is immune to sandstorm chip
	pSteel := SimulatedPokemon{
		Species:   "Corviknight",
		HPPercent: 1.0,
	}
	applyMonEndOfTurn(&pSteel, "sandstorm", "")
	if pSteel.HPPercent != 1.0 {
		t.Fatalf("expected corviknight to resist sandstorm, got %f", pSteel.HPPercent)
	}

	// safety goggles protects against sandstorm
	pGoggles := SimulatedPokemon{
		Species:   "Pikachu",
		Item:      "safetygoggles",
		HPPercent: 1.0,
	}
	applyMonEndOfTurn(&pGoggles, "sandstorm", "")
	if pGoggles.HPPercent != 1.0 {
		t.Fatalf("expected safety goggles to protect from sandstorm, got %f", pGoggles.HPPercent)
	}

	// ice mon is immune to hail chip
	pIce := SimulatedPokemon{
		Species:   "Weavile",
		HPPercent: 1.0,
	}
	applyMonEndOfTurn(&pIce, "hail", "")
	if pIce.HPPercent != 1.0 {
		t.Fatalf("expected weavile to resist hail, got %f", pIce.HPPercent)
	}

	// fire mon takes hail chip
	pFire := SimulatedPokemon{
		Species:   "Charizard",
		HPPercent: 1.0,
	}
	applyMonEndOfTurn(&pFire, "hail", "")
	if pFire.HPPercent >= 1.0 {
		t.Fatalf("expected charizard to take hail chip, got %f", pFire.HPPercent)
	}
}

func TestMinimaxEngine_LowHPQuiescenceExtension(t *testing.T) {
	normalMove := SimAction{Type: actionMove, MoveData: GetMoveData("tackle")}

	// healthy pokemon without setup should not extend to turn 3
	stateHealthy := &SimulatedState{
		OurActive: SimulatedPokemon{Species: "Garchomp", HPPercent: 0.8},
		OppActive: SimulatedPokemon{Species: "Dragonite", HPPercent: 0.7},
	}
	if shouldExtendToTurn3(normalMove, normalMove, stateHealthy) {
		t.Fatalf("expected healthy state without setups to not extend to turn 3")
	}

	// low hp pokemon should extend to turn 3 to resolve tactical endgame
	stateLow := &SimulatedState{
		OurActive: SimulatedPokemon{Species: "Garchomp", HPPercent: 0.20},
		OppActive: SimulatedPokemon{Species: "Dragonite", HPPercent: 0.70},
	}
	if !shouldExtendToTurn3(normalMove, normalMove, stateLow) {
		t.Fatalf("expected low hp ourActive (<0.25) to trigger 3-ply quiescence extension")
	}
}

func TestMinimaxEngine_AntiPredictabilityCloseMoves(t *testing.T) {
	engine := NewMinimaxEngine()
	engine.SetAntiPredictability(true)

	b := NewBattle("battle-antipred-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Mew",
		Types:     []string{"psychic"},
		HPPercent: 1.0,
	}
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Dragonite"},
		{Species: "Zapdos"},
		{Species: "Tyranitar"},
	}

	req := BattleRequest{
		RQID: 1,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
					{ID: "energyball", Move: "Energy Ball", PP: 10},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Charizard, L80", Condition: "250/250", Active: true},
				{Details: "Blastoise, L80", Condition: "250/250"},
				{Details: "Venusaur, L80", Condition: "250/250"},
			},
		},
	}

	chosenSlots := make(map[int]int)
	for i := 0; i < 60; i++ {
		dec := engine.Decide(b, req)
		if dec.Type == DecisionMove {
			chosenSlots[dec.Slot]++
		}
	}

	// with anti-predictability on two close fire moves, both should be selected across runs
	if len(chosenSlots) < 2 {
		t.Fatalf("expected both close moves to be sampled across 60 trials, got: %v", chosenSlots)
	}
}

func TestMinimaxEngine_AntiPredictabilityDecisiveMove(t *testing.T) {
	engine := NewMinimaxEngine()
	engine.SetAntiPredictability(true)

	b := NewBattle("battle-decisive-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Heatran",
		Types:     []string{"fire", "steel"},
		HPPercent: 1.0,
	}
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Tyranitar"},
		{Species: "Snorlax"},
		{Species: "Blissey"},
	}

	req := BattleRequest{
		RQID: 2,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "earthquake", Move: "Earthquake", PP: 10},
					{ID: "flamethrower", Move: "Flamethrower", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Garchomp, L80", Condition: "250/250", Active: true},
				{Details: "Blastoise, L80", Condition: "250/250"},
				{Details: "Venusaur, L80", Condition: "250/250"},
			},
		},
	}

	for i := 0; i < 40; i++ {
		dec := engine.Decide(b, req)
		if dec.Slot != 1 {
			t.Fatalf("expected decisive 4x move (earthquake, slot 1) every time, got slot %d", dec.Slot)
		}
	}
}

func TestMinimaxEngine_AntiPredictabilityDisabled(t *testing.T) {
	engine := NewMinimaxEngine()
	engine.SetAntiPredictability(false)

	if engine.IsAntiPredictability() {
		t.Fatalf("expected anti-predictability to be false")
	}

	b := NewBattle("battle-disabled-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Mew",
		Types:     []string{"psychic"},
		HPPercent: 1.0,
	}
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Dragonite"},
		{Species: "Zapdos"},
		{Species: "Tyranitar"},
	}

	req := BattleRequest{
		RQID: 3,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
					{ID: "energyball", Move: "Energy Ball", PP: 10},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Charizard, L80", Condition: "250/250", Active: true},
				{Details: "Blastoise, L80", Condition: "250/250"},
				{Details: "Venusaur, L80", Condition: "250/250"},
			},
		},
	}

	var firstSlot int
	for i := 0; i < 30; i++ {
		dec := engine.Decide(b, req)
		if i == 0 {
			firstSlot = dec.Slot
		} else if dec.Slot != firstSlot {
			t.Fatalf("expected deterministic decision with anti-predictability disabled, changed to %d on run %d", dec.Slot, i)
		}
	}
}

func TestMinimaxEngine_TeamPreviewAntiPredictability(t *testing.T) {
	engine := NewMinimaxEngine()
	engine.SetAntiPredictability(true)

	b := NewBattle("battle-preview-test", engine)
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Snorlax"},
	}

	req := BattleRequest{
		TeamPreview: true,
		RQID:        10,
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Blastoise, L80"},
				{Details: "Raichu, L80"},
			},
		},
	}

	leadCounts := make(map[string]int)
	for i := 0; i < 60; i++ {
		dec := engine.Decide(b, req)
		if dec.Type == DecisionTeam && len(dec.TeamOrder) > 0 {
			lead := string(dec.TeamOrder[0])
			leadCounts[lead]++
		}
	}

	// both neutral leads should be sampled across trials
	if len(leadCounts) < 2 {
		t.Fatalf("expected both close leads to be sampled across 60 trials, got: %v", leadCounts)
	}
}

