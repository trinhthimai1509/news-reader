package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"personal.news/reader/internal/news"
)

func TestPrivateAPIRequiresToken(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	for _, path := range []string{"/api/articles", "/api/articles/1", "/api/sources"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private API must not be cached")
		}
	}
}
func TestTokenWithoutBearerSchemeRejected(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	r := httptest.NewRequest("GET", "/api/sources", nil)
	r.Header.Set("Authorization", app.token)
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("raw token without Bearer accepted: %d", w.Code)
	}
}
func TestRepeatedWrongTokensAreThrottled(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	h := app.routes()
	code := 0
	for i := 0; i <= authFailures; i++ {
		r := httptest.NewRequest("GET", "/api/sources", nil)
		r.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		code = w.Code
	}
	if code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after %d failures, got %d", authFailures, code)
	}
}

// The only add flow is the import endpoint: unsafe or unsupported URLs are
// refused before any database access or request; the old manual POST is gone.
func TestSourceURLValidatedBeforeDatabase(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	do := func(method, path, body string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+app.token)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		return w.Code
	}
	for _, u := range []string{"https://127.0.0.1/news/rss.xml", "http://feeds.bbci.co.uk/news/rss.xml", "https://example.com/rss/a.rss", "https://vnexpress.net:8443/rss/a.rss", ""} {
		if c := do("POST", "/api/sources/import", `{"url":"`+u+`"}`); c != 422 {
			t.Errorf("%q: %d", u, c)
		}
	}
	if c := do("POST", "/api/sources/import", `not json`); c != 400 {
		t.Errorf("not json: %d", c)
	}
	if c := do("POST", "/api/sources", `{"name":"x","adapter":"bbc","feed_url":"https://feeds.bbci.co.uk/news/rss.xml"}`); c != 405 {
		t.Errorf("manual POST still served: %d", c)
	}
}
func TestArticleQueryValidatedBeforeDatabase(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	for _, q := range []string{"source=abc", "source=1%20OR%201=1", "page=-1", "page=x", "q=" + strings.Repeat("a", 201)} {
		r := httptest.NewRequest("GET", "/api/articles?"+q, nil)
		r.Header.Set("Authorization", "Bearer "+app.token)
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
}

func TestLocalAccessConfig(t *testing.T) {
	route := filepath.Join(t.TempDir(), "route")
	// Default route via 172.18.0.1 (little-endian hex as in /proc/net/route).
	os.WriteFile(route, []byte("Iface\tDestination\tGateway \tFlags\neth0\t00000000\t010012AC\t0003\neth0\t000012AC\t00000000\t0001\n"), 0o600)
	for _, c := range []struct {
		listen, published string
		ok                bool
	}{
		{"127.0.0.1:8080", "", true},
		{"[::1]:8080", "", true},
		{"localhost:8080", "", true},
		{":8080", "127.0.0.1", true},
		{":8080", "", false},
		{":8080", "0.0.0.0", false},
		{"0.0.0.0:8080", "192.168.1.5", false},
		{"192.168.1.5:8080", "", false},
	} {
		l, e := newLocalAccess(c.listen, c.published, route)
		if (e == nil) != c.ok {
			t.Errorf("%+v: %v", c, e)
		}
		if e == nil && c.listen == ":8080" && !strings.Contains(l.String(), "172.18.0.1/32") {
			t.Errorf("gateway not trusted: %s", l)
		}
	}
	if _, e := newLocalAccess(":8080", "127.0.0.1", filepath.Join(t.TempDir(), "missing")); e == nil {
		t.Error("container mode without a known gateway must refuse")
	}
}

func localReq(method, peer, host string, hdr map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/api/articles?page=x", nil)
	r.RemoteAddr, r.Host = peer, host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}
func TestLocalAccessDecision(t *testing.T) {
	l := &localAccess{peers: append(append([]netip.Prefix{}, loopbackPeers...), netip.MustParsePrefix("172.18.0.1/32"))}
	json := map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8080", "Sec-Fetch-Site": "same-origin"}
	for _, c := range []struct {
		name string
		r    *http.Request
		want bool
	}{
		{"loopback", localReq("GET", "127.0.0.1:5000", "127.0.0.1:8080", nil), true},
		{"ipv6 loopback", localReq("GET", "[::1]:5000", "localhost:8080", nil), true},
		{"docker gateway", localReq("GET", "172.18.0.1:5000", "localhost:8080", nil), true},
		{"other container", localReq("GET", "172.18.0.3:5000", "localhost:8080", nil), false},
		{"lan peer", localReq("GET", "192.168.1.20:5000", "127.0.0.1:8080", nil), false},
		{"lan peer spoofing XFF", localReq("GET", "192.168.1.20:5000", "127.0.0.1:8080", map[string]string{"X-Forwarded-For": "127.0.0.1"}), false},
		{"proxied via loopback", localReq("GET", "127.0.0.1:5000", "127.0.0.1:8080", map[string]string{"X-Forwarded-For": "203.0.113.9"}), false},
		{"forwarded header", localReq("GET", "127.0.0.1:5000", "127.0.0.1:8080", map[string]string{"Forwarded": "for=203.0.113.9"}), false},
		{"dns rebinding host", localReq("GET", "127.0.0.1:5000", "evil.example:8080", nil), false},
		{"same-origin json post", localReq("POST", "127.0.0.1:5000", "127.0.0.1:8080", json), true},
		{"cross-site post", localReq("POST", "127.0.0.1:5000", "127.0.0.1:8080", map[string]string{"Content-Type": "application/json", "Origin": "https://evil.example"}), false},
		{"cross-site fetch metadata", localReq("DELETE", "127.0.0.1:5000", "127.0.0.1:8080", map[string]string{"Sec-Fetch-Site": "cross-site"}), false},
		{"text/plain post (no preflight)", localReq("POST", "127.0.0.1:5000", "127.0.0.1:8080", map[string]string{"Content-Type": "text/plain"}), false},
	} {
		if got := l.allows(c.r); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	var off *localAccess
	if off.allows(localReq("GET", "127.0.0.1:5000", "127.0.0.1:8080", nil)) {
		t.Error("disabled local mode must not allow")
	}
}
func TestAuthModes(t *testing.T) {
	token := strings.Repeat("a", 32)
	normal := &App{token: token}
	local := &App{token: token, local: &localAccess{peers: loopbackPeers}}
	do := func(app *App, path, peer, auth string) (int, string) {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr, r.Host = peer, "127.0.0.1:8080"
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	// page=x fails validation (400) only after authentication passed.
	if c, _ := do(normal, "/api/articles?page=x", "127.0.0.1:1", ""); c != 401 {
		t.Errorf("normal mode without token: %d", c)
	}
	if c, _ := do(normal, "/api/articles?page=x", "127.0.0.1:1", "Bearer "+token); c != 400 {
		t.Errorf("normal mode with token: %d", c)
	}
	if c, _ := do(local, "/api/articles?page=x", "127.0.0.1:1", ""); c != 400 {
		t.Errorf("local mode from loopback: %d", c)
	}
	if c, _ := do(local, "/api/articles?page=x", "192.168.1.20:1", ""); c != 401 {
		t.Errorf("local mode from LAN must still need the token: %d", c)
	}
	if c, _ := do(local, "/api/articles?page=x", "192.168.1.20:1", "Bearer "+token); c != 400 {
		t.Errorf("token still works in local mode: %d", c)
	}
	for _, c := range []struct {
		app  *App
		peer string
		want string
	}{{normal, "127.0.0.1:1", `{"auth_required":true}`}, {local, "127.0.0.1:1", `{"auth_required":false}`}, {local, "192.168.1.20:1", `{"auth_required":true}`}} {
		_, body := do(c.app, "/api/session", c.peer, "")
		if strings.TrimSpace(body) != c.want || strings.Contains(body, token) {
			t.Errorf("session %s: %s", c.peer, body)
		}
	}
}
func TestParseSeen(t *testing.T) {
	if s, c, ok := parseSeen("thoi-su:10,the-gioi:0"); !ok || len(s) != 2 || c[0] != 10 {
		t.Fatal(s, c, ok)
	}
	for _, v := range []string{"thoi-su", "thoi-su:-1", "Thoi:1", "x:1;drop", "thoi-su:1," + strings.Repeat("a:1,", 20)} {
		if _, _, ok := parseSeen(v); ok {
			t.Error("accepted", v)
		}
	}
}

// Integration tests run against a disposable PostgreSQL database named by
// TEST_DATABASE_URL; they are skipped otherwise. Never point it at real data:
// each test drops and recreates the public schema.
func testDB(t *testing.T) *pgxpool.Pool {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, e := pgxpool.New(context.Background(), url)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	if _, e = db.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); e != nil {
		t.Fatal(e)
	}
	return db
}
func migratedApp(t *testing.T) (*App, context.Context) {
	db := testDB(t)
	ctx := context.Background()
	if e := migrate(ctx, db, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	return &App{db: db, token: strings.Repeat("a", 32)}, ctx
}
func addSource(t *testing.T, a *App, name, url, category string) Source {
	s := Source{Name: name, Adapter: "bbc", FeedURL: url, Category: category}
	if e := a.db.QueryRow(context.Background(), `INSERT INTO sources(name,adapter,feed_url,category) VALUES($1,'bbc',$2,$3) RETURNING id`, name, url, category).Scan(&s.ID); e != nil {
		t.Fatal(e)
	}
	return s
}
func item(path, title string, published time.Time) news.Item {
	return news.Item{Title: title, URL: "https://www.bbc.co.uk/news/articles/" + path + "?at_medium=RSS&at_campaign=rss", Date: published.Format(time.RFC1123Z)}
}
func get(t *testing.T, a *App, path string, v any) int {
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer "+a.token)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if v != nil && w.Code == 200 {
		if e := json.Unmarshal(w.Body.Bytes(), v); e != nil {
			t.Fatal(e)
		}
	}
	return w.Code
}

type listResp struct {
	Items   []Article
	HasMore bool `json:"has_more"`
	Cursor  int64
}
type catResp struct {
	Cursor     int64
	Categories []Category
}

func newCounts(t *testing.T, a *App, seen string) map[string]int64 {
	var c catResp
	if code := get(t, a, "/api/categories?seen="+seen, &c); code != 200 {
		t.Fatalf("categories: %d", code)
	}
	out := map[string]int64{}
	for _, x := range c.Categories {
		out[x.Slug] = x.NewCount
	}
	return out
}

func TestMigrationsConcurrentAndRepeatable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- migrate(ctx, db, "../../migrations") }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var n, cats int
	db.QueryRow(ctx, `SELECT count(*), (SELECT count(*) FROM categories) FROM schema_migrations`).Scan(&n, &cats)
	if n != 11 || cats != 9 {
		t.Fatalf("migrations=%d categories=%d", n, cats)
	}
}

