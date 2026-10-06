-- One administrator, signed in with a server-side session. No account is
-- created here (no default password): use `server admin create`.
CREATE TABLE IF NOT EXISTS admin_users (
 id BIGSERIAL PRIMARY KEY,
 username TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL, -- argon2id, PHC string; never the password
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 password_changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- At most one row.
CREATE UNIQUE INDEX IF NOT EXISTS admin_users_single ON admin_users ((true));

-- Only the SHA-256 of the session cookie value is stored, so a database copy
-- cannot be replayed as a cookie.
CREATE TABLE IF NOT EXISTS admin_sessions (
 token_hash BYTEA PRIMARY KEY,
 admin_id BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
 csrf_token TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS admin_sessions_expires ON admin_sessions(expires_at);
