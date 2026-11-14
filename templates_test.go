package main

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dstathis/openswiss/internal/models"
)

// TestLoadTemplates parses every embedded layout + page template through the
// real production load path (loadTemplates + templateFuncs + the embedded
// templateFS). It is the only test that exercises the actual template files —
// the handler tests use a mock renderer — so it guards against syntax errors in
// any templates/pages/*.html. Salvaged from the removed cmd/openswiss package,
// which parsed templates from disk with a hand-rolled func map.
func TestLoadTemplates(t *testing.T) {
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}

	pages, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		t.Fatalf("glob pages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no page templates embedded — check the //go:embed directive in assets.go")
	}
	if len(templates) != len(pages) {
		t.Errorf("loaded %d templates for %d page files", len(templates), len(pages))
	}
	for _, page := range pages {
		name := path.Base(page)
		tmpl, ok := templates[name]
		if !ok {
			t.Errorf("page %s was not loaded", name)
			continue
		}
		// namedTemplate.ExecuteTemplate always executes the "layout" block, so
		// a page that fails to pull in the layout would 500 at render time.
		if tmpl.Lookup("layout") == nil {
			t.Errorf("page %s does not define the layout block", name)
		}
	}
}

// Every template handlers ask for by name must exist, or the request 500s at
// render time with no compile-time warning.
func TestLoadTemplates_CoversHandlerTemplateNames(t *testing.T) {
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	for _, name := range []string{
		"home.html", "tournaments.html", "tournament_detail.html",
		"tournament_new.html", "tournament_manage.html", "tournament_staff.html",
		"dashboard.html", "decklist.html", "organizer_decklist.html",
		"login.html", "register.html", "check_email.html", "verify_email.html",
		"forgot_password.html", "reset_password.html", "admin_users.html",
		"account.html", "player_stats.html",
	} {
		if _, ok := templates[name]; !ok {
			t.Errorf("missing template %s", name)
		}
	}
}

func TestLoadTemplates_NoPages(t *testing.T) {
	empty := fstest.MapFS{}
	if _, err := loadTemplates(empty); err == nil {
		t.Error("expected an error when no pages are present")
	}
}

func TestLoadTemplates_BadSyntax(t *testing.T) {
	broken := fstest.MapFS{
		"templates/layouts/base.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}x{{end}}`)},
		"templates/pages/bad.html":    &fstest.MapFile{Data: []byte(`{{if}}`)},
	}
	if _, err := loadTemplates(broken); err == nil {
		t.Error("expected a parse error for a malformed page")
	} else if !strings.Contains(err.Error(), "bad.html") {
		t.Errorf("error should name the offending page, got: %v", err)
	}
}

func TestNamedTemplate_UnknownName(t *testing.T) {
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	nt := &namedTemplate{root: templates}
	if err := nt.ExecuteTemplate(&bytes.Buffer{}, "does_not_exist.html", nil); err == nil {
		t.Error("expected an error for an unknown template name")
	}
}

// renderPage runs a page template through the real load path, the same way
// namedTemplate does in production.
func renderPage(t *testing.T, name string, data map[string]interface{}) string {
	t.Helper()
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	var buf bytes.Buffer
	nt := &namedTemplate{root: templates}
	if err := nt.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return buf.String()
}

// fragment returns the page text between start and the next end marker, so
// an assertion about the description can't accidentally match the layout
// (which has its own <script src> and <h1>).
func fragment(t *testing.T, html, start, end string) string {
	t.Helper()
	i := strings.Index(html, start)
	if i < 0 {
		t.Fatalf("page does not contain %q:\n%s", start, html)
	}
	rest := html[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("no %q after %q:\n%s", end, start, html)
	}
	return rest[:j]
}

