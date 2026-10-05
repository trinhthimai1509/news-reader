-- Categories come only from the feed an article was listed in; nothing is
-- guessed from titles. Every statement is additive and idempotent.
CREATE TABLE IF NOT EXISTS categories (
 slug TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 position INTEGER NOT NULL
);
INSERT INTO categories(slug,name,position) VALUES
 ('thoi-su','Thời sự',1),('the-gioi','Thế giới',2),('kinh-doanh','Kinh doanh',3),
 ('cong-nghe','Công nghệ',4),('giai-tri','Giải trí',5),('the-thao','Thể thao',6),
 ('suc-khoe','Sức khỏe',7),('doi-song','Đời sống',8),('khac','Khác',9)
 ON CONFLICT(slug) DO NOTHING;

-- Each feed maps to exactly one category; aggregate feeds stay in "khac".
ALTER TABLE sources ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'khac' REFERENCES categories(slug);

-- Existing feeds: set a category only where the stored feed URL is a known
-- section feed. Everything else (front pages, latest-news, test feeds) keeps "khac".
UPDATE sources s SET category=m.category FROM (VALUES
 ('https://vnexpress.net/rss/thoi-su.rss','thoi-su'),
 ('https://vnexpress.net/rss/the-gioi.rss','the-gioi'),
 ('https://vnexpress.net/rss/kinh-doanh.rss','kinh-doanh'),
 ('https://vnexpress.net/rss/khoa-hoc-cong-nghe.rss','cong-nghe'),
 ('https://vnexpress.net/rss/giai-tri.rss','giai-tri'),
 ('https://vnexpress.net/rss/the-thao.rss','the-thao'),
 ('https://vnexpress.net/rss/suc-khoe.rss','suc-khoe'),
 ('https://vnexpress.net/rss/gia-dinh.rss','doi-song'),
 ('https://feeds.bbci.co.uk/news/world/rss.xml','the-gioi'),
 ('https://feeds.bbci.co.uk/news/business/rss.xml','kinh-doanh'),
 ('https://feeds.bbci.co.uk/news/technology/rss.xml','cong-nghe'),
 ('https://feeds.bbci.co.uk/news/entertainment_and_arts/rss.xml','giai-tri'),
 ('https://feeds.bbci.co.uk/news/health/rss.xml','suc-khoe')
) AS m(feed_url,category)
WHERE s.feed_url=m.feed_url AND s.category='khac';

-- Section feeds checked with GET on 2026-10-05. Existing rows (including
-- deleted ones) are left untouched.
INSERT INTO sources(name,adapter,feed_url,category) VALUES
 ('VnExpress Thời sự','vnexpress','https://vnexpress.net/rss/thoi-su.rss','thoi-su'),
 ('VnExpress Thế giới','vnexpress','https://vnexpress.net/rss/the-gioi.rss','the-gioi'),
 ('VnExpress Kinh doanh','vnexpress','https://vnexpress.net/rss/kinh-doanh.rss','kinh-doanh'),
 ('VnExpress Khoa học công nghệ','vnexpress','https://vnexpress.net/rss/khoa-hoc-cong-nghe.rss','cong-nghe'),
 ('VnExpress Giải trí','vnexpress','https://vnexpress.net/rss/giai-tri.rss','giai-tri'),
 ('VnExpress Thể thao','vnexpress','https://vnexpress.net/rss/the-thao.rss','the-thao'),
 ('VnExpress Sức khỏe','vnexpress','https://vnexpress.net/rss/suc-khoe.rss','suc-khoe'),
 ('VnExpress Gia đình','vnexpress','https://vnexpress.net/rss/gia-dinh.rss','doi-song'),
 ('BBC World','bbc','https://feeds.bbci.co.uk/news/world/rss.xml','the-gioi'),
 ('BBC Business','bbc','https://feeds.bbci.co.uk/news/business/rss.xml','kinh-doanh'),
 ('BBC Technology','bbc','https://feeds.bbci.co.uk/news/technology/rss.xml','cong-nghe'),
 ('BBC Entertainment & Arts','bbc','https://feeds.bbci.co.uk/news/entertainment_and_arts/rss.xml','giai-tri'),
 ('BBC Health','bbc','https://feeds.bbci.co.uk/news/health/rss.xml','suc-khoe')
 ON CONFLICT(feed_url) DO NOTHING;

-- Which feeds listed which article. An article is stored once (unique URL)
-- but belongs to the category of every feed that listed it.
CREATE TABLE IF NOT EXISTS article_feeds (
 article_id BIGINT NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
 source_id BIGINT NOT NULL REFERENCES sources(id),
 first_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(article_id, source_id)
);
CREATE INDEX IF NOT EXISTS article_feeds_source ON article_feeds(source_id, article_id);

-- Backfill: the only stored evidence for old rows is the feed that first
-- delivered the article (articles.source_id). Other feeds that may also have
-- listed it were not recorded and are not inferred.
INSERT INTO article_feeds(article_id,source_id,first_seen)
 SELECT id,source_id,fetched_at FROM articles
 ON CONFLICT DO NOTHING;
