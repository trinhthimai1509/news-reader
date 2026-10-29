-- Test data only (test stack, never the real database): 200 simulated feeds
-- and :articles articles, to see how database size and reader queries behave
-- with more data. Run with:
--   docker exec -i nr-vps-db psql -U news -d news -v articles=300000 < scripts/vps-test/seed-200.sql
--
-- The app has adapters for 3 websites only, so the 200 feeds belong to those
-- 3 websites: VnExpress 90, BBC 60, Tuổi Trẻ 50, categories in turn. This is
-- NOT the same as 200 different websites (one worker goroutine per website,
-- pages fetched per website). Feeds are created disabled: nothing is crawled.
--
-- Articles copy the content (blocks, paragraphs, authors, summary) of the
-- real full-text articles in turn, so their size matches real ones; URLs are
-- unique and fake. Published over the last 65 days (≈ 200 feeds × 23 new
-- articles/day as measured). Each belongs to one feed; 15% are also listed
-- by a second feed of the same website (duplicates across feeds, like the
-- real section feeds).
\set ON_ERROR_STOP on
BEGIN;
CREATE TEMP TABLE sim_feeds AS
SELECT g AS n, CASE WHEN g <= 90 THEN 'vnexpress' WHEN g <= 150 THEN 'bbc' ELSE 'tuoitre' END AS adapter,
       (ARRAY['thoi-su','the-gioi','kinh-te','khoa-hoc-cong-nghe','chinh-tri','quan-su','doi-song','khac'])[1 + g % 8] AS category
FROM generate_series(1,200) g;
INSERT INTO sources(name,adapter,feed_url,category,enabled)
SELECT 'Sim ' || adapter || ' ' || n, adapter,
       CASE adapter WHEN 'vnexpress' THEN 'https://vnexpress.net/rss/sim-' || n || '.rss'
                    WHEN 'bbc' THEN 'https://feeds.bbci.co.uk/news/sim-' || n || '/rss.xml'
                    ELSE 'https://tuoitre.vn/sim-' || n || '.rss' END,
       category, false
FROM sim_feeds;
CREATE TEMP TABLE sim_src AS
SELECT s.id, s.adapter, row_number() OVER (PARTITION BY s.adapter ORDER BY s.id) AS k, count(*) OVER (PARTITION BY s.adapter) AS m,
       row_number() OVER (ORDER BY s.id) - 1 AS rn
FROM sources s WHERE s.name LIKE 'Sim %';
CREATE TEMP TABLE tpl AS
SELECT row_number() OVER (ORDER BY id) AS k, title, summary, paragraphs, blocks, authors, lead_image, extract_version, has_video
FROM articles WHERE content_status='full';
CREATE TEMP TABLE sim_art AS
SELECT g, s.id AS source_id
FROM generate_series(1, :articles) g JOIN sim_src s ON s.rn = g % 200;
INSERT INTO articles(source_id,url,title,summary,paragraphs,blocks,authors,lead_image,content_status,attempts,published_at,fetched_at,extract_version,has_video)
SELECT a.source_id,
       'https://sim.invalid/' || a.g || '.html',
       t.title, t.summary, t.paragraphs, t.blocks, t.authors, t.lead_image, 'full', 1,
       now() - (a.g::float / :articles) * interval '65 days', now() - (a.g::float / :articles) * interval '65 days',
       t.extract_version, t.has_video
FROM sim_art a JOIN tpl t ON t.k = 1 + a.g % (SELECT count(*) FROM tpl);
INSERT INTO article_feeds(article_id,source_id,first_seen)
SELECT id, source_id, fetched_at FROM articles WHERE url LIKE 'https://sim.invalid/%';
-- 15% also listed by the next feed of the same website.
INSERT INTO article_feeds(article_id,source_id,first_seen)
SELECT a.id, o.id, a.fetched_at
FROM articles a JOIN sim_src s ON s.id = a.source_id
JOIN sim_src o ON o.adapter = s.adapter AND o.k = 1 + s.k % s.m
WHERE a.url LIKE 'https://sim.invalid/%' AND a.id % 100 < 15
ON CONFLICT DO NOTHING;
COMMIT;
VACUUM ANALYZE articles;
VACUUM ANALYZE article_feeds;
SELECT (SELECT count(*) FROM sources) AS feeds, (SELECT count(*) FROM articles) AS articles,
       (SELECT count(*) FROM article_feeds) AS links, pg_size_pretty(pg_database_size('news')) AS db_size;
