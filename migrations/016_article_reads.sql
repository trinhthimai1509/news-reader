-- Read counts per article replace the site-wide visit count of 014
-- (2026-10-07). The visit total is dropped, not converted: a page load is not
-- a read. visit_sessions (browser stats sessions, "Đang hoạt động") stays.
-- Every article starts at 0 reads; nothing is back-filled. Idempotent.
DROP TABLE IF EXISTS visit_events;
DROP TABLE IF EXISTS visit_totals;

-- Reads counted so far. A row exists once an article has been read; no row
-- means 0. Never cleaned; goes away only with its article.
CREATE TABLE IF NOT EXISTS article_read_counts (
 article_id BIGINT PRIMARY KEY REFERENCES articles(id) ON DELETE CASCADE,
 reads BIGINT NOT NULL DEFAULT 0 CHECK (reads >= 0)
);

-- Which article each live stats session has already been counted for: one
-- read per article per session. Rows of sessions idle for 30 minutes no
-- longer deduplicate and are cleaned in batches. Holds the SHA-256 of the
-- random session id and the article id only.
CREATE TABLE IF NOT EXISTS article_reads (
 session_hash BYTEA NOT NULL CHECK (octet_length(session_hash) = 32),
 article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
 read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (session_hash, article_id)
);
CREATE INDEX IF NOT EXISTS article_reads_article ON article_reads(article_id);
