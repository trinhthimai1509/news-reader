package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"personal.news/reader/internal/news"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

type Source struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Adapter     string     `json:"adapter"`
	FeedURL     string     `json:"feed_url"`
	Enabled     bool       `json:"enabled"`
	LastChecked *time.Time `json:"last_checked"`
	LastSuccess *time.Time `json:"last_success"`
	LastError   string     `json:"last_error"`
	Category    string     `json:"category"`
	ETag        string     `json:"-"`
	Modified    string     `json:"-"`
}
type Article struct {
	ID         int64        `json:"id"`
	SourceID   int64        `json:"source_id"`
	Source     string       `json:"source"`
	Adapter    string       `json:"adapter"`
	Title      string       `json:"title"`
	URL        string       `json:"url"`
	Summary    string       `json:"summary"`
	Paragraphs []string     `json:"paragraphs"`
	Status     string       `json:"content_status"`
	Published  time.Time    `json:"published_at"`
	Fetched    time.Time    `json:"fetched_at"`
	Categories []string     `json:"categories"`
	HasVideo   bool         `json:"has_video"`
	Authors    []string     `json:"authors"`
	Blocks     []news.Block `json:"blocks,omitempty"`
	LeadImage  *news.Image  `json:"lead_image,omitempty"`
	// Enriched: the stored content comes from the current extractor, so a
	// missing author or image really means the source page had none.
	Enriched bool `json:"enriched"`
}
type App struct {
	imports importGate
	kick    chan struct{} // import asks the worker for a round now
	db      *pgxpool.Pool
	token   string
	full    bool
	limits  limiter
	local   *localAccess // nil unless LOCAL_NO_AUTH=true passed validation
}

const (
	pageSize = 30
	// Worker budgets. Sites are polled in parallel; within a site, feeds are
	// read one after another (feedDelay apart, feedBudget each) and then at
	// most siteArticleLimit article pages are fetched, articleDelay apart,
	// within articleBudget. This keeps the load on each site low as feeds are
	// added, and one slow site cannot hold up the other.
	feedBudget       = 30 * time.Second
	feedDelay        = time.Second
	articleBudget    = 60 * time.Second
	siteArticleLimit = 12
	articleDelay     = 1500 * time.Millisecond
	// Enrichment backfill: re-read already stored full-text articles once
	// with the current extractor (authors, images). It only uses page slots
	// left over after new articles, at most siteEnrichLimit per site per round.
	siteEnrichLimit = 6
)

// limiter blocks a client address after repeated wrong tokens. It is sized for
// a single-user app; entries expire after the window.
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

const (
	authWindow   = 5 * time.Minute
	authFailures = 10
)

