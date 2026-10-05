package main

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"personal.news/reader/internal/news"
)

// MergeReport counts what mergeDuplicates found and did.
type MergeReport struct {
	Groups  int      // stable idents shared by more than one article
	Merged  int      // groups merged into one article
	Removed int      // duplicate rows removed
	Skipped []string // groups kept apart, with the reason
}

// mergeDuplicates folds articles that share a stable ident (VnExpress article
// or Tuổi Trẻ article id; see news.ArticleKey) into one row and fills ident for the others. It is
// safe to run again: once every identified row has its ident, nothing is left
// to do. It holds the ingest lock, so no feed is stored meanwhile.
//
// The oldest row (lowest id) survives, so the "seen" cursor in browsers keeps
// meaning the same and the merged article is never counted as new. It gets:
//   - the union of the feeds that listed any copy (earliest first_seen),
//   - the URL and title of the copy seen last (VnExpress 301s old slugs to
//     it); other URLs go to former_urls,
//   - the whole stored content of the best copy (full text first, then newer
//     extractor, then more blocks), never a mix of two copies' blocks; authors
//     and lead image come from another copy only when the best one has none,
//   - a non-deleted owner if any copy has one.
//
// Copies whose publication time differs are not proven to be one article:
// they are left as they are and reported; only the oldest gets the ident.
func mergeDuplicates(ctx context.Context, db pgxBeginner) (MergeReport, error) {
	var rep MergeReport
	tx, e := db.Begin(ctx)
	if e != nil {
		return rep, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(8729103)`); e != nil {
		return rep, e
	}
	type row struct {
		id        int64
		url       string
		ident     string
		published time.Time
		adapter   string
	}
	rows, e := tx.Query(ctx, `SELECT a.id,a.url,coalesce(a.ident,''),a.published_at,s.adapter FROM articles a JOIN sources s ON s.id=a.source_id WHERE s.adapter IN ('vnexpress','tuoitre') ORDER BY a.id`)
	if e != nil {
		return rep, e
	}
	groups := map[string][]row{}
	missing := map[string]bool{}
	for rows.Next() {
		var r row
		if e = rows.Scan(&r.id, &r.url, &r.ident, &r.published, &r.adapter); e != nil {
			rows.Close()
			return rep, e
		}
		k := r.ident
		if k == "" {
			if k = news.ArticleKey(r.url, r.adapter); k == "" {
				continue
			}
			missing[k] = true
		}
		groups[k] = append(groups[k], r)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return rep, e
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		if missing[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		ids := make([]int64, len(g))
		same := true
		for i, r := range g {
			ids[i] = r.id
			same = same && r.published.Equal(g[0].published)
		}
		keep := ids[0]
		if len(g) > 1 {
			rep.Groups++
			if !same {
				rep.Skipped = append(rep.Skipped, k+": publication times differ, kept apart")
			} else {
				if e = mergeGroup(ctx, tx, keep, ids); e != nil {
					return rep, e
				}
				rep.Merged++
				rep.Removed += len(ids) - 1
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE articles SET ident=$2 WHERE id=$1 AND ident IS NULL AND NOT EXISTS(SELECT 1 FROM articles WHERE ident=$2)`, keep, k); e != nil {
			return rep, e
		}
	}
	return rep, tx.Commit(ctx)
}

type pgxBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func mergeGroup(ctx context.Context, tx pgx.Tx, keep int64, ids []int64) error {
	// Feeds of every copy, before the copies (and their rows) go away.
	if _, e := tx.Exec(ctx, `INSERT INTO article_feeds(article_id,source_id,first_seen)
SELECT $1,source_id,min(first_seen) FROM article_feeds WHERE article_id=ANY($2) GROUP BY source_id
ON CONFLICT(article_id,source_id) DO UPDATE SET first_seen=least(article_feeds.first_seen,EXCLUDED.first_seen)`, keep, ids); e != nil {
		return e
	}
	var latestURL string
	var urls []string
	if e := tx.QueryRow(ctx, `SELECT (SELECT url FROM articles WHERE id=ANY($1) ORDER BY fetched_at DESC,id DESC LIMIT 1),
 (SELECT array_agg(u ORDER BY u) FROM (SELECT DISTINCT unnest(former_urls||url) u FROM articles WHERE id=ANY($1)) x)`, ids).Scan(&latestURL, &urls); e != nil {
		return e
	}
	former := []string{}
	for _, u := range urls {
		if u != latestURL {
			former = append(former, u)
		}
	}
	if _, e := tx.Exec(ctx, `WITH g AS (SELECT * FROM articles WHERE id=ANY($2)),
best AS (SELECT * FROM g ORDER BY (content_status='full') DESC, extract_version DESC, CASE jsonb_typeof(blocks) WHEN 'array' THEN jsonb_array_length(blocks) ELSE 0 END DESC, CASE jsonb_typeof(paragraphs) WHEN 'array' THEN jsonb_array_length(paragraphs) ELSE 0 END DESC, id DESC LIMIT 1),
latest AS (SELECT * FROM g ORDER BY fetched_at DESC, id DESC LIMIT 1)
UPDATE articles a SET
 paragraphs=best.paragraphs, blocks=best.blocks, content_status=best.content_status, content_error=best.content_error,
 attempts=best.attempts, next_attempt=best.next_attempt, extract_version=best.extract_version,
 enrich_attempts=best.enrich_attempts, enrich_next=best.enrich_next, enrich_error=best.enrich_error,
 authors=CASE WHEN cardinality(best.authors)>0 THEN best.authors
  ELSE coalesce((SELECT authors FROM g WHERE cardinality(authors)>0 ORDER BY extract_version DESC, id DESC LIMIT 1), best.authors) END,
 lead_image=coalesce(best.lead_image,(SELECT lead_image FROM g WHERE lead_image IS NOT NULL ORDER BY extract_version DESC, id DESC LIMIT 1)),
 title=latest.title,
 summary=CASE WHEN latest.summary<>'' THEN latest.summary ELSE a.summary END,
 source_id=coalesce((SELECT g.source_id FROM g JOIN sources s ON s.id=g.source_id WHERE NOT s.deleted ORDER BY g.id LIMIT 1), a.source_id)
FROM best, latest WHERE a.id=$1`, keep, ids); e != nil {
		return e
	}
	// article_feeds rows of the copies go with them (ON DELETE CASCADE).
	if _, e := tx.Exec(ctx, `DELETE FROM articles WHERE id=ANY($1) AND id<>$2`, ids, keep); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE articles SET url=$2,former_urls=$3 WHERE id=$1`, keep, latestURL, former)
	return e
}
