# OpenSwiss

A web application for running Swiss-system tournaments. Built with Go, PostgreSQL, and server-rendered HTML with htmx.

## Features

- **Tournament management** — Create and run Swiss-system tournaments with configurable points, rounds, and top cut
- **Player registration** — Preregistration with optional decklist submission
- **Live standings** — Real-time standings with tiebreakers (opponent match win %, game win %, opponent game win %)
- **Playoff brackets** — Top-cut single elimination playoffs
- **OTR export** — Export tournament results in Open Tournament Results v1 format
- **REST API** — Full API for programmatic tournament management
- **Mobile-friendly** — Responsive design optimized for phone and tablet use

## Requirements

- Go 1.21+
- Docker (for running PostgreSQL)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/dstathis/openswiss.git
cd openswiss

# Start PostgreSQL in Docker
docker run -d --name openswiss-db \
  -e POSTGRES_USER=openswiss \
  -e POSTGRES_PASSWORD=openswiss \
  -e POSTGRES_DB=openswiss \
  -p 5432:5432 \
  postgres:18

# Run the server
export DATABASE_URL="postgres://openswiss:openswiss@localhost:5432/openswiss?sslmode=disable"
go run ./cmd/openswiss
```

The server starts on `http://localhost:8080` by default.

Register an account through the web UI, then promote it to admin:

```bash
docker exec openswiss-db psql -U openswiss -c \
  "UPDATE users SET roles = '{player,organizer,admin}' WHERE email = 'your@email.com';"
```

## Configuration

All configuration is through environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://localhost:5432/openswiss?sslmode=disable` | PostgreSQL connection string |
| `LISTEN_ADDR` | `:8080` | Address and port to listen on |
| `MIGRATIONS_PATH` | `file://migrations` | Path to migration files |
| `RATE_LIMIT_PER_MIN` | `60` | API rate limit per IP per minute |
| `BASE_URL` | `http://localhost:8080` | Public base URL (used in password reset emails) |
| `SMTP_HOST` | *(empty)* | SMTP server hostname (enables password reset when set with `SMTP_FROM`) |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USER` | *(empty)* | SMTP username (omit for unauthenticated relay) |
| `SMTP_PASSWORD` | *(empty)* | SMTP password |
| `SMTP_FROM` | *(empty)* | Sender email address for outgoing mail |

## Project Structure

```
cmd/openswiss/       # Application entry point
internal/
  api/               # REST API handlers
  auth/              # Password hashing, session/API key generation
  db/                # Database access layer
  engine/            # swisstools engine wrapper
  export/            # OTR export
  handlers/          # Web UI handlers
  middleware/         # Auth, rate limiting middleware
  models/            # Domain types
migrations/          # SQL migrations
templates/           # HTML templates
static/              # CSS and static assets
```

## REST API

The REST API is available under `/api/v1/`. Authenticate with a Bearer token (API keys can be created from the user dashboard or via the API).

See [SPEC.md](SPEC.md) for the full API reference.

## License

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, version 3.

See [LICENSE](LICENSE) for the full license text.