func (l *limiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) >= authFailures
}
func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil || len(l.hits) > 1000 {
		l.hits = map[string][]time.Time{}
	}
	l.hits[key] = append(l.recent(key, now), now)
}
func (l *limiter) recent(key string, now time.Time) []time.Time {
	out := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < authWindow {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = out
	return out
}

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	send(w, status, map[string]string{"error": msg})
}
func (a *App) auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a.local.allows(r) {
			h.ServeHTTP(w, r)
			return
		}
		client, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			client = r.RemoteAddr
		}
		now := time.Now()
		if a.limits.blocked(client, now) {
			w.Header().Set("Retry-After", strconv.Itoa(int(authWindow.Seconds())))
			fail(w, 429, "Nhập sai mã quá nhiều lần, thử lại sau 5 phút")
			return
		}
		v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(v), []byte(a.token)) != 1 {
			a.limits.fail(client, now)
			fail(w, 401, "Mã truy cập không đúng")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// articleCategories lists the categories of the non-deleted feeds that listed
// article a, in display order.
const articleCategories = `ARRAY(SELECT c.slug FROM categories c WHERE EXISTS(SELECT 1 FROM article_feeds af JOIN sources fs ON fs.id=af.source_id
 WHERE af.article_id=a.id AND NOT fs.deleted AND fs.category=c.slug) ORDER BY c.position)`

func (a *App) sources(ctx context.Context) ([]Source, error) {
	rows, e := a.db.Query(ctx, `SELECT id,name,adapter,feed_url,enabled,last_checked,last_success,last_error,category,etag,modified FROM sources WHERE NOT deleted ORDER BY adapter,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if e = rows.Scan(&s.ID, &s.Name, &s.Adapter, &s.FeedURL, &s.Enabled, &s.LastChecked, &s.LastSuccess, &s.LastError, &s.Category, &s.ETag, &s.Modified); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (a *App) routes() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/sources", func(w http.ResponseWriter, r *http.Request) {
		s, e := a.sources(r.Context())
		if e != nil {
			fail(w, 500, "Không đọc được nguồn")
			return
		}
		send(w, 200, s)
	})
	api.HandleFunc("PATCH /api/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
		// Name, category and enabled can change; the adapter and feed URL cannot.
		var b struct {
			Enabled  *bool   `json:"enabled"`
			Category *string `json:"category"`
			Name     *string `json:"name"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		if e != nil || json.NewDecoder(r.Body).Decode(&b) != nil || (b.Enabled == nil && b.Category == nil && b.Name == nil) || (b.Category != nil && !slugPattern.MatchString(*b.Category)) {
			fail(w, 400, "Dữ liệu không hợp lệ")
			return
		}
		if b.Name != nil {
			n := strings.TrimSpace(*b.Name)
			if n == "" || utf8.RuneCountInString(n) > 100 {
				fail(w, 400, "Tên nguồn cần từ 1 đến 100 ký tự")
				return
			}
			b.Name = &n
		}
		tag, e := a.db.Exec(r.Context(), `UPDATE sources SET enabled=coalesce($2,enabled),category=coalesce($3,category),name=coalesce($4,name) WHERE id=$1 AND NOT deleted`, id, b.Enabled, b.Category, b.Name)
		if isUnknownCategory(e) {
			fail(w, 400, "Chuyên mục không tồn tại")
			return
		}
		if e != nil {
			fail(w, 500, "Không cập nhật được nguồn")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 404, "Không tìm thấy nguồn")
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
	api.HandleFunc("DELETE /api/sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if e != nil {
			fail(w, 400, "ID không hợp lệ")
			return
		}
		tag, e := a.db.Exec(r.Context(), `UPDATE sources SET deleted=true,enabled=false WHERE id=$1 AND NOT deleted`, id)
		if e != nil {
			fail(w, 500, "Không xoá được nguồn")
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 404, "Không tìm thấy nguồn")
			return
		}
		send(w, 200, map[string]bool{"ok": true})
	})
	api.HandleFunc("GET /api/articles", func(w http.ResponseWriter, r *http.Request) {
		var source int64
		if v := r.URL.Query().Get("source"); v != "" {
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil || n < 1 {
				fail(w, 400, "Nguồn không hợp lệ")
				return
			}
			source = n
		}
		category := r.URL.Query().Get("category")
		if category != "" && !slugPattern.MatchString(category) {
			fail(w, 400, "Chuyên mục không hợp lệ")
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if utf8.RuneCountInString(q) > 200 {
			fail(w, 400, "Từ khoá quá dài")
			return
		}
		// Search is literal: % and _ typed by the user are not wildcards.
		like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		page := 1
		if v := r.URL.Query().Get("page"); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 10000 {
				fail(w, 400, "Trang không hợp lệ")
				return
			}
			page = n
		}
		// The cursor is read before the list, so it never covers an article
		// that the list did not include (the browser marks "seen" up to it).
		cursor, e := a.articleCursor(r.Context())
		if e != nil {
			fail(w, 500, "Không đọc được tin")
			return
		}
		// The source filter matches every feed that listed the article, not
		// only the feed that delivered it first.
		rows, e := a.db.Query(r.Context(), `SELECT a.id,a.source_id,s.name,s.adapter,a.title,a.url,a.summary,a.content_status,a.published_at,a.fetched_at,`+articleCategories+`,a.has_video
FROM articles a JOIN sources s ON s.id=a.source_id
WHERE NOT s.deleted AND a.id <= $6
 AND ($1=0 OR EXISTS(SELECT 1 FROM article_feeds sf WHERE sf.article_id=a.id AND sf.source_id=$1))
 AND ($2='' OR a.title ILIKE '%'||$2||'%')
 AND ($5='' OR `+fmt.Sprintf(visibleInCategory, "$5")+`)
ORDER BY a.published_at DESC,a.id DESC LIMIT $3 OFFSET $4`, source, like, pageSize+1, (page-1)*pageSize, category, cursor)
		if e != nil {
			fail(w, 500, "Không đọc được tin")
			return
		}
		defer rows.Close()
		out := []Article{}
		for rows.Next() {
			var b Article
			if rows.Scan(&b.ID, &b.SourceID, &b.Source, &b.Adapter, &b.Title, &b.URL, &b.Summary, &b.Status, &b.Published, &b.Fetched, &b.Categories, &b.HasVideo) != nil {
				fail(w, 500, "Không đọc được tin")
				return
			}
			out = append(out, b)
		}
		if rows.Err() != nil {
			fail(w, 500, "Không đọc được tin")
			return
		}
		more := len(out) > pageSize
		if more {
			out = out[:pageSize]
		}
		send(w, 200, map[string]any{"items": out, "page": page, "has_more": more, "cursor": cursor})
	})
	api.HandleFunc("GET /api/articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if e != nil {
			fail(w, 400, "ID không hợp lệ")
			return
		}
		var b Article
		var body, blocks, lead []byte
		e = a.db.QueryRow(r.Context(), `SELECT a.id,a.source_id,s.name,s.adapter,a.title,a.url,a.summary,a.paragraphs,a.content_status,a.published_at,a.fetched_at,`+articleCategories+`,a.authors,a.blocks,a.lead_image,a.extract_version>=$2,a.has_video FROM articles a JOIN sources s ON s.id=a.source_id WHERE a.id=$1 AND NOT s.deleted`, id, news.ExtractVersion).Scan(&b.ID, &b.SourceID, &b.Source, &b.Adapter, &b.Title, &b.URL, &b.Summary, &body, &b.Status, &b.Published, &b.Fetched, &b.Categories, &b.Authors, &blocks, &lead, &b.Enriched, &b.HasVideo)
		if errors.Is(e, pgx.ErrNoRows) {
			fail(w, 404, "Không tìm thấy bài")
			return
		}
		if e != nil {
			fail(w, 500, "Không đọc được bài")
			return
		}
		if json.Unmarshal(body, &b.Paragraphs) != nil {
			b.Paragraphs = nil
		}
		// Rows stored before blocks existed are served as paragraph blocks.
		if blocks == nil || json.Unmarshal(blocks, &b.Blocks) != nil || len(b.Blocks) == 0 {
			b.Blocks = nil
			for _, p := range b.Paragraphs {
				b.Blocks = append(b.Blocks, news.Block{Type: "p", Text: p})
			}
		}
		if lead != nil && json.Unmarshal(lead, &b.LeadImage) != nil {
			b.LeadImage = nil
		}
		if b.Authors == nil {
			b.Authors = []string{}
		}
		send(w, 200, b)
	})
	api.HandleFunc("GET /api/categories", a.handleCategories)
	api.HandleFunc("POST /api/sources/import", a.handleImport)
	api.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		var done, pending, failed int64
		e := a.db.QueryRow(r.Context(), `SELECT count(*) FILTER (WHERE extract_version>=$1), count(*) FILTER (WHERE extract_version<$1 AND enrich_attempts<3), count(*) FILTER (WHERE extract_version<$1 AND enrich_attempts>=3)
FROM articles a JOIN sources s ON s.id=a.source_id WHERE NOT s.deleted AND `+enrichable, news.ExtractVersion).Scan(&done, &pending, &failed)
		if e != nil {
			fail(w, 500, "Không đọc được trạng thái")
			return
		}
		send(w, 200, map[string]any{"enrich": map[string]int64{"done": done, "pending": pending, "failed": failed}})
	})
	mux := http.NewServeMux()
	mux.Handle("/api/", a.auth(api))
	// Public: tells the page whether this request needs the access code.
	// It never returns the token.
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		send(w, 200, map[string]bool{"auth_required": !a.local.allows(r)})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
		defer c()
		if a.db.Ping(ctx) != nil {
			fail(w, 503, "database unavailable")
			return
		}
		send(w, 200, map[string]string{"status": "ok"})
	})
	// Static files are revalidated on every load (cheap 304s) so an upgrade
	// never mixes a cached index.html with a new app.js.
	static := http.FileServer(http.Dir("web"))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' https://ichef.bbci.co.uk https://*.vnecdn.net https://cdn2.tuoitre.vn; media-src 'self' https://cdn2.tuoitre.vn; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func (a *App) poll(ctx context.Context) {
	// A session-scoped advisory lock avoids overlapping workers across replicas.
	conn, e := a.db.Acquire(ctx)
	if e != nil {
		log.Printf("poll: %v", e)
		return
	}
	defer conn.Release()
	var locked bool
	if conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(8729101)`).Scan(&locked) != nil || !locked {
		return
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e := conn.Exec(c, `SELECT pg_advisory_unlock(8729101)`); e != nil {
			conn.Conn().Close(c)
		}
	}()
	sources, e := a.sources(ctx)
	if e != nil {
		log.Printf("poll: %v", e)
		return
	}
	bySite := map[string][]Source{}
	for _, s := range sources {
		if s.Enabled {
			bySite[s.Adapter] = append(bySite[s.Adapter], s)
		}
	}
	var wg sync.WaitGroup
	for _, list := range bySite {
		wg.Add(1)
		go func(list []Source) {
			defer wg.Done()
			a.pollSite(ctx, list)
		}(list)
	}
	wg.Wait()
}

// pollSite reads the feeds of one site one after another, then fetches a
// bounded number of pending article pages for the whole site. Sites run in
// parallel; requests to the same site are spaced out.
func (a *App) pollSite(ctx context.Context, list []Source) {
	throttled := false
	for i, s := range list {
		if i > 0 && !sleep(ctx, feedDelay) {
			return
		}
		fetchCtx, cancel := context.WithTimeout(ctx, feedBudget)
		e := a.pollFeed(ctx, fetchCtx, s)
		cancel()
		if e != nil && ctx.Err() == nil {
			log.Printf("source %d (%s): %v", s.ID, s.Name, e)
			if _, err := a.db.Exec(ctx, `UPDATE sources SET last_checked=now(),last_error=$2 WHERE id=$1`, s.ID, trim(e.Error(), 500)); err != nil {
				log.Printf("source %d: %v", s.ID, err)
			}
		}
		if errors.Is(e, news.ErrThrottled) {
			// The site asked us to slow down: skip its remaining feeds and
			// article pages for this round.
			throttled = true
			break
		}
	}
	if !a.full || throttled || ctx.Err() != nil {
		return
	}
	ids := make([]int64, len(list))
	for i, s := range list {
		ids[i] = s.ID
	}
	fetchCtx, cancel := context.WithTimeout(ctx, articleBudget)
	defer cancel()
	if e := a.fetchArticles(ctx, fetchCtx, list[0].Adapter, ids); e != nil && ctx.Err() == nil {
		log.Printf("%s articles: %v", list[0].Adapter, e)
	}
}
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}
func trim(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// pollFeed reads one feed. Network calls use fetchCtx (bounded by feedBudget);
// database writes use ctx so a timed-out fetch is still recorded.
func (a *App) pollFeed(ctx, fetchCtx context.Context, s Source) error {
	fc := news.Client(s.Adapter, true)
	defer fc.CloseIdleConnections()
	b, h, unchanged, e := news.Fetch(fetchCtx, fc, s.FeedURL, s.Adapter, true, s.ETag, s.Modified)
	if e != nil {
		return e
	}
	if !unchanged {
		items, e := news.ParseFeed(b)
		if e != nil {
			return e
		}
		if e = a.ingest(ctx, s, items, time.Now().UTC()); e != nil {
			return e
		}
		if _, e = a.db.Exec(ctx, `UPDATE sources SET etag=$2,modified=$3 WHERE id=$1`, s.ID, h.Get("ETag"), h.Get("Last-Modified")); e != nil {
			return e
		}
	}
	_, e = a.db.Exec(ctx, `UPDATE sources SET last_checked=now(),last_success=now(),last_error='' WHERE id=$1`, s.ID)
	return e
}

// ingest stores the items of one feed in a single transaction.
//
// An URL is stored once. The feed that first delivers it owns it; if that
// owner was deleted, an active feed that still lists it takes it over. Every
// feed that lists the article is recorded in article_feeds, which gives the
// article the category of each of those feeds.
//
// The transaction holds an advisory lock while it allocates article ids, so
// ids become visible in increasing order. The "new articles" cursor (the
// highest id a browser has loaded) can then never skip an article that
// commits later.
func (a *App) ingest(ctx context.Context, s Source, items []news.Item, now time.Time) error {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(8729103)`); e != nil {
		return e
	}
	for _, item := range items {
		item.Title = news.PlainIn(item.Title, s.Adapter)
		if item.Title == "" || !news.Allowed(item.URL, s.Adapter, false) {
			continue
		}
		url := news.Canonical(item.URL, s.Adapter, false)
		var ident *string
		if k := news.ArticleKey(url, s.Adapter); k != "" {
			ident = &k
		}
		title := trim(item.Title, 500)
		var id int64
		var feedImage []byte
		if im := item.FeedImage(s.Adapter); im != nil {
			feedImage, _ = json.Marshal(im)
		}
		e = tx.QueryRow(ctx, `INSERT INTO articles(source_id,url,title,summary,published_at,lead_image,ident) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING RETURNING id`,
			s.ID, url, title, trim(news.PlainIn(item.Description, s.Adapter), 2000), news.PublishedIn(item.Date, s.Adapter, now), feedImage, ident).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			e = a.existingArticle(ctx, tx, s, url, title, ident, &id)
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO article_feeds(article_id,source_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, s.ID); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}

