package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"personal.news/reader/internal/news"
)

// statsPost sends a stats event the way the page does: same-origin JSON.
func statsPost(a *App, body, peer string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/stats/events", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://example.com")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if peer != "" {
		r.RemoteAddr = peer
	}
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func hexID(n int) string { return fmt.Sprintf("%032x", n) }

func readEvent(article int64, session int) string {
	return fmt.Sprintf(`{"type":"read","session_id":%q,"article_id":%d}`, hexID(session), article)
}
func heartbeat(session int) string {
	return fmt.Sprintf(`{"type":"heartbeat","session_id":%q}`, hexID(session))
}

func activeCount(t *testing.T, a *App) int64 {
	var s struct {
		Active int64
		Window int `json:"active_window_seconds"`
	}
	if code := get(t, a, "/api/stats", &s); code != 200 || s.Window != 300 {
		t.Fatalf("stats: %d %+v", code, s)
	}
	return s.Active
}

// reads returns the read count of an article as the reader API shows it, in
// the list and in the article itself.
func reads(t *testing.T, a *App, id int64) int64 {
	var art Article
	if code := get(t, a, fmt.Sprintf("/api/articles/%d", id), &art); code != 200 {
		t.Fatalf("article %d: %d", id, code)
	}
	var l listResp
	get(t, a, "/api/articles", &l)
	for _, it := range l.Items {
		if it.ID == id && it.Reads != art.Reads {
			t.Fatalf("list %d, article %d", it.Reads, art.Reads)
		}
	}
	return art.Reads
}

func readArticles(t *testing.T, a *App, n int) []int64 {
	s := addSource(t, a, "BBC", "https://feeds.bbci.co.uk/news/t-reads.xml", "khac")
	now := time.Now()
	items := []news.Item{}
	for i := 0; i < n; i++ {
		items = append(items, item(fmt.Sprintf("r%d", i), fmt.Sprintf("Story %d", i), now.Add(-time.Duration(i)*time.Minute)))
	}
	if e := a.ingest(t.Context(), s, items, now); e != nil {
		t.Fatal(e)
	}
	ids := []int64{}
	rows, _ := a.db.Query(t.Context(), `SELECT id FROM articles WHERE source_id=$1 ORDER BY published_at DESC`, s.ID)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	return ids
}

