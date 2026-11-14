# OpenSwiss

A web application for running Swiss-system tournaments. Built with Go, PostgreSQL, and server-rendered HTML.

## Features

- **Tournament management** — Create and run Swiss-system tournaments with configurable points, rounds, and top cut
- **Markdown descriptions** — Tournament descriptions support CommonMark plus GFM tables, with a live preview in the create/edit forms
- **Player registration** — Preregistration with optional decklist submission
- **Round timer** — A shared round clock at the top of a live tournament page, set and started by any staff member down to Judge. Pause/resume, restart, or clear it; past time it counts *up* in red, because a round does not end at time. It keeps itself current without a page reload, and nothing is displayed until someone starts it
- **Live standings** — Real-time standings with tiebreakers (opponent match win %, game win %, opponent game win %). Players who drop are shown at the position their points earned them, flagged as dropped
- **Player statistics** — A public page per player at `/players/{name}` (`/stats` goes to your own): career record, per-tournament finishes, full match history, and head-to-head records
- **Playoff brackets** — Top-cut single elimination playoffs
- **Tournament deletion** — Tournament admins can delete an event at any stage; the delete cascades to registrations, staff, and results, so it also disappears from every participant's statistics. Deleting one that has already started requires the global `admin` role, and the web form makes you type a confirmation phrase. There is no undo
- **OTR export** — Export tournament results in Open Tournament Results v1 format
- **REST API** — Full API for programmatic tournament management
- **Account settings** — Self-service display name, password and email changes, plus API key management, at `/account`
- **Email verification** — New accounts must confirm their email before login (when SMTP is configured)
- **Account lockout** — Brute-force protection: per-IP rate limit on auth endpoints plus per-account lockout after repeated failures
- **Strict Content-Security-Policy** — No inline scripts/styles, no third-party origins; everything is served same-origin
- **Health probes** — `/healthz` (liveness) and `/readyz` (DB-pinging readiness) for orchestrators
- **Metrics** — Built-in `/metrics` endpoint with request counts, latency, status codes, and Go runtime stats (admin-only)
- **Structured logs** — JSON logs with per-request IDs (echoed in `X-Request-Id` header) for triage
- **Self-contained binary** — Templates, static assets, and migrations are embedded via `go:embed`
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

# Apply migrations, then run the server
export DATABASE_URL="postgres://openswiss:openswiss@localhost:5432/openswiss?sslmode=disable"
export SECURE_COOKIES=false   # local HTTP, no TLS
go run . migrate
go run . serve
```

The server starts on `http://localhost:8080` by default.

Register an account through the web UI. Without SMTP configured the account is auto-verified and you're logged in. Then promote it to admin:

```bash
docker exec openswiss-db psql -U openswiss -c \
  "UPDATE users SET roles = '{player,organizer,admin}' WHERE email = 'your@email.com';"
```

### Subcommands

| Command | What it does |
|---------|--------------|
| `openswiss serve` (default) | Run the HTTP server |
| `openswiss migrate` | Apply pending DB migrations and exit |
| `openswiss backfill-results` | Build the player statistics projection for finished tournaments that predate it |

Production deploys should run `migrate` once before rolling the server, so multiple replicas don't race each other on `migrate.Up()`.

`backfill-results` is only needed once, after upgrading to a version that has player statistics: it walks finished tournaments and derives their `matches` and `tournament_results` rows. Re-running it is a no-op. Pass `--force` to rebuild tournaments that already have rows, which is safe because `tournaments.engine_state` remains the source of truth.

## Configuration

All configuration is through environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `LISTEN_ADDR` | `:8080` | Address and port to listen on |
| `RATE_LIMIT_PER_MIN` | `60` | API rate limit per IP per minute (`/api/v1/*`) |
| `AUTH_RATE_LIMIT_PER_MIN` | `10` | Per-IP rate limit on auth endpoints (`/login`, `/register`, etc.) |
| `BASE_URL` | `http://localhost:8080` | Public base URL (used in verification + password reset emails) |
| `SECURE_COOKIES` | `true` | Set to `false` if serving over plain HTTP (e.g. local dev). Secure cookies require HTTPS or browsers will silently drop them. |
| `TRUSTED_PROXIES` | *(empty)* | Comma-separated CIDR list of reverse proxies allowed to set `X-Forwarded-For`. Required for accurate rate limiting behind a proxy; ignored otherwise. The compose stack defaults this to the docker bridge ranges. |
| `SMTP_HOST` | *(empty)* | SMTP server hostname. When set with `SMTP_FROM`, enables email verification and password reset. |
| `SMTP_PORT` | `587` | SMTP server port (587 for STARTTLS, 465 for implicit TLS) |
| `SMTP_USER` | *(empty)* | SMTP username (omit for unauthenticated relay) |
| `SMTP_PASSWORD` | *(empty)* | SMTP password |
| `SMTP_FROM` | *(empty)* | Sender email address for outgoing mail |

