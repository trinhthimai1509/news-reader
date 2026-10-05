-- Stable article identity for URL shapes whose id was verified (VnExpress
-- <slug>-<id>.html). NULL for every other URL, which keeps being identified by
-- its canonical URL. former_urls keeps slugs the article was stored under, so
-- a feed that still lists an old slug neither creates a copy nor moves the
-- link back. Additive only; existing duplicates are merged by the server at
-- start-up (mergeDuplicates), which also fills ident.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS ident TEXT;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS former_urls TEXT[] NOT NULL DEFAULT '{}';
CREATE UNIQUE INDEX IF NOT EXISTS articles_ident ON articles(ident);
CREATE INDEX IF NOT EXISTS articles_former_urls ON articles USING GIN (former_urls);
