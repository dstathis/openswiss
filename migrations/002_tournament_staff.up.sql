-- Per-tournament staff roles. Replaces the single-creator permission model
-- where only tournaments.organizer_id could manage a tournament. Going
-- forward, all permission checks for tournament management route through
-- this table; tournaments.organizer_id is retained as the "created by"
-- record and is no longer authoritative for permissions.

CREATE TABLE tournament_staff (
    tournament_id BIGINT      NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users(id)       ON DELETE CASCADE,
    tier          TEXT        NOT NULL CHECK (tier IN ('admin', 'co_organizer', 'judge')),
    granted_by    BIGINT               REFERENCES users(id)       ON DELETE SET NULL,
    granted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tournament_id, user_id)
);

CREATE INDEX idx_tournament_staff_user_id ON tournament_staff(user_id);

-- Backfill: each existing tournament's creator becomes its first admin.
-- granted_by is self-referential here (the creator granted themselves the
-- role at tournament-creation time) which the SET NULL FK allows.
INSERT INTO tournament_staff (tournament_id, user_id, tier, granted_by, granted_at)
SELECT id, organizer_id, 'admin', organizer_id, created_at
FROM tournaments;
