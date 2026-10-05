package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"personal.news/reader/internal/news"
)

const (
	importMaxFeeds = 20               // feeds checked in one import
	importBudget   = 90 * time.Second // whole import
	importPerMin   = 6                // imports accepted per minute
)

type importFeed struct {
	ID       int64  `json:"id,omitempty"`
	Name     string `json:"name"`
	FeedURL  string `json:"feed_url"`
	Category string `json:"category,omitempty"`
	Note     string `json:"note,omitempty"`
	Error    string `json:"error,omitempty"`
}

type importResult struct {
	Status   string       `json:"status"` // ok | partial | exists | failed | rejected
	Kind     string       `json:"kind"`
	Site     string       `json:"site,omitempty"`
	Message  string       `json:"message"`
	Added    []importFeed `json:"added"`
	Restored []importFeed `json:"restored"`
	Existing []importFeed `json:"existing"`
	Failed   []importFeed `json:"failed"`
}

// importGate allows one import at a time and a few per minute: an import
// makes outbound requests, so it is bounded like the worker.
type importGate struct {
	mu     sync.Mutex
	busy   bool
	recent []time.Time
}

func (g *importGate) enter(now time.Time) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy {
		return false, "Đang có một lượt import khác, hãy đợi nó xong."
	}
	keep := g.recent[:0]
	for _, t := range g.recent {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	g.recent = keep
	if len(g.recent) >= importPerMin {
		return false, "Import quá nhiều lần trong một phút, hãy thử lại sau."
	}
	g.recent = append(g.recent, now)
	g.busy = true
	return true, ""
}
func (g *importGate) leave() { g.mu.Lock(); g.busy = false; g.mu.Unlock() }

// checkFeed reads the feed once through the same guarded client as the
// worker (allowlist, public IPs only, redirects inside the allowlist,
// timeouts, size limit) and returns its channel title.
var checkFeed = func(ctx context.Context, adapter, feedURL string) (string, error) {
	c := news.Client(adapter, true)
	defer c.CloseIdleConnections()
	b, _, _, e := news.Fetch(ctx, c, feedURL, adapter, true, "", "")
	if e != nil {
		return "", e
	}
	items, e := news.ParseFeed(b)
	if e != nil {
		return "", e
	}
	if len(items) == 0 {
		return "", errors.New("RSS không có bài")
	}
	return news.FeedTitle(b, adapter), nil
}

func (a *App) handleImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		fail(w, 400, "Dữ liệu không hợp lệ")
		return
	}
	res := news.ResolveImport(in.URL)
	out := importResult{Kind: res.Kind, Site: res.Site, Added: []importFeed{}, Restored: []importFeed{}, Existing: []importFeed{}, Failed: []importFeed{}}
	if len(res.Feeds) == 0 {
		// Recognised from the URL alone; nothing was requested.
		out.Status, out.Message = "rejected", res.Message
		send(w, 422, out)
		return
	}
	ok, msg := a.imports.enter(time.Now())
	if !ok {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, msg)
		return
	}
	defer a.imports.leave()
	feeds := res.Feeds
	if len(feeds) > importMaxFeeds {
		feeds = feeds[:importMaxFeeds]
	}
	ctx, cancel := context.WithTimeout(r.Context(), importBudget)
	defer cancel()
	fetched := 0
	for _, f := range feeds {
		item, state, e := a.importOne(ctx, f, &fetched)
		switch {
		case e != nil:
			item.Error = trim(e.Error(), 300)
			out.Failed = append(out.Failed, item)
		case state == "added":
			out.Added = append(out.Added, item)
		case state == "restored":
			out.Restored = append(out.Restored, item)
		default:
			out.Existing = append(out.Existing, item)
		}
	}
	changed := len(out.Added) + len(out.Restored)
	for _, x := range out.Existing {
		if x.Note != "" {
			changed++ // re-enabled
		}
	}
	if changed > 0 {
		a.kickWorker()
	}
	n, bad, added := len(feeds), len(out.Failed), len(out.Added)+len(out.Restored)
	const auto = " Tin mới sẽ được cập nhật tự động."
	switch {
	case bad == n && n == 1:
		out.Status, out.Message = "failed", "Không thêm được nguồn: "+out.Failed[0].Error+"."
	case bad == n:
		out.Status, out.Message = "failed", fmt.Sprintf("Không thêm được nguồn: cả %d kênh đều lỗi.", n)
	case bad > 0:
		out.Status, out.Message = "partial", fmt.Sprintf("Đã thêm %d kênh, %d kênh lỗi.", added, bad)
		if added > 0 {
			out.Message += auto
		}
	case added == 0:
		out.Status, out.Message = "exists", "Nguồn này đã có trong danh sách."
	case n == 1:
		name := append(out.Added, out.Restored...)[0].Name
		out.Status, out.Message = "ok", "Đã thêm nguồn: "+name+"."+auto
	default:
		out.Status, out.Message = "ok", fmt.Sprintf("Đã thêm %d kênh.", added)+auto
	}
	if len(res.Feeds) > importMaxFeeds {
		out.Message += fmt.Sprintf(" Chỉ xử lý %d feed đầu tiên.", importMaxFeeds)
	}
	send(w, 200, out)
}