// existingArticle finds the stored article for a feed item that was not
// inserted: same stable ident (VnExpress article id), same URL, or a slug it
// was stored under before. A deleted owner hands the article to this feed.
// When the item carries a new slug of an identified article, the link and
// title follow it and the previous URL goes to former_urls; an old slug that
// a lagging feed still lists changes nothing. No new id is allocated, so the
// article never shows up as new again.
func (a *App) existingArticle(ctx context.Context, tx pgx.Tx, s Source, url, title string, ident *string, id *int64) error {
	var ownerDeleted, known bool
	var storedURL string
	e := tx.QueryRow(ctx, `SELECT a.id,o.deleted,a.url,$2=ANY(a.former_urls) FROM articles a JOIN sources o ON o.id=a.source_id
WHERE a.ident=$1 OR a.url=$2 OR $2=ANY(a.former_urls) ORDER BY (a.ident IS NOT DISTINCT FROM $1) DESC, a.id LIMIT 1`, ident, url).Scan(id, &ownerDeleted, &storedURL, &known)
	if e != nil {
		return e
	}
	if ownerDeleted {
		if _, e = tx.Exec(ctx, `UPDATE articles SET source_id=$2 WHERE id=$1`, *id, s.ID); e != nil {
			return e
		}
	}
	if ident == nil {
		return nil
	}
	if _, e = tx.Exec(ctx, `UPDATE articles SET ident=$2 WHERE id=$1 AND ident IS NULL AND NOT EXISTS(SELECT 1 FROM articles WHERE ident=$2)`, *id, *ident); e != nil {
		return e
	}
	if storedURL != url && !known {
		_, e = tx.Exec(ctx, `UPDATE articles SET former_urls=array_append(former_urls,url),url=$2,title=$3 WHERE id=$1 AND ident=$4 AND NOT EXISTS(SELECT 1 FROM articles WHERE url=$2)`, *id, url, title, *ident)
	}
	return e
}