// Refusals that need no database: they come before any query.
func TestStatsEventRefusals(t *testing.T) {
	a := newTestApp(nil)
	ok := readEvent(1, 1)
	for name, c := range map[string]struct {
		body string
		hdr  map[string]string
		code int
	}{
		"other origin":          {ok, map[string]string{"Origin": "https://evil.example"}, 403},
		"origin null":           {ok, map[string]string{"Origin": "null"}, 403},
		"no origin":             {ok, map[string]string{"Origin": ""}, 403},
		"cross-site fetch":      {ok, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		"same-site fetch":       {ok, map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		"text/plain":            {ok, map[string]string{"Content-Type": "text/plain"}, 415},
		"form":                  {ok, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 415},
		"not json":              {`type=read`, nil, 400},
		"empty":                 {``, nil, 400},
		"old pageview":          {fmt.Sprintf(`{"type":"pageview","event_id":%q,"session_id":%q}`, hexID(1), hexID(1)), nil, 400},
		"unknown type":          {fmt.Sprintf(`{"type":"click","session_id":%q}`, hexID(1)), nil, 400},
		"unknown field":         {fmt.Sprintf(`{"type":"heartbeat","session_id":%q,"url":"/x"}`, hexID(1)), nil, 400},
		"read no article":       {fmt.Sprintf(`{"type":"read","session_id":%q}`, hexID(1)), nil, 400},
		"read article 0":        {readEvent(0, 1), nil, 400},
		"read article -5":       {readEvent(-5, 1), nil, 400},
		"read article 1.5":      {fmt.Sprintf(`{"type":"read","session_id":%q,"article_id":1.5}`, hexID(1)), nil, 400},
		"read article string":   {fmt.Sprintf(`{"type":"read","session_id":%q,"article_id":"1"}`, hexID(1)), nil, 400},
		"read article huge":     {fmt.Sprintf(`{"type":"read","session_id":%q,"article_id":99999999999999999999}`, hexID(1)), nil, 400},
		"heartbeat w/ article":  {fmt.Sprintf(`{"type":"heartbeat","session_id":%q,"article_id":1}`, hexID(1)), nil, 400},
		"short id":              {`{"type":"heartbeat","session_id":"abc"}`, nil, 400},
		"upper-case id":         {`{"type":"heartbeat","session_id":"` + strings.ToUpper(hexID(0xabc)) + `"}`, nil, 400},
		"no session":            {`{"type":"read","article_id":1}`, nil, 400},
		"two objects":           {ok + ok, nil, 400},
		"array":                 {`[` + ok + `]`, nil, 400},
		"too large":             {`{"type":"heartbeat","session_id":"` + hexID(1) + `","x":"` + strings.Repeat("a", 400) + `"}`, nil, 400},
		"numbers for strings":   {`{"type":1,"session_id":2}`, nil, 400},
		"null article for read": {fmt.Sprintf(`{"type":"read","session_id":%q,"article_id":null}`, hexID(1)), nil, 400},
	} {
		if w := statsPost(a, c.body, "", c.hdr); w.Code != c.code {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	// Only POST writes; the read path takes GET only.
	for _, m := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(m, "/api/stats/events", strings.NewReader(ok)))
		if w.Code != 404 && w.Code != 405 {
			t.Errorf("%s /api/stats/events: %d", m, w.Code)
		}
	}
}

// The per-address limit uses the connection address only: a forged
// X-Forwarded-For neither escapes it nor blocks someone else.
func TestStatsRateLimitIgnoresForwardedFor(t *testing.T) {
	a := newTestApp(nil)
	bad := `{"type":"heartbeat"}` // refused after the limit check, without a database
	for i := 0; i < statsAddrMax; i++ {
		if w := statsPost(a, bad, "198.51.100.7:1000", map[string]string{"X-Forwarded-For": fmt.Sprintf("10.0.0.%d", i%250)}); w.Code != 400 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	w := statsPost(a, bad, "198.51.100.7:1001", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("over the limit: %d", w.Code)
	}
	if w := statsPost(a, bad, "198.51.100.8:1000", map[string]string{"X-Forwarded-For": "198.51.100.7"}); w.Code != 400 {
		t.Fatalf("another address was blocked: %d", w.Code)
	}
}

func TestRateWindowBounded(t *testing.T) {
	var w rateWindow
	now := time.Now()
	for i := 0; i < statsKeysMax; i++ {
		if !w.allow(fmt.Sprint(i), 1, now) {
			t.Fatal("refused early")
		}
	}
	if w.allow("new", 1, now) || len(w.counts) != statsKeysMax {
		t.Fatal("map not bounded")
	}
	if w.allow("1", 1, now) {
		t.Fatal("per-key limit")
	}
	if !w.allow("new", 1, now.Add(time.Minute)) || len(w.counts) != 1 {
		t.Fatal("window does not reset")
	}
}

func TestArticleReads(t *testing.T) {
	a, ctx := migratedApp(t)
	ids := readArticles(t, a, 2)
	art, other := ids[0], ids[1]
	if reads(t, a, art) != 0 || activeCount(t, a) != 0 {
		t.Fatal("articles must start at 0 reads")
	}
	// Opening an article, opening it again, reloading, another tab and retries
	// in one browser session: one read.
	for i := 0; i < 4; i++ {
		w := statsPost(a, readEvent(art, 100), "", nil)
		var b struct{ Reads int64 }
		json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 200 || b.Reads != 1 {
			t.Fatalf("read %d: %d %s", i, w.Code, w.Body)
		}
	}
	if reads(t, a, art) != 1 || reads(t, a, other) != 0 || activeCount(t, a) != 1 {
		t.Fatal("same session counted twice, or another article changed")
	}
	// Heartbeats never add reads; listing, searching, paging do not either.
	for i := 0; i < 3; i++ {
		statsPost(a, heartbeat(100), "", nil)
	}
	get(t, a, "/api/articles?q=Story&page=1", nil)
	if reads(t, a, art) != 1 {
		t.Fatal("heartbeat or list counted")
	}
	// Another browser session counts; the same session on another article too.
	statsPost(a, readEvent(art, 200), "", nil)
	statsPost(a, readEvent(other, 100), "", nil)
	if reads(t, a, art) != 2 || reads(t, a, other) != 1 || activeCount(t, a) != 2 {
		t.Fatal("new session / other article")
	}
	// Unknown or hidden articles are refused and leave no row.
	a.db.Exec(ctx, `UPDATE sources SET deleted=true WHERE feed_url LIKE '%t-reads%'`)
	if w := statsPost(a, readEvent(art, 300), "", nil); w.Code != 404 {
		t.Fatalf("hidden article: %d", w.Code)
	}
	if w := statsPost(a, readEvent(987654, 300), "", nil); w.Code != 404 {
		t.Fatalf("unknown article: %d", w.Code)
	}
	a.db.Exec(ctx, `UPDATE sources SET deleted=false WHERE feed_url LIKE '%t-reads%'`)
	var rows int
	a.db.QueryRow(ctx, `SELECT count(*) FROM article_reads WHERE session_hash=$1`, tokenHash(hexID(300))).Scan(&rows)
	if rows != 0 || reads(t, a, art) != 2 {
		t.Fatal("refused read left a trace")
	}
	// /api/stats has the active count only.
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/stats", nil))
	var raw map[string]any
	json.Unmarshal(w.Body.Bytes(), &raw)
	if len(raw) != 2 || raw["active"] == nil || strings.Contains(w.Body.String(), hexID(100)) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read endpoint: %s", w.Body)
	}
	// The tables hold no address, browser or user data; the visit total is gone.
	var cols []string
	a.db.QueryRow(ctx, `SELECT array_agg(table_name||'.'||column_name ORDER BY table_name,column_name) FROM information_schema.columns WHERE table_schema='public' AND (table_name LIKE 'visit_%' OR table_name LIKE 'article_read%')`).Scan(&cols)
	if strings.Join(cols, ",") != "article_read_counts.article_id,article_read_counts.reads,article_reads.article_id,article_reads.read_at,article_reads.session_hash,visit_sessions.id_hash,visit_sessions.last_seen" {
		t.Fatalf("columns: %v", cols)
	}
	a.db.QueryRow(ctx, `SELECT count(*) FROM visit_sessions WHERE id_hash=decode($1,'hex')`, hexID(100)).Scan(&rows)
	if rows != 0 {
		t.Fatal("session id stored in clear")
	}
}

// Concurrent requests: one session sending the same read 20 times at once
// counts once; 20 sessions count 20; nothing is lost.
func TestArticleReadsConcurrent(t *testing.T) {
	a, _ := migratedApp(t)
	art := readArticles(t, a, 1)[0]
	var wg sync.WaitGroup
	codes := make(chan int, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		// Distinct addresses: this is about the database, not the rate limit.
		go func(i int) {
			defer wg.Done()
			codes <- statsPost(a, readEvent(art, 1), fmt.Sprintf("192.0.2.%d:1", i), nil).Code
		}(i)
		go func(i int) {
			defer wg.Done()
			codes <- statsPost(a, readEvent(art, 1000+i), fmt.Sprintf("192.0.2.%d:1", 100+i), nil).Code
		}(i)
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 200 {
			t.Fatalf("status %d", c)
		}
	}
	if n := reads(t, a, art); n != 21 {
		t.Fatalf("concurrent reads: %d", n)
	}
}

// A stats session ends after 30 minutes without events (server time): a read
// in the new session counts again. A heartbeat after the pause also starts
// the new session.
func TestReadSessionExpiry(t *testing.T) {
	a, ctx := migratedApp(t)
	ids := readArticles(t, a, 2)
	idle := func(session int, d string) {
		a.db.Exec(ctx, `UPDATE visit_sessions SET last_seen=now()-$2::interval WHERE id_hash=$1`, tokenHash(hexID(session)), d)
	}
	statsPost(a, readEvent(ids[0], 1), "", nil)
	idle(1, "29 minutes")
	statsPost(a, readEvent(ids[0], 1), "", nil)
	if reads(t, a, ids[0]) != 1 {
		t.Fatal("counted again within the session")
	}
	idle(1, "31 minutes")
	statsPost(a, readEvent(ids[0], 1), "", nil)
	if reads(t, a, ids[0]) != 2 {
		t.Fatal("new session not counted")
	}
	statsPost(a, readEvent(ids[1], 2), "", nil)
	idle(2, "31 minutes")
	statsPost(a, heartbeat(2), "", nil)
	statsPost(a, readEvent(ids[1], 2), "", nil)
	if reads(t, a, ids[1]) != 2 {
		t.Fatal("heartbeat after the pause did not start a new session")
	}
}

// A session is active for 5 minutes after its last event, by server time.
// Heartbeats more often than sessionTouchMin do not write.
func TestActiveSessionsExpire(t *testing.T) {
	a, ctx := migratedApp(t)
	statsPost(a, heartbeat(1), "", nil)
	statsPost(a, heartbeat(2), "", nil)
	var before, after time.Time
	a.db.QueryRow(ctx, `SELECT max(last_seen) FROM visit_sessions`).Scan(&before)
	statsPost(a, heartbeat(1), "", nil)
	statsPost(a, heartbeat(2), "", nil)
	a.db.QueryRow(ctx, `SELECT max(last_seen) FROM visit_sessions`).Scan(&after)
	if !after.Equal(before) {
		t.Fatal("session written twice within 30 s")
	}
	a.db.Exec(ctx, `UPDATE visit_sessions SET last_seen=now()-interval '290 seconds' WHERE id_hash=$1`, tokenHash(hexID(1)))
	a.db.Exec(ctx, `UPDATE visit_sessions SET last_seen=now()-interval '310 seconds' WHERE id_hash=$1`, tokenHash(hexID(2)))
	if n := activeCount(t, a); n != 1 {
		t.Fatalf("expiry: %d", n)
	}
	statsPost(a, heartbeat(2), "", nil)
	if n := activeCount(t, a); n != 2 {
		t.Fatalf("back: %d", n)
	}
}

// Read counts survive a restart (a new App on the same database, migrations
// run again) and cleanup; cleanup removes only the marks of ended sessions,
// in batches, so a live session still does not count twice.
func TestArticleReadsPersistAndCleanup(t *testing.T) {
	a, ctx := migratedApp(t)
	art := readArticles(t, a, 1)[0]
	statsPost(a, readEvent(art, 1), "", nil)
	statsPost(a, readEvent(art, 2), "", nil)
	if e := migrate(ctx, a.db, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	b := newTestApp(a.db)
	if reads(t, b, art) != 2 {
		t.Fatal("after restart")
	}
	// 6000 marks of ended sessions (more than one batch); session 2 ended too.
	if _, e := b.db.Exec(ctx, `INSERT INTO article_reads(session_hash,article_id) SELECT sha256(int8send(g)),$1 FROM generate_series(1,6000) g`, art); e != nil {
		t.Fatal(e)
	}
	b.db.Exec(ctx, `UPDATE visit_sessions SET last_seen=now()-interval '31 minutes' WHERE id_hash=$1`, tokenHash(hexID(2)))
	b.db.Exec(ctx, `INSERT INTO visit_sessions(id_hash,last_seen) VALUES($1,now()-interval '2 hours')`, tokenHash(hexID(9)))
	marks, sessions := b.cleanStats(ctx)
	var left int
	b.db.QueryRow(ctx, `SELECT count(*) FROM article_reads`).Scan(&left)
	if marks != 6001 || sessions != 1 || left != 1 {
		t.Fatalf("cleanup: marks=%d sessions=%d left=%d", marks, sessions, left)
	}
	statsPost(b, readEvent(art, 1), "", nil) // live session: still counted already
	if reads(t, b, art) != 2 {
		t.Fatal("cleanup made a live session count again")
	}
	if m, s := b.cleanStats(ctx); m != 0 || s != 0 {
		t.Fatal("second cleanup removed rows")
	}
}

// Migration 016 drops the site-wide visit count of 014 without turning it
// into article reads: every article starts at 0.
func TestReadMigrationDropsVisitTotal(t *testing.T) {
	db := testDB(t)
	a := newTestApp(db)
	ctx := t.Context()
	migrateUpTo(t, a, "015_categories_v2.sql")
	art := readArticles(t, a, 1)[0]
	a.db.Exec(ctx, `UPDATE visit_totals SET total=57`)
	a.db.Exec(ctx, `INSERT INTO visit_sessions(id_hash) VALUES($1)`, tokenHash(hexID(5)))
	migrateUpTo(t, a, "999")
	var gone bool
	a.db.QueryRow(ctx, `SELECT to_regclass('visit_totals') IS NULL AND to_regclass('visit_events') IS NULL`).Scan(&gone)
	var total int64
	a.db.QueryRow(ctx, `SELECT coalesce(sum(reads),0) FROM article_read_counts`).Scan(&total)
	if !gone || total != 0 || reads(t, a, art) != 0 || activeCount(t, a) != 1 {
		t.Fatalf("gone=%v total=%d", gone, total)
	}
	a.db.Exec(ctx, `DELETE FROM schema_migrations WHERE name='016_article_reads.sql'`)
	statsPost(a, readEvent(art, 5), "", nil)
	migrateUpTo(t, a, "999")
	if reads(t, a, art) != 1 {
		t.Fatal("re-run lost reads")
	}
}

// Global new-row budget: once used up, reads and new sessions are refused
// (429) but known sessions still work and reading is not affected.
func TestStatsNewRowBudget(t *testing.T) {
	a, _ := migratedApp(t)
	art := readArticles(t, a, 1)[0]
	statsPost(a, heartbeat(1), "", nil)
	for i := 1; i < statsNewRowsMax; i++ {
		if !a.stats.rows.allow("", statsNewRowsMax, time.Now()) {
			t.Fatal("budget")
		}
	}
	if w := statsPost(a, heartbeat(2), "198.51.100.1:1", nil); w.Code != 429 {
		t.Fatalf("new session over budget: %d", w.Code)
	}
	if w := statsPost(a, readEvent(art, 1), "198.51.100.1:1", nil); w.Code != 429 {
		t.Fatalf("read over budget: %d", w.Code)
	}
	if w := statsPost(a, heartbeat(1), "198.51.100.1:1", nil); w.Code != 200 {
		t.Fatalf("known session: %d", w.Code)
	}
	if code := get(t, a, fmt.Sprintf("/api/articles/%d", art), nil); code != 200 {
		t.Fatalf("reading affected: %d", code)
	}
}

// Per-address read limit.
func TestReadRateLimit(t *testing.T) {
	a, _ := migratedApp(t)
	art := readArticles(t, a, 1)[0]
	for i := 0; i < statsReadAddrMax; i++ {
		if w := statsPost(a, readEvent(art, i+1), "198.51.100.3:1", nil); w.Code != 200 {
			t.Fatalf("read %d: %d", i, w.Code)
		}
	}
	if w := statsPost(a, readEvent(art, 999), "198.51.100.3:1", nil); w.Code != 429 {
		t.Fatalf("over the read limit: %d", w.Code)
	}
	if reads(t, a, art) != int64(statsReadAddrMax) {
		t.Fatal("refused read counted")
	}
}

// The stats write opens nothing else: an admin cookie sent along is ignored,
// and admin writes still need the session's CSRF token.
func TestStatsDoNotTouchAdmin(t *testing.T) {
	a, _ := migratedApp(t)
	c := adminFor(t, a)
	r := statsPost(a, heartbeat(3), "", map[string]string{"Cookie": c.cookie.Name + "=" + c.cookie.Value})
	if r.Code != 200 || strings.Contains(r.Body.String(), "csrf") || len(r.Result().Cookies()) != 0 {
		t.Fatalf("stats with admin cookie: %d %s", r.Code, r.Body)
	}
	if w := c.req("PATCH", "/api/admin/sources/1", `{"enabled":true}`, map[string]string{"X-CSRF-Token": ""}); w.Code != 403 {
		t.Fatalf("admin write without CSRF: %d", w.Code)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("POST", "/api/admin/stats/events", strings.NewReader(heartbeat(3))))
	if w.Code != 401 {
		t.Fatalf("admin path: %d", w.Code)
	}
}
