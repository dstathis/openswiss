-- Round timers: the shared clock for a live round.
--
-- Rounds themselves have no table -- they live inside the opaque
-- tournaments.engine_state swisstools blob -- so the timer needs its own
-- durable home. It is keyed by (tournament_id, round, playoff), the same key
-- shape as `matches`, rather than being a single per-tournament row. Two
-- things follow from that choice:
--
--   * Advancing a round leaves the new round with no row, so "display nothing
--     until a judge starts the clock" needs no clearing code on the
--     round-advance path, and a finished round's clock can never leak forward
--     into the current one.
--   * A past round's timer is still on disk, which is harmless and leaves the
--     door open to showing how long each round actually took.
--
-- ends_at is authoritative; there is deliberately no stored "remaining"
-- column. Pausing stamps paused_at, and resuming pushes ends_at forward by the
-- paused interval. That keeps one value to reason about, and makes overtime
-- fall out as a negative (ends_at - now) rather than needing its own state.

-- Default round length, used only to prefill the start-timer form. A human
-- always presses Start, so this never starts a clock on its own. NULL = the
-- organizer has not set one and the field comes up blank.
ALTER TABLE tournaments ADD COLUMN round_minutes INT;

CREATE TABLE round_timers (
    tournament_id    BIGINT      NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    round            INT         NOT NULL,   -- 1-based, matching matches.round
    playoff          BOOLEAN     NOT NULL DEFAULT false,
    duration_seconds INT         NOT NULL,   -- what was asked for; survives pause/resume shifting ends_at
    ends_at          TIMESTAMPTZ NOT NULL,   -- authoritative; past it the round is in overtime
    paused_at        TIMESTAMPTZ,            -- non-NULL = frozen, remaining is ends_at - paused_at
    started_by       BIGINT               REFERENCES users(id) ON DELETE SET NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tournament_id, round, playoff)
);
