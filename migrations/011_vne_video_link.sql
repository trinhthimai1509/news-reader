-- VnExpress video playlists are hotlink-protected (403-style answers without
-- a vnexpress.net Referer, re-checked 2026-10-05). Stored VnExpress videos
-- become link-only: the playlist URL is removed, poster/caption are kept.
UPDATE articles a SET blocks=(
  SELECT jsonb_agg(CASE WHEN b->>'type'='video' AND b->'video'->>'kind'='hls'
    THEN jsonb_set(b, '{video}', (b->'video') - 'src' || '{"kind":"link"}') ELSE b END ORDER BY i)
  FROM jsonb_array_elements(a.blocks) WITH ORDINALITY x(b,i))
WHERE jsonb_typeof(a.blocks)='array' AND EXISTS(SELECT 1 FROM jsonb_array_elements(a.blocks) b WHERE b->'video'->>'kind'='hls');
