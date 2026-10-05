-- VnExpress videos stored before the poster fallback (plain img inside
-- box_img_video) have no poster: re-read only those articles once.
UPDATE articles a SET extract_version=3, enrich_attempts=0, enrich_next=now()
FROM sources s WHERE s.id=a.source_id AND s.adapter='vnexpress' AND a.has_video AND a.extract_version>=4
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(CASE jsonb_typeof(a.blocks) WHEN 'array' THEN a.blocks ELSE '[]' END) b
            WHERE b->>'type'='video' AND coalesce(b->'video'->>'poster','')='');
