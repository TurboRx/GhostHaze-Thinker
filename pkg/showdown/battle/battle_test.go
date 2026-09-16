package battle

import (
	"strings"
	"testing"
)

func TestTypechart(t *testing.T) {
	// single type effectiveness
	if eff := GetEffectiveness("fire", "grass"); eff != 2.0 {
		t.Fatalf("expected fire vs grass to be 2.0, got %f", eff)
	}
	if eff := GetEffectiveness("water", "fire"); eff != 2.0 {
		t.Fatalf("expected water vs fire to be 2.0, got %f", eff)
	}
	if eff := GetEffectiveness("electric", "ground"); eff != 0.0 {
		t.Fatalf("expected electric vs ground to be 0.0, got %f", eff)
	}
	if eff := GetEffectiveness("normal", "ghost"); eff != 0.0 {
		t.Fatalf("expected normal vs ghost to be 0.0, got %f", eff)
	}
	if eff := GetEffectiveness("fairy", "dragon"); eff != 2.0 {
		t.Fatalf("expected fairy vs dragon to be 2.0, got %f", eff)
	}
	if eff := GetEffectiveness("dragon", "fairy"); eff != 0.0 {
		t.Fatalf("expected dragon vs fairy to be 0.0, got %f", eff)
	}
	if eff := GetEffectiveness("fighting", "normal"); eff != 2.0 {
		t.Fatalf("expected fighting vs normal to be 2.0, got %f", eff)
	}

	// dual type effectiveness
	if eff := GetMultipleEffectiveness("fire", "grass", "steel"); eff != 4.0 {
		t.Fatalf("expected fire vs grass/steel to be 4.0, got %f", eff)
	}
	if eff := GetMultipleEffectiveness("ice", "dragon", "flying"); eff != 4.0 {
		t.Fatalf("expected ice vs dragon/flying to be 4.0, got %f", eff)
	}
	if eff := GetMultipleEffectiveness("electric", "water", "ground"); eff != 0.0 {
		t.Fatalf("expected electric vs water/ground to be 0.0, got %f", eff)
	}
	if eff := GetMultipleEffectiveness("ground", "electric", "flying"); eff != 0.0 {
		t.Fatalf("expected ground vs electric/flying to be 0.0, got %f", eff)
	}

	// unknown type defaults to neutral
	if eff := GetEffectiveness("cosmic", "water"); eff != 1.0 {
		t.Fatalf("expected unknown type to be 1.0, got %f", eff)
	}

	// species typing mapping
	garchompTypes := GetSpeciesTypes("Garchomp")
	if len(garchompTypes) != 2 || garchompTypes[0] != "dragon" || garchompTypes[1] != "ground" {
		t.Fatalf("unexpected garchomp types: %v", garchompTypes)
	}

	unknownTypes := GetSpeciesTypes("RandomFakemon")
	if len(unknownTypes) != 1 || unknownTypes[0] != "normal" {
		t.Fatalf("expected unknown species to default to normal, got %v", unknownTypes)
	}
}

func TestCalculateDamage(t *testing.T) {
	// neutral physical hit
	dmg := CalculateDamage(80, 80, 100, 100, 1.0, 1.0, false, true)
	if dmg <= 0 {
		t.Fatalf("expected positive damage, got %f", dmg)
	}

	// super effective hit should double damage
	dmgSuper := CalculateDamage(80, 80, 100, 100, 1.0, 2.0, false, true)
	if dmgSuper <= dmg {
		t.Fatalf("expected super effective damage %f > neutral %f", dmgSuper, dmg)
	}

	// stab multiplier
	dmgSTAB := CalculateDamage(80, 80, 100, 100, 1.5, 1.0, false, true)
	if dmgSTAB <= dmg {
		t.Fatalf("expected stab damage %f > non-stab %f", dmgSTAB, dmg)
	}

	// immunity results in zero damage
	dmgImmune := CalculateDamage(80, 80, 100, 100, 1.5, 0.0, false, true)
	if dmgImmune != 0.0 {
		t.Fatalf("expected 0 damage on immunity, got %f", dmgImmune)
	}

	// burn should halve physical damage
	dmgBurnPhysical := CalculateDamage(80, 80, 100, 100, 1.0, 1.0, true, true)
	if dmgBurnPhysical >= dmg {
		t.Fatalf("expected burn to reduce physical damage, got %f vs %f", dmgBurnPhysical, dmg)
	}

	// burn should not halve special damage
	dmgBurnSpecial := CalculateDamage(80, 80, 100, 100, 1.0, 1.0, true, false)
	if dmgBurnSpecial != dmg {
		t.Fatalf("burn should not affect special damage: got %f vs %f", dmgBurnSpecial, dmg)
	}
}