// detailData mirrors the map TournamentHandler.Detail passes to
// tournament_detail.html.
func detailData(description string) map[string]interface{} {
	tourn := &models.Tournament{
		ID: 1, Name: "Test Cup", Status: models.TournamentStatusScheduled,
	}
	if description != "" {
		tourn.Description = &description
	}
	return map[string]interface{}{
		"User":           nil,
		"Tournament":     tourn,
		"Registrations":  []models.Registration{},
		"MyRegistration": nil,
		"Standings":      []interface{}{},
		"Pairings":       []interface{}{},
		"PairingsLabel":  "",
		"CurrentRound":   0,
		"CanManage":      false,
		"Staff":          []interface{}{},
		"RoundIsLive":    false,
		"RoundTimer":     (*models.RoundTimer)(nil),
		"TimerLabel":     "",
	}
}

// liveDetailData is detailData for a tournament mid-round: a live round, some
// pairings and standings. timer may be nil, which is the state before any
// judge has started a clock.
func liveDetailData(timer *models.RoundTimer) map[string]interface{} {
	data := detailData("Some **description**.")
	data["Tournament"].(*models.Tournament).Status = models.TournamentStatusInProgress
	data["RoundIsLive"] = true
	data["TimerLabel"] = "Round 3"
	data["PairingsLabel"] = "Round 3"
	data["CurrentRound"] = 3
	data["Pairings"] = []map[string]interface{}{
		{"PlayerAName": "Alice", "PlayerBName": "Bob", "IsBye": false,
			"PlayerAWins": 2, "PlayerBWins": 0, "Draws": 0,
			"PlayerAID": 1, "PlayerBID": 2},
	}
	data["Standings"] = []map[string]interface{}{
		{"Rank": 1, "Name": "Alice", "Points": 9, "Wins": 3, "Losses": 0, "Draws": 0,
			"Removed": false, "RemovedInRound": 0,
			"Tiebreakers": map[string]interface{}{
				"OpponentMatchWinPct": 0.5, "GameWinPercentage": 0.75, "OpponentGameWinPct": 0.5,
			}},
	}
	data["RoundTimer"] = timer
	return data
}

// During a live round the clock comes first and the pairings second: those are
// the two things a player opening the page mid-round actually wants, and they
// must outrank the standings and the description.
func TestTournamentDetail_TimerAndPairingsComeFirst(t *testing.T) {
	ends := time.Now().Add(20 * time.Minute)
	html := renderPage(t, "tournament_detail.html", liveDetailData(&models.RoundTimer{
		TournamentID: 1, Round: 3, DurationSeconds: 3000, EndsAt: ends,
	}))

	order := []string{
		`class="round-timer`, // the clock
		"Round 3 Pairings",   // then who you are playing
		"Standings",          // then where you stand
		`<div class="markdown">`,
		"Registered Players",
	}
	prev := -1
	for _, marker := range order {
		i := strings.Index(html, marker)
		if i < 0 {
			t.Fatalf("page is missing %q", marker)
		}
		if i < prev {
			t.Errorf("%q appears before the section that should precede it", marker)
		}
		prev = i
	}
}

// With a live round but no clock started, the section is emitted but hidden, so
// the poll can reveal it without the viewer reloading -- while nothing is
// visible until a judge actually starts it.
func TestTournamentDetail_TimerHiddenUntilStarted(t *testing.T) {
	html := renderPage(t, "tournament_detail.html", liveDetailData(nil))

	section := fragment(t, html, `class="round-timer`, ">")
	if !strings.Contains(section, "hidden") {
		t.Errorf("timer section should carry the hidden attribute when no timer is started: %s", section)
	}
	if !strings.Contains(html, "/static/round-timer.js") {
		t.Error("page should load the countdown script")
	}
}

// A started clock renders unhidden, with the data the script ticks from.
func TestTournamentDetail_TimerVisibleWhenStarted(t *testing.T) {
	ends := time.Now().Add(20 * time.Minute)
	html := renderPage(t, "tournament_detail.html", liveDetailData(&models.RoundTimer{
		TournamentID: 1, Round: 3, DurationSeconds: 3000, EndsAt: ends,
	}))

	section := fragment(t, html, `class="round-timer`, ">")
	if strings.Contains(section, "hidden") {
		t.Errorf("timer section should not be hidden once a timer exists: %s", section)
	}
	if !strings.Contains(html, `data-timer-url="/api/v1/tournaments/1/round-timer"`) {
		t.Error("timer section should carry the poll URL as a data attribute")
	}
	if !strings.Contains(html, ends.UTC().Format(time.RFC3339)) {
		t.Error("timer section should carry ends_at so the clock renders before the first poll")
	}
}

