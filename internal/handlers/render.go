package handlers

import "io"

// TemplateRenderer is implemented by types that can render named templates.
type TemplateRenderer interface {
	ExecuteTemplate(wr io.Writer, name string, data interface{}) error
}