func TestMoveDatabase(t *testing.T) {
	eq := GetMoveData("Earthquake")
	if eq.Type != "ground" || eq.BasePower != 100 || eq.Category != CategoryPhysical {
		t.Fatalf("unexpected data for earthquake: %+v", eq)
	}

	es := GetMoveData("Extreme Speed")
	if es.Priority != 2 {
		t.Fatalf("expected extreme speed priority 2, got %d", es.Priority)
	}

	protect := GetMoveData("protect")
	if protect.Priority != 4 || protect.Category != CategoryStatus {
		t.Fatalf("unexpected data for protect: %+v", protect)
	}

	roost := GetMoveData("roost")
	if !roost.IsHealing {
		t.Fatalf("expected roost to be marked as healing")
	}

	sr := GetMoveData("stealthrock")
	if !sr.IsHazard {
		t.Fatalf("expected stealthrock to be marked as hazard")
	}

	twave := GetMoveData("thunderwave")
	if !twave.IsStatus {
		t.Fatalf("expected thunderwave to be marked as status")
	}

	// fallback for unlisted moves
	mystery := GetMoveData("mysterymove")
	if mystery.BasePower != 60 || mystery.Type != "normal" {
		t.Fatalf("unexpected fallback data: %+v", mystery)
	}
}

func TestRequestTypesAndDetails(t *testing.T) {
	// details parsing
	species, level, gender := ParsePokemonDetails("Garchomp, L85, M")
	if species != "Garchomp" || level != 85 || gender != "M" {
		t.Fatalf("failed to parse pokemon details: %s, %d, %s", species, level, gender)
	}

	species2, level2, gender2 := ParsePokemonDetails("Pikachu")
	if species2 != "Pikachu" || level2 != 100 || gender2 != "N" {
		t.Fatalf("unexpected defaults in details parsing: %s, %d, %s", species2, level2, gender2)
	}

	// request move disabled checks
	m1 := RequestMove{Disabled: false}
	if m1.IsDisabled() {
		t.Fatalf("expected disabled false")
	}
	m2 := RequestMove{Disabled: true}
	if !m2.IsDisabled() {
		t.Fatalf("expected disabled true")
	}
	m3 := RequestMove{Disabled: "disabled"}
	if !m3.IsDisabled() {
		t.Fatalf("expected string disabled to be true")
	}
	m4 := RequestMove{Disabled: nil}
	if m4.IsDisabled() {
		t.Fatalf("expected nil disabled to be false")
	}

	// request pokemon condition checks
	pokeActive := RequestPokemon{
		Details:   "Garchomp, L80",
		Condition: "250/250",
		Active:    true,
	}
	if pokeActive.IsFainted() {
		t.Fatalf("expected poke not to be fainted")
	}
	if hp := pokeActive.HPPercent(); hp != 1.0 {
		t.Fatalf("expected full hp 1.0, got %f", hp)
	}

	pokeInjured := RequestPokemon{
		Condition: "125/250 brn",
	}
	if hp := pokeInjured.HPPercent(); hp != 0.5 {
		t.Fatalf("expected hp 0.5, got %f", hp)
	}
	if st := pokeInjured.Status(); st != "brn" {
		t.Fatalf("expected status brn, got %s", st)
	}

	pokeFainted := RequestPokemon{
		Condition: "0 fnt",
	}
	if !pokeFainted.IsFainted() {
		t.Fatalf("expected poke to be fainted")
	}
	if hp := pokeFainted.HPPercent(); hp != 0.0 {
		t.Fatalf("expected fainted hp to be 0.0, got %f", hp)
	}
}

func TestBattleEngine_TeamPreview(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Charizard"}, // fire/flying: 4x weak to rock, 2x weak to electric/water
	}

	req := BattleRequest{
		TeamPreview: true,
		RQID:        1,
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Venusaur, L80"}, // grass/poison: weak to fire/flying
				{Details: "Zapdos, L80"},   // electric/flying: strong against charizard
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Type != DecisionTeam {
		t.Fatalf("expected decision team, got %v", dec.Type)
	}
	// zapdos is at index 1 (slot 2), should be placed first
	if !strings.HasPrefix(dec.TeamOrder, "2") {
		t.Fatalf("expected zapdos (slot 2) to lead against charizard, got team order %s", dec.TeamOrder)
	}
}