// Data written by the previous schema (001+002) survives 003, is backfilled
// only from stored evidence, and existing feeds are not duplicated.
func TestCategoryMigrationPreservesData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	old := t.TempDir()
	for _, f := range []string{"001_init.sql", "002_indexes.sql"} {
		b, _ := os.ReadFile("../../migrations/" + f)
		os.WriteFile(filepath.Join(old, f), b, 0o600)
	}
	if e := migrate(ctx, db, old); e != nil {
		t.Fatal(e)
	}
	db.Exec(ctx, `INSERT INTO sources(name,adapter,feed_url) VALUES('VnExpress Thời sự','vnexpress','https://vnexpress.net/rss/thoi-su.rss'),('Gone','bbc','https://feeds.bbci.co.uk/news/gone.xml')`)
	db.Exec(ctx, `UPDATE sources SET deleted=true WHERE name='Gone'`)
	db.Exec(ctx, `INSERT INTO articles(source_id,url,title,published_at,content_status,paragraphs) SELECT id,'https://vnexpress.net/a-'||id||'.html','T'||id,now(),'full','["p"]' FROM sources`)
	var before int
	db.QueryRow(ctx, `SELECT count(*) FROM articles`).Scan(&before)
	for i := 0; i < 2; i++ { // second run must be a no-op
		if e := migrate(ctx, db, "../../migrations"); e != nil {
			t.Fatal(e)
		}
	}
	var after, links, full, thoiSu, vneFeeds, khac int
	db.QueryRow(ctx, `SELECT (SELECT count(*) FROM articles),(SELECT count(*) FROM article_feeds),(SELECT count(*) FROM articles WHERE content_status='full'),
 (SELECT count(*) FROM sources WHERE feed_url='https://vnexpress.net/rss/thoi-su.rss' AND category='thoi-su'),
 (SELECT count(*) FROM sources WHERE feed_url LIKE 'https://vnexpress.net/%'),
 (SELECT count(*) FROM sources WHERE category='khac')`).Scan(&after, &links, &full, &thoiSu, &vneFeeds, &khac)
	var legacy int
	db.QueryRow(ctx, `SELECT count(*) FROM articles WHERE extract_version=0 AND blocks IS NULL AND paragraphs='["p"]' AND authors='{}'`).Scan(&legacy)
	if after != before || links != before || full != before || thoiSu != 1 || legacy != before {
		t.Fatalf("before=%d after=%d links=%d full=%d thoiSu=%d", before, after, links, full, thoiSu)
	}
	if vneFeeds != 9 || khac != 4 { // latest-news + thoi-su + 7 new section feeds; khac: both front pages, Gone and Tuổi Trẻ Video
		t.Fatalf("vnexpress feeds=%d khac=%d", vneFeeds, khac)
	}
}

