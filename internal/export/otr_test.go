package export

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

func setupTestTournament(t *testing.T) (*models.Tournament, *swisstools.Tournament) {
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

	pairings := eng.GetRound()
	for _, p := range pairings {
		if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
			t.Fatalf("AddResult: %v", err)
		}
	}

	if err := eng.NextRound(); err != nil {
		t.Fatalf("NextRound: %v", err)
	}
	if err := eng.Pair(false); err != nil {
		t.Fatalf("Pair: %v", err)
	}

	pairings = eng.GetRound()
	for _, p := range pairings {
		if err := eng.AddResult(p.PlayerA(), 2, 1, 0); err != nil {
			t.Fatalf("AddResult round 2: %v", err)
		}
	}

	scheduled := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	location := "Test Venue"
	desc := "Test tournament"
	mt := &models.Tournament{
		ID:          1,
		Name:        "Test Open",
		Description: &desc,
		ScheduledAt: &scheduled,
		Location:    &location,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		TopCut:      0,
	}

	return mt, &eng
}

func TestGenerateOTR_BasicStructure(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, err := GenerateOTR(mt, eng)
	if err != nil {
		t.Fatalf("GenerateOTR error: %v", err)
	}
	var otr OTR
	if err := json.Unmarshal(data, &otr); err != nil {
		t.Fatalf("failed to unmarshal OTR: %v", err)
	}
	if otr.OTRVersion != 1 {
		t.Errorf("expected OTR version 1, got %d", otr.OTRVersion)
	}
	if otr.Tournament.Name != "Test Open" {
		t.Errorf("expected name Test Open, got %q", otr.Tournament.Name)
	}
	if otr.Tournament.Format != "Swiss" {
		t.Errorf("expected format Swiss, got %q", otr.Tournament.Format)
	}
	if otr.Tournament.PointsWin != 3 {
		t.Errorf("expected points_win 3, got %d", otr.Tournament.PointsWin)
	}
}

func TestGenerateOTR_HasPlayers(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, err := GenerateOTR(mt, eng)
	if err != nil {
		t.Fatalf("GenerateOTR error: %v", err)
	}
	var otr OTR
	json.Unmarshal(data, &otr)
	if len(otr.Players) != 4 {
		t.Fatalf("expected 4 players, got %d", len(otr.Players))
	}
	names := make(map[string]bool)
	for _, p := range otr.Players {
		names[p.Name] = true
		if p.FinalRank < 1 {
			t.Errorf("player %q has invalid rank %d", p.Name, p.FinalRank)
		}
	}
	for _, name := range []string{"Alice", "Bob", "Charlie", "Diana"} {
		if !names[name] {
			t.Errorf("expected player %q in OTR output", name)
		}
	}
}

func TestGenerateOTR_HasRounds(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	if len(otr.Rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(otr.Rounds))
	}
	if otr.Rounds[0].RoundNumber != 1 {
		t.Errorf("expected round 1, got %d", otr.Rounds[0].RoundNumber)
	}
	if otr.Rounds[1].RoundNumber != 2 {
		t.Errorf("expected round 2, got %d", otr.Rounds[1].RoundNumber)
	}
	if len(otr.Rounds[0].Pairings) != 2 {
		t.Errorf("expected 2 pairings in round 1, got %d", len(otr.Rounds[0].Pairings))
	}
}

func TestGenerateOTR_DateAndLocation(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	if otr.Tournament.Date == "" {
		t.Error("expected date to be set")
	}
	if otr.Tournament.Location != "Test Venue" {
		t.Errorf("expected location Test Venue, got %q", otr.Tournament.Location)
	}
}

func TestGenerateOTR_NoDateLocation(t *testing.T) {
	mt, eng := setupTestTournament(t)
	mt.ScheduledAt = nil
	mt.Location = nil
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	if otr.Tournament.Date != "" {
		t.Errorf("expected empty date, got %q", otr.Tournament.Date)
	}
	if otr.Tournament.Location != "" {
		t.Errorf("expected empty location, got %q", otr.Tournament.Location)
	}
}

func TestGenerateOTR_NoPlayoff(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	if otr.Playoff != nil {
		t.Error("expected no playoff for tournament without top cut")
	}
}

func TestGenerateOTR_TopCut(t *testing.T) {
	mt, eng := setupTestTournament(t)
	mt.TopCut = 4
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	if otr.Tournament.TopCut != 4 {
		t.Errorf("expected top_cut 4, got %d", otr.Tournament.TopCut)
	}
}

func TestGenerateOTR_ValidJSON(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, err := GenerateOTR(mt, eng)
	if err != nil {
		t.Fatalf("GenerateOTR error: %v", err)
	}
	if !json.Valid(data) {
		t.Error("GenerateOTR output is not valid JSON")
	}
}

func TestPlayoffRoundName(t *testing.T) {
	tests := []struct {
		total    int
		index    int
		expected string
	}{
		{3, 0, "Quarterfinals"},
		{3, 1, "Semifinals"},
		{3, 2, "Finals"},
		{2, 0, "Semifinals"},
		{2, 1, "Finals"},
		{1, 0, "Finals"},
		{4, 0, "Round 1"},
		{4, 1, "Quarterfinals"},
		{4, 2, "Semifinals"},
		{4, 3, "Finals"},
	}
	for _, tt := range tests {
		name := playoffRoundName(tt.total, tt.index)
		if name != tt.expected {
			t.Errorf("playoffRoundName(%d, %d) = %q, want %q", tt.total, tt.index, name, tt.expected)
		}
	}
}

func TestGenerateOTR_PairingStructure(t *testing.T) {
	mt, eng := setupTestTournament(t)
	data, _ := GenerateOTR(mt, eng)
	var otr OTR
	json.Unmarshal(data, &otr)
	for _, round := range otr.Rounds {
		for _, pairing := range round.Pairings {
			if pairing.PlayerA == nil {
				t.Error("PlayerA should never be nil")
			}
			if pairing.PlayerB == nil {
				t.Error("no byes expected in 4-player tournament")
			}
		}
	}
}
