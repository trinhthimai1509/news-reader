CREATE INDEX IF NOT EXISTS articles_source_published ON articles(source_id, published_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS articles_pending_content ON articles(source_id, next_attempt) WHERE content_status <> 'full' AND attempts < 3;
