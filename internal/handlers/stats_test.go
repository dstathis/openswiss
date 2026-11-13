//go:build integration

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestStatsHandler_UnknownPlayerIs404(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &StatsHandler{DB: database, Tmpl: tmpl}

	rec := httptest.NewRecorder()
	h.PlayerStats(rec, requestWithUser(http.MethodGet, "/players/nobody", "", nil,
		map[string]string{"name": "nobody"}))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if len(tmpl.calls) != 0 {
		t.Errorf("expected no template render for a missing player, got %d", len(tmpl.calls))
	}
}

func TestStatsHandler_PlayerWithNoEvents(t *testing.T) {
	database := testDB(t)
	u := mustCreateUser(t, database, "noevents@example.com", "NoEvents")
	tmpl := &mockTemplate{}
	h := &StatsHandler{DB: database, Tmpl: tmpl}

	rec := httptest.NewRecorder()
	h.PlayerStats(rec, requestWithUser(http.MethodGet, "/players/NoEvents", "", nil,
		map[string]string{"name": "NoEvents"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(tmpl.calls) != 1 || tmpl.calls[0].Name != "player_stats.html" {
		t.Fatalf("unexpected render: %+v", tmpl.calls)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	career := data["Career"].(*models.CareerStats)
	if career.Events != 0 {
		t.Errorf("Events = %d, want 0", career.Events)
	}
	if data["Profile"].(*models.User).ID != u.ID {
		t.Error("Profile is not the player being viewed")
	}
	// Anonymous viewer: User must be a nil *models.User so the layout renders
	// logged-out nav. Compare the typed pointer, not the interface -- a nil
	// pointer inside an interface is not equal to a bare nil.
	if viewer, _ := data["User"].(*models.User); viewer != nil {
		t.Errorf("expected a nil viewer, got %+v", viewer)
	}
	if data["IsSelf"].(bool) {
		t.Error("IsSelf must be false for an anonymous viewer")
	}
}

// Viewing your own page sets IsSelf, and Profile and User are the same person.
func TestStatsHandler_IsSelf(t *testing.T) {
	database := testDB(t)
	u := mustCreateUser(t, database, "self@example.com", "SelfViewer")
	tmpl := &mockTemplate{}
	h := &StatsHandler{DB: database, Tmpl: tmpl}

	rec := httptest.NewRecorder()
	h.PlayerStats(rec, requestWithUser(http.MethodGet, "/players/SelfViewer", "", u,
		map[string]string{"name": "SelfViewer"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if !data["IsSelf"].(bool) {
		t.Error("IsSelf should be true when viewing your own page")
	}
}

// Display names are looked up case-insensitively, matching the unique index.
func TestStatsHandler_LookupIsCaseInsensitive(t *testing.T) {
	database := testDB(t)
	mustCreateUser(t, database, "case@example.com", "MixedCase")
	tmpl := &mockTemplate{}
	h := &StatsHandler{DB: database, Tmpl: tmpl}

	rec := httptest.NewRecorder()
	h.PlayerStats(rec, requestWithUser(http.MethodGet, "/players/mixedcase", "", nil,
		map[string]string{"name": "mixedcase"}))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestStatsHandler_MyStatsRedirects(t *testing.T) {
	database := testDB(t)
	// A name with a space exercises the path escaping.
	u := mustCreateUser(t, database, "redir@example.com", "Red Irect")
	h := &StatsHandler{DB: database, Tmpl: &mockTemplate{}}

	rec := httptest.NewRecorder()
	h.MyStats(rec, requestWithUser(http.MethodGet, "/stats", "", u, nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/players/Red%20Irect" {
		t.Errorf("Location = %q, want the escaped player URL", loc)
	}
}

// An in-progress tournament contributes nothing to the page.
func TestStatsHandler_InProgressTournamentIsAbsent(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := startedTournament(t, database)

	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	user, err := db.GetUserByID(ctx, database, *regs[0].UserID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	tmpl := &mockTemplate{}
	h := &StatsHandler{DB: database, Tmpl: tmpl}
	rec := httptest.NewRecorder()
	h.PlayerStats(rec, requestWithUser(http.MethodGet, "/", "", nil,
		map[string]string{"name": user.DisplayName}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if got := data["Career"].(*models.CareerStats).Events; got != 0 {
		t.Errorf("expected 0 events for an unfinished tournament, got %d", got)
	}
}

func TestGroupByTournament(t *testing.T) {
	matches := []models.PlayerMatch{
		{TournamentID: 1, TournamentName: "A", Round: 1},
		{TournamentID: 1, TournamentName: "A", Round: 2},
		{TournamentID: 2, TournamentName: "B", Round: 1},
		{TournamentID: 1, TournamentName: "A", Round: 3},
	}
	got := groupByTournament(matches)

	// Grouping is by adjacency, preserving the query's ordering rather than
	// re-sorting, so a repeated id later in the list opens a new group.
	if len(got) != 3 {
		t.Fatalf("expected 3 adjacent groups, got %d", len(got))
	}
	if len(got[0].Matches) != 2 || got[0].TournamentName != "A" {
		t.Errorf("first group = %+v", got[0])
	}
	if len(got[1].Matches) != 1 || got[1].TournamentName != "B" {
		t.Errorf("second group = %+v", got[1])
	}
}