// importOne adds one feed, or reports it as already present. Existing rows
// keep the name and category the user set; a deleted row is restored (the
// usual rule: its articles come back) and a disabled one is switched on.
// A new or restored feed must be read and parsed before it is stored.
func (a *App) importOne(ctx context.Context, f news.CatalogFeed, fetched *int) (importFeed, string, error) {
	item := importFeed{FeedURL: f.FeedURL, Name: f.Name, Category: f.Category}
	var id int64
	var name, category string
	var enabled, deleted bool
	e := a.db.QueryRow(ctx, `SELECT id,name,category,enabled,deleted FROM sources WHERE feed_url=$1`, f.FeedURL).Scan(&id, &name, &category, &enabled, &deleted)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return item, "", errors.New("không đọc được cấu hình nguồn")
	}
	found := e == nil
	if found {
		item.Name = name
	}
	if item.Name == "" {
		item.Name = f.FeedURL // shown until the feed is read
	}
	if found && !deleted {
		item.ID, item.Name, item.Category = id, name, category
		if !enabled {
			if _, e = a.db.Exec(ctx, `UPDATE sources SET enabled=true WHERE id=$1`, id); e != nil {
				return item, "", errors.New("không bật lại được nguồn")
			}
			item.Note = "đã bật lại"
		}
		return item, "existing", nil
	}
	// Space requests to the same site like the worker does.
	if *fetched > 0 && !sleep(ctx, feedDelay) {
		return item, "", ctx.Err()
	}
	*fetched++
	fctx, cancel := context.WithTimeout(ctx, feedBudget)
	title, err := checkFeed(fctx, f.Adapter, f.FeedURL)
	cancel()
	if err != nil {
		return item, "", fmt.Errorf("không đọc được RSS: %w", err)
	}
	if found {
		if _, e = a.db.Exec(ctx, `UPDATE sources SET deleted=false,enabled=true WHERE id=$1`, id); e != nil {
			return item, "", errors.New("không khôi phục được nguồn")
		}
		item.ID, item.Name, item.Category, item.Note = id, name, category, "đã khôi phục cùng các bài trước đây"
		return item, "restored", nil
	}
	if f.Name == "" {
		item.Name = importName(f, title)
	}
	if item.Category == "" {
		item.Category = "khac" // unknown section: editable on the source card
	}
	e = a.db.QueryRow(ctx, `INSERT INTO sources(name,adapter,feed_url,category) VALUES($1,$2,$3,$4) ON CONFLICT(feed_url) DO NOTHING RETURNING id`, item.Name, f.Adapter, f.FeedURL, item.Category).Scan(&item.ID)
	if errors.Is(e, pgx.ErrNoRows) {
		// Added meanwhile by another request: report the stored row.
		if a.db.QueryRow(ctx, `SELECT id,name,category FROM sources WHERE feed_url=$1`, f.FeedURL).Scan(&item.ID, &item.Name, &item.Category) == nil {
			return item, "existing", nil
		}
	}
	if e != nil {
		return item, "", errors.New("không lưu được nguồn")
	}
	return item, "added", nil
}

// importName names a feed outside the catalog from its channel title (plain
// text, shown with textContent), with the BBC section since every BBC feed is
// titled "BBC News".
func importName(f news.CatalogFeed, title string) string {
	name := strings.TrimSpace(title)
	switch f.Adapter {
	case "tuoitre": // "TUOI TRE ONLINE - Giáo dục - RSS Feed"
		if s := strings.TrimSuffix(strings.TrimPrefix(name, "TUOI TRE ONLINE - "), " - RSS Feed"); s != name {
			name = "Tuổi Trẻ " + s
		}
	case "vnexpress": // "Du lịch - VnExpress RSS"
		if s := strings.TrimSuffix(name, " - VnExpress RSS"); s != name {
			name = "VnExpress " + s
		}
	}
	if f.Adapter == "bbc" {
		sec := strings.TrimSuffix(strings.TrimPrefix(f.FeedURL, "https://feeds.bbci.co.uk/news/"), "rss.xml")
		if sec = strings.Trim(sec, "/"); sec != "" {
			name = "BBC " + strings.ReplaceAll(sec, "_", " ")
		}
	}
	if name == "" {
		name = f.FeedURL
	}
	return trim(name, 100)
}

// kickWorker asks the worker for a round now. The channel holds one request,
// so repeated imports never queue more than one extra round, and the round
// itself keeps all the usual limits (advisory lock, per-site pacing, 429).
func (a *App) kickWorker() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}
