package export

import (
	"encoding/json"
	"testing"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

func setupPlayoffTournament(t *testing.T) (*models.Tournament, *swisstools.Tournament) {
	t.Helper()
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	eng.SetMaxRounds(2)

	for _, name := range []string{"Alice", "Bob", "Charlie", "Diana"} {
		if err := eng.AddPlayer(name); err != nil {
			t.Fatalf("AddPlayer(%q): %v", name, err)
		}
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	for round := 1; round <= 2; round++ {
		for _, p := range eng.GetRound() {
			if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
				continue
			}
			if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
				t.Fatalf("AddResult: %v", err)
			}
		}
		if err := eng.NextRound(); err != nil {
			t.Fatalf("NextRound: %v", err)
		}
		if round < 2 {
			if err := eng.Pair(false); err != nil {
				t.Fatalf("Pair: %v", err)
			}
		}
	}
	if err := eng.StartPlayoff(4); err != nil {
		t.Fatalf("StartPlayoff: %v", err)
	}

	mt := &models.Tournament{
		ID:         1,
		Name:       "Playoff Test",
		PointsWin:  3,
		PointsDraw: 1,
		PointsLoss: 0,
		TopCut:     4,
	}
	return mt, &eng
}

func TestGenerateOTR_WithPlayoffStarted(t *testing.T) {
	mt, eng := setupPlayoffTournament(t)
	data, err := GenerateOTR(mt, eng)
	if err != nil {
		t.Fatalf("GenerateOTR: %v", err)
	}
	var otr OTR
	if err := json.Unmarshal(data, &otr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if otr.Playoff == nil {
		t.Fatal("expected playoff in OTR output")
	}
	if len(otr.Playoff.Seeds) == 0 {
		t.Error("expected non-empty seeds")
	}
	if len(otr.Playoff.Rounds) == 0 {
		t.Error("expected at least one playoff round")
	}
}

func TestGenerateOTR_WithPlayoffFinished(t *testing.T) {
	mt, eng := setupPlayoffTournament(t)
	// Run all playoff rounds to completion
	for {
		pairings := eng.GetPlayoffRound()
		if len(pairings) == 0 {
			break
		}
		for _, p := range pairings {
			if err := eng.AddPlayoffResult(p.PlayerA(), 2, 0, 0); err != nil {
				t.Fatalf("AddPlayoffResult: %v", err)
			}
		}
		if err := eng.NextPlayoffRound(); err != nil {
			t.Fatalf("NextPlayoffRound: %v", err)
		}
		if eng.GetPlayoffStatus() == "finished" {
			break
		}
	}

	data, err := GenerateOTR(mt, eng)
	if err != nil {
		t.Fatalf("GenerateOTR: %v", err)
	}
	var otr OTR
	json.Unmarshal(data, &otr)
	if otr.Playoff == nil {
		t.Fatal("expected playoff in OTR output")
	}
	if otr.Playoff.Winner == nil {
		t.Error("expected winner to be set after playoff finished")
	}
}

func TestGenerateOTR_DecklistPublic(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	eng.SetMaxRounds(1)
	for _, name := range []string{"Alice", "Bob"} {
		eng.AddPlayer(name)
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	// Set a decklist for Alice
	playerID, _ := eng.GetPlayerID("Alice")
	dl := swisstools.Decklist{
		Main:      map[string]int{"Brainstorm": 4},
		Sideboard: map[string]int{"Pyroblast": 2},
	}
	if err := eng.SetPlayerDecklist(playerID, dl); err != nil {
		t.Fatalf("SetPlayerDecklist: %v", err)
	}

	mt := &models.Tournament{
		Name:           "Decklist Public",
		PointsWin:      3,
		PointsDraw:     1,
		PointsLoss:     0,
		DecklistPublic: true,
	}
	data, err := GenerateOTR(mt, &eng)
	if err != nil {
		t.Fatalf("GenerateOTR: %v", err)
	}
	var otr OTR
	json.Unmarshal(data, &otr)

	var found bool
	for _, p := range otr.Players {
		if p.Name == "Alice" {
			if p.Decklist == nil {
				t.Error("expected Alice's decklist exposed when DecklistPublic=true")
			} else if p.Decklist.Main["Brainstorm"] != 4 {
				t.Errorf("Brainstorm = %d, want 4", p.Decklist.Main["Brainstorm"])
			}
			found = true
		}
	}
	if !found {
		t.Error("Alice not found in OTR players")
	}
}

func TestGenerateOTR_DecklistPrivate(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	eng.SetMaxRounds(1)
	for _, name := range []string{"Alice", "Bob"} {
		eng.AddPlayer(name)
	}
	eng.StartTournament()

	playerID, _ := eng.GetPlayerID("Alice")
	dl := swisstools.Decklist{Main: map[string]int{"Brainstorm": 4}, Sideboard: map[string]int{}}
	eng.SetPlayerDecklist(playerID, dl)

	mt := &models.Tournament{
		Name:           "Decklist Private",
		PointsWin:      3,
		PointsDraw:     1,
		PointsLoss:     0,
		DecklistPublic: false,
	}
	data, _ := GenerateOTR(mt, &eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	for _, p := range otr.Players {
		if p.Decklist != nil {
			t.Errorf("expected no decklist when DecklistPublic=false, got %+v", p.Decklist)
		}
	}
}

func TestGenerateOTR_WithBye(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	eng.SetMaxRounds(1)
	for _, n := range []string{"A", "B", "C"} {
		eng.AddPlayer(n)
	}
	eng.StartTournament()
	for _, p := range eng.GetRound() {
		if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
			continue
		}
		eng.AddResult(p.PlayerA(), 2, 0, 0)
	}

	mt := &models.Tournament{Name: "With Bye", PointsWin: 3, PointsDraw: 1, PointsLoss: 0}
	data, err := GenerateOTR(mt, &eng)
	if err != nil {
		t.Fatalf("GenerateOTR: %v", err)
	}
	var otr OTR
	json.Unmarshal(data, &otr)
	var sawBye bool
	for _, round := range otr.Rounds {
		for _, pairing := range round.Pairings {
			if pairing.PlayerB == nil {
				sawBye = true
			}
		}
	}
	if !sawBye {
		t.Error("expected to see a bye (PlayerB nil) in 3-player tournament")
	}
}
