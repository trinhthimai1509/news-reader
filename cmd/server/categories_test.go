package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"personal.news/reader/internal/news"
)

// migrateUpTo applies the migrations whose file names sort at or before last.
func migrateUpTo(t *testing.T, a *App, last string) {
	dir := t.TempDir()
	files, _ := filepath.Glob("../../migrations/*.sql")
	for _, f := range files {
		if filepath.Base(f) > last {
			continue
		}
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	if e := migrate(context.Background(), a.db, dir); e != nil {
		t.Fatal(e)
	}
}

const categoriesV2 = "thoi-su:Thời sự,the-gioi:Thế giới,kinh-te:Kinh tế,khoa-hoc-cong-nghe:Khoa học công nghệ,chinh-tri:Chính trị,quan-su:Quân sự,doi-song:Đời sống,khac:Khác"

// Migration 015 on data written with the old list: renamed categories keep
// their feeds, dropped ones move to Khác, articles and feed links are
// untouched, an article from several merged categories is listed once in
// Khác, the new categories are empty, and running it again changes nothing.
func TestCategoryV2Migration(t *testing.T) {
	db := testDB(t)
	a := newTestApp(db)
	ctx := context.Background()
	migrateUpTo(t, a, "014_visit_stats.sql")
	src := func(name, url, cat string, deleted bool) Source {
		s := addSource(t, a, name, url, cat)
		if deleted {
			a.db.Exec(ctx, `UPDATE sources SET deleted=true,enabled=false WHERE id=$1`, s.ID)
		}
		return s
	}
	biz := src("Biz", "https://feeds.bbci.co.uk/news/t-biz.xml", "kinh-doanh", false)
	tech := src("Tech", "https://feeds.bbci.co.uk/news/t-tech.xml", "cong-nghe", false)
	sport := src("Sport", "https://feeds.bbci.co.uk/news/t-sport.xml", "the-thao", false)
	fun := src("Fun", "https://feeds.bbci.co.uk/news/t-fun.xml", "giai-tri", false)
	front := src("Front", "https://feeds.bbci.co.uk/news/t-front.xml", "khac", false)
	src("Old health", "https://feeds.bbci.co.uk/news/t-health.xml", "suc-khoe", true)
	now := time.Now()
	a.ingest(ctx, biz, []news.Item{item("b", "Biz story", now)}, now)
	a.ingest(ctx, tech, []news.Item{item("t", "Tech story", now)}, now)
	// One story in Thể thao, Giải trí and Khác at once.
	for _, s := range []Source{sport, fun, front} {
		a.ingest(ctx, s, []news.Item{item("m", "Merged story", now)}, now)
	}
	a.ingest(ctx, sport, []news.Item{item("s", "Sport only", now)}, now)
	snapshot := func() string {
		var v string
		a.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM articles)||'/'||(SELECT count(*) FROM article_feeds)||'/'||(SELECT count(*) FROM sources)||'/'||(SELECT count(*) FROM sources WHERE deleted)||'/'||(SELECT string_agg(id||'='||url,',' ORDER BY id) FROM articles)`).Scan(&v)
		return v
	}
	before := snapshot()

	migrateUpTo(t, a, "999")
	if after := snapshot(); after != before {
		t.Fatalf("data changed: %s -> %s", before, after)
	}
	var cats string
	a.db.QueryRow(ctx, `SELECT string_agg(slug||':'||name,',' ORDER BY position) FROM categories`).Scan(&cats)
	if cats != categoriesV2 {
		t.Fatalf("categories: %s", cats)
	}
	var moved string
	a.db.QueryRow(ctx, `SELECT string_agg(name||'='||category,',' ORDER BY name) FROM sources WHERE feed_url LIKE '%/t-%'`).Scan(&moved)
	if moved != "Biz=kinh-te,Front=khac,Fun=khac,Old health=khac,Sport=khac,Tech=khoa-hoc-cong-nghe" {
		t.Fatalf("feeds: %s", moved)
	}
	var l listResp
	get(t, a, "/api/articles?category=khac", &l)
	titles := []string{}
	for _, it := range l.Items {
		titles = append(titles, it.Title)
		if it.Title == "Merged story" && fmt.Sprint(it.Categories) != "[khac]" {
			t.Fatalf("merged categories: %v", it.Categories)
		}
	}
	if strings.Join(titles, ",") != "Merged story,Sport only" && strings.Join(titles, ",") != "Sport only,Merged story" {
		t.Fatalf("khac: %v", titles)
	}
	for q, want := range map[string]int{"kinh-te": 1, "khoa-hoc-cong-nghe": 1, "chinh-tri": 0, "quan-su": 0} {
		if get(t, a, "/api/articles?category="+q, &l); len(l.Items) != want {
			t.Errorf("%s: %d", q, len(l.Items))
		}
	}
	var c catResp
	get(t, a, "/api/categories", &c)
	if len(c.Categories) != 8 || c.Categories[4].Slug != "chinh-tri" || c.Categories[5].Slug != "quan-su" {
		t.Fatalf("api categories: %+v", c.Categories)
	}
	// Red dot counts in Khác: the merged story counts once.
	if n := newCounts(t, a, "khac:0"); n["khac"] != 2 {
		t.Fatalf("khac new count: %d", n["khac"])
	}
	// Old slugs are refused everywhere.
	for _, old := range []string{"kinh-doanh", "cong-nghe", "giai-tri", "the-thao", "suc-khoe"} {
		if code := get(t, a, "/api/articles?category="+old, nil); code != 400 {
			t.Errorf("list %s: %d", old, code)
		}
		if w := adminFor(t, a).do("PATCH", fmt.Sprintf("/api/admin/sources/%d", biz.ID), `{"category":"`+old+`"}`); w.Code != 400 {
			t.Errorf("patch %s: %d", old, w.Code)
		}
	}
	// Running 015 again changes nothing.
	a.db.Exec(ctx, `DELETE FROM schema_migrations WHERE name='015_categories_v2.sql'`)
	migrateUpTo(t, a, "999")
	var cats2, moved2 string
	a.db.QueryRow(ctx, `SELECT string_agg(slug||':'||name,',' ORDER BY position) FROM categories`).Scan(&cats2)
	a.db.QueryRow(ctx, `SELECT string_agg(name||'='||category,',' ORDER BY name) FROM sources WHERE feed_url LIKE '%/t-%'`).Scan(&moved2)
	if cats2 != cats || moved2 != moved || snapshot() != before {
		t.Fatal("re-run changed data")
	}
}

// Every catalog feed maps to a current category, so importing again never
// brings an old category back.
func TestCatalogUsesCurrentCategories(t *testing.T) {
	a, ctx := migratedApp(t)
	for _, f := range news.Catalog {
		var ok bool
		a.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM categories WHERE slug=$1)`, f.Category).Scan(&ok)
		if !ok {
			t.Errorf("%s: %s", f.FeedURL, f.Category)
		}
	}
	var stale int
	a.db.QueryRow(ctx, `SELECT count(*) FROM sources WHERE category NOT IN (SELECT slug FROM categories)`).Scan(&stale)
	if stale != 0 {
		t.Fatal("seeded feed with unknown category")
	}
}