func TestCategoryMappingAndDedupAcrossFeeds(t *testing.T) {
	a, ctx := migratedApp(t)
	world := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	biz := addSource(t, a, "Biz", "https://feeds.bbci.co.uk/news/t-biz.xml", "kinh-doanh")
	front := addSource(t, a, "Front", "https://feeds.bbci.co.uk/news/t-front.xml", "khac")
	now := time.Now()
	a.ingest(ctx, world, []news.Item{item("x", "Shared story", now)}, now)
	shared := item("x", "Shared story", now)
	shared.URL = "https://www.bbc.com/news/articles/x?utm_source=other#top" // same article, other host/tracking
	a.ingest(ctx, biz, []news.Item{shared, item("y", "Only biz", now)}, now)
	a.ingest(ctx, front, []news.Item{item("x", "Shared story", now)}, now)
	a.ingest(ctx, front, []news.Item{item("x", "Shared story", now)}, now) // re-read: no duplicates

	var l listResp
	get(t, a, "/api/articles", &l)
	if len(l.Items) != 2 {
		t.Fatalf("articles: %d", len(l.Items))
	}
	for _, it := range l.Items {
		if it.Title == "Shared story" {
			if fmt.Sprint(it.Categories) != "[the-gioi kinh-doanh khac]" || it.SourceID != world.ID {
				t.Fatalf("shared: owner=%d categories=%v", it.SourceID, it.Categories)
			}
		}
	}
	get(t, a, "/api/articles?category=kinh-doanh", &l)
	if len(l.Items) != 2 {
		t.Fatalf("kinh-doanh: %d", len(l.Items))
	}
	get(t, a, "/api/articles?category=the-thao", &l)
	if len(l.Items) != 0 {
		t.Fatal("empty category returned items")
	}
	// Moving a feed to another category moves its mapping; deleting a feed
	// removes its category but the owner rule for readability is unchanged.
	a.db.Exec(ctx, `UPDATE sources SET deleted=true WHERE id=$1`, biz.ID)
	get(t, a, "/api/articles?category=kinh-doanh", &l)
	if len(l.Items) != 0 {
		t.Fatalf("deleted feed still maps: %d", len(l.Items))
	}
	get(t, a, "/api/articles", &l)
	if len(l.Items) != 1 {
		t.Fatalf("article owned by deleted feed must be hidden: %d", len(l.Items))
	}
}

func TestDeletedSourceArticlesMoveToActiveFeed(t *testing.T) {
	a, ctx := migratedApp(t)
	old := addSource(t, a, "A", "https://feeds.bbci.co.uk/news/a.xml", "khac")
	active := addSource(t, a, "B", "https://feeds.bbci.co.uk/news/b.xml", "the-gioi")
	now := time.Now()
	a.ingest(ctx, old, []news.Item{item("x", "T", now)}, now)
	a.db.Exec(ctx, `UPDATE sources SET deleted=true WHERE id=$1`, old.ID)
	a.ingest(ctx, active, []news.Item{item("x", "T", now)}, now)
	var owner int64
	var count int
	a.db.QueryRow(ctx, `SELECT source_id,(SELECT count(*) FROM articles) FROM articles`).Scan(&owner, &count)
	if owner != active.ID || count != 1 {
		t.Fatalf("owner=%d count=%d", owner, count)
	}
	var l listResp
	if code := get(t, a, "/api/articles?q=%25", &l); code != 200 || len(l.Items) != 0 {
		t.Fatalf("literal %% search must not match everything: %d %d", code, len(l.Items))
	}
}

// New = first received after the browser's cursor for that category, whatever
// the published date says.
func TestNewArticleCounts(t *testing.T) {
	a, ctx := migratedApp(t)
	world := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	health := addSource(t, a, "Health", "https://feeds.bbci.co.uk/news/t-health.xml", "suc-khoe")
	now := time.Now()
	a.ingest(ctx, world, []news.Item{item("old1", "Old 1", now), item("old2", "Old 2", now)}, now)

	// First visit: the browser takes the current cursor as its baseline.
	var c catResp
	get(t, a, "/api/categories", &c)
	if c.Cursor == 0 || c.Categories[0].NewCount != 0 {
		t.Fatalf("baseline: %+v", c)
	}
	seen := fmt.Sprintf("the-gioi:%d,suc-khoe:%d", c.Cursor, c.Cursor)
	if n := newCounts(t, a, seen); n["the-gioi"] != 0 || n["suc-khoe"] != 0 {
		t.Fatalf("nothing new yet: %v", n)
	}
	// A late article (published two days ago) arrives, listed by both feeds,
	// and an already known article shows up again in the health feed.
	late := item("late", "Late", now.Add(-48*time.Hour))
	a.ingest(ctx, world, []news.Item{late}, now)
	a.ingest(ctx, health, []news.Item{late, item("old1", "Old 1", now)}, now)
	n := newCounts(t, a, seen)
	if n["the-gioi"] != 1 || n["suc-khoe"] != 1 || n["khac"] != 0 {
		t.Fatalf("late article must count once per category: %v", n)
	}
	// Opening the category loads a list with a cursor; marking seen up to it
	// clears the count, and an article arriving afterwards is new again.
	var l listResp
	get(t, a, "/api/articles?category=the-gioi", &l)
	seen = fmt.Sprintf("the-gioi:%d,suc-khoe:%d", l.Cursor, c.Cursor)
	a.ingest(ctx, world, []news.Item{item("after", "After", now)}, now)
	n = newCounts(t, a, seen)
	if n["the-gioi"] != 1 || n["suc-khoe"] != 1 {
		t.Fatalf("after marking seen: %v", n)
	}
	// Articles of a deleted owner are not readable, so not counted.
	a.db.Exec(ctx, `UPDATE sources SET deleted=true WHERE id=$1`, world.ID)
	n = newCounts(t, a, seen)
	if n["the-gioi"] != 0 || n["suc-khoe"] != 0 {
		t.Fatalf("deleted owner still counted: %v", n)
	}
	if code := get(t, a, "/api/categories?seen=bad", nil); code != 400 {
		t.Fatalf("bad seen: %d", code)
	}
}

// The list never contains an article newer than the cursor it returns, so a
// browser marking "seen" up to that cursor cannot hide an unseen article.
func TestListCursorCoversOnlyListedArticles(t *testing.T) {
	a, ctx := migratedApp(t)
	world := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	now := time.Now()
	a.ingest(ctx, world, []news.Item{item("a", "A", now)}, now)
	var l listResp
	get(t, a, "/api/articles?category=the-gioi", &l)
	for _, it := range l.Items {
		if it.ID > l.Cursor {
			t.Fatalf("item %d beyond cursor %d", it.ID, l.Cursor)
		}
	}
}

