package showdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePokepaste(t *testing.T) {
	paste := `
Great Tusk @ Booster Energy
Ability: Protosynthesis
Tera Type: Ice
EVs: 252 Atk / 4 SpD / 252 Spe
Jolly Nature
- Headlong Rush
- Close Combat
- Ice Spinner
- Rapid Spin

Kingambit @ Leftovers
Ability: Supreme Overlord
Tera Type: Flying
EVs: 212 HP / 252 Atk / 44 Spe
Adamant Nature
- Kowtow Cleave
- Sucker Punch
- Iron Head
- Swords Dance
`

	pokes, packed, err := ParsePokepaste(paste)
	if err != nil {
		t.Fatalf("unexpected error parsing paste: %v", err)
	}

	if len(pokes) != 2 {
		t.Fatalf("expected 2 pokemon, got %d", len(pokes))
	}
	if pokes[0] != "Great Tusk" || pokes[1] != "Kingambit" {
		t.Fatalf("unexpected pokemon names: %v", pokes)
	}

	if packed == "" {
		t.Fatal("packed string should not be empty")
	}
}

func TestParsePokepaste_PackedInput(t *testing.T) {
	packedInput := "Pikachu||lightball|lightningrod|thunderbolt,voltswitch,surf,grassknot|Timid|,,,252,,252||||]Raichu||||thunderbolt||||||"
	pokes, packed, err := ParsePokepaste(packedInput)
	if err != nil {
		t.Fatalf("unexpected error parsing packed input: %v", err)
	}
	if len(pokes) != 2 {
		t.Fatalf("expected 2 pokemon, got %d", len(pokes))
	}
	if packed != packedInput {
		t.Fatalf("expected packed string to remain identical")
	}
}

func TestTeamStore_Operations(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "teams.json")

	store := NewTeamStore(filePath)
	if len(store.List()) != 0 {
		t.Fatalf("expected empty store")
	}

	err := store.AddOrUpdate(BattleTeam{
		Name:    "OU Balance",
		Format:  "gen9ou",
		TeamRaw: "Pikachu @ Light Ball\nAbility: Lightning Rod\n- Thunderbolt\n",
		Active:  true,
	})
	if err != nil {
		t.Fatalf("failed to add team: %v", err)
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 team, got %d", len(list))
	}
	teamID := list[0].ID

	// test format match
	matchedTeam := store.GetTeamForFormat("gen9ou")
	if matchedTeam == "" {
		t.Fatal("expected match for gen9ou")
	}

	// toggle active
	active, err := store.Toggle(teamID)
	if err != nil {
		t.Fatalf("failed to toggle team: %v", err)
	}
	if active {
		t.Fatal("expected team to be inactive after toggle")
	}

	// now getteamforformat should return empty since team is inactive
	if store.GetTeamForFormat("gen9ou") != "" {
		t.Fatal("expected empty team for inactive status")
	}

	// test delete
	if !store.Delete(teamID) {
		t.Fatal("expected delete to succeed")
	}
	if len(store.List()) != 0 {
		t.Fatal("expected 0 teams after delete")
	}

	_ = os.Remove(filePath)
}