## Project Structure

```
main.go              # Subcommand dispatcher
serve.go             # `openswiss serve` — runs the HTTP server
migrate.go           # `openswiss migrate` — applies DB migrations
assets.go            # go:embed declarations for templates/static/migrations
internal/
  api/               # REST API handlers
  auth/              # Password hashing, session/API key generation
  db/                # Database access layer
  engine/            # swisstools engine wrapper
  export/            # OTR export
  handlers/          # Web UI handlers
  middleware/        # Recover, RealIP, RequestID, CSRF, rate limit, auth, etc.
  models/            # Domain types
  results/           # Derives the statistics projection from engine state
migrations/          # SQL migrations (embedded into the binary)
templates/           # HTML templates (embedded into the binary)
static/              # CSS and JS (embedded into the binary)
  app.js             #   Theme toggle, nav, CSRF injection, confirm-on-submit
  round-timer.js     #   Round clock: ticks locally, polls for start/pause/clear
  style.css          #   The whole stylesheet; no framework, no build step
cmd/
  devseed/           # Seeds a demo database with a sample tournament history
scripts/
  backup.sh          # Looped pg_dump used by the compose backup sidecar
```

## Demo Environment

To click around the app with realistic data already in it:

```bash
make demo
```

That starts a throwaway PostgreSQL container, applies migrations, seeds a sample
tournament history, and serves it from source at <http://localhost:8080>. Sign in as
`dylan@openswiss.test` (password `password123`) and click **Stats** — the seeded account
plays in four finished tournaments, so the statistics pages have real history to show.
Every seeded account uses the same password; `organizer@openswiss.test` has the organizer
and admin roles.

The dataset deliberately covers the awkward cases: byes from odd-sized fields, guest
players with no account, both kinds of drop (a player's unactioned drop *request* versus a
real organizer drop), top-cut brackets, an in-progress tournament that must **not** appear
in anyone's statistics, and one open for registration.

| Target | What it does |
|---|---|
| `make demo` | Seed and serve (Ctrl-C to stop) |
| `make demo-seed` | Rebuild the dataset only, leaving any running server alone |
| `make demo-db-down` | Stop and delete the demo database |

It uses its own container on port 5434, so it cannot collide with the integration-test
database (5433) or the compose stack from `make dev`. Note that `make dev` runs the
*published* Docker image, whereas `make demo` runs your working tree. Re-seeding is
destructive: `cmd/devseed` truncates every table before rebuilding, from a fixed random
seed so runs are reproducible.

## Testing

Run the unit tests (no external services required):

```bash
go test ./...
```

### Integration tests

The `db` and `engine` packages have integration tests that run against a real PostgreSQL database. These are gated behind a build tag and skipped by default.

The Makefile targets automatically create and tear down a test PostgreSQL container:

```bash
# Run integration tests
make test-integration

# Run 5000-player load test
make test-load
```

## REST API

The REST API is available under `/api/v1/`. Authenticate with a Bearer token (API keys can be created from the account settings page at `/account` or via the API).

Player statistics are public and need no authentication:

| Endpoint | Returns |
|----------|---------|
| `GET /api/v1/players/{name}/stats` | Career totals and per-tournament finishes |
| `GET /api/v1/players/{name}/matches` | Match history, paginated with `page` / `per_page` |
| `GET /api/v1/players/{name}/head-to-head` | Record against each opponent who has an account |
| `GET /api/v1/users/me/stats` | The authenticated caller's own stats |

So is the round clock, which is what a live tournament page polls:

