package main

import (
	"bytes"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"
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
