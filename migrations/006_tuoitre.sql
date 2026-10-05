-- Tuổi Trẻ Online adapter. Three feeds from the official list
-- https://tuoitre.vn/rss.htm, each checked with GET on 2026-10-05 (200
-- text/xml, 50 items, links on tuoitre.vn). Existing rows are left untouched.
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_adapter_check;
ALTER TABLE sources ADD CONSTRAINT sources_adapter_check CHECK (adapter IN ('vnexpress','bbc','tuoitre'));
INSERT INTO sources(name,adapter,feed_url,category) VALUES
 ('Tuổi Trẻ Thời sự','tuoitre','https://tuoitre.vn/thoi-su.rss','thoi-su'),
 ('Tuổi Trẻ Thế giới','tuoitre','https://tuoitre.vn/the-gioi.rss','the-gioi'),
 ('Tuổi Trẻ Kinh doanh','tuoitre','https://tuoitre.vn/kinh-doanh.rss','kinh-doanh')
 ON CONFLICT(feed_url) DO NOTHING;