// No live round means no timer markup at all -- not a hidden section.
func TestTournamentDetail_NoTimerSectionWithoutALiveRound(t *testing.T) {
	html := renderPage(t, "tournament_detail.html", detailData("x"))
	if strings.Contains(html, "round-timer") {
		t.Error("a tournament with no live round should emit no timer markup")
	}
}

// The description is the one value in the app rendered as trusted HTML. These
// tests run the real template + the real markdown func, so they catch both a
// template that forgets the markdown pipeline and a markdown package that
// stops sanitizing.
func TestTournamentDetail_DescriptionRendersMarkdown(t *testing.T) {
	html := renderPage(t, "tournament_detail.html", detailData(
		"# Format\n\n**Modern** Swiss, 5 rounds.\n\n- Decklists due 9am\n\n| Round | Time |\n|-------|------|\n| 1 | 10am |"))

	for _, want := range []string{
		`<div class="markdown">`,
		"<h1>Format</h1>",
		"<strong>Modern</strong>",
		"<li>Decklists due 9am",
		"<table>",
		"<td>10am</td>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
}

func TestTournamentDetail_DescriptionIsSanitized(t *testing.T) {
	cases := []struct {
		name string
		desc string
		bad  string
	}{
		{"script tag", "<script>alert(1)</script>", "<script"},
		{"event handler", `<img src=x onerror="alert(1)">`, "onerror"},
		{"javascript link", "[x](javascript:alert(1))", "javascript:"},
		{"data uri link", "[x](data:text/html;base64,PHN2Zz4=)", "data:text/html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := renderPage(t, "tournament_detail.html", detailData(tc.desc))
			desc := fragment(t, page, `<div class="markdown">`, "</div>")
			if strings.Contains(desc, tc.bad) {
				t.Errorf("description block contains unsanitized %q: %s", tc.bad, desc)
			}
		})
	}
}

func TestTournamentDetail_NoDescription(t *testing.T) {
	html := renderPage(t, "tournament_detail.html", detailData(""))

	if strings.Contains(html, `class="markdown"`) {
		t.Error("empty description should render no markdown block")
	}
}

// Cards get a plain-text excerpt: rendered HTML would break the grid, and a
// raw markdown string would leak "#" and "**" into the card.
func TestTournamentsList_CardExcerptIsPlainText(t *testing.T) {
	desc := "# Format\n\n**Modern** Swiss with a [rules doc](https://example.com)."
	html := renderPage(t, "tournaments.html", map[string]interface{}{
		"User":   nil,
		"Status": "",
		"Tournaments": []models.Tournament{
			{ID: 1, Name: "Test Cup", Status: models.TournamentStatusScheduled, Description: &desc},
		},
	})

	if !strings.Contains(html, `<p class="card-excerpt">Format Modern Swiss with a rules doc.</p>`) {
		t.Errorf("card excerpt not rendered as plain text:\n%s", html)
	}
	excerpt := fragment(t, html, `<p class="card-excerpt">`, "</p>")
	if strings.Contains(excerpt, "<") || strings.Contains(excerpt, "&lt;") {
		t.Errorf("card excerpt should contain no markup, got %q", excerpt)
	}
}

// A 160-rune excerpt keeps the two-line clamp honest.
func TestTournamentsList_CardExcerptTruncates(t *testing.T) {
	desc := strings.Repeat("word ", 100)
	html := renderPage(t, "tournaments.html", map[string]interface{}{
		"User":   nil,
		"Status": "",
		"Tournaments": []models.Tournament{
			{ID: 1, Name: "Test Cup", Status: models.TournamentStatusScheduled, Description: &desc},
		},
	})

	if !strings.Contains(html, "…</p>") {
		t.Errorf("long excerpt was not truncated:\n%s", html)
	}
}

// deleteConfirmPhrase duplicates the constant in internal/handlers rather than
// importing it: this test exists to pin the exact wording the page asks the
// user to type, so it should fail if that wording changes underneath it.
const deleteConfirmPhrase = "I want to permanently delete the tournament."

