CREATE TABLE IF NOT EXISTS sources (
 id BIGSERIAL PRIMARY KEY,
 name TEXT NOT NULL,
 adapter TEXT NOT NULL CHECK (adapter IN ('vnexpress','bbc')),
 feed_url TEXT NOT NULL UNIQUE,
 enabled BOOLEAN NOT NULL DEFAULT true,
 deleted BOOLEAN NOT NULL DEFAULT false,
 last_checked TIMESTAMPTZ,
 last_success TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 etag TEXT NOT NULL DEFAULT '',
 modified TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS articles (
 id BIGSERIAL PRIMARY KEY,
 source_id BIGINT NOT NULL REFERENCES sources(id),
 url TEXT NOT NULL UNIQUE,
 title TEXT NOT NULL,
 summary TEXT NOT NULL DEFAULT '',
 paragraphs JSONB NOT NULL DEFAULT '[]',
 content_status TEXT NOT NULL DEFAULT 'summary' CHECK(content_status IN ('summary','full','unavailable')),
 content_error TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt TIMESTAMPTZ NOT NULL DEFAULT now(),
 published_at TIMESTAMPTZ NOT NULL,
 fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS articles_published ON articles(published_at DESC,id DESC);
INSERT INTO sources(name,adapter,feed_url) VALUES
 ('VnExpress','vnexpress','https://vnexpress.net/rss/tin-moi-nhat.rss'),
 ('BBC News','bbc','https://feeds.bbci.co.uk/news/rss.xml')
 ON CONFLICT(feed_url) DO NOTHING;