func TestCombinedFiltersAndPaging(t *testing.T) {
	a, ctx := migratedApp(t)
	world := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	biz := addSource(t, a, "Biz", "https://feeds.bbci.co.uk/news/t-biz.xml", "kinh-doanh")
	now := time.Now()
	var w, b []news.Item
	for i := 0; i < 35; i++ {
		w = append(w, item(fmt.Sprintf("w%d", i), fmt.Sprintf("Election news %d", i), now.Add(-time.Duration(i)*time.Minute)))
	}
	for i := 0; i < 5; i++ {
		b = append(b, item(fmt.Sprintf("b%d", i), fmt.Sprintf("Market election %d", i), now))
		b = append(b, item(fmt.Sprintf("c%d", i), fmt.Sprintf("Market other %d", i), now))
	}
	b = append(b, w[0]) // also listed by the business feed
	a.ingest(ctx, world, w, now)
	a.ingest(ctx, biz, b, now)
	cases := []struct {
		q          string
		page1, all int
	}{
		{"category=the-gioi", 30, 35},
		{"category=kinh-doanh", 11, 11},
		{"category=kinh-doanh&q=election", 6, 6},
		{fmt.Sprintf("category=kinh-doanh&source=%d&q=election", world.ID), 1, 1},
		{fmt.Sprintf("source=%d", biz.ID), 11, 11},
		{"q=Election", 30, 40},
		{"category=the-thao&q=election", 0, 0},
	}
	for _, c := range cases {
		var l listResp
		get(t, a, "/api/articles?"+c.q, &l)
		total, pages := len(l.Items), 1
		for more := l.HasMore; more; {
			pages++
			var p listResp
			get(t, a, fmt.Sprintf("/api/articles?%s&page=%d", c.q, pages), &p)
			total += len(p.Items)
			more = p.HasMore
		}
		if len(l.Items) != c.page1 || total != c.all {
			t.Errorf("%s: page1=%d total=%d", c.q, len(l.Items), total)
		}
	}
	if code := get(t, a, "/api/articles?category=Bad!", nil); code != 400 {
		t.Errorf("bad category: %d", code)
	}
}
func TestStaticFilesRevalidate(t *testing.T) {
	wd, _ := os.Getwd()
	os.Chdir("../..")
	defer os.Chdir(wd)
	app := &App{token: strings.Repeat("a", 32)}
	for _, p := range []string{"/", "/app.js", "/seen.js"} {
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: %d %q", p, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}

// Rows written before blocks/authors existed are still served, as paragraph blocks.
func TestLegacyArticleStillReadable(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	var id int64
	a.db.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,published_at,content_status,paragraphs) VALUES($1,'https://www.bbc.co.uk/news/articles/old','Old',now(),'full','["One","Two"]') RETURNING id`, s.ID).Scan(&id)
	a.db.Exec(ctx, `INSERT INTO article_feeds(article_id,source_id) VALUES($1,$2)`, id, s.ID)
	var art Article
	if code := get(t, a, fmt.Sprintf("/api/articles/%d", id), &art); code != 200 {
		t.Fatalf("legacy article: %d", code)
	}
	if len(art.Blocks) != 2 || art.Blocks[1].Type != "p" || art.Blocks[1].Text != "Two" || len(art.Authors) != 0 || art.LeadImage != nil {
		t.Fatalf("legacy blocks: %+v", art)
	}
}

func TestEnrichmentKeepsContentOnFailure(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	var id int64
	a.db.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,published_at,content_status,paragraphs) VALUES($1,'https://www.bbc.co.uk/news/articles/e','E',now(),'full','["Good text"]') RETURNING id`, s.ID).Scan(&id)
	status := func() map[string]int64 {
		var st struct{ Enrich map[string]int64 }
		get(t, a, "/api/status", &st)
		return st.Enrich
	}
	if st := status(); st["pending"] != 1 || st["done"] != 0 {
		t.Fatalf("status before: %v", st)
	}
	for i := 0; i < 3; i++ {
		if e := a.storeEnrichment(ctx, id, news.Content{}, fmt.Errorf("nguồn trả HTTP 404"), nil); e != nil {
			t.Fatal(e)
		}
	}
	var paras, status2 string
	var version, attempts int
	a.db.QueryRow(ctx, `SELECT paragraphs::text,content_status,extract_version,enrich_attempts FROM articles WHERE id=$1`, id).Scan(&paras, &status2, &version, &attempts)
	if paras != `["Good text"]` || status2 != "full" || version != 0 || attempts != 3 {
		t.Fatalf("failure changed content: %s %s v%d a%d", paras, status2, version, attempts)
	}
	if st := status(); st["failed"] != 1 || st["pending"] != 0 {
		t.Fatalf("status after failures: %v", st)
	}
	c := news.Content{Authors: []string{"Jo Adnitt"}, Blocks: []news.Block{{Type: "p", Text: "New text"}, {Type: "img", Image: &news.Image{Src: "https://ichef.bbci.co.uk/a.jpg", Caption: "Cap"}}}}
	if e := a.storeEnrichment(ctx, id, c, nil, nil); e != nil {
		t.Fatal(e)
	}
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if len(art.Blocks) != 2 || art.Blocks[1].Image.Caption != "Cap" || art.Authors[0] != "Jo Adnitt" || art.Paragraphs[0] != "New text" {
		t.Fatalf("enriched: %+v", art)
	}
	if st := status(); st["done"] != 1 {
		t.Fatalf("status after success: %v", st)
	}
}

// A failed first fetch keeps the RSS summary and the feed's image; authors and
// the og:image found on the page are added.
func TestFailedFetchKeepsFeedImage(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	it := item("v", "Video", time.Now())
	it.Thumbnail.URL = "https://ichef.bbci.co.uk/ace/standard/240/v.jpg"
	a.ingest(ctx, s, []news.Item{it}, time.Now())
	var id int64
	a.db.QueryRow(ctx, `SELECT id FROM articles`).Scan(&id)
	a.storeContent(ctx, id, news.Content{}, fmt.Errorf("nội dung không đủ"))
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if art.Status != "unavailable" || art.LeadImage == nil || art.LeadImage.Src != "https://ichef.bbci.co.uk/ace/standard/240/v.jpg" {
		t.Fatalf("feed image lost: %+v", art)
	}
	a.storeContent(ctx, id, news.Content{Authors: []string{"A B"}, Lead: &news.Image{Src: "https://ichef.bbci.co.uk/ace/standard/1200/v.jpg"}}, fmt.Errorf("video"))
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if art.LeadImage.Src != "https://ichef.bbci.co.uk/ace/standard/1200/v.jpg" || len(art.Authors) != 1 {
		t.Fatalf("page metadata not kept: %+v", art)
	}
}

func TestCSPAllowsOnlyImageCDNs(t *testing.T) {
	app := &App{token: strings.Repeat("a", 32)}
	w := httptest.NewRecorder()
	app.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/session", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "img-src 'self' https://ichef.bbci.co.uk https://*.vnecdn.net https://cdn2.tuoitre.vn; media-src 'self' https://cdn2.tuoitre.vn;") || strings.Contains(csp, "img-src *") || strings.Contains(csp, "https: ") || strings.Contains(csp, "data:") {
		t.Fatal(csp)
	}
}

func addVnSource(t *testing.T, a *App, name, url, category string) Source {
	s := Source{Name: name, Adapter: "vnexpress", FeedURL: url, Category: category}
	if e := a.db.QueryRow(context.Background(), `INSERT INTO sources(name,adapter,feed_url,category) VALUES($1,'vnexpress',$2,$3) RETURNING id`, name, url, category).Scan(&s.ID); e != nil {
		t.Fatal(e)
	}
	return s
}
func vnItem(slug, title string, published time.Time) news.Item {
	return news.Item{Title: title, URL: "https://vnexpress.net/" + slug + ".html?utm_source=rss", Date: published.Format(time.RFC1123Z)}
}