// fetchArticles fetches article pages of one site, spaced by articleDelay:
// first up to siteArticleLimit articles still waiting for full text (new
// articles come first), then, with the slots left, up to siteEnrichLimit
// stored full-text articles that were extracted by an older extractor.
func (a *App) fetchArticles(ctx, fetchCtx context.Context, adapter string, sourceIDs []int64) error {
	type job struct {
		id     int64
		url    string
		enrich bool
	}
	query := func(sql string, limit int, enrich bool) ([]job, error) {
		if limit <= 0 {
			return nil, nil
		}
		args := []any{sourceIDs, limit}
		if enrich {
			args = append(args, news.ExtractVersion)
		}
		rows, e := a.db.Query(ctx, sql, args...)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		out := []job{}
		for rows.Next() {
			j := job{enrich: enrich}
			if e = rows.Scan(&j.id, &j.url); e != nil {
				return nil, e
			}
			out = append(out, j)
		}
		return out, rows.Err()
	}
	todo, e := query(`SELECT id,url FROM articles WHERE source_id=ANY($1) AND content_status<>'full' AND attempts<3 AND next_attempt<=now() ORDER BY published_at DESC LIMIT $2`, siteArticleLimit, false)
	if e != nil {
		return e
	}
	more, e := query(`SELECT id,url FROM articles WHERE source_id=ANY($1) AND `+enrichable+` AND extract_version<$3 AND enrich_attempts<3 AND enrich_next<=now() ORDER BY published_at DESC LIMIT $2`, min(siteEnrichLimit, siteArticleLimit-len(todo)), true)
	if e != nil {
		return e
	}
	todo = append(todo, more...)
	ac := news.Client(adapter, false)
	defer ac.CloseIdleConnections()
	for i, j := range todo {
		if i > 0 {
			sleep(fetchCtx, articleDelay)
		}
		if fetchCtx.Err() != nil {
			// Out of budget: leave the rest for the next round without
			// counting it as a failed attempt.
			return nil
		}
		b, _, _, e := news.Fetch(fetchCtx, ac, j.url, adapter, false, "", "")
		if e != nil && fetchCtx.Err() != nil {
			return nil
		}
		if errors.Is(e, news.ErrThrottled) {
			return e
		}
		var c news.Content
		fetchErr := e
		if e == nil {
			c, e = news.ExtractArticle(b, adapter, j.url)
		}
		if j.enrich {
			if err := a.storeEnrichment(ctx, j.id, c, fetchErr, e); err != nil {
				return err
			}
			continue
		}
		if err := a.storeContent(ctx, j.id, c, e); err != nil {
			return err
		}
	}
	return nil
}