func TestBattleEngine_ForcedSwitch(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Charizard",
		Types:     []string{"fire", "flying"},
		HPPercent: 1.0,
	}

	req := BattleRequest{
		ForceSwitch: []bool{true},
		RQID:        2,
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{Details: "Venusaur, L80", Condition: "0 fnt"}, // slot 1 fainted
				{Details: "Blastoise, L80", Condition: "200/200", Moves: []string{"surf", "icebeam"}}, // slot 2 water
				{Details: "Breloom, L80", Condition: "200/200", Moves: []string{"bulletseed"}},       // slot 3 grass/fighting
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Type != DecisionSwitch {
		t.Fatalf("expected switch decision, got %v", dec.Type)
	}
	// blastoise (slot 2) resists fire and has water moves against charizard
	if dec.Slot != 2 {
		t.Fatalf("expected slot 2 (blastoise) to be chosen, got slot %d", dec.Slot)
	}
}

func TestBattleEngine_ActiveTurn_LethalKOAndImmunity(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	// opponent is gengar (ghost/poison) with low hp (10%)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Gengar",
		Types:     []string{"ghost", "poison"},
		HPPercent: 0.10,
	}

	req := BattleRequest{
		RQID: 3,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "bodyslam", Move: "Body Slam", PP: 15}, // normal: immune against ghost!
					{ID: "earthquake", Move: "Earthquake", PP: 10}, // ground: super effective + lethal ko!
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
	if dec.Type != DecisionMove {
		t.Fatalf("expected move decision, got %v", dec.Type)
	}
	// slot 2 (earthquake) should be selected over slot 1 (body slam) which is immune
	if dec.Slot != 2 {
		t.Fatalf("expected earthquake (slot 2) to be chosen, got slot %d", dec.Slot)
	}
}

func TestBattleEngine_PriorityFinisher(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Pikachu",
		Types:     []string{"electric"},
		HPPercent: 0.05, // very low hp
	}

	req := BattleRequest{
		RQID: 4,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
					{ID: "aquajet", Move: "Aqua Jet", PP: 20}, // priority 1 move securing quick ko
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
	if dec.Type != DecisionMove {
		t.Fatalf("expected move decision, got %v", dec.Type)
	}
	// aqua jet has priority finisher bonus
	if dec.Slot != 2 {
		t.Fatalf("expected aqua jet (slot 2) to be chosen, got slot %d", dec.Slot)
	}
}

func TestBattleEngine_HealingWhenLow(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Blissey",
		Types:     []string{"normal"},
		HPPercent: 1.0,
	}

	// healthy pokemon should attack, not heal
	reqHealthy := BattleRequest{
		RQID: 5,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "recover", Move: "Recover", PP: 10},
					{ID: "surf", Move: "Surf", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Toxapex, L80",
					Condition: "250/250",
				},
			},
		},
	}
	decHealthy := engine.Decide(b, reqHealthy)
	if decHealthy.Slot != 2 {
		t.Fatalf("expected healthy pokemon to attack (slot 2), got slot %d", decHealthy.Slot)
	}

	// critical low hp pokemon should heal
	reqLow := BattleRequest{
		RQID: 6,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "recover", Move: "Recover", PP: 10},
					{ID: "surf", Move: "Surf", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Toxapex, L80",
					Condition: "50/250", // 20% hp
				},
			},
		},
	}
	decLow := engine.Decide(b, reqLow)
	if decLow.Slot != 1 {
		t.Fatalf("expected critically low hp pokemon to recover (slot 1), got slot %d", decLow.Slot)
	}
}

func TestBattleEngine_EntryHazardOnTurn1(t *testing.T) {
	engine := NewDefaultEngine()
	b := NewBattle("battle-test", engine)
	b.Turn = 1
	b.OpponentActive = OpponentActivePoke{
		Species:   "Tyranitar",
		Types:     []string{"rock", "dark"},
		HPPercent: 1.0,
	}
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Pikachu"},
		{Species: "Charizard"},
	}

	req := BattleRequest{
		RQID: 7,
		Active: []RequestActive{
			{
				Moves: []RequestMove{
					{ID: "stealthrock", Move: "Stealth Rock", PP: 20},
					{ID: "ironhead", Move: "Iron Head", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Corviknight, L80",
					Condition: "250/250",
				},
			},
		},
	}

	dec := engine.Decide(b, req)
	if dec.Slot != 1 {
		t.Fatalf("expected stealth rock (slot 1) on turn 1, got slot %d", dec.Slot)
	}
}

