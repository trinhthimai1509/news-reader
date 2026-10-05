-- Video blocks (see news.Video). has_video says the article contains a video;
-- it does not mean the video plays or that the text is complete. Existing
-- rows start as false and are re-read once by the bounded enrichment backfill
-- (news.ExtractVersion 4). Additive only.
ALTER TABLE articles ADD COLUMN IF NOT EXISTS has_video BOOLEAN NOT NULL DEFAULT false;

-- Tuổi Trẻ video pages carry their videos; the feed is on the official list
-- https://tuoitre.vn/rss.htm (GET 2026-10-05: 200 text/xml, links under
-- https://tuoitre.vn/video/). Added as requested; existing rows untouched.
INSERT INTO sources(name,adapter,feed_url,category) VALUES
 ('Tuổi Trẻ Video','tuoitre','https://tuoitre.vn/video.rss','khac')
 ON CONFLICT(feed_url) DO NOTHING;
