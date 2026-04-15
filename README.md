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
| `SECURE_COOKIES` | `false` | Set to `true` to mark session cookies as Secure (requires HTTPS) |

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

## Testing

Run the unit tests (no external services required):

```bash
go test ./...
```

### Integration tests

The `db` and `engine` packages have integration tests that run against a real PostgreSQL database. These are gated behind a build tag and skipped by default.

To run them, start a test database and set `TEST_DATABASE_URL`:

```bash
# Start a test database
docker run -d --name openswiss-test-db \
  -e POSTGRES_USER=openswiss_test \
  -e POSTGRES_PASSWORD=openswiss_test \
  -e POSTGRES_DB=openswiss_test \
  -p 5433:5432 \
  postgres:18

# Run integration tests
TEST_DATABASE_URL="postgres://openswiss_test:openswiss_test@localhost:5433/openswiss_test?sslmode=disable" \
  go test -tags integration -p 1 ./internal/db/ ./internal/engine/
```

To run all tests together:

```bash
TEST_DATABASE_URL="postgres://openswiss_test:openswiss_test@localhost:5433/openswiss_test?sslmode=disable" \
  go test -tags integration -p 1 ./...
```

## REST API

The REST API is available under `/api/v1/`. Authenticate with a Bearer token (API keys can be created from the user dashboard or via the API).

See [SPEC.md](SPEC.md) for the full API reference.

## Deployment

### Docker

Build and run with Docker:

```bash
docker build -t openswiss .

docker run -d --name openswiss \
  -e DATABASE_URL="postgres://openswiss:openswiss@db:5432/openswiss?sslmode=disable" \
  -e SECURE_COOKIES=true \
  -e BASE_URL="https://tournaments.example.com" \
  -p 8080:8080 \
  openswiss
```

### Reverse Proxy (Recommended)

In production, run OpenSwiss behind a reverse proxy that handles TLS termination. Example nginx configuration:

```nginx
server {
    listen 443 ssl http2;
    server_name tournaments.example.com;

    ssl_certificate     /etc/letsencrypt/live/tournaments.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tournaments.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 80;
    server_name tournaments.example.com;
    return 301 https://$host$request_uri;
}
```

When running behind a reverse proxy with TLS, set `SECURE_COOKIES=true` so session cookies are marked `Secure`.

## License

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, version 3.

See [LICENSE](LICENSE) for the full license text.