| Endpoint | Returns |
|----------|---------|
| `GET /api/v1/tournaments/{id}/round-timer` | The live round's clock, or `{"state": "none"}` |

It always answers `200`, even with no round live and no clock started, because every open tournament
page polls it and a `404` would be indistinguishable from an outage. `remaining_seconds` goes
negative in overtime. It is sent `Cache-Control: no-store`: the page polls one unchanging URL, and a
cached response would freeze the clock until a manual refresh. Controlling the clock needs the Judge tier on that tournament:

| Endpoint | Body | Effect |
|----------|------|--------|
| `POST /api/v1/tournaments/{id}/round-timer` | `{"minutes": 50}` | Start, or restart with a new length |
| `POST /api/v1/tournaments/{id}/round-timer` | `{"action": "pause"}` | Freeze the clock |
| `POST /api/v1/tournaments/{id}/round-timer` | `{"action": "resume"}` | Continue, adding back the paused interval |
| `DELETE /api/v1/tournaments/{id}/round-timer` | — | Clear the clock |

See [SPEC.md](SPEC.md) for the full API reference.

## Deployment

A `Makefile` provides shortcuts for common tasks. Run `make help` to see all targets.

### Docker Compose (Recommended)

Docker Compose sets up PostgreSQL, OpenSwiss, and Caddy (automatic HTTPS) together.
The compose file pulls the pre-built image from Docker Hub, so no build step is needed on the server.

```bash
# First time only: generate a .env with a strong POSTGRES_PASSWORD
make setup

# Local development (self-signed TLS on localhost)
make dev

# Production — set your domain to get a real Let's Encrypt certificate
make deploy DOMAIN=tournaments.example.com

# Tail logs
make dev-logs      # or: make deploy-logs
```

The compose stack refuses to start if `.env` is missing or doesn't define `POSTGRES_PASSWORD`. `make setup` writes one with a freshly generated 32-char password; edit the file afterwards to add SMTP credentials, `DOMAIN`, etc.

The `DOMAIN` environment variable controls the Caddy server name. When set to a
public domain, Caddy automatically obtains and renews TLS certificates from
Let's Encrypt. When omitted it defaults to `localhost` with a self-signed cert.

You can pin a specific image version with `IMAGE_TAG`:

```bash
make deploy DOMAIN=tournaments.example.com IMAGE_TAG=v1.2.0
```

To pass additional OpenSwiss configuration (e.g. SMTP), copy the example
environment file and edit it:

```bash
cp .env.example .env
```

Docker Compose reads `.env` automatically. See [.env.example](.env.example) for
all available options.

After startup, register an account and promote it to admin:

```bash
make promote-admin EMAIL=your@email.com
```

When SMTP is configured, the registered account starts unverified — clicking the verification link in the email is required before login. For testing without working email delivery you can short-circuit it:

```bash
make verify-user EMAIL=your@email.com
```

### Backups

The compose stack includes a `backup` sidecar that runs `pg_dump` on a configurable interval (nightly by default), gzips the output, rotates old files, and writes them to `./backups/` on the host.

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_INTERVAL` | `86400` | Seconds between dumps |
| `BACKUP_RETENTION` | `14` | How many recent dumps to keep |
| `BACKUP_OFFSITE_CMD` | *(empty)* | Optional shell command run after each successful dump with the new file path as `$1` (use to push to S3, B2, rclone, etc.) |

Test a restore at least once before relying on these — backups you've never restored are aspirational backups.

### Building & Pushing Images

```bash
make build                  # Build the Docker image (tagged dstathis/openswiss:latest)
make push                   # Build and push to Docker Hub
make push IMAGE_TAG=v1.2.0  # Build and push a specific tag
```

### Docker (Manual)

Build and run with Docker. Apply migrations first, then start the server:

```bash
docker build -t openswiss .

docker run --rm \
  -e DATABASE_URL="postgres://openswiss:openswiss@db:5432/openswiss?sslmode=disable" \
  openswiss migrate

docker run -d --name openswiss \
  -e DATABASE_URL="postgres://openswiss:openswiss@db:5432/openswiss?sslmode=disable" \
  -e SECURE_COOKIES=true \
  -e BASE_URL="https://tournaments.example.com" \
  -p 8080:8080 \
  openswiss
```

### Reverse Proxy (nginx)

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