// A retitled VnExpress article (same id, new slug) stays one article: the link
// and title follow the new slug, no new id is allocated (so it is not "new"
// again), and a lagging feed that still lists the old slug changes nothing.
func TestVnExpressSlugChangeKeepsOneArticle(t *testing.T) {
	a, ctx := migratedApp(t)
	world := addVnSource(t, a, "VnE Thế giới", "https://vnexpress.net/rss/t-the-gioi.rss", "the-gioi")
	latest := addVnSource(t, a, "VnE mới nhất", "https://vnexpress.net/rss/t-moi.rss", "khac")
	now := time.Now()
	a.ingest(ctx, world, []news.Item{vnItem("thu-tuong-nhat-goi-trung-quoc-5128557", "Old title", now)}, now)
	var c catResp
	get(t, a, "/api/categories", &c)
	seen := fmt.Sprintf("the-gioi:%d,khac:%d", c.Cursor, c.Cursor)
	a.ingest(ctx, world, []news.Item{vnItem("thu-tuong-nhat-diu-giong-5128557", "New title", now)}, now)
	a.ingest(ctx, latest, []news.Item{vnItem("thu-tuong-nhat-goi-trung-quoc-5128557", "Old title", now)}, now)
	var n int
	var id, maxID int64
	var url, title, former string
	a.db.QueryRow(ctx, `SELECT count(*) OVER (),id,url,title,former_urls::text,(SELECT max(id) FROM articles) FROM articles`).Scan(&n, &id, &url, &title, &former, &maxID)
	if n != 1 || url != "https://vnexpress.net/thu-tuong-nhat-diu-giong-5128557.html" || title != "New title" || former != "{https://vnexpress.net/thu-tuong-nhat-goi-trung-quoc-5128557.html}" {
		t.Fatalf("n=%d url=%s title=%s former=%s", n, url, title, former)
	}
	if cnt := newCounts(t, a, seen); cnt["the-gioi"] != 0 || cnt["khac"] != 0 {
		t.Fatalf("slug change counted as new: %v", cnt)
	}
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if fmt.Sprint(art.Categories) != "[the-gioi khac]" || art.Adapter != "vnexpress" {
		t.Fatalf("categories=%v adapter=%s", art.Categories, art.Adapter)
	}
	// URLs without a verified id shape keep URL identity.
	a.ingest(ctx, world, []news.Item{vnItem("tran-dau-5128002-tong-thuat", "Live", now), vnItem("tran-dau-5128003-tong-thuat", "Live 2", now)}, now)
	a.db.QueryRow(ctx, `SELECT count(*) FROM articles WHERE ident IS NULL`).Scan(&n)
	if n != 2 {
		t.Fatalf("unrecognised URLs: %d without ident", n)
	}
}

// Duplicates stored before ident existed are merged once into the oldest row,
// keeping the best content whole, all feed links, and nothing orphaned.
func TestMergeDuplicatesKeepsBestContent(t *testing.T) {
	a, ctx := migratedApp(t)
	front := addVnSource(t, a, "VnE", "https://vnexpress.net/rss/t-moi.rss", "khac")
	sport := addVnSource(t, a, "VnE Thể thao", "https://vnexpress.net/rss/t-the-thao.rss", "the-thao")
	bbc := addSource(t, a, "BBC", "https://feeds.bbci.co.uk/news/t.xml", "khac")
	pub := time.Date(2026, 10, 5, 4, 15, 0, 0, time.UTC)
	ins := func(src int64, url, title, status string, version int, blocks, authors string, fetched time.Time) int64 {
		var id int64
		if e := a.db.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,summary,published_at,fetched_at,content_status,extract_version,paragraphs,blocks,authors)
VALUES($1,$2,$3,'Sum',$4,$5,$6,$7,'["p1"]',$8::jsonb,$9::text[]) RETURNING id`, src, url, title, pub, fetched, status, version, blocks, authors).Scan(&id); e != nil {
			t.Fatal(e)
		}
		a.db.Exec(ctx, `INSERT INTO article_feeds(article_id,source_id,first_seen) VALUES($1,$2,$3)`, id, src, fetched)
		return id
	}
	t0 := time.Now().Add(-time.Hour)
	full := `[{"type":"p","text":"p1"},{"type":"img","image":{"src":"https://i1-vnexpress.vnecdn.net/a.jpg","caption":"Cap","credit":"Ảnh: X"}},{"type":"p","text":"p2"}]`
	keep := ins(front.ID, "https://vnexpress.net/vozinha-5128310.html", "Vozinha", "full", 3, full, "{An}", t0)
	dup := ins(sport.ID, "https://vnexpress.net/thu-mon-vozinha-5128310.html", "Thủ môn Vozinha", "summary", 0, "null", "{}", t0.Add(30*time.Minute))
	// Same id, different publication time: not proven, kept apart.
	o1 := ins(front.ID, "https://vnexpress.net/a-5129000.html", "A", "full", 3, full, "{}", t0)
	o2 := ins(front.ID, "https://vnexpress.net/b-5129000.html", "B", "full", 3, full, "{}", t0)
	a.db.Exec(ctx, `UPDATE articles SET published_at=published_at+interval '1 day' WHERE id=$1`, o2)
	// Not a recognised shape, and BBC: untouched.
	ins(front.ID, "https://vnexpress.net/live-5128002-tong-thuat.html", "Live", "full", 3, full, "{}", t0)
	ins(bbc.ID, "https://www.bbc.co.uk/news/articles/c-5128310.html", "BBC", "full", 3, full, "{}", t0)

	rep, e := mergeDuplicates(ctx, a.db)
	if e != nil {
		t.Fatal(e)
	}
	if rep.Groups != 2 || rep.Merged != 1 || rep.Removed != 1 || len(rep.Skipped) != 1 || !strings.HasPrefix(rep.Skipped[0], "vnexpress:5129000") {
		t.Fatalf("report: %+v", rep)
	}
	var art Article
	if code := get(t, a, fmt.Sprintf("/api/articles/%d", keep), &art); code != 200 {
		t.Fatalf("kept article: %d", code)
	}
	if art.URL != "https://vnexpress.net/thu-mon-vozinha-5128310.html" || art.Title != "Thủ môn Vozinha" || art.Status != "full" ||
		len(art.Blocks) != 3 || art.Blocks[1].Image.Caption != "Cap" || fmt.Sprint(art.Authors) != "[An]" || fmt.Sprint(art.Categories) != "[the-thao khac]" {
		t.Fatalf("merged: %+v", art)
	}
	if code := get(t, a, fmt.Sprintf("/api/articles/%d", dup), nil); code != 404 {
		t.Fatalf("duplicate still served: %d", code)
	}
	var orphans, total, idents int
	a.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM article_feeds af WHERE NOT EXISTS(SELECT 1 FROM articles x WHERE x.id=af.article_id)),
 (SELECT count(*) FROM articles),(SELECT count(*) FROM articles WHERE ident IS NOT NULL)`).Scan(&orphans, &total, &idents)
	if orphans != 0 || total != 5 || idents != 2 {
		t.Fatalf("orphans=%d total=%d idents=%d", orphans, total, idents)
	}
	var o1Ident, o2Ident *string
	a.db.QueryRow(ctx, `SELECT (SELECT ident FROM articles WHERE id=$1),(SELECT ident FROM articles WHERE id=$2)`, o1, o2).Scan(&o1Ident, &o2Ident)
	if o1Ident == nil || o2Ident != nil {
		t.Fatal("unproven group: only the oldest row gets the ident")
	}
	// Running again changes nothing further.
	rep, e = mergeDuplicates(ctx, a.db)
	if e != nil || rep.Merged != 0 || rep.Removed != 0 {
		t.Fatalf("second run: %+v %v", rep, e)
	}
	// The old slug arriving again from a lagging feed maps to the kept row.
	a.ingest(ctx, front, []news.Item{vnItem("vozinha-5128310", "Vozinha", pub)}, time.Now())
	a.db.QueryRow(ctx, `SELECT count(*) FROM articles`).Scan(&total)
	get(t, a, fmt.Sprintf("/api/articles/%d", keep), &art)
	if total != 5 || art.URL != "https://vnexpress.net/thu-mon-vozinha-5128310.html" {
		t.Fatalf("old slug: total=%d url=%s", total, art.URL)
	}
}

