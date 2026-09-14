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
		"account.html",
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

// manageData mirrors the map TournamentHandler.ManagePage passes to
// tournament_manage.html, with the two fields the cancel control keys off.
func manageData(status string, isAdmin bool) map[string]interface{} {
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
		"Standings":       []interface{}{},
		"Pairings":        []interface{}{},
		"CurrentRound":    0,
		"PlayoffStatus":   "",
		"PlayoffPairings": []interface{}{},
		"IsAdmin":         isAdmin,
	}
}

func renderManage(t *testing.T, status string, isAdmin bool) string {
	t.Helper()
	templates, err := loadTemplates(templateFS)
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	nt := &namedTemplate{root: templates}
	var buf bytes.Buffer
	if err := nt.ExecuteTemplate(&buf, "tournament_manage.html", manageData(status, isAdmin)); err != nil {
		t.Fatalf("render tournament_manage.html (%s, admin=%v): %v", status, isAdmin, err)
	}
	return buf.String()
}

// The Cancel Tournament button is a destructive control: it must appear only
// for tournament admins, and only before the tournament has started. The
// handler enforces the same rule, but a button that shows up where it cannot
// work (or hides where it should) is its own bug.
func TestManagePage_CancelButtonVisibility(t *testing.T) {
	const action = `action="/tournaments/1/cancel"`
	cases := []struct {
		status  string
		isAdmin bool
		want    bool
	}{
		{models.TournamentStatusScheduled, true, true},
		{models.TournamentStatusRegistrationOpen, true, true},
		{models.TournamentStatusInProgress, true, false},
		{models.TournamentStatusPlayoff, true, false},
		{models.TournamentStatusFinished, true, false},
		{models.TournamentStatusScheduled, false, false},
		{models.TournamentStatusRegistrationOpen, false, false},
		{models.TournamentStatusInProgress, false, false},
	}
	for _, tc := range cases {
		name := tc.status
		if tc.isAdmin {
			name += "/admin"
		} else {
			name += "/non-admin"
		}
		t.Run(name, func(t *testing.T) {
			got := strings.Contains(renderManage(t, tc.status, tc.isAdmin), action)
			if got != tc.want {
				t.Errorf("cancel form present = %v, want %v", got, tc.want)
			}
		})
	}
}

// Destructive actions in this app are gated client-side by data-confirm
// (static/app.js). Losing the attribute would make cancel a single misclick.
func TestManagePage_CancelButtonHasConfirm(t *testing.T) {
	html := renderManage(t, models.TournamentStatusRegistrationOpen, true)
	idx := strings.Index(html, `action="/tournaments/1/cancel"`)
	if idx == -1 {
		t.Fatal("cancel form not rendered")
	}
	// Widen to the enclosing <form ...>...</form> so attributes that precede
	// the action (method, data-confirm) are in scope.
	start := strings.LastIndex(html[:idx], "<form")
	if start == -1 {
		t.Fatal("cancel action is not inside a form element")
	}
	form := html[start:]
	if end := strings.Index(form, "</form>"); end != -1 {
		form = form[:end]
	}
	if !strings.Contains(form, "data-confirm=") {
		t.Error("cancel form is missing data-confirm")
	}
	if !strings.Contains(form, "permanently deleted") {
		t.Error("confirm text should warn that the tournament is permanently deleted")
	}
	if !strings.Contains(form, `method="POST"`) {
		t.Error("cancel form must POST")
	}
}

// Other manage-page controls must be unaffected by the cancel addition.
func TestManagePage_OtherActionsStillRender(t *testing.T) {
	scheduled := renderManage(t, models.TournamentStatusScheduled, true)
	for _, want := range []string{
		`action="/tournaments/1/open-registration"`,
		`action="/tournaments/1/start"`,
		`href="/tournaments/1/staff"`,
	} {
		if !strings.Contains(scheduled, want) {
			t.Errorf("scheduled manage page missing %s", want)
		}
	}
	inProgress := renderManage(t, models.TournamentStatusInProgress, true)
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
