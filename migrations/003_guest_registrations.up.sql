-- Allow registrations to be either a real user OR a guest added by the organizer.
-- Adds a denormalized display_name column so a single unique index covers both cases.

ALTER TABLE registrations ADD COLUMN guest_name TEXT;
ALTER TABLE registrations ADD COLUMN display_name TEXT;

-- Backfill display_name for existing rows from the joined users table.
UPDATE registrations r SET display_name = u.display_name
FROM users u WHERE r.user_id = u.id;

ALTER TABLE registrations ALTER COLUMN display_name SET NOT NULL;
ALTER TABLE registrations ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE registrations
    ADD CONSTRAINT registrations_user_or_guest_check
    CHECK ((user_id IS NULL) <> (guest_name IS NULL));

ALTER TABLE registrations DROP CONSTRAINT registrations_tournament_id_user_id_key;

CREATE UNIQUE INDEX idx_registrations_user_per_tournament
    ON registrations (tournament_id, user_id) WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX idx_registrations_display_name_per_tournament
    ON registrations (tournament_id, lower(display_name));
