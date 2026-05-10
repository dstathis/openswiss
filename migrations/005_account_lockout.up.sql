-- Track failed login attempts per account so we can lock it out after
-- repeated failures, defending against credential stuffing across many IPs.
ALTER TABLE users ADD COLUMN failed_login_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN locked_until         TIMESTAMPTZ;
