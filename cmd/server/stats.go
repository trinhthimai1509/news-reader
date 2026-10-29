package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Public reading statistics (migrations 014, 016):
//   - "Lượt đọc" of each article: the page reports a "read" once the article
//     opened and its content loaded. One read per article per browser stats
//     session; opening it again, reloading or another tab of the same browser
//     add nothing. A session idle for sessionIdle (server time) is over: its
//     reads no longer deduplicate, so a read in the new session counts.
//   - "Đang hoạt động": stats sessions with an event within activeWindow.
//     Expired sessions fall out of the count; no "tab closed" event is needed.
//
// Reads are served with the articles (field "reads"); GET /api/stats returns
// only the active count. POST /api/stats/events is the only public write:
// same-origin JSON, a small fixed payload, per-address and global rate
// limits. It never reads the admin cookie and grants nothing; the admin API
// keeps its own session and CSRF checks. The client address is used in memory
// for rate limiting only; the stats tables hold no IP, user agent, username,
// and no reading history beyond "this session already counted article N",
// which is dropped once the session is over.

const (
	activeWindow     = 5 * time.Minute
	sessionIdle      = 30 * time.Minute // a stats session ends after this long without events
	sessionTouchMin  = 30 * time.Second // heartbeats move last_seen at most this often
	sessionRetention = time.Hour
	statsBodyMax     = 256
	statsAddrMax     = 120 // stats writes per minute per client address
	statsReadAddrMax = 60  // reads per minute per client address
	statsNewRowsMax  = 600 // reads + new sessions per minute, all clients
	statsKeysMax     = 10000
	cleanupBatch     = 5000
	cleanupBatches   = 20 // per table per run: at most 100k rows
	statsCleanEvery  = 10 * time.Minute
	statsLockKey     = 8729104 // advisory lock namespace: one stats session
)

var statsID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// rateWindow counts hits per key in fixed one-minute windows. When the map
// holds statsKeysMax keys, unseen keys are refused, so memory stays bounded.
// The zero value is ready to use.
type rateWindow struct {
	mu     sync.Mutex
	start  time.Time
	counts map[string]int
}

func (w *rateWindow) allow(key string, max int, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.counts == nil || now.Sub(w.start) >= time.Minute || now.Before(w.start) {
		w.start, w.counts = now, map[string]int{}
	}
	n, ok := w.counts[key]
	if (!ok && len(w.counts) >= statsKeysMax) || n >= max {
		return false
	}
	w.counts[key] = n + 1
	return true
}

type statsGate struct {
	writes, reads, rows rateWindow
}

type statsEvent struct {
	Type    string `json:"type"` // "heartbeat" or "read"
	Session string `json:"session_id"`
	Article *int64 `json:"article_id,omitempty"`
}

var (
	errStatsBusy   = errors.New("stats: new-row budget used up")
	errNotReadable = errors.New("stats: article not readable")
)

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	var active int64
	if e := a.db.QueryRow(r.Context(), `SELECT count(*) FROM visit_sessions WHERE last_seen>now()-$1*interval '1 second'`, int64(activeWindow.Seconds())).Scan(&active); e != nil {
		fail(w, 500, "Không đọc được thống kê")
		return
	}
	send(w, 200, map[string]any{"active": active, "active_window_seconds": int(activeWindow.Seconds())})
}

func (a *App) handleStatsEvent(w http.ResponseWriter, r *http.Request) {
	// A page always sends Origin on POST; requiring it (and JSON, which a
	// cross-site form cannot send without a CORS preflight we never allow)
	// keeps other sites from posting. Scripts outside a browser can still
	// forge these headers: see the rate limits below.
	if r.Header.Get("Origin") == "" || !sameOrigin(r) {
		fail(w, 403, "Yêu cầu không hợp lệ")
		return
	}
	if !isJSON(r) {
		fail(w, 415, "Yêu cầu cần dữ liệu JSON")
		return
	}
	client, now := clientAddr(r), time.Now()
	if !a.stats.writes.allow(client, statsAddrMax, now) {
		tooMany(w)
		return
	}
	var in statsEvent
	r.Body = http.MaxBytesReader(w, r.Body, statsBodyMax)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || !statsID.MatchString(in.Session) ||
		!((in.Type == "read" && in.Article != nil && *in.Article > 0) || (in.Type == "heartbeat" && in.Article == nil)) {
		fail(w, 400, "Dữ liệu không hợp lệ")
		return
	}
	session := tokenHash(in.Session)
	if in.Type == "heartbeat" {
		e := a.touchSession(r.Context(), session, false, now)
		if errors.Is(e, errStatsBusy) {
			tooMany(w)
			return
		}
		if e != nil {
			fail(w, 500, "Không ghi được thống kê")
			return
		}
		send(w, 200, map[string]bool{"ok": true})
		return
	}
	if !a.stats.reads.allow(client, statsReadAddrMax, now) || !a.stats.rows.allow("", statsNewRowsMax, now) {
		tooMany(w)
		return
	}
	reads, e := a.countRead(r.Context(), session, *in.Article, now)
	switch {
	case errors.Is(e, errNotReadable):
		fail(w, 404, "Không tìm thấy bài")
	case errors.Is(e, errStatsBusy):
		tooMany(w)
	case e != nil:
		fail(w, 500, "Không ghi được thống kê")
	default:
		send(w, 200, map[string]any{"ok": true, "reads": reads})
	}
}

