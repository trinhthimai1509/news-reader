-- Authors, ordered content blocks (paragraphs and images) and a lead image.
-- Additive only: existing rows keep their paragraphs and stay readable; the
-- API falls back to paragraphs while blocks is NULL.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS authors TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS blocks JSONB;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS lead_image JSONB;

-- extract_version records which extractor produced the stored content
-- (0 = before this migration). Full-text articles below the current version
-- are re-read once by the enrichment backfill; its own retry state is kept
-- apart from content fetching so a failure never touches stored content.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS extract_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS enrich_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS enrich_next TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE articles ADD COLUMN IF NOT EXISTS enrich_error TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS articles_enrich_pending ON articles(source_id, extract_version, enrich_next) WHERE content_status='full';