// enrichable rows are re-read by the backfill: full-text articles, and pages
// without full text whose own retries are over (video pages, for videos).
const enrichable = `(content_status='full' OR (content_status='unavailable' AND attempts>=3))`

func marshalContent(c news.Content) (paragraphs, blocks, lead []byte) {
	paragraphs, _ = json.Marshal(c.Paragraphs())
	blocks, _ = json.Marshal(c.Blocks)
	if c.Lead != nil {
		lead, _ = json.Marshal(c.Lead)
	}
	return
}

// storeContent records the first full-text fetch of an article. On failure the
// RSS summary stays; authors and lead image found on the page are still kept.
func (a *App) storeContent(ctx context.Context, id int64, c news.Content, fetchErr error) error {
	paragraphs, blocks, lead := marshalContent(c)
	if c.Authors == nil {
		c.Authors = []string{}
	}
	if fetchErr != nil && c.HasVideo() {
		// A video page (or a page that is mostly video): keep the RSS summary,
		// store only its video blocks, and do not retry; the page will not
		// gain text by being read again.
		videos, _ := json.Marshal(c.VideoBlocks())
		_, e := a.db.Exec(ctx, `UPDATE articles SET content_status='unavailable',content_error=$2,attempts=3,blocks=$3,has_video=true,extract_version=$6,
 authors=CASE WHEN cardinality($4::text[])>0 THEN $4 ELSE authors END, lead_image=coalesce($5,lead_image) WHERE id=$1`, id, trim(fetchErr.Error(), 500), videos, c.Authors, lead, news.ExtractVersion)
		return e
	}
	if fetchErr != nil {
		// Back off 30 min, then 60 min; after 3 attempts the summary stays.
		_, e := a.db.Exec(ctx, `UPDATE articles SET content_status='unavailable',content_error=$2,attempts=attempts+1,next_attempt=now()+(attempts+1)*interval '30 minutes',
 authors=CASE WHEN cardinality($3::text[])>0 THEN $3 ELSE authors END, lead_image=coalesce($4,lead_image) WHERE id=$1`, id, trim(fetchErr.Error(), 500), c.Authors, lead)
		return e
	}
	_, e := a.db.Exec(ctx, `UPDATE articles SET paragraphs=$2,blocks=$3,authors=$4,lead_image=$5,content_status='full',content_error='',attempts=attempts+1,extract_version=$6,has_video=$7 WHERE id=$1`,
		id, paragraphs, blocks, c.Authors, lead, news.ExtractVersion, c.HasVideo())
	return e
}