func TestBattleHandleLine(t *testing.T) {
	b := NewBattle("battle-gen9randombattle-100", nil)

	// player identification
	b.HandleLine([]string{"player", "p1", "GhostHaze Thinker"}, "GhostHaze Thinker")
	b.HandleLine([]string{"player", "p2", "EnemyUser"}, "GhostHaze Thinker")

	if b.MyPlayerID != "p1" || b.OpponentID != "p2" {
		t.Fatalf("failed player identification: my=%s, opp=%s", b.MyPlayerID, b.OpponentID)
	}

	// tier and turn
	b.HandleLine([]string{"tier", "[Gen 9] Random Battle"}, "GhostHaze Thinker")
	if b.Tier != "[Gen 9] Random Battle" {
		t.Fatalf("unexpected tier: %s", b.Tier)
	}

	b.HandleLine([]string{"turn", "2"}, "GhostHaze Thinker")
	if b.Turn != 2 {
		t.Fatalf("unexpected turn: %d", b.Turn)
	}

	// opponent switch
	b.HandleLine([]string{"switch", "p2a: Garchomp", "Garchomp, L80, M", "100/100"}, "GhostHaze Thinker")
	if b.OpponentActive.Species != "Garchomp" {
		t.Fatalf("unexpected opponent active species: %s", b.OpponentActive.Species)
	}
	if b.OpponentActive.HPPercent != 1.0 {
		t.Fatalf("expected opponent active hp 1.0, got %f", b.OpponentActive.HPPercent)
	}

	// opponent damage and status
	b.HandleLine([]string{"-damage", "p2a: Garchomp", "50/100"}, "GhostHaze Thinker")
	if b.OpponentActive.HPPercent != 0.5 {
		t.Fatalf("expected opponent hp 0.5 after damage, got %f", b.OpponentActive.HPPercent)
	}

	b.HandleLine([]string{"-status", "p2a: Garchomp", "brn"}, "GhostHaze Thinker")
	if b.OpponentActive.Status != "brn" {
		t.Fatalf("expected opponent status brn, got %s", b.OpponentActive.Status)
	}

	// hazards
	b.HandleLine([]string{"-sidestart", "p2: EnemyUser", "move: Stealth Rock"}, "GhostHaze Thinker")
	if !b.OpponentHasHazard("stealthrock") {
		t.Fatalf("expected stealth rock hazard to be active")
	}

	b.HandleLine([]string{"-sideend", "p2: EnemyUser", "move: Stealth Rock"}, "GhostHaze Thinker")
	if b.OpponentHasHazard("stealthrock") {
		t.Fatalf("expected stealth rock hazard to be removed")
	}

	// invalid choice recovery
	choice, shouldSend := b.HandleLine([]string{"error", "[Invalid choice] Can't move"}, "GhostHaze Thinker")
	if !shouldSend || !strings.Contains(choice, "/choose default") {
		t.Fatalf("expected /choose default on invalid choice, got choice=%s, shouldSend=%v", choice, shouldSend)
	}

	// win line
	b.HandleLine([]string{"win", "GhostHaze Thinker"}, "GhostHaze Thinker")
	if !b.IsEnded() || b.WinnerName() != "GhostHaze Thinker" {
		t.Fatalf("expected battle ended with winner GhostHaze Thinker, got ended=%v, winner=%s", b.IsEnded(), b.WinnerName())
	}

	// test prematureend
	b2 := NewBattle("battle-gen9randombattle-101", nil)
	b2.HandleLine([]string{"prematureend"}, "GhostHaze Thinker")
	if !b2.IsEnded() {
		t.Fatalf("expected prematureend to end battle")
	}

	// test expire
	b3 := NewBattle("battle-gen9randombattle-102", nil)
	b3.HandleLine([]string{"expire"}, "GhostHaze Thinker")
	if !b3.IsEnded() {
		t.Fatalf("expected expire to end battle")
	}
}

