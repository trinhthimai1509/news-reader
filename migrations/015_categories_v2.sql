-- Category list v2 (2026-10-07). Display order: Thời sự, Thế giới, Kinh tế,
-- Khoa học công nghệ, Chính trị, Quân sự, Đời sống, Khác ("Tất cả" is the
-- unfiltered list, not a category).
--   - Kinh doanh → Kinh tế (kinh-te), Công nghệ → Khoa học công nghệ
--     (khoa-hoc-cong-nghe): their feeds move over unchanged.
--   - Giải trí, Thể thao, Sức khỏe are dropped: their feeds move to Khác.
--   - Chính trị, Quân sự are new and start empty: no feed is assigned and
--     nothing is inferred from titles or from other categories.
-- Only sources.category changes. Articles, feeds and article_feeds (which
-- links an article to the feeds that listed it) are untouched, so an article
-- in several old categories that now meet in Khác is still listed once.
-- Soft-deleted feeds move too (the foreign key needs it). Idempotent.
INSERT INTO categories(slug,name,position) VALUES
 ('kinh-te','Kinh tế',3),('khoa-hoc-cong-nghe','Khoa học công nghệ',4),
 ('chinh-tri','Chính trị',5),('quan-su','Quân sự',6)
 ON CONFLICT(slug) DO NOTHING;

UPDATE sources SET category='kinh-te' WHERE category='kinh-doanh';
UPDATE sources SET category='khoa-hoc-cong-nghe' WHERE category='cong-nghe';
UPDATE sources SET category='khac' WHERE category IN ('giai-tri','the-thao','suc-khoe');

DELETE FROM categories WHERE slug IN ('kinh-doanh','cong-nghe','giai-tri','the-thao','suc-khoe');

UPDATE categories c SET name=v.name, position=v.position FROM (VALUES
 ('thoi-su','Thời sự',1),('the-gioi','Thế giới',2),('kinh-te','Kinh tế',3),
 ('khoa-hoc-cong-nghe','Khoa học công nghệ',4),('chinh-tri','Chính trị',5),
 ('quan-su','Quân sự',6),('doi-song','Đời sống',7),('khac','Khác',8)
) AS v(slug,name,position) WHERE c.slug=v.slug;