// Source icons are local files, so reading never contacts a third party.
func TestSourceIconsServedLocally(t *testing.T) {
	wd, _ := os.Getwd()
	os.Chdir("../..")
	defer os.Chdir(wd)
	app := &App{token: strings.Repeat("a", 32)}
	for _, p := range []string{"/icons/vnexpress.png", "/icons/bbc.png"} {
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: %d %q", p, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

// Tuổi Trẻ: one row per article id across feeds and slugs, the feed's local
// date read as Vietnam time, and the migration's feeds added once.
func TestTuoiTreIngest(t *testing.T) {
	a, ctx := migratedApp(t)
	var feeds int
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources WHERE adapter='tuoitre'`).Scan(&feeds)
	if e := migrate(ctx, a.db, "../../migrations"); e != nil || feeds != 4 {
		t.Fatalf("feeds=%d err=%v", feeds, e)
	}
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources WHERE adapter='tuoitre'`).Scan(&feeds)
	if feeds != 4 {
		t.Fatalf("migration not idempotent: %d feeds", feeds)
	}
	src := func(url string) Source {
		var s Source
		a.db.QueryRow(ctx, `SELECT id,name,adapter,feed_url,category FROM sources WHERE feed_url=$1`, url).Scan(&s.ID, &s.Name, &s.Adapter, &s.FeedURL, &s.Category)
		return s
	}
	ts, kd := src("https://tuoitre.vn/thoi-su.rss"), src("https://tuoitre.vn/kinh-doanh.rss")
	now := time.Now()
	it := news.Item{Title: "Bài A", URL: "https://tuoitre.vn/bai-a-100261005142210397.htm", Date: "10/5/2026 2:22:00 PM"}
	a.ingest(ctx, ts, []news.Item{it}, now)
	it2 := it
	it2.Title, it2.URL = "Bài A (sửa)", "https://tuoitre.vn/bai-a-da-sua-100261005142210397.htm"
	a.ingest(ctx, kd, []news.Item{it2}, now)
	var n int
	var id int64
	var url, ident string
	var pub time.Time
	a.db.QueryRow(ctx, `SELECT count(*) OVER (),id,url,ident,published_at FROM articles`).Scan(&n, &id, &url, &ident, &pub)
	if n != 1 || url != it2.URL || ident != "tuoitre:100261005142210397" || !pub.Equal(time.Date(2026, 10, 5, 7, 22, 0, 0, time.UTC)) {
		t.Fatalf("n=%d url=%s ident=%s pub=%v", n, url, ident, pub)
	}
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if fmt.Sprint(art.Categories) != "[thoi-su kinh-doanh]" || art.Adapter != "tuoitre" || art.Source != "Tuổi Trẻ Thời sự" {
		t.Fatalf("%+v", art)
	}
}

func videoBlock(kind, src string) news.Content {
	return news.Content{Blocks: []news.Block{{Type: "p", Text: "Mô tả ngắn"}, {Type: "video", Video: &news.Video{Kind: kind, Src: src, Poster: "https://cdn2.tuoitre.vn/thumb_w/1200/a.jpg", PageURL: "https://tuoitre.vn/video/a-1.htm"}}}}
}

// A video page keeps its RSS summary and is not "full"; only its video is
// stored, it is labelled as video and not retried.
func TestVideoPageStoredAsVideoNotFullText(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	a.ingest(ctx, s, []news.Item{item("v", "Video page", time.Now())}, time.Now())
	var id int64
	a.db.QueryRow(ctx, `SELECT id FROM articles`).Scan(&id)
	if e := a.storeContent(ctx, id, videoBlock("mp4", "https://cdn2.tuoitre.vn/a.mp4"), fmt.Errorf("trang video, không có toàn văn dạng chữ")); e != nil {
		t.Fatal(e)
	}
	var l listResp
	get(t, a, "/api/articles", &l)
	if len(l.Items) != 1 || !l.Items[0].HasVideo || l.Items[0].Status != "unavailable" {
		t.Fatalf("list: %+v", l.Items)
	}
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", id), &art)
	if len(art.Blocks) != 1 || art.Blocks[0].Type != "video" || art.Blocks[0].Video.Src != "https://cdn2.tuoitre.vn/a.mp4" || len(art.Paragraphs) != 0 {
		t.Fatalf("detail: %+v", art)
	}
	var attempts int
	a.db.QueryRow(ctx, `SELECT attempts FROM articles WHERE id=$1`, id).Scan(&attempts)
	if attempts != 3 {
		t.Fatalf("video page will be retried: attempts=%d", attempts)
	}
	// A page without video and a network error still back off and retry.
	a.ingest(ctx, s, []news.Item{item("n", "Net error", time.Now())}, time.Now())
	var id2 int64
	a.db.QueryRow(ctx, `SELECT id FROM articles WHERE title='Net error'`).Scan(&id2)
	a.storeContent(ctx, id2, news.Content{}, fmt.Errorf("timeout"))
	a.db.QueryRow(ctx, `SELECT attempts FROM articles WHERE id=$1`, id2).Scan(&attempts)
	if attempts != 1 {
		t.Fatalf("attempts=%d", attempts)
	}
}

// The backfill adds videos to stored articles once, never changes the text of
// a summary-only article, and leaves everything as it was on failure.
func TestVideoBackfill(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	var full, summary int64
	a.db.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,summary,published_at,content_status,paragraphs,extract_version) VALUES($1,'https://www.bbc.co.uk/news/articles/f','F','S',now(),'full','["Văn bản cũ tốt"]',3) RETURNING id`, s.ID).Scan(&full)
	a.db.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,summary,published_at,content_status,attempts,extract_version) VALUES($1,'https://www.bbc.co.uk/news/videos/v','V','Tóm tắt RSS',now(),'unavailable',3,3) RETURNING id`, s.ID).Scan(&summary)
	var st struct{ Enrich map[string]int64 }
	get(t, a, "/api/status", &st)
	if st.Enrich["pending"] != 2 {
		t.Fatalf("both rows are due for the video backfill: %v", st.Enrich)
	}
	// Network failure: nothing changes.
	a.storeEnrichment(ctx, full, news.Content{}, fmt.Errorf("HTTP 503"), nil)
	// Re-extraction of the full article fails: stored text is kept.
	a.storeEnrichment(ctx, full, news.Content{}, nil, fmt.Errorf("cấu trúc trang đã thay đổi"))
	var paras string
	var hv bool
	a.db.QueryRow(ctx, `SELECT paragraphs::text,has_video FROM articles WHERE id=$1`, full).Scan(&paras, &hv)
	if paras != `["Văn bản cũ tốt"]` || hv {
		t.Fatalf("failure changed content: %s %v", paras, hv)
	}
	// Summary-only video page: only the video is added.
	v := videoBlock("link", "")
	if e := a.storeEnrichment(ctx, summary, v, nil, fmt.Errorf("nội dung không đủ")); e != nil {
		t.Fatal(e)
	}
	var status, sum, blocks string
	var version int
	a.db.QueryRow(ctx, `SELECT content_status,summary,blocks::text,has_video,extract_version FROM articles WHERE id=$1`, summary).Scan(&status, &sum, &blocks, &hv, &version)
	if status != "unavailable" || sum != "Tóm tắt RSS" || !hv || version != news.ExtractVersion || !strings.Contains(blocks, `"type": "video"`) || strings.Contains(blocks, "Mô tả ngắn") {
		t.Fatalf("summary row: %s %s %s %v v%d", status, sum, blocks, hv, version)
	}
	get(t, a, "/api/status", &st)
	if st.Enrich["pending"] != 1 || st.Enrich["done"] != 1 {
		t.Fatalf("status: %v", st.Enrich)
	}
	// Old rows without blocks or video are still read as before.
	var art Article
	get(t, a, fmt.Sprintf("/api/articles/%d", full), &art)
	if art.HasVideo || len(art.Blocks) != 1 || art.Blocks[0].Text != "Văn bản cũ tốt" {
		t.Fatalf("legacy: %+v", art)
	}
}

