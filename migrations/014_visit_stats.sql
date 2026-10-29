-- Public visit statistics: "Lượt truy cập" and "Đang hoạt động gần đây".
-- Nothing here identifies a person: no IP address, user agent, username or
-- reading history; only random ids chosen by the browser. Counting starts
-- when this migration runs (no back-filled history). Additive, idempotent.

-- One row: page loads counted since started_at. Never cleaned.
CREATE TABLE IF NOT EXISTS visit_totals (
 id SMALLINT PRIMARY KEY CHECK (id = 1),
 total BIGINT NOT NULL DEFAULT 0 CHECK (total >= 0),
 started_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO visit_totals(id) VALUES (1) ON CONFLICT DO NOTHING;

-- Event id of each counted page load (16 random bytes), so a retry of the
-- same load is not counted twice. Kept 24 h, far longer than the browser
-- retries (about 2 minutes), then cleaned in batches.
CREATE TABLE IF NOT EXISTS visit_events (
 event_id BYTEA PRIMARY KEY CHECK (octet_length(event_id) = 16),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS visit_events_created ON visit_events(created_at);

-- Browser stats sessions (shared by the tabs of one browser), stored as the
-- SHA-256 of the random id. last_seen is server time; "active" means
-- last_seen within 5 minutes. Rows unused for 1 h are cleaned.
CREATE TABLE IF NOT EXISTS visit_sessions (
 id_hash BYTEA PRIMARY KEY CHECK (octet_length(id_hash) = 32),
 last_seen TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS visit_sessions_last_seen ON visit_sessions(last_seen);