func TestBattleRequestParsingAndChoice(t *testing.T) {
	b := NewBattle("battle-gen9randombattle-200", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	reqJSON := `{"rqid":10,"active":[{"moves":[{"id":"surf","move":"Surf","pp":15}]}],"side":{"pokemon":[{"details":"Blastoise, L80","condition":"250/250"}]}}`
	choice, shouldSend := b.HandleLine([]string{"request", reqJSON}, "GhostHaze Thinker")

	if !shouldSend {
		t.Fatalf("expected shouldSend to be true")
	}
	if choice != "/choose move 1|10" {
		t.Fatalf("expected /choose move 1|10, got %s", choice)
	}

	// wait request should not produce a choice
	waitJSON := `{"rqid":11,"wait":true}`
	waitChoice, waitSend := b.HandleLine([]string{"request", waitJSON}, "GhostHaze Thinker")
	if waitSend || waitChoice != "" {
		t.Fatalf("expected wait request to not send choice, got send=%v, choice=%s", waitSend, waitChoice)
	}
}

func TestEmbeddedDataset(t *testing.T) {
	// test expanded moves from embedded dataset
	knockOff := GetMoveData("Knock Off")
	if knockOff.Type != "dark" || knockOff.BasePower != 65 || knockOff.Category != CategoryPhysical {
		t.Fatalf("unexpected data for knock off: %+v", knockOff)
	}

	freezeDry := GetMoveData("Freeze-Dry")
	if freezeDry.Type != "ice" || freezeDry.BasePower != 70 || freezeDry.Category != CategorySpecial {
		t.Fatalf("unexpected data for freeze-dry: %+v", freezeDry)
	}

	ceaselessEdge := GetMoveData("Ceaseless Edge")
	if !ceaselessEdge.IsHazard || ceaselessEdge.BasePower != 65 {
		t.Fatalf("unexpected data for ceaseless edge: %+v", ceaselessEdge)
	}

	// test expanded species types from embedded dataset
	clodsireTypes := GetSpeciesTypes("Clodsire")
	if len(clodsireTypes) != 2 || clodsireTypes[0] != "poison" || clodsireTypes[1] != "ground" {
		t.Fatalf("unexpected types for clodsire: %v", clodsireTypes)
	}

	samurottTypes := GetSpeciesTypes("Samurott-Hisui")
	if len(samurottTypes) != 2 || samurottTypes[0] != "water" || samurottTypes[1] != "dark" {
		t.Fatalf("unexpected types for samurott-hisui: %v", samurottTypes)
	}

	// test base stats lookups
	blisseyStats := GetSpeciesBaseStats("Blissey")
	if blisseyStats["def"] != 10 || blisseyStats["spd"] != 135 {
		t.Fatalf("unexpected blissey stats: %v", blisseyStats)
	}

	cloysterStats := GetSpeciesBaseStats("Cloyster")
	if cloysterStats["def"] != 180 || cloysterStats["spd"] != 45 {
		t.Fatalf("unexpected cloyster stats: %v", cloysterStats)
	}

	// test unknown species base stats fallback
	unknownStats := GetSpeciesBaseStats("NonExistentMon")
	if unknownStats["def"] != 80 || unknownStats["spd"] != 80 {
		t.Fatalf("unexpected default stats for unknown mon: %v", unknownStats)
	}

	// test full pokedex entry lookup
	entry, found := GetSpeciesPokedexEntry("Garchomp")
	if !found || len(entry.Types) != 2 || entry.BaseStats["atk"] != 130 {
		t.Fatalf("unexpected entry for garchomp: %+v (found=%v)", entry, found)
	}

	_, notFound := GetSpeciesPokedexEntry("NonExistentMon")
	if notFound {
		t.Fatalf("expected not found for nonexistent species")
	}

	// test random battle sets lookup
	venusaurSet, hasSet := GetRandomBattleSet("Venusaur")
	if !hasSet || len(venusaurSet.Sets) == 0 || venusaurSet.Level <= 0 {
		t.Fatalf("unexpected random set for venusaur: %+v (hasSet=%v)", venusaurSet, hasSet)
	}

	_, noSet := GetRandomBattleSet("NonExistentMon")
	if noSet {
		t.Fatalf("expected no random set for nonexistent mon")
	}

	// test ability data lookup
	ability, hasAbility := GetAbilityData("Wonder Guard")
	if !hasAbility || ability.Rating < 4 {
		t.Fatalf("unexpected ability data for wonder guard: %+v (hasAbility=%v)", ability, hasAbility)
	}

	_, noAbility := GetAbilityData("NonExistentAbility")
	if noAbility {
		t.Fatalf("expected no ability data for nonexistent ability")
	}
}

func TestStatAwareDamageEvaluation(t *testing.T) {
	b := NewBattle("battle-gen9randombattle-301", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	// opponent is blissey: 10 def vs 135 spd
	b.OpponentActive = OpponentActivePoke{
		Ident:     "p2a: Blissey",
		Species:   "Blissey",
		Types:     []string{"normal"},
		HPPercent: 1.0,
		Boosts:    make(map[string]int),
	}

	// active pokemon has aura sphere (special, 80 bp) and close combat (physical, 120 bp)
	// against blissey's 10 def vs 135 spd, close combat deals vastly more damage
	reqJSON := `{
		"rqid": 1,
		"active": [{
			"moves": [
				{"id": "aurasphere", "move": "Aura Sphere", "pp": 20},
				{"id": "closecombat", "move": "Close Combat", "pp": 5}
			]
		}],
		"side": {
			"pokemon": [{
				"ident": "p1a: Lucario",
				"details": "Lucario, L80",
				"condition": "250/250",
				"active": true,
				"stats": {"atk": 110, "def": 70, "spa": 115, "spd": 70, "spe": 90}
			}]
		}
	}`

	choice, shouldSend := b.HandleLine([]string{"request", reqJSON}, "GhostHaze Thinker")
	if !shouldSend {
		t.Fatalf("expected shouldSend to be true")
	}
	// should pick close combat (slot 2) against physically fragile blissey
	if choice != "/choose move 2|1" {
		t.Fatalf("expected move 2 against blissey, got %s", choice)
	}

	// now test against cloyster: 180 def vs 45 spd
	bCloyster := NewBattle("battle-gen9randombattle-302", nil)
	bCloyster.MyPlayerID = "p1"
	bCloyster.OpponentID = "p2"
	bCloyster.OpponentActive = OpponentActivePoke{
		Ident:     "p2a: Cloyster",
		Species:   "Cloyster",
		Types:     []string{"water", "ice"},
		HPPercent: 1.0,
		Boosts:    make(map[string]int),
	}

	choiceCloyster, shouldSendCloyster := bCloyster.HandleLine([]string{"request", reqJSON}, "GhostHaze Thinker")
	if !shouldSendCloyster {
		t.Fatalf("expected shouldSendCloyster to be true")
	}
	// against cloyster (180 def vs 45 spd), special aura sphere (slot 1) should deal much more damage than physical close combat
	if choiceCloyster != "/choose move 1|1" {
		t.Fatalf("expected move 1 (aura sphere) against physically bulky cloyster, got %s", choiceCloyster)
	}
}

func TestAbilityImmunities(t *testing.T) {
	if !IsAbilityImmune("levitate", "ground") {
		t.Fatalf("expected levitate to grant ground immunity")
	}
	if !IsAbilityImmune("eartheater", "ground") {
		t.Fatalf("expected eartheater to grant ground immunity")
	}
	if !IsAbilityImmune("flashfire", "fire") {
		t.Fatalf("expected flash fire to grant fire immunity")
	}
	if !IsAbilityImmune("voltabsorb", "electric") {
		t.Fatalf("expected volt absorb to grant electric immunity")
	}
	if !IsAbilityImmune("waterabsorb", "water") {
		t.Fatalf("expected water absorb to grant water immunity")
	}
	if !IsAbilityImmune("sapsipper", "grass") {
		t.Fatalf("expected sap sipper to grant grass immunity")
	}
	if IsAbilityImmune("levitate", "water") {
		t.Fatalf("levitate should not grant water immunity")
	}
}

func TestGenerationAndFormatParsing(t *testing.T) {
	b1 := NewBattle("battle-1", nil)
	b1.HandleLine([]string{"tier", "[Gen 1] Random Battle"}, "GhostHaze Thinker")
	if b1.Generation() != 1 || !b1.IsRandomBattle() {
		t.Fatalf("expected gen 1 random battle, got gen=%d, isRandom=%v", b1.Generation(), b1.IsRandomBattle())
	}

	b8 := NewBattle("battle-8", nil)
	b8.HandleLine([]string{"tier", "[Gen 8] OU"}, "GhostHaze Thinker")
	if b8.Generation() != 8 || b8.IsRandomBattle() {
		t.Fatalf("expected gen 8 ou, got gen=%d, isRandom=%v", b8.Generation(), b8.IsRandomBattle())
	}

	b9 := NewBattle("battle-9", nil)
	b9.HandleLine([]string{"tier", "[Gen 9] Random Battle"}, "GhostHaze Thinker")
	if b9.Generation() != 9 || !b9.IsRandomBattle() {
		t.Fatalf("expected gen 9 random battle, got gen=%d, isRandom=%v", b9.Generation(), b9.IsRandomBattle())
	}
}

func TestRevealedDataTrackingAndAbilityAvoidance(t *testing.T) {
	b := NewBattle("battle-test-tracking", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	// switch in rotom-wash
	b.HandleLine([]string{"switch", "p2a: Rotom-Wash", "Rotom-Wash, L80", "100/100"}, "GhostHaze Thinker")
	b.HandleLine([]string{"-ability", "p2a: Rotom-Wash", "Levitate"}, "GhostHaze Thinker")
	b.HandleLine([]string{"move", "p2a: Rotom-Wash", "Hydro Pump", "p1a: Garchomp"}, "GhostHaze Thinker")

	if b.OpponentActive.Ability != "levitate" {
		t.Fatalf("expected ability levitate to be tracked, got %s", b.OpponentActive.Ability)
	}
	if len(b.OpponentActive.Moves) != 1 || b.OpponentActive.Moves[0] != "hydropump" {
		t.Fatalf("expected hydropump to be tracked in moves: %v", b.OpponentActive.Moves)
	}

	// active pokemon has earthquake (ground, 100 bp) and dragon claw (dragon, 80 bp)
	// rotom-wash is electric/water (normally 2x weak to ground), but levitate grants ground immunity
	reqJSON := `{
		"rqid": 5,
		"active": [{
			"moves": [
				{"id": "earthquake", "move": "Earthquake", "pp": 10},
				{"id": "dragonclaw", "move": "Dragon Claw", "pp": 15}
			]
		}],
		"side": {
			"pokemon": [{
				"ident": "p1a: Garchomp",
				"details": "Garchomp, L80",
				"condition": "250/250",
				"active": true,
				"stats": {"atk": 250, "def": 180, "spa": 150, "spd": 150, "spe": 200}
			}]
		}
	}`

	choice, shouldSend := b.HandleLine([]string{"request", reqJSON}, "GhostHaze Thinker")
	if !shouldSend {
		t.Fatalf("expected shouldSend to be true")
	}
	// should choose dragon claw (slot 2) over immune earthquake (slot 1)
	if choice != "/choose move 2|5" {
		t.Fatalf("expected move 2 (dragon claw) against levitate rotom, got %s", choice)
	}

	// test switch out and back in retains revealed moves and ability
	b.HandleLine([]string{"switch", "p2a: Tyranitar", "Tyranitar, L80", "100/100"}, "GhostHaze Thinker")
	if b.OpponentActive.Species != "Tyranitar" {
		t.Fatalf("expected active species tyranitar")
	}
	// switch back to rotom-wash
	b.HandleLine([]string{"switch", "p2a: Rotom-Wash", "Rotom-Wash, L80", "100/100"}, "GhostHaze Thinker")
	if b.OpponentActive.Ability != "levitate" || len(b.OpponentActive.Moves) != 1 {
		t.Fatalf("expected remembered ability levitate and move hydropump after switch back: %+v", b.OpponentActive)
	}
}

func TestBattle_ReplaceZoroarkIllusion(t *testing.T) {
	b := NewBattle("battle-replace-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	// initial appearance is hydreigon
	b.HandleLine([]string{"switch", "p2a: Hydreigon", "Hydreigon, L80, M", "100/100"}, "GhostHaze Thinker")
	if b.OpponentActive.Species != "Hydreigon" {
		t.Fatalf("expected initial species hydreigon, got %s", b.OpponentActive.Species)
	}

	// illusion breaks via replace protocol message
	b.HandleLine([]string{"replace", "p2a: Zoroark", "Zoroark, L80, M", "60/100"}, "GhostHaze Thinker")
	if b.OpponentActive.Species != "Zoroark" {
		t.Fatalf("expected species zoroark after replace, got %s", b.OpponentActive.Species)
	}
	if b.OpponentActive.HPPercent < 0.59 || b.OpponentActive.HPPercent > 0.61 {
		t.Fatalf("expected 60%% hp after replace, got %f", b.OpponentActive.HPPercent)
	}
}

func TestBattle_TerastallizeAndFormeChange(t *testing.T) {
	b := NewBattle("battle-tera-forme-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	b.HandleLine([]string{"switch", "p2a: Dragonite", "Dragonite, L80, M", "100/100"}, "GhostHaze Thinker")

	// opponent terastallizes into normal
	b.HandleLine([]string{"-terastallize", "p2a: Dragonite", "Normal"}, "GhostHaze Thinker")
	if b.OpponentActive.Terastallized != "normal" {
		t.Fatalf("expected terastallized normal, got %s", b.OpponentActive.Terastallized)
	}
	if len(b.OpponentActive.Types) != 1 || b.OpponentActive.Types[0] != "normal" {
		t.Fatalf("expected typing to be [normal], got %v", b.OpponentActive.Types)
	}

	// forme change e.g. palafin-hero
	b.HandleLine([]string{"-formechange", "p2a: Palafin", "Palafin-Hero"}, "GhostHaze Thinker")
	if b.OpponentActive.Species != "Palafin-Hero" {
		t.Fatalf("expected palafin-hero after forme change, got %s", b.OpponentActive.Species)
	}
}

func TestBattle_ItemExtractionFromDamageAndHeal(t *testing.T) {
	b := NewBattle("battle-item-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	b.HandleLine([]string{"switch", "p2a: Garchomp", "Garchomp, L80", "100/100"}, "GhostHaze Thinker")

	// life orb recoil
	b.HandleLine([]string{"-damage", "p2a: Garchomp", "90/100", "[from] item: Life Orb"}, "GhostHaze Thinker")
	if b.OpponentActive.Item != "lifeorb" {
		t.Fatalf("expected revealed item lifeorb, got %s", b.OpponentActive.Item)
	}

	// leftovers heal
	b.OpponentActive.Item = ""
	b.HandleLine([]string{"-heal", "p2a: Garchomp", "96/100", "[from] item: Leftovers"}, "GhostHaze Thinker")
	if b.OpponentActive.Item != "leftovers" {
		t.Fatalf("expected revealed item leftovers, got %s", b.OpponentActive.Item)
	}
}

func TestBattle_SetBoostAndCureTeam(t *testing.T) {
	b := NewBattle("battle-boost-test", nil)
	b.MyPlayerID = "p1"
	b.OpponentID = "p2"

	b.HandleLine([]string{"switch", "p2a: Azumarill", "Azumarill, L80", "100/100"}, "GhostHaze Thinker")

	// belly drum sets atk to +6
	b.HandleLine([]string{"-setboost", "p2a: Azumarill", "atk", "6"}, "GhostHaze Thinker")
	if b.OpponentActive.Boosts["atk"] != 6 {
		t.Fatalf("expected +6 atk boost after belly drum, got %d", b.OpponentActive.Boosts["atk"])
	}

	// white herb clearing negative boosts
	b.OpponentActive.Boosts["def"] = -1
	b.HandleLine([]string{"-clearnegativeboost", "p2a: Azumarill", "[from] item: White Herb"}, "GhostHaze Thinker")
	if _, exists := b.OpponentActive.Boosts["def"]; exists {
		t.Fatalf("expected negative def boost to be cleared by white herb")
	}
	if b.OpponentActive.Boosts["atk"] != 6 {
		t.Fatalf("expected positive atk boost to remain after white herb")
	}

	// cure team
	b.OpponentActive.Status = "psn"
	b.OpponentTeam = []OpponentBenchPoke{
		{Species: "Blissey", Status: "brn"},
	}
	b.HandleLine([]string{"-cureteam", "p2a: Azumarill", "[from] move: Heal Bell"}, "GhostHaze Thinker")
	if b.OpponentActive.Status != "" {
		t.Fatalf("expected active status cleared by heal bell, got %s", b.OpponentActive.Status)
	}
	if b.OpponentTeam[0].Status != "" {
		t.Fatalf("expected bench status cleared by heal bell, got %s", b.OpponentTeam[0].Status)
	}
}

func TestMinimaxEngine_TrappedPreventsSwitch(t *testing.T) {
	engine := NewMinimaxEngine()
	b := NewBattle("battle-trapped-test", engine)
	b.OpponentActive = OpponentActivePoke{
		Species:   "Gothitelle",
		Types:     []string{"psychic"},
		HPPercent: 1.0,
	}

	req := BattleRequest{
		RQID: 10,
		Active: []RequestActive{
			{
				Trapped: true, // trapped by shadow tag
				Moves: []RequestMove{
					{ID: "surf", Move: "Surf", PP: 15},
				},
			},
		},
		Side: RequestSide{
			Pokemon: []RequestPokemon{
				{
					Details:   "Blastoise, L80",
					Condition: "250/250",
					Active:    true,
				},
				{
					Details:   "Corviknight, L80",
					Condition: "250/250",
					Active:    false,
				},
			},
		},
	}

	state := buildSimulatedState(b, req)
	actions := generateOurActions(req, state)

	for _, act := range actions {
		if act.Type == actionSwitch {
			t.Fatalf("expected no switch actions when trapped, found switch to slot %d", act.SwitchSlot)
		}
	}
}


