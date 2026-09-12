package battle

import (
	"strings"
)

type MoveCategory string

const (
	CategoryPhysical MoveCategory = "Physical"
	CategorySpecial  MoveCategory = "Special"
	CategoryStatus   MoveCategory = "Status"
)

type MoveData struct {
	ID        string
	Name      string
	Type      string
	Category  MoveCategory
	BasePower int
	Accuracy  int
	Priority  int
	IsHealing bool
	IsHazard  bool
	IsSetup   bool
	IsStatus  bool
}

var movesDatabase = map[string]MoveData{
	// normal
	"extremespeed": {ID: "extremespeed", Name: "Extreme Speed", Type: "normal", Category: CategoryPhysical, BasePower: 80, Accuracy: 100, Priority: 2},
	"quickattack":  {ID: "quickattack", Name: "Quick Attack", Type: "normal", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},
	"bodyslam":     {ID: "bodyslam", Name: "Body Slam", Type: "normal", Category: CategoryPhysical, BasePower: 85, Accuracy: 100},
	"doubleedge":   {ID: "doubleedge", Name: "Double-Edge", Type: "normal", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"hypervoice":   {ID: "hypervoice", Name: "Hyper Voice", Type: "normal", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"boomburst":    {ID: "boomburst", Name: "Boomburst", Type: "normal", Category: CategorySpecial, BasePower: 140, Accuracy: 100},
	"bloodmoon":    {ID: "bloodmoon", Name: "Blood Moon", Type: "normal", Category: CategorySpecial, BasePower: 140, Accuracy: 100},
	"protect":      {ID: "protect", Name: "Protect", Type: "normal", Category: CategoryStatus, Priority: 4},
	"substitute":   {ID: "substitute", Name: "Substitute", Type: "normal", Category: CategoryStatus},
	"softboiled":   {ID: "softboiled", Name: "Soft-Boiled", Type: "normal", Category: CategoryStatus, IsHealing: true},
	"recover":      {ID: "recover", Name: "Recover", Type: "normal", Category: CategoryStatus, IsHealing: true},
	"slackoff":     {ID: "slackoff", Name: "Slack Off", Type: "normal", Category: CategoryStatus, IsHealing: true},
	"swordsdance":  {ID: "swordsdance", Name: "Swords Dance", Type: "normal", Category: CategoryStatus, IsSetup: true},

	// fire
	"flamethrower": {ID: "flamethrower", Name: "Flamethrower", Type: "fire", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"fireblast":    {ID: "fireblast", Name: "Fire Blast", Type: "fire", Category: CategorySpecial, BasePower: 110, Accuracy: 85},
	"flareblitz":   {ID: "flareblitz", Name: "Flare Blitz", Type: "fire", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"overheat":     {ID: "overheat", Name: "Overheat", Type: "fire", Category: CategorySpecial, BasePower: 130, Accuracy: 90},
	"lavaplume":    {ID: "lavaplume", Name: "Lava Plume", Type: "fire", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"torchsong":    {ID: "torchsong", Name: "Torch Song", Type: "fire", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"armorcannon":  {ID: "armorcannon", Name: "Armor Cannon", Type: "fire", Category: CategorySpecial, BasePower: 120, Accuracy: 100},
	"willowisp":    {ID: "willowisp", Name: "Will-O-Wisp", Type: "fire", Category: CategoryStatus, Accuracy: 85, IsStatus: true},

	// water
	"surf":         {ID: "surf", Name: "Surf", Type: "water", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"hydropump":    {ID: "hydropump", Name: "Hydro Pump", Type: "water", Category: CategorySpecial, BasePower: 110, Accuracy: 80},
	"scald":        {ID: "scald", Name: "Scald", Type: "water", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"waterfall":    {ID: "waterfall", Name: "Waterfall", Type: "water", Category: CategoryPhysical, BasePower: 80, Accuracy: 100},
	"aquajet":      {ID: "aquajet", Name: "Aqua Jet", Type: "water", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},
	"aquastep":     {ID: "aquastep", Name: "Aqua Step", Type: "water", Category: CategoryPhysical, BasePower: 80, Accuracy: 100},
	"flipturn":     {ID: "flipturn", Name: "Flip Turn", Type: "water", Category: CategoryPhysical, BasePower: 60, Accuracy: 100},

	// grass
	"gigadrain":    {ID: "gigadrain", Name: "Giga Drain", Type: "grass", Category: CategorySpecial, BasePower: 75, Accuracy: 100, IsHealing: true},
	"energyball":   {ID: "energyball", Name: "Energy Ball", Type: "grass", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"leafstorm":    {ID: "leafstorm", Name: "Leaf Storm", Type: "grass", Category: CategorySpecial, BasePower: 130, Accuracy: 90},
	"powerwhip":    {ID: "powerwhip", Name: "Power Whip", Type: "grass", Category: CategoryPhysical, BasePower: 120, Accuracy: 85},
	"woodhammer":   {ID: "woodhammer", Name: "Wood Hammer", Type: "grass", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"flowertrick":  {ID: "flowertrick", Name: "Flower Trick", Type: "grass", Category: CategoryPhysical, BasePower: 70, Accuracy: 100},
	"spore":        {ID: "spore", Name: "Spore", Type: "grass", Category: CategoryStatus, Accuracy: 100, IsStatus: true},
	"sleeppowder":  {ID: "sleeppowder", Name: "Sleep Powder", Type: "grass", Category: CategoryStatus, Accuracy: 75, IsStatus: true},
	"synthesis":    {ID: "synthesis", Name: "Synthesis", Type: "grass", Category: CategoryStatus, IsHealing: true},

	// electric
	"thunderbolt":  {ID: "thunderbolt", Name: "Thunderbolt", Type: "electric", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"thunder":      {ID: "thunder", Name: "Thunder", Type: "electric", Category: CategorySpecial, BasePower: 110, Accuracy: 70},
	"voltswitch":   {ID: "voltswitch", Name: "Volt Switch", Type: "electric", Category: CategorySpecial, BasePower: 70, Accuracy: 100},
	"wildcharge":   {ID: "wildcharge", Name: "Wild Charge", Type: "electric", Category: CategoryPhysical, BasePower: 90, Accuracy: 100},
	"electrodrift": {ID: "electrodrift", Name: "Electro Drift", Type: "electric", Category: CategorySpecial, BasePower: 100, Accuracy: 100},
	"thunderwave":  {ID: "thunderwave", Name: "Thunder Wave", Type: "electric", Category: CategoryStatus, Accuracy: 90, IsStatus: true},

	// ice
	"icebeam":      {ID: "icebeam", Name: "Ice Beam", Type: "ice", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"blizzard":     {ID: "blizzard", Name: "Blizzard", Type: "ice", Category: CategorySpecial, BasePower: 110, Accuracy: 70},
	"iceshard":     {ID: "iceshard", Name: "Ice Shard", Type: "ice", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},
	"iciclecrash":  {ID: "iciclecrash", Name: "Icicle Crash", Type: "ice", Category: CategoryPhysical, BasePower: 85, Accuracy: 90},
	"tripleaxel":   {ID: "tripleaxel", Name: "Triple Axel", Type: "ice", Category: CategoryPhysical, BasePower: 120, Accuracy: 90},

	// fighting
	"closecombat":  {ID: "closecombat", Name: "Close Combat", Type: "fighting", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"superpower":   {ID: "superpower", Name: "Superpower", Type: "fighting", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"focusblast":   {ID: "focusblast", Name: "Focus Blast", Type: "fighting", Category: CategorySpecial, BasePower: 120, Accuracy: 70},
	"aurasphere":   {ID: "aurasphere", Name: "Aura Sphere", Type: "fighting", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"machpunch":    {ID: "machpunch", Name: "Mach Punch", Type: "fighting", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},
	"drainpunch":   {ID: "drainpunch", Name: "Drain Punch", Type: "fighting", Category: CategoryPhysical, BasePower: 75, Accuracy: 100, IsHealing: true},
	"collisioncourse": {ID: "collisioncourse", Name: "Collision Course", Type: "fighting", Category: CategoryPhysical, BasePower: 100, Accuracy: 100},
	"bulkup":       {ID: "bulkup", Name: "Bulk Up", Type: "fighting", Category: CategoryStatus, IsSetup: true},

	// poison
	"sludgebomb":   {ID: "sludgebomb", Name: "Sludge Bomb", Type: "poison", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"sludgewave":   {ID: "sludgewave", Name: "Sludge Wave", Type: "poison", Category: CategorySpecial, BasePower: 95, Accuracy: 100},
	"gunkshot":     {ID: "gunkshot", Name: "Gunk Shot", Type: "poison", Category: CategoryPhysical, BasePower: 120, Accuracy: 80},
	"poisonjab":    {ID: "poisonjab", Name: "Poison Jab", Type: "poison", Category: CategoryPhysical, BasePower: 80, Accuracy: 100},
	"toxic":        {ID: "toxic", Name: "Toxic", Type: "poison", Category: CategoryStatus, Accuracy: 90, IsStatus: true},
	"toxicspikes":  {ID: "toxicspikes", Name: "Toxic Spikes", Type: "poison", Category: CategoryStatus, IsHazard: true},

	// ground
	"earthquake":   {ID: "earthquake", Name: "Earthquake", Type: "ground", Category: CategoryPhysical, BasePower: 100, Accuracy: 100},
	"earthpower":   {ID: "earthpower", Name: "Earth Power", Type: "ground", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"headlongrush": {ID: "headlongrush", Name: "Headlong Rush", Type: "ground", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"spikes":       {ID: "spikes", Name: "Spikes", Type: "ground", Category: CategoryStatus, IsHazard: true},

	// flying
	"bravebird":    {ID: "bravebird", Name: "Brave Bird", Type: "flying", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"airslash":     {ID: "airslash", Name: "Air Slash", Type: "flying", Category: CategorySpecial, BasePower: 75, Accuracy: 95},
	"hurricane":    {ID: "hurricane", Name: "Hurricane", Type: "flying", Category: CategorySpecial, BasePower: 110, Accuracy: 70},
	"roost":        {ID: "roost", Name: "Roost", Type: "flying", Category: CategoryStatus, IsHealing: true},

	// psychic
	"psychic":      {ID: "psychic", Name: "Psychic", Type: "psychic", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"psyshock":     {ID: "psyshock", Name: "Psyshock", Type: "psychic", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"calmmind":     {ID: "calmmind", Name: "Calm Mind", Type: "psychic", Category: CategoryStatus, IsSetup: true},
	"agility":      {ID: "agility", Name: "Agility", Type: "psychic", Category: CategoryStatus, IsSetup: true},

	// bug
	"uturn":        {ID: "uturn", Name: "U-turn", Type: "bug", Category: CategoryPhysical, BasePower: 70, Accuracy: 100},
	"bugbuzz":      {ID: "bugbuzz", Name: "Bug Buzz", Type: "bug", Category: CategorySpecial, BasePower: 90, Accuracy: 100},
	"stickyweb":    {ID: "stickyweb", Name: "Sticky Web", Type: "bug", Category: CategoryStatus, IsHazard: true},
	"quiverdance":  {ID: "quiverdance", Name: "Quiver Dance", Type: "bug", Category: CategoryStatus, IsSetup: true},

	// rock
	"stoneedge":    {ID: "stoneedge", Name: "Stone Edge", Type: "rock", Category: CategoryPhysical, BasePower: 100, Accuracy: 80},
	"rockslide":    {ID: "rockslide", Name: "Rock Slide", Type: "rock", Category: CategoryPhysical, BasePower: 75, Accuracy: 90},
	"powergem":     {ID: "powergem", Name: "Power Gem", Type: "rock", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"stealthrock":  {ID: "stealthrock", Name: "Stealth Rock", Type: "rock", Category: CategoryStatus, IsHazard: true},

	// ghost
	"shadowball":   {ID: "shadowball", Name: "Shadow Ball", Type: "ghost", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"shadowclaw":   {ID: "shadowclaw", Name: "Shadow Claw", Type: "ghost", Category: CategoryPhysical, BasePower: 70, Accuracy: 100},
	"shadowsneak":  {ID: "shadowsneak", Name: "Shadow Sneak", Type: "ghost", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},

	// dragon
	"dracometeor":  {ID: "dracometeor", Name: "Draco Meteor", Type: "dragon", Category: CategorySpecial, BasePower: 130, Accuracy: 90},
	"dragonpulse":  {ID: "dragonpulse", Name: "Dragon Pulse", Type: "dragon", Category: CategorySpecial, BasePower: 85, Accuracy: 100},
	"outrage":      {ID: "outrage", Name: "Outrage", Type: "dragon", Category: CategoryPhysical, BasePower: 120, Accuracy: 100},
	"dragondance":  {ID: "dragondance", Name: "Dragon Dance", Type: "dragon", Category: CategoryStatus, IsSetup: true},

	// steel
	"ironhead":     {ID: "ironhead", Name: "Iron Head", Type: "steel", Category: CategoryPhysical, BasePower: 80, Accuracy: 100},
	"flashcannon":  {ID: "flashcannon", Name: "Flash Cannon", Type: "steel", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"makeitrain":   {ID: "makeitrain", Name: "Make It Rain", Type: "steel", Category: CategorySpecial, BasePower: 120, Accuracy: 100},
	"bulletpunch":  {ID: "bulletpunch", Name: "Bullet Punch", Type: "steel", Category: CategoryPhysical, BasePower: 40, Accuracy: 100, Priority: 1},

	// dark
	"knockoff":     {ID: "knockoff", Name: "Knock Off", Type: "dark", Category: CategoryPhysical, BasePower: 65, Accuracy: 100},
	"darkpulse":    {ID: "darkpulse", Name: "Dark Pulse", Type: "dark", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"suckerpunch":  {ID: "suckerpunch", Name: "Sucker Punch", Type: "dark", Category: CategoryPhysical, BasePower: 70, Accuracy: 100, Priority: 1},
	"crunch":       {ID: "crunch", Name: "Crunch", Type: "dark", Category: CategoryPhysical, BasePower: 80, Accuracy: 100},
	"nastyplot":    {ID: "nastyplot", Name: "Nasty Plot", Type: "dark", Category: CategoryStatus, IsSetup: true},
	"taunt":        {ID: "taunt", Name: "Taunt", Type: "dark", Category: CategoryStatus, Accuracy: 100},

	// fairy
	"moonblast":    {ID: "moonblast", Name: "Moonblast", Type: "fairy", Category: CategorySpecial, BasePower: 95, Accuracy: 100},
	"dazzlinggleam":{ID: "dazzlinggleam", Name: "Dazzling Gleam", Type: "fairy", Category: CategorySpecial, BasePower: 80, Accuracy: 100},
	"playrough":    {ID: "playrough", Name: "Play Rough", Type: "fairy", Category: CategoryPhysical, BasePower: 90, Accuracy: 90},
}

// getmovedata retrieves the stats for a given move id, or returns sensible defaults.
func GetMoveData(moveID string) MoveData {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(moveID, "-", ""), " ", ""))
	if m, exists := movesDatabase[clean]; exists {
		return m
	}
	// fallback for unlisted moves
	return MoveData{
		ID:        clean,
		Name:      moveID,
		Type:      "normal",
		Category:  CategoryPhysical,
		BasePower: 60,
		Accuracy:  100,
	}
}

// calculatedamage estimates damage dealt using standard pokemon damage mechanics.
func CalculateDamage(level int, basePower int, atk int, def int, stab float64, typeEff float64, isBurned bool, isPhysical bool) float64 {
	if basePower <= 0 || typeEff == 0.0 {
		return 0.0
	}
	if level <= 0 {
		level = 80
	}
	if atk <= 0 {
		atk = 100
	}
	if def <= 0 {
		def = 100
	}
	burnFactor := 1.0
	if isBurned && isPhysical {
		burnFactor = 0.5
	}
	base := (((2.0*float64(level)/5.0 + 2.0) * float64(basePower) * float64(atk) / float64(def)) / 50.0 + 2.0)
	return base * stab * typeEff * burnFactor
}