func tooMany(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	fail(w, 429, "Quá nhiều yêu cầu thống kê, hãy thử lại sau")
}

// beginSession starts a transaction holding the lock of one stats session
// (events of a session are serialised, also when its row does not exist
// yet) and marks the session active now. A session idle for sessionIdle is
// over: the reads it counted stop deduplicating. Unless force, a session seen
// within sessionTouchMin is not written again.
func (a *App) beginSession(ctx context.Context, hash []byte, force bool, now time.Time) (pgx.Tx, error) {
	tx, e := a.db.Begin(ctx)
	if e != nil {
		return nil, e
	}
	abort := func(e error) (pgx.Tx, error) { tx.Rollback(ctx); return nil, e }
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1,hashtext(encode($2,'hex')))`, statsLockKey, hash); e != nil {
		return abort(e)
	}
	var recent, alive bool
	e = tx.QueryRow(ctx, `SELECT last_seen>now()-$2*interval '1 second',last_seen>now()-$3*interval '1 second' FROM visit_sessions WHERE id_hash=$1`,
		hash, int64(sessionTouchMin.Seconds()), int64(sessionIdle.Seconds())).Scan(&recent, &alive)
	switch {
	case errors.Is(e, pgx.ErrNoRows):
		if !a.stats.rows.allow("", statsNewRowsMax, now) {
			return abort(errStatsBusy)
		}
		_, e = tx.Exec(ctx, `INSERT INTO visit_sessions(id_hash) VALUES($1)`, hash)
	case e != nil:
	case !alive:
		if _, e = tx.Exec(ctx, `DELETE FROM article_reads WHERE session_hash=$1`, hash); e == nil {
			_, e = tx.Exec(ctx, `UPDATE visit_sessions SET last_seen=now() WHERE id_hash=$1`, hash)
		}
	case force || !recent:
		_, e = tx.Exec(ctx, `UPDATE visit_sessions SET last_seen=now() WHERE id_hash=$1`, hash)
	}
	if e != nil {
		return abort(e)
	}
	return tx, nil
}

// touchSession marks a stats session active (heartbeat).
func (a *App) touchSession(ctx context.Context, hash []byte, force bool, now time.Time) error {
	tx, e := a.beginSession(ctx, hash, force, now)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	return tx.Commit(ctx)
}

// countRead records that this session read the article and returns its read
// count. Only the first read of an article in a session adds one; the
// (session, article) primary key and the session lock make concurrent
// requests count once.
func (a *App) countRead(ctx context.Context, hash []byte, article int64, now time.Time) (int64, error) {
	tx, e := a.beginSession(ctx, hash, true, now)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var readable bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM articles a JOIN sources s ON s.id=a.source_id WHERE a.id=$1 AND NOT s.deleted)`, article).Scan(&readable); e != nil {
		return 0, e
	}
	if !readable {
		return 0, errNotReadable
	}
	tag, e := tx.Exec(ctx, `INSERT INTO article_reads(session_hash,article_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, hash, article)
	if e != nil {
		return 0, e
	}
	var reads int64
	if tag.RowsAffected() == 1 {
		e = tx.QueryRow(ctx, `INSERT INTO article_read_counts(article_id,reads) VALUES($1,1) ON CONFLICT(article_id) DO UPDATE SET reads=article_read_counts.reads+1 RETURNING reads`, article).Scan(&reads)
	} else {
		e = tx.QueryRow(ctx, `SELECT coalesce((SELECT reads FROM article_read_counts WHERE article_id=$1),0)`, article).Scan(&reads)
	}
	if e != nil {
		return 0, e
	}
	return reads, tx.Commit(ctx)
}

// cleanStats deletes, in bounded batches, the dedup rows of sessions that are
// over and sessions idle for sessionRetention. Read counts are never touched.
func (a *App) cleanStats(ctx context.Context) (reads, sessions int64) {
	clean := func(sql string, keep time.Duration) (n int64) {
		for i := 0; i < cleanupBatches; i++ {
			tag, e := a.db.Exec(ctx, sql, int64(keep.Seconds()), cleanupBatch)
			if e != nil {
				log.Printf("clean stats: %v", e)
				return
			}
			n += tag.RowsAffected()
			if tag.RowsAffected() < cleanupBatch {
				return
			}
		}
		return
	}
	reads = clean(`DELETE FROM article_reads WHERE (session_hash,article_id) IN (SELECT r.session_hash,r.article_id FROM article_reads r
 LEFT JOIN visit_sessions s ON s.id_hash=r.session_hash AND s.last_seen>now()-$1*interval '1 second' WHERE s.id_hash IS NULL LIMIT $2)`, sessionIdle)
	sessions = clean(`DELETE FROM visit_sessions WHERE id_hash IN (SELECT id_hash FROM visit_sessions WHERE last_seen<now()-$1*interval '1 second' ORDER BY last_seen LIMIT $2)`, sessionRetention)
	if reads+sessions > 0 {
		log.Printf("clean stats: %d read marks of ended sessions, %d idle sessions removed", reads, sessions)
	}
	return
}
