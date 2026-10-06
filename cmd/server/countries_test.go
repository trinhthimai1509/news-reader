package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"personal.news/reader/internal/news"
)

func addTTSource(t *testing.T, a *App, name, url, category string) Source {
	s := Source{Name: name, Adapter: "tuoitre", FeedURL: url, Category: category}
	if e := a.db.QueryRow(context.Background(), `INSERT INTO sources(name,adapter,feed_url,category) VALUES($1,'tuoitre',$2,$3) RETURNING id`, name, url, category).Scan(&s.ID); e != nil {
		t.Fatal(e)
	}
	return s
}

func countAll(t *testing.T, a *App, q string) (page1, total int) {
	var l listResp
	if code := get(t, a, "/api/articles?"+q, &l); code != 200 {
		t.Fatalf("%s: %d", q, code)
	}
	page1, total = len(l.Items), len(l.Items)
	for p, more := 2, l.HasMore; more; p++ {
		var n listResp
		get(t, a, fmt.Sprintf("/api/articles?%s&page=%d", q, p), &n)
		total += len(n.Items)
		more = n.HasMore
	}
	return
}

// The country comes from the publisher: VnExpress and Tuổi Trẻ are Việt Nam,
// BBC is Vương quốc Anh, whatever the article is about. It combines with
// category, source, keyword and paging, and does not change the new-article
// counts of the categories.
func TestCountryFilter(t *testing.T) {
	a, ctx := migratedApp(t)
	bbc := addSource(t, a, "BBC World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	vne := addVnSource(t, a, "VnE Thế giới", "https://vnexpress.net/rss/t-the-gioi.rss", "the-gioi")
	vneBiz := addVnSource(t, a, "VnE Kinh doanh", "https://vnexpress.net/rss/t-kd.rss", "kinh-doanh")
	tt := addTTSource(t, a, "TT Thế giới", "https://tuoitre.vn/t-the-gioi.rss", "the-gioi")
	now := time.Now()
	var b, v []news.Item
	for i := 0; i < 33; i++ {
		// A BBC article about Vietnam is still a British-publisher article.
		b = append(b, item(fmt.Sprintf("b%d", i), fmt.Sprintf("Vietnam election %d", i), now.Add(-time.Duration(i)*time.Minute)))
	}
	for i := 0; i < 4; i++ {
		v = append(v, vnItem(fmt.Sprintf("bau-cu-anh-%d", 5000000+i), fmt.Sprintf("Bầu cử Anh election %d", i), now))
	}
	a.ingest(ctx, bbc, b, now)
	a.ingest(ctx, vne, v, now)
	a.ingest(ctx, vneBiz, []news.Item{vnItem("thi-truong-5100000", "Thị trường", now), v[0]}, now)
	a.ingest(ctx, tt, []news.Item{{Title: "Tin Thái Lan", URL: "https://tuoitre.vn/tin-thai-lan-100261006120000001.htm", Date: now.Format(time.RFC1123Z)}}, now)

	var seenBefore catResp
	get(t, a, "/api/categories", &seenBefore)
	seen := fmt.Sprintf("the-gioi:%d,kinh-doanh:%d", seenBefore.Cursor-10, seenBefore.Cursor-10)
	countsBefore := newCounts(t, a, seen)

	cases := []struct {
		q           string
		page1, all  int
		wantCountry string
	}{
		{"country=GB", 30, 33, "GB"},
		{"country=VN", 6, 6, "VN"},
		{"country=TH", 0, 0, ""},
		{"country=unknown", 0, 0, ""},
		{"country=VN&category=kinh-doanh", 2, 2, "VN"},
		{"country=GB&category=kinh-doanh", 0, 0, ""},
		{"country=VN&q=election", 4, 4, "VN"},
		{"country=GB&q=election", 30, 33, "GB"},
		{fmt.Sprintf("country=VN&source=%d", tt.ID), 1, 1, "VN"},
		{fmt.Sprintf("country=GB&source=%d", vne.ID), 0, 0, ""},
		{"country=VN&category=the-gioi&q=election", 4, 4, "VN"},
		{"q=election", 30, 37, ""},
	}
	for _, c := range cases {
		p1, all := countAll(t, a, c.q)
		if p1 != c.page1 || all != c.all {
			t.Errorf("%s: page1=%d total=%d", c.q, p1, all)
		}
		if c.wantCountry != "" {
			var l listResp
			get(t, a, "/api/articles?"+c.q, &l)
			for _, it := range l.Items {
				if it.Country != c.wantCountry {
					t.Errorf("%s: item %d country %q", c.q, it.ID, it.Country)
				}
			}
		}
	}
	// Category counts (red dots) ignore the country filter entirely.
	if after := newCounts(t, a, seen); fmt.Sprint(after) != fmt.Sprint(countsBefore) {
		t.Fatalf("counts changed: %v -> %v", countsBefore, after)
	}
	// A publisher without a country shows up only under "unknown".
	a.db.Exec(ctx, `UPDATE publishers SET country=NULL WHERE adapter='tuoitre'`)
	if _, all := countAll(t, a, "country=unknown"); all != 1 {
		t.Fatalf("unknown: %d", all)
	}
	if _, all := countAll(t, a, "country=VN"); all != 5 {
		t.Fatalf("VN without Tuổi Trẻ: %d", all)
	}
	var art Article
	var id int64
	a.db.QueryRow(ctx, `SELECT id FROM articles WHERE source_id=$1`, tt.ID).Scan(&id)
	if get(t, a, fmt.Sprintf("/api/articles/%d", id), &art); art.Country != "" {
		t.Fatalf("detail country: %q", art.Country)
	}
	var cs struct {
		Countries []Country
		Unknown   bool `json:"unknown_in_use"`
	}
	get(t, a, "/api/countries", &cs)
	inUse := []string{}
	for _, c := range cs.Countries {
		if c.InUse {
			inUse = append(inUse, c.Code)
		}
	}
	if !cs.Unknown || strings.Join(inUse, ",") != "VN,GB" || len(cs.Countries) < 60 {
		t.Fatalf("countries: unknown=%v in use=%v n=%d", cs.Unknown, inUse, len(cs.Countries))
	}
}

// The administrator sets the country per website; every feed of it follows.
func TestPublisherCountryAdmin(t *testing.T) {
	a, ctx := migratedApp(t)
	c := adminFor(t, a)
	for body, want := range map[string]int{
		`{"country":"TH"}`: 200, `{"country":"th"}`: 400, `{"country":"XX"}`: 400, `{"country":""}`: 400,
		`{"country":1}`: 400, `{}`: 400, `not json`: 400,
	} {
		if w := c.do("PATCH", "/api/admin/publishers/tuoitre", body); w.Code != want {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	if w := c.do("PATCH", "/api/admin/publishers/nope", `{"country":"VN"}`); w.Code != 404 {
		t.Errorf("unknown publisher: %d", w.Code)
	}
	var src []PublicSource
	get(t, a, "/api/sources", &src)
	n := 0
	for _, s := range src {
		if s.Adapter == "tuoitre" {
			n++
			if s.Country != "TH" {
				t.Errorf("feed %d: %q", s.ID, s.Country)
			}
		}
	}
	if n < 3 {
		t.Fatalf("tuoitre feeds: %d", n)
	}
	if w := c.do("PATCH", "/api/admin/publishers/tuoitre", `{"country":null}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	var pubs []Publisher
	adminGet(t, a, "/api/admin/publishers", &pubs)
	got := map[string]string{}
	for _, p := range pubs {
		got[p.Adapter] = p.Country
	}
	if got["tuoitre"] != "" || got["vnexpress"] != "VN" || got["bbc"] != "GB" {
		t.Fatalf("publishers: %v", got)
	}
	// Feed edits cannot set a per-feed country.
	var id int64
	a.db.QueryRow(ctx, `SELECT id FROM sources WHERE adapter='bbc' LIMIT 1`).Scan(&id)
	c.do("PATCH", fmt.Sprintf("/api/admin/sources/%d", id), `{"country":"VN","name":"BBC News"}`)
	var country string
	a.db.QueryRow(ctx, `SELECT country FROM publishers WHERE adapter='bbc'`).Scan(&country)
	if country != "GB" {
		t.Fatal(country)
	}
}

// Running the new migrations again (e.g. after a restore that kept the
// files but not schema_migrations) changes nothing and duplicates nothing;
// an administrator's choice is kept.
func TestCountryMigrationRerunSafe(t *testing.T) {
	a, ctx := migratedApp(t)
	addSource(t, a, "BBC", "https://feeds.bbci.co.uk/news/t-x.xml", "khac")
	a.db.Exec(ctx, `UPDATE publishers SET country='TH' WHERE adapter='tuoitre'`)
	snapshot := func() string {
		var s string
		a.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM sources)||'/'||(SELECT count(*) FROM articles)||'/'||(SELECT count(*) FROM countries)||'/'||(SELECT string_agg(adapter||'='||coalesce(country,'-'),',' ORDER BY adapter) FROM publishers)`).Scan(&s)
		return s
	}
	before := snapshot()
	a.db.Exec(ctx, `DELETE FROM schema_migrations WHERE name IN ('012_countries.sql','013_admin_auth.sql')`)
	if e := migrate(ctx, a.db, "../../migrations"); e != nil {
		t.Fatal(e)
	}
	if after := snapshot(); after != before || !strings.Contains(after, "tuoitre=TH") || !strings.Contains(after, "bbc=GB") {
		t.Fatalf("%s -> %s", before, after)
	}
}