// manageData mirrors the map TournamentHandler.ManagePage passes to
// tournament_manage.html, with the fields the destructive controls key off.
func manageData(status string, isAdmin, canDelete bool) map[string]interface{} {
	playerUserID := int64(42)
	return map[string]interface{}{
		"User": nil,
		"Tournament": &models.Tournament{
			ID: 1, Name: "Test Cup", Status: status,
			MaxPlayers: 16, PointsWin: 3, PointsDraw: 1, PointsLoss: 0,
		},
		// One registered player so the player-list rows render too. The
		// remaining slices are only ranged over, so empty is enough.
		"Registrations": []models.Registration{
			{ID: 7, TournamentID: 1, UserID: &playerUserID, DisplayName: "Player One", Status: "registered"},
		},
		"Standings":           []interface{}{},
		"Pairings":            []interface{}{},
		"CurrentRound":        0,
		"PlayoffStatus":       "",
		"PlayoffPairings":     []interface{}{},
		"IsAdmin":             isAdmin,
		"CanDelete":           canDelete,
		"DeleteConfirmPhrase": deleteConfirmPhrase,
		"RoundIsLive":         false,
		"RoundTimer":          (*models.RoundTimer)(nil),
		"TimerLabel":          "",
	}
}

// liveManageData is manageData for a tournament mid-round, so the timer
// controls render. timer may be nil, meaning no clock has been started.
func liveManageData(timer *models.RoundTimer) map[string]interface{} {
	data := manageData(models.TournamentStatusInProgress, true, false)
	data["RoundIsLive"] = true
	data["TimerLabel"] = "Round 3"
	data["CurrentRound"] = 3
	data["RoundTimer"] = timer
	return data
}

