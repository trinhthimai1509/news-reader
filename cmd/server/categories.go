package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type Category struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	NewCount int64  `json:"new_count"`
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z-]{0,29}$`)

// visibleInCategory is true when the article is readable (its owner source is
// not deleted) and a non-deleted feed mapped to category $N listed it.
const visibleInCategory = `EXISTS(SELECT 1 FROM article_feeds af JOIN sources fs ON fs.id=af.source_id
 WHERE af.article_id=a.id AND NOT fs.deleted AND fs.category=%s)`

// articleCursor is the highest article id currently committed. Article ids are
// allocated and committed in order (see ingest), so every article that becomes
// visible later has a larger id.
func (a *App) articleCursor(ctx context.Context) (int64, error) {
	var c int64
	e := a.db.QueryRow(ctx, `SELECT coalesce(max(id),0) FROM articles`).Scan(&c)
	return c, e
}

// parseSeen reads "slug:cursor,slug:cursor" sent by the browser.
func parseSeen(v string) (slugs []string, cursors []int64, ok bool) {
	if v == "" {
		return []string{}, []int64{}, true
	}
	parts := strings.Split(v, ",")
	if len(parts) > 20 {
		return nil, nil, false
	}
	for _, p := range parts {
		s, c, found := strings.Cut(p, ":")
		n, e := strconv.ParseInt(c, 10, 64)
		if !found || !slugPattern.MatchString(s) || e != nil || n < 0 {
			return nil, nil, false
		}
		slugs, cursors = append(slugs, s), append(cursors, n)
	}
	return slugs, cursors, true
}

// handleCategories returns the categories and, for each one the browser sent a
// "seen" cursor for, the number of readable articles in it with a larger id.
// Categories without a cursor report 0; the browser sets their baseline from
// the returned cursor on first use.
func (a *App) handleCategories(w http.ResponseWriter, r *http.Request) {
	slugs, cursors, ok := parseSeen(r.URL.Query().Get("seen"))
	if !ok {
		fail(w, 400, "Tham số seen không hợp lệ")
		return
	}
	cursor, e := a.articleCursor(r.Context())
	if e != nil {
		fail(w, 500, "Không đọc được chuyên mục")
		return
	}
	rows, e := a.db.Query(r.Context(), `WITH seen AS (SELECT * FROM unnest($1::text[], $2::bigint[]) AS s(slug, cursor))
SELECT c.slug, c.name, CASE WHEN s.cursor IS NULL THEN 0 ELSE (
  SELECT count(DISTINCT af.article_id) FROM article_feeds af
  JOIN sources fs ON fs.id=af.source_id AND NOT fs.deleted AND fs.category=c.slug
  JOIN articles a ON a.id=af.article_id
  JOIN sources o ON o.id=a.source_id AND NOT o.deleted
  WHERE af.article_id > s.cursor AND af.article_id <= $3) END
FROM categories c LEFT JOIN seen s ON s.slug=c.slug ORDER BY c.position`, slugs, cursors, cursor)
	if e != nil {
		fail(w, 500, "Không đọc được chuyên mục")
		return
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if rows.Scan(&c.Slug, &c.Name, &c.NewCount) != nil {
			fail(w, 500, "Không đọc được chuyên mục")
			return
		}
		out = append(out, c)
	}
	if rows.Err() != nil {
		fail(w, 500, "Không đọc được chuyên mục")
		return
	}
	send(w, 200, map[string]any{"cursor": cursor, "categories": out})
}

// isUnknownCategory reports a foreign-key violation on sources.category.
func isUnknownCategory(e error) bool {
	var pe *pgconn.PgError
	return errors.As(e, &pe) && pe.Code == "23503"
}