func postImport(t *testing.T, a *App, u string) (int, importResult) {
	b, _ := json.Marshal(map[string]string{"url": u})
	r := httptest.NewRequest("POST", "/api/sources/import", strings.NewReader(string(b)))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	var res importResult
	json.Unmarshal(w.Body.Bytes(), &res)
	return w.Code, res
}

// stubFeeds replaces the network check: URLs in bad fail, the rest parse.
func stubFeeds(t *testing.T, bad map[string]bool) *[]string {
	calls := &[]string{}
	var mu sync.Mutex
	old := checkFeed
	checkFeed = func(ctx context.Context, adapter, u string) (string, error) {
		mu.Lock()
		*calls = append(*calls, u)
		mu.Unlock()
		if bad[u] {
			return "", fmt.Errorf("nguồn trả HTTP 404")
		}
		return "TUOI TRE ONLINE - Giáo dục - RSS Feed", nil
	}
	t.Cleanup(func() { checkFeed = old })
	return calls
}

func TestImportHomePageKeepsUserSettings(t *testing.T) {
	a, ctx := migratedApp(t)
	a.kick = make(chan struct{}, 1)
	// The migrations seed most VnExpress feeds; the user renamed one,
	// disabled one and deleted one.
	a.db.Exec(ctx, `UPDATE sources SET name='Tin TG của tôi',category='khac' WHERE feed_url='https://vnexpress.net/rss/the-gioi.rss'`)
	a.db.Exec(ctx, `UPDATE sources SET enabled=false WHERE feed_url='https://vnexpress.net/rss/kinh-doanh.rss'`)
	a.db.Exec(ctx, `UPDATE sources SET deleted=true,enabled=false,name='Thể thao riêng' WHERE feed_url='https://vnexpress.net/rss/the-thao.rss'`)
	a.db.Exec(ctx, `DELETE FROM sources WHERE feed_url='https://vnexpress.net/rss/suc-khoe.rss'`)
	calls := stubFeeds(t, nil)
	code, res := postImport(t, a, "https://vnexpress.net/")
	if code != 200 || res.Status != "ok" || len(res.Added) != 1 || len(res.Restored) != 1 || len(res.Existing) != 7 || len(res.Failed) != 0 {
		t.Fatalf("%d %+v", code, res)
	}
	// Only the new and the restored feed were read.
	if len(*calls) != 2 {
		t.Fatalf("feeds read: %v", *calls)
	}
	var name, cat string
	var enabled bool
	a.db.QueryRow(ctx, `SELECT name,category FROM sources WHERE feed_url='https://vnexpress.net/rss/the-gioi.rss'`).Scan(&name, &cat)
	if name != "Tin TG của tôi" || cat != "khac" {
		t.Fatalf("user settings overwritten: %s %s", name, cat)
	}
	a.db.QueryRow(ctx, `SELECT name,enabled FROM sources WHERE feed_url='https://vnexpress.net/rss/the-thao.rss'`).Scan(&name, &enabled)
	if name != "Thể thao riêng" || !enabled {
		t.Fatalf("restore: %s %v", name, enabled)
	}
	a.db.QueryRow(ctx, `SELECT enabled FROM sources WHERE feed_url='https://vnexpress.net/rss/kinh-doanh.rss'`).Scan(&enabled)
	a.db.QueryRow(ctx, `SELECT category FROM sources WHERE feed_url='https://vnexpress.net/rss/suc-khoe.rss'`).Scan(&cat)
	if !enabled || cat != "suc-khoe" {
		t.Fatalf("re-enable %v / catalog category %s", enabled, cat)
	}
	select {
	case <-a.kick:
	default:
		t.Fatal("worker was not asked for a round")
	}
	// Importing again changes nothing and adds nothing.
	var before, after int
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&before)
	code, res = postImport(t, a, "https://www.vnexpress.net")
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources`).Scan(&after)
	if code != 200 || res.Status != "exists" || before != after || len(*calls) != 2 {
		t.Fatalf("repeat: %d %+v %d/%d", code, res, before, after)
	}
}

func TestImportRSSPartialAndRejected(t *testing.T) {
	a, ctx := migratedApp(t)
	calls := stubFeeds(t, map[string]bool{"https://tuoitre.vn/video.rss": true})
	a.db.Exec(ctx, `DELETE FROM sources WHERE adapter='tuoitre'`)
	code, res := postImport(t, a, "https://tuoitre.vn/")
	if code != 200 || res.Status != "partial" || len(res.Added) != 3 || len(res.Failed) != 1 || res.Failed[0].FeedURL != "https://tuoitre.vn/video.rss" || res.Failed[0].Name != "Tuổi Trẻ Video" {
		t.Fatalf("partial: %d %+v", code, res)
	}
	var n int
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources WHERE adapter='tuoitre'`).Scan(&n)
	if n != 3 {
		t.Fatalf("failed feed stored: %d", n)
	}
	// A feed outside the catalog: name from its title, category Khác.
	code, res = postImport(t, a, "https://tuoitre.vn/rss/giao-duc.rss")
	if code != 200 || res.Status != "ok" || res.Added[0].Name != "Tuổi Trẻ Giáo dục" || res.Added[0].Category != "khac" || res.Added[0].FeedURL != "https://tuoitre.vn/giao-duc.rss" {
		t.Fatalf("rss: %+v", res)
	}
	// Article, unsupported site, invalid URL: 422, nothing read or stored.
	*calls = nil
	for _, u := range []string{"https://vnexpress.net/a-5128557.html", "https://example.com/", "http://vnexpress.net/", "https://vnexpress.net/thoi-su"} {
		if code, res = postImport(t, a, u); code != 422 || res.Status != "rejected" || res.Message == "" {
			t.Errorf("%s: %d %+v", u, code, res)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("rejected URLs were fetched: %v", *calls)
	}
}