// storeEnrichment re-reads a stored article with the current extractor.
//   - Full text: only a successful extraction replaces the stored content; any
//     failure (page removed, layout changed, network) leaves it untouched and
//     is retried later (1 h, then 2 h).
//   - No full text (summary kept): the page may only add video blocks; the
//     summary, status and text are never changed.
func (a *App) storeEnrichment(ctx context.Context, id int64, c news.Content, fetchErr, extractErr error) error {
	failed := func(err error) error {
		_, e := a.db.Exec(ctx, `UPDATE articles SET enrich_attempts=enrich_attempts+1,enrich_next=now()+(enrich_attempts+1)*interval '1 hour',enrich_error=$2 WHERE id=$1`, id, trim(err.Error(), 500))
		return e
	}
	if fetchErr != nil {
		return failed(fetchErr)
	}
	var status string
	if e := a.db.QueryRow(ctx, `SELECT content_status FROM articles WHERE id=$1`, id).Scan(&status); e != nil {
		return e
	}
	if status != "full" {
		if !c.HasVideo() {
			_, e := a.db.Exec(ctx, `UPDATE articles SET extract_version=$2,enrich_error='' WHERE id=$1 AND content_status<>'full'`, id, news.ExtractVersion)
			return e
		}
		videos, _ := json.Marshal(c.VideoBlocks())
		_, e := a.db.Exec(ctx, `UPDATE articles SET blocks=$2,has_video=true,extract_version=$3,enrich_error='' WHERE id=$1 AND content_status<>'full'`, id, videos, news.ExtractVersion)
		return e
	}
	if extractErr != nil {
		return failed(extractErr)
	}
	paragraphs, blocks, lead := marshalContent(c)
	if c.Authors == nil {
		c.Authors = []string{}
	}
	_, e := a.db.Exec(ctx, `UPDATE articles SET paragraphs=$2,blocks=$3,authors=$4,lead_image=$5,extract_version=$6,enrich_error='',has_video=$7 WHERE id=$1 AND content_status='full'`,
		id, paragraphs, blocks, c.Authors, lead, news.ExtractVersion, c.HasVideo())
	return e
}

