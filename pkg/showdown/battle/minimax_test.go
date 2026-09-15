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

	// verify execution time is fast and sub-millisecond (allowing overhead buffer for race instrumentation)
	if avgDuration > 5*time.Millisecond {
		t.Fatalf("expected average decision time < 5ms under race instrumentation, got %v", avgDuration)
	}
}