func TestImportConcurrentNoDuplicates(t *testing.T) {
	a, ctx := migratedApp(t)
	stubFeeds(t, nil)
	a.db.Exec(ctx, `DELETE FROM sources WHERE adapter='bbc'`)
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, _ := postImport(t, a, "https://www.bbc.co.uk/"); codes <- c }()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		if c == 200 {
			ok++
		} else if c != 429 {
			t.Fatalf("code %d", c)
		}
	}
	var n, distinct int
	a.db.QueryRow(ctx, `SELECT count(*),count(DISTINCT feed_url) FROM sources WHERE adapter='bbc'`).Scan(&n, &distinct)
	if ok == 0 || n != 6 || distinct != 6 {
		t.Fatalf("ok=%d rows=%d distinct=%d", ok, n, distinct)
	}
}

func TestImportRequiresAuthAndRateLimit(t *testing.T) {
	a := &App{token: strings.Repeat("a", 32)}
	r := httptest.NewRequest("POST", "/api/sources/import", strings.NewReader(`{"url":"https://vnexpress.net/"}`))
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	now := time.Now()
	for i := 0; i < importPerMin; i++ {
		if ok, _ := a.imports.enter(now); !ok {
			t.Fatal("rejected early")
		}
		a.imports.leave()
	}
	if ok, _ := a.imports.enter(now); ok {
		t.Fatal("rate limit not applied")
	}
	if ok, _ := a.imports.enter(now.Add(61 * time.Second)); !ok {
		t.Fatal("limit does not expire")
	}
	if ok, _ := a.imports.enter(now.Add(62 * time.Second)); ok {
		t.Fatal("second concurrent import allowed")
	}
}

func TestImportNames(t *testing.T) {
	for _, c := range [][3]string{
		{"tuoitre", "TUOI TRE ONLINE - Giáo dục - RSS Feed", "Tuổi Trẻ Giáo dục"},
		{"vnexpress", "Du lịch - VnExpress RSS", "VnExpress Du lịch"},
		{"tuoitre", "Tên khác", "Tên khác"},
	} {
		if got := importName(news.CatalogFeed{Adapter: c[0], FeedURL: "https://x/a.rss"}, c[1]); got != c[2] {
			t.Errorf("%q: %q", c[1], got)
		}
	}
	if got := importName(news.CatalogFeed{Adapter: "bbc", FeedURL: "https://feeds.bbci.co.uk/news/science_and_environment/rss.xml"}, "BBC News"); got != "BBC science and environment" {
		t.Error(got)
	}
	if got := importName(news.CatalogFeed{Adapter: "tuoitre", FeedURL: "https://tuoitre.vn/a.rss"}, ""); got != "https://tuoitre.vn/a.rss" {
		t.Error(got)
	}
}

// A source card can rename a source and change its category; the adapter and
// feed URL never change through it.
func TestEditSourceNameAndCategory(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "BBC thử", "https://feeds.bbci.co.uk/news/t-x.xml", "khac")
	patch := func(body string) int {
		r := httptest.NewRequest("PATCH", fmt.Sprintf("/api/sources/%d", s.ID), strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+a.token)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, r)
		return w.Code
	}
	if c := patch(`{"name":"  Khoa học BBC  ","category":"cong-nghe","adapter":"vnexpress","feed_url":"https://vnexpress.net/rss/x.rss"}`); c != 200 {
		t.Fatal(c)
	}
	var name, cat, adapter, feed string
	a.db.QueryRow(ctx, `SELECT name,category,adapter,feed_url FROM sources WHERE id=$1`, s.ID).Scan(&name, &cat, &adapter, &feed)
	if name != "Khoa học BBC" || cat != "cong-nghe" || adapter != "bbc" || feed != "https://feeds.bbci.co.uk/news/t-x.xml" {
		t.Fatalf("%s %s %s %s", name, cat, adapter, feed)
	}
	for _, b := range []string{`{"name":"   "}`, `{"name":"` + strings.Repeat("x", 101) + `"}`, `{"adapter":"vnexpress"}`, `{"category":"BAD"}`} {
		if c := patch(b); c != 400 {
			t.Errorf("%s: %d", b, c)
		}
	}
	a.db.QueryRow(ctx, `SELECT name FROM sources WHERE id=$1`, s.ID).Scan(&name)
	if name != "Khoa học BBC" {
		t.Fatal(name)
	}
}

func TestImportMessages(t *testing.T) {
	a, ctx := migratedApp(t)
	stubFeeds(t, map[string]bool{"https://tuoitre.vn/video.rss": true})
	if _, res := postImport(t, a, "https://tuoitre.vn/rss/giao-duc.rss"); res.Message != "Đã thêm nguồn: Tuổi Trẻ Giáo dục. Tin mới sẽ được cập nhật tự động." {
		t.Fatal(res.Message)
	}
	if _, res := postImport(t, a, "https://tuoitre.vn/giao-duc.rss"); res.Status != "exists" || res.Message != "Nguồn này đã có trong danh sách." {
		t.Fatal(res.Message)
	}
	a.db.Exec(ctx, `DELETE FROM sources WHERE adapter='tuoitre' AND feed_url<>'https://tuoitre.vn/giao-duc.rss'`)
	if _, res := postImport(t, a, "https://tuoitre.vn/"); res.Message != "Đã thêm 3 kênh, 1 kênh lỗi. Tin mới sẽ được cập nhật tự động." {
		t.Fatal(res.Message)
	}
	if _, res := postImport(t, a, "https://tuoitre.vn/video.rss"); res.Status != "failed" || !strings.HasPrefix(res.Message, "Không thêm được nguồn: không đọc được RSS") {
		t.Fatal(res.Message)
	}
	a.db.Exec(ctx, `DELETE FROM sources WHERE adapter='bbc'`)
	if _, res := postImport(t, a, "https://www.bbc.co.uk/"); res.Message != "Đã thêm 6 kênh. Tin mới sẽ được cập nhật tự động." {
		t.Fatal(res.Message)
	}
}