// migrate applies migrations/*.sql in name order, once each, under a
// transaction-scoped advisory lock so concurrent starts are serialised.
func migrate(ctx context.Context, db *pgxpool.Pool, dir string) error {
	files, e := filepath.Glob(filepath.Join(dir, "*.sql"))
	if e != nil {
		return e
	}
	if len(files) == 0 {
		return errors.New("no migrations found in " + dir)
	}
	sort.Strings(files)
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(8729102)`); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); e != nil {
		return e
	}
	for _, f := range files {
		name := filepath.Base(f)
		var done bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&done); e != nil {
			return e
		}
		if done {
			continue
		}
		sql, e := os.ReadFile(f)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(sql)); e != nil {
			return errors.New(name + ": " + e.Error())
		}
		if _, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); e != nil {
			return e
		}
		log.Printf("migration applied: %s", name)
	}
	return tx.Commit(ctx)
}
func main() {
	token := os.Getenv("ADMIN_TOKEN")
	if len(token) < 32 {
		log.Fatal("ADMIN_TOKEN must contain at least 32 characters")
	}
	interval := 2 * time.Minute
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		d, e := time.ParseDuration(v)
		if e != nil || d < time.Minute {
			log.Fatal("POLL_INTERVAL must be >= 1m")
		}
		interval = d
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, e := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal("DATABASE_URL is invalid")
	}
	cfg.MaxConns = 10
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	if e = migrate(ctx, db, "migrations"); e != nil {
		log.Fatal("migration: ", e)
	}
	rep, e := mergeDuplicates(ctx, db)
	if e != nil {
		log.Fatal("merge duplicates: ", e)
	}
	if rep.Groups > 0 {
		log.Printf("merge duplicates: %d groups found, %d merged, %d rows removed, %d kept apart %v", rep.Groups, rep.Merged, rep.Removed, len(rep.Skipped), rep.Skipped)
	}
	app := &App{db: db, token: token, full: os.Getenv("FULL_TEXT_ENABLED") != "false", kick: make(chan struct{}, 1)}
	switch os.Getenv("LOCAL_NO_AUTH") {
	case "", "false":
	case "true":
		if app.local, e = newLocalAccess(addr, os.Getenv("PUBLISHED_HOST"), "/proc/net/route"); e != nil {
			log.Fatal(e)
		}
		log.Printf("LOCAL_NO_AUTH: requests from %s with a localhost Host header need no access code. Do not put this behind a reverse proxy.", app.local)
	default:
		log.Fatal("LOCAL_NO_AUTH must be true or false")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.poll(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				app.poll(ctx)
			case <-app.kick:
				app.poll(ctx)
			}
		}
	}()
	srv := &http.Server{Addr: addr, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		log.Printf("Personal News Reader listening on %s", addr)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Print(e)
			stop()
		}
	}()
	<-ctx.Done()
	log.Print("shutting down")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
	<-done
}
