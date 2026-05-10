-- Initial schema. Squashed from the original incremental migrations
-- (initial / password_resets / guest_registrations / email_verification /
-- account_lockout) before the first deployment, so this represents the
-- final shape of the schema as one transaction. Future schema changes
-- should be added as new numbered migration files on top of this one.

CREATE TABLE users (
    id                    BIGSERIAL PRIMARY KEY,
    email                 TEXT NOT NULL UNIQUE,
    display_name          TEXT NOT NULL UNIQUE,
    password_hash         TEXT NOT NULL,
    roles                 TEXT[] NOT NULL DEFAULT '{player}',
    email_verified_at     TIMESTAMPTZ,
    failed_login_attempts INT NOT NULL DEFAULT 0,
    locked_until          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE api_keys (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash   TEXT NOT NULL,
    prefix     TEXT NOT NULL,
    name       TEXT NOT NULL,
    last_used  TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_prefix ON api_keys(prefix);

CREATE TABLE tournaments (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    description      TEXT,
    scheduled_at     TIMESTAMPTZ,
    location         TEXT,
    max_players      INT NOT NULL DEFAULT 0,
    num_rounds       INT,
    require_decklist BOOL NOT NULL DEFAULT false,
    decklist_public  BOOL NOT NULL DEFAULT false,
    points_win       INT NOT NULL DEFAULT 3,
    points_draw      INT NOT NULL DEFAULT 1,
    points_loss      INT NOT NULL DEFAULT 0,
    top_cut          INT NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'scheduled',
    organizer_id     BIGINT NOT NULL REFERENCES users(id),
    engine_state     JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tournaments_status ON tournaments(status);
CREATE INDEX idx_tournaments_organizer_id ON tournaments(organizer_id);
CREATE INDEX idx_tournaments_scheduled_at ON tournaments(scheduled_at);

-- A registration belongs to either a real user OR an organizer-added guest.
-- display_name is denormalized so a single unique index can enforce per-
-- tournament name uniqueness across both real and guest entries atomically.
CREATE TABLE registrations (
    id               BIGSERIAL PRIMARY KEY,
    tournament_id    BIGINT NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id          BIGINT REFERENCES users(id) ON DELETE CASCADE,
    guest_name       TEXT,
    display_name     TEXT NOT NULL,
    decklist         JSONB,
    status           TEXT NOT NULL DEFAULT 'pending',
    engine_player_id INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT registrations_user_or_guest_check
        CHECK ((user_id IS NULL) <> (guest_name IS NULL))
);

CREATE INDEX idx_registrations_tournament_id ON registrations(tournament_id);
CREATE INDEX idx_registrations_user_id ON registrations(user_id);
CREATE UNIQUE INDEX idx_registrations_user_per_tournament
    ON registrations (tournament_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX idx_registrations_display_name_per_tournament
    ON registrations (tournament_id, lower(display_name));

CREATE TABLE password_resets (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_password_resets_token_hash ON password_resets(token_hash);
CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);

CREATE TABLE email_verifications (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_verifications_token_hash ON email_verifications(token_hash);
CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);
