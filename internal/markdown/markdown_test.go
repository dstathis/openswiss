package markdown

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

// TestToHTMLSanitizes pins the security properties the rest of the app relies
// on: descriptions are attacker-controlled by anyone who can create or
// co-organize a tournament, and the rendered output is injected into a page
// as trusted HTML.
func TestToHTMLSanitizes(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		mustNot  []string
		contains string
	}{
		{
			// The tags themselves are replaced; the text between them
			// survives as inert, escaped body text, which is the correct
			// CommonMark-safe outcome — it displays, it does not execute.
			name:     "script tag is not passed through",
			src:      "hello <script>alert(1)</script> world",
			mustNot:  []string{"<script", "</script"},
			contains: "raw HTML omitted",
		},
		{
			name:    "img onerror is not passed through",
			src:     `<img src=x onerror="alert(1)">`,
			mustNot: []string{"<img", "onerror"},
		},
		{
			name:    "raw html block is not passed through",
			src:     "<div onclick=\"steal()\">\n\ntext\n",
			mustNot: []string{"onclick", "steal()"},
		},
		{
			name:     "javascript: link destination is dropped",
			src:      "[click me](javascript:alert(1))",
			mustNot:  []string{"javascript:"},
			contains: "click me",
		},
		{
			name:    "data: link destination is dropped",
			src:     "[x](data:text/html;base64,PHNjcmlwdD4=)",
			mustNot: []string{"data:text/html"},
		},
		{
			name:    "vbscript: link destination is dropped",
			src:     "[x](vbscript:msgbox)",
			mustNot: []string{"vbscript:"},
		},
		{
			name:    "javascript: image destination is dropped",
			src:     "![x](javascript:alert(1))",
			mustNot: []string{"javascript:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(ToHTML(&tc.src))
			for _, bad := range tc.mustNot {
				if strings.Contains(got, bad) {
					t.Errorf("output contains %q, which must be sanitized away\ngot: %s", bad, got)
				}
			}
			if tc.contains != "" && !strings.Contains(got, tc.contains) {
				t.Errorf("output missing %q\ngot: %s", tc.contains, got)
			}
		})
	}
}

func TestToHTMLRenders(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"heading", "# Format", []string{"<h1>Format</h1>"}},
		{"bold", "**Modern** Swiss", []string{"<strong>Modern</strong>"}},
		{"emphasis", "*five* rounds", []string{"<em>five</em>"}},
		{"list", "- one\n- two", []string{"<ul>", "<li>one", "<li>two"}},
		{"link", "[Rules](https://example.com/rules)", []string{`href="https://example.com/rules"`, ">Rules<"}},
		{"code", "use `swiss` pairing", []string{"<code>swiss</code>"}},
		{"blockquote", "> note", []string{"<blockquote>"}},
		{
			name: "gfm table",
			src:  "| Round | Time |\n|-------|------|\n| 1     | 10am |",
			want: []string{"<table>", "<th>Round</th>", "<td>10am</td>"},
		},
		{"gfm strikethrough", "~~cancelled~~", []string{"<del>cancelled</del>"}},
		{"gfm autolink", "see https://example.com", []string{`href="https://example.com"`}},
		{"hard wrap", "line one\nline two", []string{"<br>"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(ToHTML(&tc.src))
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot: %s", want, got)
				}
			}
		})
	}
}

func TestToHTMLEmpty(t *testing.T) {
	if got := ToHTML(nil); got != "" {
		t.Errorf("ToHTML(nil) = %q, want empty", got)
	}
	if got := ToHTML(ptr("")); got != "" {
		t.Errorf("ToHTML(\"\") = %q, want empty", got)
	}
}

func TestPlain(t *testing.T) {
	tests := []struct {
		name string
		src  string
		max  int
		want string
	}{
		{"nil-safe empty", "", 50, ""},
		{"strips heading marker", "# Format", 50, "Format"},
		{"strips emphasis", "**Modern** *Swiss*", 50, "Modern Swiss"},
		{"keeps link text drops url", "see the [rules doc](https://example.com)", 50, "see the rules doc"},
		{"joins blocks", "# Format\n\nModern Swiss.", 50, "Format Modern Swiss."},
		{"collapses newlines", "one\ntwo\nthree", 50, "one two three"},
		{"strips list markers", "- one\n- two", 50, "one two"},
		{"drops code block content", "intro\n\n```\nsecret code\n```\n\nouter", 50, "intro outer"},
		{"keeps table cell text", "| a | b |\n|---|---|\n| 1 | 2 |", 50, "a b 1 2"},
		{"truncates on word boundary", "the quick brown fox jumps", 13, "the quick…"},
		{"no truncation when short enough", "exactly right", 13, "exactly right"},
		{"zero max", "anything", 0, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Plain(&tc.src, tc.max); got != tc.want {
				t.Errorf("Plain(%q, %d) = %q, want %q", tc.src, tc.max, got, tc.want)
			}
		})
	}

	if got := Plain(nil, 50); got != "" {
		t.Errorf("Plain(nil) = %q, want empty", got)
	}
}

// TestPlainTruncateRuneBoundary guards against slicing a multi-byte rune in
// half, which would emit an invalid UTF-8 replacement character on the page.
func TestPlainTruncateRuneBoundary(t *testing.T) {
	src := strings.Repeat("é", 40)
	got := Plain(&src, 10)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis, got %q", got)
	}
	body := strings.TrimSuffix(got, "…")
	if strings.ContainsRune(body, '�') {
		t.Errorf("output contains a replacement char, a rune was split: %q", got)
	}
	if n := len([]rune(body)); n != 10 {
		t.Errorf("expected 10 runes before the ellipsis, got %d (%q)", n, got)
	}
}
