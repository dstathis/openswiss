-- Normalized read model for player statistics.
--
-- All play data lives in tournaments.engine_state, an opaque swisstools
-- DumpTournament() blob. That is fine as the write-side source of truth, but
-- it cannot answer cross-tournament questions: a player's career record or a
-- head-to-head tally would mean deserializing every tournament they ever
-- entered. These two tables are a projection of that blob, written once when a
-- tournament reaches 'finished'.
--
-- The projection is derived and disposable: engine_state remains authoritative
-- and `openswiss backfill-results --force` can rebuild these tables from it.
--
-- Because 'finished' is a terminal status (nothing transitions out of it and
-- swisstools seals the engine), the projection is append-only and never needs
-- invalidating.

-- One row per player per finished tournament: their final placing and record.
CREATE TABLE tournament_results (
    tournament_id     BIGINT NOT NULL REFERENCES tournaments(id)   ON DELETE CASCADE,
    registration_id   BIGINT NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    -- NULL for guests (walk-ins with no account). Guests are recorded so they
    -- show up in other players' history, but they have no cross-event identity.
    user_id           BIGINT          REFERENCES users(id)         ON DELETE SET NULL,
    display_name      TEXT   NOT NULL,          -- name at the time of the event
    -- From swisstools GetFinalStandings(), which ranks dropped players in
    -- place rather than omitting them, so this is never null.
    final_rank        INT    NOT NULL,
    points            INT    NOT NULL,
    wins              INT    NOT NULL,
    losses            INT    NOT NULL,
    draws             INT    NOT NULL,
    game_wins         INT    NOT NULL,
    game_losses       INT    NOT NULL,
    game_draws        INT    NOT NULL,
    opp_match_win_pct DOUBLE PRECISION NOT NULL,
    game_win_pct      DOUBLE PRECISION NOT NULL,
    opp_game_win_pct  DOUBLE PRECISION NOT NULL,
    -- Taken from the engine's Player.Removed, NOT registrations.status. The
    -- two disagree: the player-facing "Request Drop" button sets the
    -- registration status without removing the player from the engine, so a
    -- request an organizer never actioned must not be recorded as a drop.
    dropped           BOOLEAN NOT NULL DEFAULT false,
    dropped_in_round  INT,                      -- NULL unless dropped
    player_count      INT    NOT NULL,          -- so "top 8" means something
    PRIMARY KEY (tournament_id, registration_id)
);

CREATE INDEX idx_tournament_results_user_id
    ON tournament_results (user_id) WHERE user_id IS NOT NULL;

-- Two rows per match, one from each player's perspective; byes get a single
-- row. Mirroring keeps every query symmetric: a head-to-head tally is a plain
-- WHERE player_user_id = $1 GROUP BY opponent_user_id, with no need to check
-- both sides of a pairing and swap columns.
CREATE TABLE matches (
    id               BIGSERIAL PRIMARY KEY,
    tournament_id    BIGINT  NOT NULL REFERENCES tournaments(id)   ON DELETE CASCADE,
    round            INT     NOT NULL,
    playoff          BOOLEAN NOT NULL DEFAULT false,
    round_label      TEXT    NOT NULL,          -- 'Round 3' / 'Semifinals'

    player_reg_id    BIGINT  NOT NULL REFERENCES registrations(id) ON DELETE CASCADE,
    player_user_id   BIGINT           REFERENCES users(id)         ON DELETE SET NULL,
    player_name      TEXT    NOT NULL,          -- name at the time of the event

    -- SET NULL rather than CASCADE, unlike player_reg_id above. Deleting a
    -- user cascades to their registrations; if this cascaded too, removing one
    -- account would delete the mirrored rows belonging to their opponents and
    -- silently rewrite the stats of accounts that still exist. The opponent
    -- keeps the row and falls back to opponent_name.
    opponent_reg_id  BIGINT           REFERENCES registrations(id) ON DELETE SET NULL,
    opponent_user_id BIGINT           REFERENCES users(id)         ON DELETE SET NULL,
    opponent_name    TEXT,                      -- NULL for a bye

    game_wins        INT     NOT NULL,
    game_losses      INT     NOT NULL,
    game_draws       INT     NOT NULL,
    outcome          TEXT    NOT NULL CHECK (outcome IN ('win', 'loss', 'draw', 'bye')),

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Idempotency guard: a player plays at most one match per round per bracket,
-- so the writer can be re-run safely (backfill, retries, a repeated finish).
CREATE UNIQUE INDEX idx_matches_unique
    ON matches (tournament_id, playoff, round, player_reg_id);

CREATE INDEX idx_matches_player_user_id
    ON matches (player_user_id) WHERE player_user_id IS NOT NULL;

-- Covers the head-to-head aggregate, which filters opponent_user_id IS NOT
-- NULL to exclude byes and guests in one condition.
CREATE INDEX idx_matches_head_to_head
    ON matches (player_user_id, opponent_user_id) WHERE player_user_id IS NOT NULL;

CREATE INDEX idx_matches_tournament_id ON matches (tournament_id);
