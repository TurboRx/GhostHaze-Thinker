package showdown

import (
	"net/http"
	"net/http/httptest"
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

func TestPokepasteImport(t *testing.T) {
	// 1. test extract paste id
	if id := ExtractPokepasteID("https://pokepast.es/3141f2388e6308cf"); id != "3141f2388e6308cf" {
		t.Fatalf("unexpected extracted id: %s", id)
	}
	if id := ExtractPokepasteID("https://pokepast.es/raw/abcdef123?utm=1#tag"); id != "abcdef123" {
		t.Fatalf("unexpected extracted id with params: %s", id)
	}
	if id := ExtractPokepasteID("custom123"); id != "custom123" {
		t.Fatalf("unexpected extracted raw id: %s", id)
	}

	// 2. test import with local mock server
	samplePaste := `
Great Tusk @ Booster Energy
Ability: Protosynthesis
- Headlong Rush
- Close Combat
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(samplePaste))
	}))
	defer server.Close()

	// temporarily override pokepaste client and base url
	origClient := pokepasteHTTPClient
	origBase := pokepasteBaseURL
	SetPokepasteHTTPClient(server.Client())
	SetPokepasteBaseURL(server.URL)
	defer func() {
		SetPokepasteHTTPClient(origClient)
		SetPokepasteBaseURL(origBase)
	}()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "teams.json")
	store := NewTeamStore(filePath)

	team, err := store.ImportPokepaste("https://pokepast.es/sample123", "gen9ou", "Mock Imported Team")
	if err != nil {
		t.Fatalf("unexpected error importing pokepaste: %v", err)
	}

	if team.Name != "Mock Imported Team" || team.Format != "gen9ou" {
		t.Fatalf("unexpected team attributes: %+v", team)
	}
	if len(team.Pokemon) != 1 || team.Pokemon[0] != "Great Tusk" {
		t.Fatalf("unexpected team pokemon: %v", team.Pokemon)
	}
	if team.TeamPacked == "" || !team.Active {
		t.Fatalf("expected active team with non-empty packed data")
	}

	// verify saved in store
	list := store.List()
	if len(list) != 1 || list[0].ID != team.ID {
		t.Fatalf("expected team saved in store: %+v", list)
	}
}
