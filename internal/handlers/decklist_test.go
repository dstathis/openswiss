package handlers

import (
	"strings"
	"testing"

	"github.com/dstathis/swisstools"
)

func TestParseDecklist_MainOnly(t *testing.T) {
	input := "4 Lightning Bolt\n2 Counterspell\n"
	dl := parseDecklist(input)
	if dl.Main["Lightning Bolt"] != 4 {
		t.Errorf("Lightning Bolt = %d, want 4", dl.Main["Lightning Bolt"])
	}
	if dl.Main["Counterspell"] != 2 {
		t.Errorf("Counterspell = %d, want 2", dl.Main["Counterspell"])
	}
	if len(dl.Sideboard) != 0 {
		t.Errorf("expected empty sideboard, got %v", dl.Sideboard)
	}
}

func TestParseDecklist_WithSideboard(t *testing.T) {
	input := "4 Brainstorm\n\nSideboard\n3 Pyroblast\n2 Force of Will\n"
	dl := parseDecklist(input)
	if dl.Main["Brainstorm"] != 4 {
		t.Errorf("Brainstorm = %d, want 4", dl.Main["Brainstorm"])
	}
	if dl.Sideboard["Pyroblast"] != 3 {
		t.Errorf("Pyroblast = %d, want 3", dl.Sideboard["Pyroblast"])
	}
	if dl.Sideboard["Force of Will"] != 2 {
		t.Errorf("Force of Will = %d, want 2", dl.Sideboard["Force of Will"])
	}
}

func TestParseDecklist_SideboardCaseInsensitive(t *testing.T) {
	input := "4 Brainstorm\nSIDEBOARD\n2 Pyroblast"
	dl := parseDecklist(input)
	if dl.Sideboard["Pyroblast"] != 2 {
		t.Errorf("expected 2 Pyroblast in sideboard, got %d", dl.Sideboard["Pyroblast"])
	}
}

func TestParseDecklist_SkipsBlankLines(t *testing.T) {
	input := "4 Brainstorm\n\n\n2 Daze\n"
	dl := parseDecklist(input)
	if len(dl.Main) != 2 {
		t.Errorf("expected 2 main entries, got %d", len(dl.Main))
	}
}

func TestParseDecklist_SkipsInvalidLines(t *testing.T) {
	input := "no quantity here\nfourteen Brainstorm\n4 Brainstorm\n"
	dl := parseDecklist(input)
	if len(dl.Main) != 1 {
		t.Errorf("expected only valid line to parse, got %d entries", len(dl.Main))
	}
	if dl.Main["Brainstorm"] != 4 {
		t.Error("expected Brainstorm 4")
	}
}

func TestParseDecklist_TrimsWhitespace(t *testing.T) {
	input := "  4   Lightning Bolt  \n"
	dl := parseDecklist(input)
	if dl.Main["Lightning Bolt"] == 0 {
		t.Errorf("expected Lightning Bolt to parse with leading/trailing whitespace, got %v", dl.Main)
	}
}

func TestParseDecklist_EmptyInput(t *testing.T) {
	dl := parseDecklist("")
	if len(dl.Main) != 0 || len(dl.Sideboard) != 0 {
		t.Error("empty input should yield empty decklist")
	}
}

func TestFormatDecklist_MainOnly(t *testing.T) {
	dl := swisstools.Decklist{
		Main:      map[string]int{"Brainstorm": 4},
		Sideboard: map[string]int{},
	}
	out := formatDecklist(dl)
	if !strings.Contains(out, "4 Brainstorm") {
		t.Errorf("expected '4 Brainstorm' in output, got %q", out)
	}
	if strings.Contains(out, "Sideboard") {
		t.Errorf("did not expect Sideboard header, got %q", out)
	}
}

func TestFormatDecklist_WithSideboard(t *testing.T) {
	dl := swisstools.Decklist{
		Main:      map[string]int{"Brainstorm": 4},
		Sideboard: map[string]int{"Pyroblast": 3},
	}
	out := formatDecklist(dl)
	if !strings.Contains(out, "4 Brainstorm") {
		t.Errorf("expected '4 Brainstorm' in output, got %q", out)
	}
	if !strings.Contains(out, "Sideboard") {
		t.Errorf("expected Sideboard header, got %q", out)
	}
	if !strings.Contains(out, "3 Pyroblast") {
		t.Errorf("expected '3 Pyroblast' in output, got %q", out)
	}
}

func TestFormatDecklist_RoundTrip(t *testing.T) {
	original := swisstools.Decklist{
		Main:      map[string]int{"Brainstorm": 4, "Daze": 2},
		Sideboard: map[string]int{"Pyroblast": 3},
	}
	formatted := formatDecklist(original)
	parsed := parseDecklist(formatted)
	if parsed.Main["Brainstorm"] != 4 {
		t.Errorf("Brainstorm round-trip = %d, want 4", parsed.Main["Brainstorm"])
	}
	if parsed.Main["Daze"] != 2 {
		t.Errorf("Daze round-trip = %d, want 2", parsed.Main["Daze"])
	}
	if parsed.Sideboard["Pyroblast"] != 3 {
		t.Errorf("Pyroblast round-trip = %d, want 3", parsed.Sideboard["Pyroblast"])
	}
}

func TestResolvePairings_NamesAndByes(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	for _, n := range []string{"Alice", "Bob", "Charlie"} {
		if err := eng.AddPlayer(n); err != nil {
			t.Fatalf("AddPlayer: %v", err)
		}
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}

	pairings := eng.GetRound()
	resolved := resolvePairings(&eng, pairings)
	if len(resolved) != len(pairings) {
		t.Fatalf("len(resolved) = %d, want %d", len(resolved), len(pairings))
	}

	var sawBye bool
	for _, rp := range resolved {
		if rp.IsBye {
			sawBye = true
			if rp.PlayerBID != swisstools.BYE_OPPONENT_ID {
				t.Errorf("bye pairing should have PlayerBID == BYE_OPPONENT_ID, got %d", rp.PlayerBID)
			}
		}
		if rp.PlayerAID != 0 && rp.PlayerAName == "" {
			t.Errorf("expected PlayerAName for player %d", rp.PlayerAID)
		}
	}

	// 3 players => one round with one match plus a bye
	if !sawBye {
		t.Error("expected a bye in 3-player tournament")
	}
}

func TestResolvePairings_NegativeWinsClampedToZero(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	for _, n := range []string{"Alice", "Bob"} {
		if err := eng.AddPlayer(n); err != nil {
			t.Fatalf("AddPlayer: %v", err)
		}
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	pairings := eng.GetRound()
	resolved := resolvePairings(&eng, pairings)
	for _, rp := range resolved {
		if rp.PlayerAWins < 0 || rp.PlayerBWins < 0 || rp.Draws < 0 {
			t.Errorf("expected non-negative wins/draws, got %+v", rp)
		}
	}
}
