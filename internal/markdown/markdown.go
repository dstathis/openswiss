// Package markdown renders user-authored Markdown (currently tournament
// descriptions) to HTML that is safe to embed in a page.
//
// Safety comes from goldmark's default renderer configuration: html.WithUnsafe
// is deliberately NOT set, so raw HTML in the source is replaced with an
// "omitted" comment rather than passed through, and link/image destinations
// using javascript:, vbscript:, file: or non-image data: schemes are dropped.
// That makes a separate sanitizer unnecessary; the strict Content-Security-
// Policy in internal/middleware is the second layer. See markdown_test.go —
// those properties are asserted, not assumed.
package markdown

import (
	"bytes"
	"html/template"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// md is safe for concurrent use, so one instance serves every request.
var md = goldmark.New(
	// GFM gives tables, strikethrough, task lists and autolinks on top of
	// CommonMark.
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		// People type descriptions into a textarea and expect the line
		// breaks they pressed enter for. CommonMark would swallow them.
		html.WithHardWraps(),
	),
)

// ToHTML renders Markdown source to sanitized HTML. A nil or empty source
// renders to nothing.
func ToHTML(src *string) template.HTML {
	if src == nil || *src == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(*src), &buf); err != nil {
		// Convert only fails on a writer error, which a bytes.Buffer can't
		// produce. Fall back to escaped plain text rather than emitting the
		// half-written document sitting in buf.
		return template.HTML(template.HTMLEscapeString(*src))
	}
	return template.HTML(buf.String())
}

// Plain strips Markdown syntax and returns at most max runes of plain text,
// truncated on a word boundary with an ellipsis. Used for the excerpt on
// tournament cards, where rendered HTML would wreck the grid layout.
//
// The return value is a plain string, so html/template escapes it normally.
func Plain(src *string, max int) string {
	if src == nil || *src == "" {
		return ""
	}
	source := []byte(*src)
	doc := md.Parser().Parse(text.NewReader(source))

	var b strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch node := n.(type) {
		case *ast.Text:
			if entering {
				b.Write(node.Segment.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					b.WriteByte(' ')
				}
			}
		case *ast.String:
			if entering {
				b.Write(node.Value)
			}
		case *ast.AutoLink:
			if entering {
				b.Write(node.Label(source))
			}
		default:
			// Separate blocks so "# Title\n\nBody" doesn't come out as
			// "TitleBody". Code block content lives in node.Lines() rather
			// than in Text children, so it is skipped for free.
			if !entering && n.Type() == ast.TypeBlock {
				b.WriteByte(' ')
			}
		}
		return ast.WalkContinue, nil
	})

	// strings.Fields collapses every run of whitespace, including the
	// newlines the walk preserved.
	return truncate(strings.Join(strings.Fields(b.String()), " "), max)
}

// truncate cuts s to at most max runes, preferring the last word boundary.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	cut, count := len(s), 0
	for i := range s { // range over a string yields rune start offsets
		if count == max {
			cut = i
			break
		}
		count++
	}
	out := s[:cut]
	if idx := strings.LastIndexByte(out, ' '); idx > 0 {
		out = out[:idx]
	}
	return strings.TrimRight(out, " ") + "…"
}
