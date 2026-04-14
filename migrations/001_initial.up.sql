-- Users
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    roles         TEXT[] NOT NULL DEFAULT '{player}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessions
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- API Keys
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

-- Tournaments
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

-- Registrations
CREATE TABLE registrations (
    id               BIGSERIAL PRIMARY KEY,
    tournament_id    BIGINT NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    decklist         JSONB,
    status           TEXT NOT NULL DEFAULT 'pending',
    engine_player_id INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tournament_id, user_id)
);

CREATE INDEX idx_registrations_tournament_id ON registrations(tournament_id);
CREATE INDEX idx_registrations_user_id ON registrations(user_id);
