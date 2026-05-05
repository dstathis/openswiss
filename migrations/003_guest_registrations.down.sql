-- Reverse 003. Will fail if any guest registrations exist (no user_id).
DROP INDEX IF EXISTS idx_registrations_display_name_per_tournament;
DROP INDEX IF EXISTS idx_registrations_user_per_tournament;
ALTER TABLE registrations DROP CONSTRAINT IF EXISTS registrations_user_or_guest_check;
ALTER TABLE registrations ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE registrations ADD CONSTRAINT registrations_tournament_id_user_id_key
    UNIQUE (tournament_id, user_id);
ALTER TABLE registrations DROP COLUMN IF EXISTS display_name;
ALTER TABLE registrations DROP COLUMN IF EXISTS guest_name;
