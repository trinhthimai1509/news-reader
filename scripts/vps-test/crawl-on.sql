-- One real worker round on the test stack, only on feeds the real app does
-- not crawl (deleted there): Tuổi Trẻ Giáo dục and BBC Science. ETag cleared
-- so the feed is downloaded and parsed in full; 12 articles per feed sent
-- back to "waiting for full text" so the worker also fetches article pages
-- (its normal limit: 12 pages per site per round, 1.5 s apart).
UPDATE sources SET enabled=true, deleted=false, etag='', modified=''
 WHERE feed_url IN ('https://tuoitre.vn/giao-duc.rss','https://feeds.bbci.co.uk/news/science_and_environment/rss.xml');
UPDATE articles SET content_status='summary', attempts=0, next_attempt=now()
 WHERE id IN (SELECT id FROM (SELECT a.id, row_number() OVER (PARTITION BY a.source_id ORDER BY a.published_at DESC) n
   FROM articles a JOIN sources s ON s.id=a.source_id
   WHERE s.feed_url IN ('https://tuoitre.vn/giao-duc.rss','https://feeds.bbci.co.uk/news/science_and_environment/rss.xml')) x WHERE n<=12);
