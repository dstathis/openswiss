package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func previewRequest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := &TournamentHandler{} // no DB: the preview handler never touches one
	req := httptest.NewRequest("POST", "/tournaments/preview-description",
		strings.NewReader("description="+body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.PreviewDescription(rec, req)
	return rec
}

func TestPreviewDescription_RendersMarkdown(t *testing.T) {
	rec := previewRequest(t, "%23+Format%0A%0A**Modern**+Swiss")

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"<h1>Format</h1>", "<strong>Modern</strong>"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\ngot: %s", want, body)
		}
	}
}

// The preview response is assigned to innerHTML by
// static/description-preview.js, so this endpoint echoing unsanitized input
// would be a self-XSS at best and a stored one the moment the value is saved.
func TestPreviewDescription_Sanitizes(t *testing.T) {
	rec := previewRequest(t, "%3Cscript%3Ealert(1)%3C%2Fscript%3E")

	body := rec.Body.String()
	if strings.Contains(body, "<script") {
		t.Errorf("preview echoed a script tag: %s", body)
	}
	if !strings.Contains(body, "raw HTML omitted") {
		t.Errorf("expected the raw HTML to be replaced, got: %s", body)
	}
}

func TestPreviewDescription_Empty(t *testing.T) {
	rec := previewRequest(t, "")

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}
