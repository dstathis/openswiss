# OpenSwiss — Software Specification

**Version:** 0.4 (Draft)
**License:** AGPL-3.0
**Date:** 2026-05-10

---

## 1. Overview

OpenSwiss is an open-source web application for running Swiss-style trading card game tournaments. It provides tournament scheduling, player registration, live tournament management, optional single-elimination top cut (playoff), and results export. The application is game-agnostic and contains no game-specific branding.

The core Swiss pairing engine and playoff bracket are provided by [`github.com/dstathis/swisstools`](https://github.com/dstathis/swisstools) (v0.2.0+).

---

## 2. Technology Stack

| Layer | Technology |
|---|---|
| Language | Go |
| Web Framework | Standard library (`net/http`) + [chi](https://github.com/go-chi/chi) router |
| Frontend | Server-side rendered Go templates, responsive/mobile-first CSS |
| Database | PostgreSQL |
| DB Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) (embedded via `go:embed`) |
| Authentication | Session-based (web) and bearer API keys (REST), bcrypt password hashing, email verification, per-account lockout |
| Swiss Engine | `github.com/dstathis/swisstools` (v0.2.0+) |
| Configuration | Environment variables |

**Rationale:** Keeping the entire stack in Go (server-rendered HTML, plain HTML forms) minimizes build complexity, makes the project easy to contribute to, and avoids a separate frontend build pipeline. State changes use full-page POST/redirect; partial-update interactivity (live standings updates without a refresh) is deferred — see §11.

### 2.1 Mobile-Friendly Design

The UI must be fully usable on phones and tablets. Tournament organizers may enter results from a phone at an event, and players will check pairings/standings on their devices.

**Approach:**
- **Mobile-first responsive CSS** — All layouts designed for small screens first, scaled up with media queries.
- **No CSS framework dependency** — Use a small custom stylesheet with CSS Grid/Flexbox. No Bootstrap/Tailwind to keep the project dependency-free on the frontend.
- **Key mobile considerations:**
  - Pairings and standings tables scroll horizontally on small screens or reflow to a card-based layout.
  - Result entry forms use large touch targets (minimum 44×44px tap areas).
  - Navigation collapses to a hamburger menu on narrow viewports.
  - Font sizes and spacing follow accessibility best practices (minimum 16px base font to prevent iOS zoom).
- **Viewport meta tag** on all pages: `<meta name="viewport" content="width=device-width, initial-scale=1">`.

---

## 3. User System

### 3.1 Roles

| Role | Description |
|---|---|
| **Admin** | Full system access. Can manage all users, events, and settings. Created via CLI seed command. Implicitly holds `admin` tier on every tournament. |
| **Organizer** | Can **create** tournaments. Management of an individual tournament is controlled by the per-tournament tier (see 3.2), not the global Organizer role — so most management endpoints don't require this role. |
| **Player** | Can browse events, register/unregister, submit decklists, and view results. |

A user can hold multiple roles (e.g., an Organizer is also a Player).

### 3.2 Per-tournament staff tiers

Beyond the global Roles in 3.1, each tournament has its own staff list with three tiers — **Admin**, **Co-organizer**, and **Judge** — that decide who can do what on that specific tournament. The creator is automatically inserted as the tournament's first Admin; any Admin can grant or revoke tiers for other users (subject to a "can't remove the last Admin" safeguard).

Any registered user can hold any tier; the invitee does **not** need the global Organizer role.

Tier authority (higher tiers inherit lower-tier abilities):

| Action | Admin | Co-organizer | Judge |
|---|:---:|:---:|:---:|
| Grant / revoke staff, delete tournament | ✓ | | |
| Edit settings, open/close registration, start/finish, advance/repair rounds, add player, start/advance playoff | ✓ | ✓ | |
| Drop player, submit results (Swiss + playoff), view/submit decklists | ✓ | ✓ | ✓ |

The global `admin` role transparently maps to per-tournament `Admin` everywhere, so system admins can intervene on any tournament without explicit grants.

### 3.3 Authentication & Accounts

- Email + password registration with bcrypt hashing.
- Session-based auth using secure, HTTP-only cookies backed by a DB session table.
- Password reset via email token (requires SMTP configuration; optional — if unconfigured, admins reset passwords manually).
- Admins can promote users to Organizer role.

### 3.4 User Profile

Fields:
- Display name (unique, used in tournaments)
- Email (unique, used for login)
- Password (hashed)
- Role(s)

---

## 4. Tournament Lifecycle

### 4.1 States

```
Scheduled → Registration Open → In Progress → [Playoff] → Finished → (Archived)
```

| State | Description |
|---|---|
| **Scheduled** | Event created with date/time/details. Not yet accepting registrations. |
| **Registration Open** | Players can register (and optionally submit decklists). |
| **In Progress** | Tournament is live. Swiss rounds are paired, results are entered. |
| **Playoff** | (Optional) Single-elimination top cut bracket is running. |
| **Finished** | All rounds (and playoff, if applicable) complete. Final standings available. Results exportable. |

### 4.2 Tournament Settings (set at creation)

| Setting | Type | Description |
|---|---|---|
| Name | string | Tournament name |
| Description | text | Free-form description (format, rules, etc.) |
| Date/Time | timestamp | Scheduled start time |
| Location | string | Venue or "Online" |
| Max Players | int (optional) | Player cap; 0 = unlimited |
| Number of Rounds | int (optional) | If unset, organizer advances rounds manually. When set, `NextRound()` auto-finishes the Swiss portion after this many rounds. Uses `swisstools.SetMaxRounds()`. |
| Top Cut | int (optional) | Number of players for single-elimination playoff (must be a power of 2: 4, 8, 16…). 0 = no top cut. |
| Require Decklist | bool | If true, players must submit a decklist to complete registration |
| Decklist Public | bool | If true, decklists are visible to all players after the tournament starts |
| Points for Win | int | Default: 3 |
| Points for Draw | int | Default: 1 |
| Points for Loss | int | Default: 0 |

### 4.3 Registration

- Players register via the event page when registration is open.
- If decklists are required, registration is considered **pending** until a decklist is submitted.
- Organizers can view the registration list and manually add/remove players.
- Players can unregister before the tournament starts.
- **Manual add (guests):** Organizers can add players who never created an account. Guest registrations have `user_id = NULL` and a stored `guest_name`/`display_name`; they appear in the registrations list and standings just like real-user registrations. The same flow works pre-tournament (`scheduled` / `registration_open`) and mid-tournament (`in_progress`); pre-tournament writes only the registration row, mid-tournament also adds the player to the engine and stores the engine player id back. Guests are created `confirmed` regardless of `require_decklist`; the organizer can submit a decklist on their behalf later via the per-registration decklist editor.
- **Display name collisions:** The denormalized `registrations.display_name` is enforced unique per tournament (case-insensitive). When the organizer adds a guest whose name collides with any existing entry in the tournament, a "(2)", "(3)", … suffix is appended until unique. When a real user registers and their `display_name` collides with an existing **guest**, the guest is renamed to the next free suffix and the real user keeps their account name; real users never have their name suffixed (and two real users can never collide because `users.display_name` is globally unique).

### 4.4 Decklists

Decklists use the swisstools `Decklist` type (`Main map[string]int`, `Sideboard map[string]int`).

**Input format:** Plain text, one entry per line, in the form `<quantity> <card name>`. A blank line or the header `Sideboard` separates main deck from sideboard.

Example:
```
4 Lightning Bolt
4 Monastery Swiftspear
20 Mountain

Sideboard
3 Smash to Smithereens
2 Tormod's Crypt
```

### 4.5 Running a Tournament

The organizer drives the tournament through a management dashboard:

#### Swiss Rounds

1. **Start Tournament** — Locks registration (no new registrations unless organizer manually adds late entries). If `num_rounds` is set, calls `swisstools.SetMaxRounds()`. Calls `swisstools.StartTournament()` which pairs Round 1.
2. **View Pairings** — Current round pairings displayed with table numbers. Past rounds viewable via `swisstools.GetRoundByNumber()`. Match results readable via `Pairing.PlayerAWins()`, `PlayerBWins()`, `Draws()`. Players also see their pairing on their own dashboard.
3. **Enter Results** — Organizer enters match results (wins/losses/draws for each match). Calls `swisstools.AddResult()`.
4. **Advance Round** — Once all results are in, organizer clicks "Next Round". Calls `swisstools.NextRound()` then `swisstools.Pair()`. If max rounds is set and reached, `NextRound()` automatically finishes the Swiss portion.
5. **Drop Player** — Organizer can drop a player between rounds. Calls `swisstools.RemovePlayerById()`.
6. **Finish Swiss** — Organizer can explicitly end Swiss rounds via `swisstools.FinishTournament()`, or let `NextRound()` auto-finish when max rounds is reached.
7. **View Standings** — Live standings available to all via `swisstools.GetStandings()`. Full player data available via `swisstools.GetPlayers()`.

#### Top Cut (Playoff)

If the tournament has a top cut configured:

8. **Start Playoff** — After Swiss rounds finish, organizer starts the single-elimination bracket. Calls `swisstools.StartPlayoff(topN)` which seeds the top N players by Swiss standings into a bracket (seed 1 vs seed N, seed 2 vs seed N-1, etc.).
9. **View Playoff Bracket** — The bracket is displayed showing all rounds, seeds, and matchups. Current round pairings via `swisstools.GetPlayoffRound()`, historical via `GetPlayoffRoundByNumber()`.
10. **Enter Playoff Results** — Organizer enters game results for each playoff match. Calls `swisstools.AddPlayoffResult()`. Draws are not allowed — one player must advance.
11. **Advance Playoff Round** — Calls `swisstools.NextPlayoffRound()` which validates results, determines winners, and either pairs the next round or finishes the playoff.
12. **Playoff Complete** — When the final match is decided, the playoff auto-finishes. The tournament status transitions to Finished.

### 4.6 Player Self-Service During Tournament

- View current round pairing and table assignment.
- View live standings.
- Request a drop (organizer approves).

---

## 5. Database Schema

The application uses PostgreSQL for all persistent data. The swisstools tournament state (pairing engine state) is stored as a JSON blob via `DumpTournament()` / `LoadTournament()` and is the source of truth for pairings and standings.

### 5.1 Tables

```sql
-- Users
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    email       TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    roles       TEXT[] NOT NULL DEFAULT '{player}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessions
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,  -- secure random token
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Tournaments
CREATE TABLE tournaments (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT,
    scheduled_at     TIMESTAMPTZ,
    location         TEXT,
    max_players      INT NOT NULL DEFAULT 0,
    num_rounds       INT,                         -- NULL = manual advancement
    require_decklist BOOL NOT NULL DEFAULT false,
    decklist_public  BOOL NOT NULL DEFAULT false,
    points_win       INT NOT NULL DEFAULT 3,
    points_draw      INT NOT NULL DEFAULT 1,
    points_loss      INT NOT NULL DEFAULT 0,
    top_cut          INT NOT NULL DEFAULT 0,             -- 0 = no top cut; must be power of 2 (4, 8, 16...)
    status           TEXT NOT NULL DEFAULT 'scheduled',  -- scheduled, registration_open, in_progress, playoff, finished
    organizer_id     BIGINT NOT NULL REFERENCES users(id), -- creator-of-record; not authoritative for permissions (see tournament_staff)
    engine_state     JSONB,                       -- swisstools DumpTournament() output
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-tournament staff (admin / co_organizer / judge). The creator is
-- inserted as the first admin atomically with the tournament insert.
-- All tournament-management permission checks route through this table.
CREATE TABLE tournament_staff (
    tournament_id BIGINT      NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users(id)       ON DELETE CASCADE,
    tier          TEXT        NOT NULL CHECK (tier IN ('admin', 'co_organizer', 'judge')),
    granted_by    BIGINT               REFERENCES users(id)       ON DELETE SET NULL,
    granted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tournament_id, user_id)
);

-- Registrations
-- A registration is either a real user (user_id NOT NULL, guest_name NULL)
-- or a guest added by the organizer (user_id NULL, guest_name NOT NULL).
-- display_name is denormalized so a single unique index covers both cases.
CREATE TABLE registrations (
    id            BIGSERIAL PRIMARY KEY,
    tournament_id BIGINT NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id       BIGINT REFERENCES users(id) ON DELETE CASCADE,
    guest_name    TEXT,
    display_name  TEXT NOT NULL,                  -- copied from users.display_name or guest_name input
    decklist      JSONB,                          -- {main: {card: count}, sideboard: {card: count}}
    status        TEXT NOT NULL DEFAULT 'pending', -- pending (awaiting decklist), confirmed, dropped
    engine_player_id INT,                          -- swisstools internal player ID
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((user_id IS NULL) <> (guest_name IS NULL))
);
CREATE UNIQUE INDEX idx_registrations_user_per_tournament
    ON registrations (tournament_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_registrations_display_name_per_tournament
    ON registrations (tournament_id, lower(display_name));
```

### 5.2 Design Notes

- **engine_state (JSONB):** The full swisstools tournament state is serialized and stored after every mutation (result entry, round advance, etc.). This is the authoritative source for pairings, standings, and playoff bracket. It is loaded with `LoadTournament()` before every operation and saved with `DumpTournament()` after. The dump includes the playoff state when present.
- **engine_player_id:** Links a registration row to the swisstools internal player ID so we can map pairings/standings back to users.
- **display_name on registrations:** Denormalized from `users.display_name` for real-user registrations (or copied from the organizer's guest input). This lets a single unique index `(tournament_id, lower(display_name))` enforce per-tournament name uniqueness across both real-user and guest entries atomically. Writes that risk collision (`Register`, `CreateGuestRegistration`) take `SELECT … FOR UPDATE` on the tournament row to serialize collision resolution.
- **Roles** are stored as a PostgreSQL text array for simplicity.
- **top_cut:** Stored in the tournaments table so the UI knows whether to offer the "Start Playoff" action after Swiss rounds complete.

---

## 6. Web UI Routes

The application is server-rendered. All routes return full HTML pages; state-changing actions are plain HTML POST forms that redirect on success (`303 See Other`). All pages are responsive and mobile-friendly.

### 6.1 Public Routes

| Method | Path | Description |
|---|---|---|
| GET | `/` | Homepage — upcoming tournaments |
| GET | `/tournaments` | Browse all tournaments |
| GET | `/tournaments/{id}` | Tournament detail (schedule, standings, registrations) |
| GET | `/login` | Login page |
| POST | `/login` | Login |
| GET | `/register` | Registration page |
| POST | `/register` | Create account |
| POST | `/logout` | Logout |

### 6.2 Player Routes (auth required)

| Method | Path | Description |
|---|---|---|
| POST | `/tournaments/{id}/register` | Register for a tournament |
| POST | `/tournaments/{id}/unregister` | Unregister from a tournament |
| GET | `/tournaments/{id}/decklist` | Decklist submission form |
| POST | `/tournaments/{id}/decklist` | Submit/update decklist |
| GET | `/dashboard` | Player dashboard — upcoming registrations, active tournaments |
| POST | `/tournaments/{id}/drop` | Request drop from active tournament |

### 6.3 Tournament Management Routes

Creating a tournament requires the global `organizer` role. Once it exists, access to each management route is governed by the per-tournament staff tier (see 3.2). The **Min tier** column shows the lowest tier that may invoke the route.

| Method | Path | Min tier | Description |
|---|---|---|---|
| GET | `/tournaments/new` | _global `organizer`_ | Create tournament form |
| POST | `/tournaments/new` | _global `organizer`_ | Create tournament (creator becomes the first Admin) |
| GET | `/tournaments/{id}/manage` | Judge | Tournament management dashboard |
| POST | `/tournaments/{id}/edit` | Co-organizer | Edit tournament settings |
| POST | `/tournaments/{id}/open-registration` | Co-organizer | Open registration |
| POST | `/tournaments/{id}/start` | Co-organizer | Start tournament (lock reg, pair round 1) |
| POST | `/tournaments/{id}/results` | Judge | Submit match results for current round |
| POST | `/tournaments/{id}/next-round` | Co-organizer | Advance to next round |
| POST | `/tournaments/{id}/re-pair` | Co-organizer | Re-pair current round |
| POST | `/tournaments/{id}/finish` | Co-organizer | Finish Swiss rounds explicitly |
| POST | `/tournaments/{id}/add-player` | Co-organizer | Manually add a guest player. Form field: `player_name`. |
| POST | `/tournaments/{id}/drop-player` | Judge | Drop a player. Form field is `registration_id` pre-tournament or `player_id` mid-tournament. |
| GET  | `/tournaments/{id}/registrations/{regID}/decklist` | Judge | Organizer-side decklist editor for any registration (works for guests). |
| POST | `/tournaments/{id}/registrations/{regID}/decklist` | Judge | Submit/replace a decklist on a player's behalf. |
| POST | `/tournaments/{id}/start-playoff` | Co-organizer | Start single-elimination top cut bracket |
| POST | `/tournaments/{id}/playoff-results` | Judge | Submit playoff match results |
| POST | `/tournaments/{id}/next-playoff-round` | Co-organizer | Advance playoff bracket |
| GET | `/tournaments/{id}/export` | Public | Export results |
| GET  | `/tournaments/{id}/staff` | Admin | Staff management page: list current staff with controls to change tier or remove, plus a grant form with DisplayName typeahead. |
| POST | `/tournaments/{id}/staff` | Admin | Grant a user staff access. Form fields: `display_name`, `tier`. Sends a best-effort email to the new staff member. |
| POST | `/tournaments/{id}/staff/{userID}/tier` | Admin | Change a staff member's tier. Form field: `tier`. Refused (409) if it would demote the last admin. |
| POST | `/tournaments/{id}/staff/{userID}/remove` | Admin (or self) | Remove a staff member. Any user may remove themselves; everyone else needs Admin. Refused (409) if it would remove the last admin. |

### 6.4 Admin Routes (admin role required)

| Method | Path | Description |
|---|---|---|
| GET | `/admin/users` | User management |
| POST | `/admin/users/{id}/role` | Update user roles |

---

## 7. REST API

OpenSwiss exposes a JSON REST API under the `/api/v1/` prefix for programmatic access. The API supports all operations available through the web UI, enabling third-party tools, bots, and automation.

### 7.1 Authentication

- API requests authenticate via **API key** passed in the `Authorization` header: `Authorization: Bearer <api_key>`.
- API keys are tied to a user account and inherit that user's roles/permissions.
- Users generate and revoke API keys from their profile page (or via the API itself after session auth).
- API keys are stored as bcrypt hashes in the database (only the prefix is shown to the user after creation).

### 7.2 Database Addition

```sql
CREATE TABLE api_keys (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash    TEXT NOT NULL,           -- bcrypt hash of the full key
    prefix      TEXT NOT NULL,           -- first 8 chars, for display/identification
    name        TEXT NOT NULL,           -- user-provided label (e.g. "CI bot")
    last_used   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ              -- NULL = never expires
);
```

### 7.3 General Conventions

- All request/response bodies are `application/json`.
- Errors return a JSON object: `{"error": "message"}`.
- List endpoints support pagination via `?page=N&per_page=N` (default 50, max 100).
- Timestamps are ISO 8601 / RFC 3339.
- Rate limiting: 60 requests/minute per API key (configurable).

### 7.4 Endpoints

#### Tournaments

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments` | Public | List tournaments (filterable by status, date) |
| POST | `/api/v1/tournaments` | Global `organizer` | Create a tournament (creator becomes the first Admin) |
| GET | `/api/v1/tournaments/{id}` | Public | Get tournament details |
| PATCH | `/api/v1/tournaments/{id}` | Co-organizer | Update tournament settings |
| DELETE | `/api/v1/tournaments/{id}` | Admin | Delete a tournament (only if scheduled/registration_open) |
| POST | `/api/v1/tournaments/{id}/open-registration` | Co-organizer | Open registration |
| POST | `/api/v1/tournaments/{id}/start` | Co-organizer | Start tournament |
| POST | `/api/v1/tournaments/{id}/finish` | Co-organizer | Finish Swiss rounds |
| GET | `/api/v1/tournaments/{id}/export` | Public | Export OTR results (finished tournaments only) |

#### Rounds & Results

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments/{id}/rounds` | Public | List all rounds with pairings and results |
| GET | `/api/v1/tournaments/{id}/rounds/current` | Public | Get current round pairings |
| GET | `/api/v1/tournaments/{id}/rounds/{round}` | Public | Get specific round pairings/results |
| POST | `/api/v1/tournaments/{id}/rounds/current/results` | Judge | Submit match results (batch) |
| POST | `/api/v1/tournaments/{id}/rounds/next` | Co-organizer | Advance to next round |

#### Standings

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments/{id}/standings` | Public | Get current standings |

#### Players & Registration

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments/{id}/players` | Public | List registered players |
| POST | `/api/v1/tournaments/{id}/players` | Player | Register for tournament |
| DELETE | `/api/v1/tournaments/{id}/players/me` | Player | Unregister from tournament |
| POST | `/api/v1/tournaments/{id}/players/add` | Co-organizer | Add a guest player. JSON body: `{"player_name": "..."}`. Returns the created registration. Works in `scheduled`, `registration_open`, `in_progress`. |
| POST | `/api/v1/tournaments/{id}/players/{pid}/drop` | Judge | Drop a player. `pid` is interpreted as a `registration_id` pre-tournament (deletes the row) or as the swisstools `engine_player_id` once `in_progress`. |
| GET  | `/api/v1/tournaments/{id}/registrations/{regID}/decklist` | Judge | View the decklist on any registration (works for guests). |
| PUT  | `/api/v1/tournaments/{id}/registrations/{regID}/decklist` | Judge | Submit/replace a decklist on a player's behalf. |

#### Decklists

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments/{id}/players/me/decklist` | Player | Get own decklist |
| PUT | `/api/v1/tournaments/{id}/players/me/decklist` | Player | Submit/update decklist |
| GET | `/api/v1/tournaments/{id}/players/{pid}/decklist` | Judge | View a player's decklist |

#### Playoff

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/tournaments/{id}/playoff/start` | Co-organizer | Start top cut bracket |
| GET | `/api/v1/tournaments/{id}/playoff` | Public | Get playoff bracket state |
| GET | `/api/v1/tournaments/{id}/playoff/rounds/current` | Public | Get current playoff round |
| POST | `/api/v1/tournaments/{id}/playoff/rounds/current/results` | Judge | Submit playoff results |
| POST | `/api/v1/tournaments/{id}/playoff/rounds/next` | Co-organizer | Advance playoff round |

#### Staff

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/tournaments/{id}/staff` | Public | List the tournament's staff (user ID, display name, tier, granted-by, granted-at). |
| GET | `/api/v1/tournaments/{id}/staff/search?q=...` | Admin | DisplayName typeahead for the grant form. Returns up to 10 `{id, display_name}` matches (case-insensitive substring). Empty query returns `[]`. |
| POST | `/api/v1/tournaments/{id}/staff` | Admin | Grant access. JSON body: `{"user_id": N, "tier": "..."}` or `{"display_name": "...", "tier": "..."}` (when both are present `user_id` wins). Returns `409` if the user is already on staff — use `PATCH` instead. Sends a best-effort email to the new staff member. |
| PATCH | `/api/v1/tournaments/{id}/staff/{userID}` | Admin | Change a staff member's tier. JSON body: `{"tier": "..."}`. Returns `409` if demoting the last admin. |
| DELETE | `/api/v1/tournaments/{id}/staff/{userID}` | Admin or self | Revoke access. Authenticated users may revoke their own row. Returns `409` if removing the last admin. |

#### Users & API Keys

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/users/me` | Any | Get current user profile |
| POST | `/api/v1/users/me/api-keys` | Any | Create a new API key (returns full key once) |
| GET | `/api/v1/users/me/api-keys` | Any | List API keys (prefix + name only) |
| DELETE | `/api/v1/users/me/api-keys/{id}` | Any | Revoke an API key |

#### Admin

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/admin/users` | Admin | List all users |
| PATCH | `/api/v1/admin/users/{id}` | Admin | Update user roles |

---

## 8. Tournament Results Export Format

Since no widely-adopted open standard exists for Swiss tournament results, OpenSwiss defines a JSON export format called **OpenSwiss Tournament Record (OTR)**.

### 8.1 OTR Schema (v1)

```json
{
  "otr_version": 1,
  "tournament": {
    "name": "Friday Night Swiss",
    "date": "2026-04-14T19:00:00Z",
    "location": "Local Game Store",
    "format": "Swiss",
    "swiss_rounds": 4,
    "player_count": 16,
    "points_win": 3,
    "points_draw": 1,
    "points_loss": 0,
    "top_cut": 8
  },
  "players": [
    {
      "id": 1,
      "name": "Alice",
      "external_id": 10101,
      "final_rank": 1,
      "points": 12,
      "record": { "wins": 4, "losses": 0, "draws": 0 },
      "tiebreakers": {
        "opponent_match_win_pct": 0.6875,
        "game_win_pct": 0.8333,
        "opponent_game_win_pct": 0.5912
      },
      "decklist": {
        "main": { "Lightning Bolt": 4, "Mountain": 20 },
        "sideboard": { "Tormod's Crypt": 2 }
      }
    }
  ],
  "rounds": [
    {
      "round_number": 1,
      "pairings": [
        {
          "player_a": { "id": 1, "wins": 2 },
          "player_b": { "id": 2, "wins": 1 },
          "draws": 0
        },
        {
          "player_a": { "id": 3, "wins": 0 },
          "player_b": null,
          "draws": 0
        }
      ]
    }
  ],
  "playoff": {
    "seeds": [1, 3, 5, 7, 2, 4, 6, 8],
    "rounds": [
      {
        "round_name": "Quarterfinals",
        "pairings": [
          {
            "player_a": { "id": 1, "wins": 2 },
            "player_b": { "id": 8, "wins": 0 },
            "draws": 0
          }
        ]
      },
      {
        "round_name": "Semifinals",
        "pairings": []
      },
      {
        "round_name": "Finals",
        "pairings": []
      }
    ],
    "winner": { "id": 1, "name": "Alice" }
  }
}
```

### 8.2 Design Notes

- `player_b: null` represents a bye.
- `external_id` is optional and intended for linking to an external player database.
- Decklists are included only if the tournament had decklists enabled and public.
- The `playoff` key is only present if the tournament had a top cut. It includes seeding, all bracket rounds with results, and the winner.
- `top_cut` in the tournament metadata is 0 or absent if no top cut was used.
- Bracket round names are derived from the bracket size (Quarterfinals, Semifinals, Finals, etc.).
- The format is intentionally game-agnostic.

---

## 9. Key Implementation Details

### 9.1 swisstools Integration Strategy

Each tournament operation follows this pattern:

```
1. Load engine_state from DB → swisstools.LoadTournament(data)
2. Perform operation (AddResult, NextRound, Pair, etc.)
3. Serialize state → tournament.DumpTournament()
4. Save engine_state back to DB
```

All engine mutations are wrapped in a database transaction. The engine_state column is loaded with `SELECT ... FOR UPDATE` to prevent concurrent modifications.

### 9.2 Mapping Users to Engine Players

When a player registers for a tournament, we:

1. Call `swisstools.AddPlayer(displayName)` on the engine.
2. Call `swisstools.GetPlayerID(displayName)` to get the internal ID.
3. Call `swisstools.SetPlayerExternalID(internalID, user.ID)` to link back.
4. Store `engine_player_id` in the `registrations` table.

This two-way mapping lets us translate between swisstools pairings/standings and our user system.

### 9.3 Project Structure

```
openswiss/
├── cmd/
│   └── openswiss/
│       └── main.go              # Entry point, config loading, server startup
├── internal/
│   ├── auth/                    # Authentication, sessions, middleware, API key validation
│   ├── db/                      # Database connection, queries
│   ├── engine/                  # swisstools wrapper (load/mutate/save pattern)
│   ├── handlers/                # HTTP handlers organized by domain
│   │   ├── admin.go
│   │   ├── auth.go
│   │   ├── player.go
│   │   └── tournament.go
│   ├── api/                     # REST API handlers (JSON)
│   │   ├── tournaments.go
│   │   ├── players.go
│   │   ├── rounds.go
│   │   ├── playoff.go
│   │   ├── users.go
│   │   └── admin.go
│   ├── models/                  # Domain types
│   ├── export/                  # OTR export logic
│   └── middleware/              # HTTP middleware (auth, roles, logging, rate limiting)
├── migrations/                  # SQL migration files
├── templates/                   # Go HTML templates (embedded into the binary)
│   ├── layouts/
│   └── pages/
├── static/                      # CSS, JS (minimal), favicon (embedded into the binary)
├── go.mod
├── go.sum
├── LICENSE                      # AGPL-3.0
├── README.md
└── SPEC.md
```

---

## 10. swisstools v0.2.0 API Summary

All previously identified library gaps have been resolved in v0.2.0. Key additions used by OpenSwiss:

| Feature | Method(s) | Notes |
|---|---|---|
| Round history | `GetRoundByNumber(round int) ([]Pairing, error)` | 1-based round number |
| Match result access | `Pairing.PlayerAWins()`, `PlayerBWins()`, `Draws()` | Readable on any pairing |
| Explicit finish | `FinishTournament() error` | Records standings, marks finished |
| Auto-finish | `SetMaxRounds(n int)` / `GetMaxRounds() int` | `NextRound()` auto-finishes when cap reached |
| Player listing | `GetPlayers() map[int]Player` | Full copy, includes removed players |
| Player type | All fields exported | `Name`, `Points`, `Wins`, `Losses`, `Draws`, `GameWins`, `GameLosses`, `GameDraws`, `Notes`, `Removed`, `RemovedInRound`, `ExternalID`, `Decklist` |
| Playoff bracket | `StartPlayoff(topN int) error` | Seeds top N from Swiss standings; N must be power of 2 |
| Playoff pairings | `GetPlayoffRound()`, `GetPlayoffRoundByNumber(round int)` | 0-based round index for bracket |
| Playoff results | `AddPlayoffResult(id, wins, losses, draws int) error` | Draws recorded but one player must have more wins |
| Playoff advance | `NextPlayoffRound() error` | Validates results, pairs winners or finishes |
| Playoff state | `GetPlayoff() *Playoff`, `GetPlayoffStatus() string` | `none`, `in_progress`, `finished` |
| Persistence | `DumpTournament()` / `LoadTournament()` | Includes playoff state when present |

---

## 11. Out of Scope (Future Work)

These are explicitly deferred and not part of the initial build:

- Live partial-update interactivity (htmx, Hotwire/Turbo, or WebSockets). For now standings/pairings refresh on full page reload.
- Judge/staff role
- Multi-day events with separate Swiss and playoff scheduling
- Payment integration
- Native mobile app (the responsive web UI serves mobile users)
- GraphQL API
- OAuth / social login
- API webhook/event notifications

---

## 12. Summary

| Aspect | Decision |
|---|---|
| Stack | Go + chi + PostgreSQL |
| Auth | Sessions (web) + API keys (REST), bcrypt, email verification, per-account lockout |
| Tournament Engine | swisstools v0.2.0 (JSON state persisted in DB, includes playoff) |
| Frontend | Server-rendered Go templates, mobile-first responsive CSS |
| API | REST JSON under `/api/v1/`, bearer token auth |
| Export Format | OTR v1 (JSON, game-agnostic) |
| License | AGPL-3.0 |

---

**Please review this specification and let me know if everything looks good, or if there are any features to add, remove, or modify.**
