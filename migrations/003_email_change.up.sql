-- Support self-service email changes. A row in email_verifications with a
-- non-NULL new_email is a *pending address change*: the user's current
-- users.email stays authoritative until the token is consumed, at which point
-- new_email is written onto the account. NULL keeps the original signup
-- semantics (confirm the address already stored on the user).

ALTER TABLE email_verifications ADD COLUMN new_email TEXT;