// With no clock started, the manage page offers Start and nothing else.
func TestTournamentManage_TimerOffersStart(t *testing.T) {
	html := renderPage(t, "tournament_manage.html", liveManageData(nil))

	for _, want := range []string{
		"/tournaments/1/round-timer/start",
		`name="minutes"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("manage page missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"/tournaments/1/round-timer/pause",
		"/tournaments/1/round-timer/clear",
	} {
		if strings.Contains(html, unwanted) {
			t.Errorf("manage page should not offer %q before a timer exists", unwanted)
		}
	}
}

// A running clock offers Pause and Clear.
func TestTournamentManage_TimerOffersPauseAndClearWhenRunning(t *testing.T) {
	html := renderPage(t, "tournament_manage.html", liveManageData(&models.RoundTimer{
		TournamentID: 1, Round: 3, DurationSeconds: 3000, EndsAt: time.Now().Add(20 * time.Minute),
	}))

	for _, want := range []string{
		"/tournaments/1/round-timer/pause",
		"/tournaments/1/round-timer/clear",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("manage page missing %q for a running timer", want)
		}
	}
	if strings.Contains(html, "/tournaments/1/round-timer/resume") {
		t.Error("a running timer should not offer Resume")
	}
}

// A paused clock offers Resume rather than Pause.
func TestTournamentManage_TimerOffersResumeWhenPaused(t *testing.T) {
	paused := time.Now()
	html := renderPage(t, "tournament_manage.html", liveManageData(&models.RoundTimer{
		TournamentID: 1, Round: 3, DurationSeconds: 3000,
		EndsAt: time.Now().Add(20 * time.Minute), PausedAt: &paused,
	}))

	if !strings.Contains(html, "/tournaments/1/round-timer/resume") {
		t.Error("a paused timer should offer Resume")
	}
	if strings.Contains(html, "/tournaments/1/round-timer/pause") {
		t.Error("a paused timer should not offer Pause")
	}
}

// Between rounds there is nothing to time, so no controls at all.
func TestTournamentManage_NoTimerControlsBetweenRounds(t *testing.T) {
	html := renderPage(t, "tournament_manage.html", manageData(models.TournamentStatusRegistrationOpen, true, false))
	if strings.Contains(html, "round-timer") {
		t.Error("timer controls should not render without a live round")
	}
}

// round_minutes is a tournament setting, so both the create form and the edit
// form must expose it or an organizer can never set the default.
func TestRoundMinutesFieldOnTournamentForms(t *testing.T) {
	newPage := renderPage(t, "tournament_new.html", map[string]interface{}{"User": nil})
	if !strings.Contains(newPage, `name="round_minutes"`) {
		t.Error("create form missing the round_minutes field")
	}

	fifty := 50
	data := manageData(models.TournamentStatusScheduled, true, false)
	data["Tournament"].(*models.Tournament).RoundMinutes = &fifty
	editPage := renderPage(t, "tournament_manage.html", data)
	if !strings.Contains(editPage, `name="round_minutes"`) {
		t.Error("edit form missing the round_minutes field")
	}
	if !strings.Contains(editPage, `value="50"`) {
		t.Error("edit form should prefill the stored round_minutes")
	}
}

func renderManage(t *testing.T, status string, isAdmin, canDelete bool) string {
	t.Helper()
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	nt := &namedTemplate{root: templates}
	var buf bytes.Buffer
	if err := nt.ExecuteTemplate(&buf, "tournament_manage.html", manageData(status, isAdmin, canDelete)); err != nil {
		t.Fatalf("render tournament_manage.html (%s, admin=%v, canDelete=%v): %v", status, isAdmin, canDelete, err)
	}
	return buf.String()
}

// The delete control is rendered strictly from CanDelete, which ManagePage
// computes with the same rule the Delete handler enforces. A control that
// shows up where it cannot work (or hides where it should work) is its own
// bug, so the template must not second-guess that flag by status.
func TestManagePage_DeleteFormVisibility(t *testing.T) {
	const action = `action="/tournaments/1/delete"`
	for _, status := range []string{
		models.TournamentStatusScheduled,
		models.TournamentStatusRegistrationOpen,
		models.TournamentStatusInProgress,
		models.TournamentStatusPlayoff,
		models.TournamentStatusFinished,
	} {
		for _, canDelete := range []bool{true, false} {
			name := status
			if canDelete {
				name += "/can-delete"
			} else {
				name += "/cannot-delete"
			}
			t.Run(name, func(t *testing.T) {
				// isAdmin is deliberately the opposite of canDelete in half
				// these cases: the delete form keys off CanDelete alone.
				got := strings.Contains(renderManage(t, status, true, canDelete), action)
				if got != canDelete {
					t.Errorf("delete form present = %v, want %v", got, canDelete)
				}
			})
		}
	}
}

// Deleting is irreversible and, past the start line, rewrites other players'
// statistics. The gate is a phrase the user has to type, checked server-side —
// so the form must actually ask for it, by name and in full.
func TestManagePage_DeleteFormRequiresTypedPhrase(t *testing.T) {
	html := renderManage(t, models.TournamentStatusFinished, true, true)
	idx := strings.Index(html, `action="/tournaments/1/delete"`)
	if idx == -1 {
		t.Fatal("delete form not rendered")
	}
	start := strings.LastIndex(html[:idx], "<form")
	if start == -1 {
		t.Fatal("delete action is not inside a form element")
	}
	form := html[start:]
	if end := strings.Index(form, "</form>"); end != -1 {
		form = form[:end]
	}
	if !strings.Contains(form, `method="POST"`) {
		t.Error("delete form must POST")
	}
	if !strings.Contains(form, `name="confirm"`) {
		t.Error("delete form is missing the confirm input")
	}
	// html/template escapes the apostrophe-free phrase as-is, so a plain
	// substring check is enough to catch a reworded or truncated prompt.
	if !strings.Contains(form, deleteConfirmPhrase) {
		t.Errorf("delete form does not show the required phrase %q", deleteConfirmPhrase)
	}

	// The surrounding section has to say what is actually destroyed; the
	// stats consequence is the part a user will not guess.
	section := html[strings.LastIndex(html[:idx], "<section"):]
	if !strings.Contains(section, "cannot be undone") {
		t.Error("delete section should warn that this cannot be undone")
	}
	if !strings.Contains(section, "statistics") {
		t.Error("delete section should warn that participants lose the event from their statistics")
	}
}

// After a mistyped phrase the handler re-renders this page with DeleteError
// set. The message must be visible and the form must still be there to retype
// into — a re-render that dropped either would be a dead end.
func TestManagePage_DeleteError(t *testing.T) {
	const msg = "That did not match. To delete this tournament, type the phrase exactly: " + deleteConfirmPhrase

	data := manageData(models.TournamentStatusFinished, true, true)
	data["DeleteError"] = msg
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	var buf bytes.Buffer
	if err := (&namedTemplate{root: templates}).ExecuteTemplate(&buf, "tournament_manage.html", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, `class="error"`) {
		t.Error("DeleteError is not rendered in an .error block")
	}
	if !strings.Contains(html, "That did not match.") {
		t.Error("the error message text is missing from the page")
	}
	if !strings.Contains(html, `action="/tournaments/1/delete"`) {
		t.Error("delete form is gone from the error re-render, leaving no way to retry")
	}
	if !strings.Contains(html, `name="confirm"`) {
		t.Error("confirm input is gone from the error re-render")
	}
	// The failed attempt must not come back pre-filled: a near-miss phrase
	// sitting in the box would put deletion one click away.
	if strings.Contains(html, `value="`+deleteConfirmPhrase) {
		t.Error("the confirm input is pre-filled, which defeats the typed confirmation")
	}

	// Without DeleteError the page carries no error block at all.
	clean := renderManage(t, models.TournamentStatusFinished, true, true)
	if strings.Contains(clean, `class="error"`) {
		t.Error("manage page shows an error block when DeleteError is unset")
	}
}

