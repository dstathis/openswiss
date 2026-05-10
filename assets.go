package main

import "embed"

// templateFS holds the HTML layouts and pages. Embedding lets the binary run
// without bundling the templates/ tree alongside it — important for k8s and
// for distroless / scratch container images.
//
//go:embed templates/layouts/*.html templates/pages/*.html
var templateFS embed.FS

// staticFS holds CSS and other static assets served under /static/.
//
//go:embed static
var staticFS embed.FS

// migrationsFS holds the SQL migration files run by `openswiss migrate`.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS
