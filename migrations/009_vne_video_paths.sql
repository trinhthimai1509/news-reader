-- VnExpress HLS playlists also live under section paths (giaitri, thethao…),
-- which the first video build stored as link-only. Re-read only those
-- articles once; nothing else is touched.
UPDATE articles a SET extract_version=3, enrich_attempts=0, enrich_next=now()
FROM sources s WHERE s.id=a.source_id AND s.adapter='vnexpress' AND a.has_video
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(CASE jsonb_typeof(a.blocks) WHEN 'array' THEN a.blocks ELSE '[]' END) b
            WHERE b->>'type'='video' AND b->'video'->>'kind'='link');