// Other manage-page controls must be unaffected by the delete control.
func TestManagePage_OtherActionsStillRender(t *testing.T) {
	scheduled := renderManage(t, models.TournamentStatusScheduled, true, true)
	for _, want := range []string{
		`action="/tournaments/1/open-registration"`,
		`action="/tournaments/1/start"`,
		`href="/tournaments/1/staff"`,
	} {
		if !strings.Contains(scheduled, want) {
			t.Errorf("scheduled manage page missing %s", want)
		}
	}
	inProgress := renderManage(t, models.TournamentStatusInProgress, true, true)
	for _, want := range []string{
		`action="/tournaments/1/next-round"`,
		`action="/tournaments/1/re-pair"`,
		`action="/tournaments/1/finish"`,
	} {
		if !strings.Contains(inProgress, want) {
			t.Errorf("in-progress manage page missing %s", want)
		}
	}
}

// accountData mirrors the map AccountHandler.render passes to account.html.
func accountData(extra map[string]interface{}) map[string]interface{} {
	lastUsed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	verified := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	data := map[string]interface{}{
		"User": &models.User{
			ID: 1, Email: "player@example.com", DisplayName: "Player One",
			Roles: []string{"player"}, EmailVerifiedAt: &verified,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		"APIKeys": []models.APIKey{
			{ID: 9, Name: "laptop", Prefix: "osk_abc", CreatedAt: verified, LastUsed: &lastUsed},
		},
		"SMTPEnabled": true,
		"CSRFToken":   "token",
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func renderAccount(t *testing.T, extra map[string]interface{}) string {
	t.Helper()
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	nt := &namedTemplate{root: templates}
	var buf bytes.Buffer
	if err := nt.ExecuteTemplate(&buf, "account.html", accountData(extra)); err != nil {
		t.Fatalf("render account.html: %v", err)
	}
	return buf.String()
}

func TestAccountPage_Sections(t *testing.T) {
	html := renderAccount(t, nil)
	for _, action := range []string{
		`action="/account/display-name"`,
		`action="/account/password"`,
		`action="/account/email"`,
		`action="/account/api-keys"`,
		`action="/account/api-keys/9/delete"`,
	} {
		if !strings.Contains(html, action) {
			t.Errorf("account page is missing a form with %s", action)
		}
	}
	if !strings.Contains(html, "player@example.com") {
		t.Error("account page should show the current email")
	}
}

// A freshly minted key is shown once and never again, so the reveal block must
// appear only on the render that created it.
func TestAccountPage_KeyRevealOnlyOnCreate(t *testing.T) {
	if html := renderAccount(t, nil); strings.Contains(html, "key-reveal") {
		t.Error("key reveal block rendered without a new key")
	}
	html := renderAccount(t, map[string]interface{}{"NewAPIKey": "osk_secret_value", "NewAPIKeyName": "laptop"})
	if !strings.Contains(html, "osk_secret_value") {
		t.Error("new key was not revealed on the render that created it")
	}
}

// Revoking a key breaks anything using it, so it's gated by data-confirm
// (static/app.js) like the other destructive actions.
func TestAccountPage_RevokeHasConfirm(t *testing.T) {
	html := renderAccount(t, nil)
	idx := strings.Index(html, `action="/account/api-keys/9/delete"`)
	if idx == -1 {
		t.Fatal("revoke form not rendered")
	}
	start := strings.LastIndex(html[:idx], "<form")
	form := html[start:]
	if end := strings.Index(form, "</form>"); end != -1 {
		form = form[:end]
	}
	if !strings.Contains(form, "data-confirm=") {
		t.Error("revoke form is missing data-confirm")
	}
}

// The user's own name in the nav is the entry point to account settings, and
// only exists when someone is logged in.
func TestNav_AccountLink(t *testing.T) {
	html := renderAccount(t, nil)
	if !strings.Contains(html, `href="/account" class="nav-user"`) {
		t.Error("logged-in nav should link the display name to /account")
	}

	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	nt := &namedTemplate{root: templates}
	var buf bytes.Buffer
	if err := nt.ExecuteTemplate(&buf, "login.html", map[string]interface{}{"User": nil}); err != nil {
		t.Fatalf("render login.html: %v", err)
	}
	if strings.Contains(buf.String(), `href="/account"`) {
		t.Error("logged-out nav should not link to /account")
	}
}

// statsData mirrors the map StatsHandler.PlayerStats passes to
// player_stats.html.
func statsData(career *models.CareerStats, tournaments []models.TournamentResult,
	history []map[string]interface{}, h2h []models.HeadToHead) map[string]interface{} {
	return map[string]interface{}{
		// A nil *models.User, not a bare nil: that is what the handler passes
		// for an anonymous viewer, and the layout nav keys off it.
		"User":        (*models.User)(nil),
		"Profile":     &models.User{ID: 7, DisplayName: "Alice", CreatedAt: time.Now()},
		"IsSelf":      false,
		"Career":      career,
		"Tournaments": tournaments,
		"History":     history,
		"HeadToHead":  h2h,
	}
}

func TestPlayerStatsEmptyState(t *testing.T) {
	html := renderPage(t, "player_stats.html", statsData(&models.CareerStats{}, nil, nil, nil))

	if !strings.Contains(html, "has not finished any tournaments yet") {
		t.Errorf("expected the empty state:\n%s", html)
	}
	// The stat tiles must not render at all for a player with no events.
	if strings.Contains(html, "stat-tiles") {
		t.Error("stat tiles should be hidden when there are no events")
	}
	// An anonymous viewer gets the logged-out nav.
	if strings.Contains(html, `href="/dashboard"`) {
		t.Error("anonymous viewer should not see the Dashboard link")
	}
	if !strings.Contains(html, `href="/login"`) {
		t.Error("anonymous viewer should see the Login link")
	}
}

func TestPlayerStatsRendersRecordAndDroppedBadge(t *testing.T) {
	dropRound := 3
	career := &models.CareerStats{
		Events: 2, EventsWon: 1, EventsDropped: 1, BestFinish: 1,
		Wins: 5, Losses: 2, Draws: 1, Byes: 1,
		GameWins: 11, GameLosses: 5, GameDraws: 1,
		MatchWinPct: 0.625, GameWinPct: 0.6470588,
	}
	tournaments := []models.TournamentResult{
		{TournamentID: 1, TournamentName: "Spring Open", FinalRank: 1, PlayerCount: 8,
			Points: 9, Wins: 3, Losses: 0, Draws: 0, OppMatchWinPct: 0.61, GameWinPct: 0.75, OppGameWinPct: 0.58},
		{TournamentID: 2, TournamentName: "Autumn Cup", FinalRank: 5, PlayerCount: 8,
			Points: 6, Wins: 2, Losses: 2, Draws: 1, Dropped: true, DroppedInRound: &dropRound},
	}
	html := renderPage(t, "player_stats.html", statsData(career, tournaments, nil, nil))

	if !strings.Contains(html, "5–2–1") {
		t.Errorf("career record missing:\n%s", html)
	}
	if !strings.Contains(html, "62.5%") {
		t.Error("match win rate not rendered as a percentage")
	}
	if !strings.Contains(html, `<a href="/tournaments/1">Spring Open</a>`) {
		t.Error("tournament link missing")
	}
	// The dropped event keeps its real rank and is badged.
	if !strings.Contains(html, "5 of 8") {
		t.Error("a dropped player still shows their finishing position")
	}
	if !strings.Contains(html, ">dropped R3</span>") {
		t.Errorf("dropped badge with round missing:\n%s", html)
	}
}

// Opponents with an account link to their page; guests render as plain text.
func TestPlayerStatsLinksOnlyAccountOpponents(t *testing.T) {
	bobID := int64(19)
	history := []map[string]interface{}{{
		"TournamentID":   int64(1),
		"TournamentName": "Spring Open",
		"Matches": []models.PlayerMatch{
			{Round: 1, RoundLabel: "Round 1", Outcome: "win", GameWins: 2,
				OpponentUserID: &bobID, OpponentName: "Bob", OpponentDisplayName: "Bobby"},
			{Round: 2, RoundLabel: "Round 2", Outcome: "loss", GameLosses: 2,
				OpponentName: "Walk In"},
			{Round: 3, RoundLabel: "Round 3", Outcome: "bye", GameWins: 2},
		},
	}}
	html := renderPage(t, "player_stats.html",
		statsData(&models.CareerStats{Events: 1}, nil, history, nil))

	// Linked by current name, labelled with the name played under.
	if !strings.Contains(html, `<a href="/players/Bobby">Bob</a>`) {
		t.Errorf("account opponent should link by current name, labelled by name at event:\n%s", html)
	}
	if strings.Contains(html, `/players/Walk In`) || strings.Contains(html, `/players/Walk%20In`) {
		t.Error("a guest opponent must not be linked")
	}
	if !strings.Contains(html, "Walk In") {
		t.Error("guest opponent name should still be shown")
	}
	if !strings.Contains(html, `badge-bye`) {
		t.Error("bye should be badged")
	}
}

func TestPlayerStatsHeadToHeadNotesExclusions(t *testing.T) {
	h2h := []models.HeadToHead{{OpponentUserID: 19, OpponentDisplayName: "Bobby", Wins: 3, Losses: 1, Matches: 4}}
	html := renderPage(t, "player_stats.html",
		statsData(&models.CareerStats{Events: 1}, nil, nil, h2h))

	if !strings.Contains(html, `<a href="/players/Bobby">Bobby</a>`) {
		t.Error("head-to-head opponent link missing")
	}
	if !strings.Contains(html, "3–1–0") {
		t.Error("head-to-head record missing")
	}
	// The caption explaining why these totals are lower than the career record
	// is load-bearing: without it the numbers look like a bug.
	if !strings.Contains(html, "Byes and guest players are not counted") {
		t.Errorf("missing the explanation of what head-to-head excludes:\n%s", html)
	}
}

// A display name needing URL escaping must be escaped in links.
func TestPlayerStatsEscapesOpponentNamesInLinks(t *testing.T) {
	h2h := []models.HeadToHead{{OpponentUserID: 20, OpponentDisplayName: "Ann O'Hara", Wins: 1, Matches: 1}}
	html := renderPage(t, "player_stats.html",
		statsData(&models.CareerStats{Events: 1}, nil, nil, h2h))

	if !strings.Contains(html, `/players/Ann%20O%27Hara`) {
		t.Errorf("opponent name not path-escaped in the link:\n%s",
			fragment(t, html, `<h2>Head to Head</h2>`, `</section>`))
	}
}
