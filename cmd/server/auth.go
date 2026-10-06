package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
)

// Reading is public. Managing sources needs the single administrator, signed
// in with a server-side session:
//   - the cookie holds a random token (HttpOnly, SameSite=Strict, Secure when
//     COOKIE_SECURE=true); the database keeps only its SHA-256;
//   - a session ends after sessionTTL, after sessionIdle without requests, on
//     logout, or when the password is reset;
//   - every write also needs the session's CSRF token in X-CSRF-Token, a
//     same-origin request (Origin / Sec-Fetch-Site) and a JSON body;
//   - failed logins are limited per client address and in total.

// Password hashing: argon2id (RFC 9106), stored as a PHC string.
const (
	argonTime      = 3
	argonMemory    = 64 * 1024 // KiB
	argonThreads   = 2
	argonKeyLen    = 32
	minPasswordLen = 12
	maxPasswordLen = 128
)

var b64 = base64.RawStdEncoding

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// verifyPassword checks pw against a stored hash. Parameters are read from the
// hash (bounded, so a tampered row cannot exhaust memory).
func verifyPassword(encoded, pw string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[0] != "" || p[1] != "argon2id" || p[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false
	}
	var m, t uint32
	var threads uint8
	if n, e := fmt.Sscanf(p[3], "m=%d,t=%d,p=%d", &m, &t, &threads); n != 3 || e != nil || m < 8 || m > 256*1024 || t < 1 || t > 10 || threads < 1 {
		return false
	}
	salt, e1 := b64.DecodeString(p[4])
	key, e2 := b64.DecodeString(p[5])
	if e1 != nil || e2 != nil || len(salt) < 8 || len(key) < 16 || len(key) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

// dummyHash is verified when the username is unknown, so the answer takes
// the same time whether or not the account exists.
var dummyHash = sync.OnceValue(func() string {
	h, _ := hashPassword(randomToken())
	return h
})

// hashSlots bounds concurrent argon2 runs (64 MiB each).
var hashSlots = make(chan struct{}, 2)

func checkPassword(encoded, pw string) bool {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return verifyPassword(encoded, pw)
}

func validPassword(pw string) error {
	if n := len([]rune(pw)); n < minPasswordLen || n > maxPasswordLen {
		return fmt.Errorf("mật khẩu cần từ %d đến %d ký tự", minPasswordLen, maxPasswordLen)
	}
	return nil
}

func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenHash(v string) []byte {
	h := sha256.Sum256([]byte(v))
	return h[:]
}

type authConfig struct {
	ttl, idle time.Duration
	secure    bool // Secure cookie: set COOKIE_SECURE=true when served over HTTPS
}

func defaultAuthConfig() authConfig { return authConfig{ttl: 12 * time.Hour, idle: 2 * time.Hour} }

// cookieName: with Secure the __Host- prefix also pins the cookie to this
// host and path "/" (browsers refuse it otherwise).
func (c authConfig) cookieName() string {
	if c.secure {
		return "__Host-nr_admin"
	}
	return "nr_admin"
}

// limiter counts failures per key within a window; entries expire after it.
type limiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	window time.Duration
	max    int
}

const (
	loginWindow       = 15 * time.Minute
	loginFailures     = 5  // per client address
	loginFailuresAll  = 30 // all addresses together
	loginAllKey       = "*"
	sessionCookieSize = 100
)

func newLoginLimiter() *limiter { return &limiter{window: loginWindow} }

func (l *limiter) blocked(key string, max int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) >= max
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
		if now.Sub(t) < l.window {
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

func clientAddr(r *http.Request) string {
	if h, _, e := net.SplitHostPort(r.RemoteAddr); e == nil {
		return h
	}
	return r.RemoteAddr
}

// sameOrigin refuses requests a browser marks as coming from another site.
// Headers are only used to refuse, never to grant access.
func sameOrigin(r *http.Request) bool {
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" {
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, e := url.Parse(o)
		if e != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}

func isJSON(r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt == "application/json"
}

type adminSession struct {
	AdminID  int64
	Username string
	CSRF     string
	hash     []byte
}

type sessionKey struct{}

// currentSession returns the valid session named by the request cookie, or
// nil. A valid session's last_seen moves forward (idle timeout).
func (a *App) currentSession(ctx context.Context, r *http.Request) (*adminSession, error) {
	c, e := r.Cookie(a.authCfg.cookieName())
	if e != nil || c.Value == "" || len(c.Value) > sessionCookieSize {
		return nil, nil
	}
	s := &adminSession{hash: tokenHash(c.Value)}
	e = a.db.QueryRow(ctx, `UPDATE admin_sessions s SET last_seen=now() FROM admin_users u
WHERE s.token_hash=$1 AND u.id=s.admin_id AND s.expires_at>now() AND s.last_seen>now()-$2*interval '1 second'
RETURNING s.admin_id,u.username,s.csrf_token`, s.hash, int64(a.authCfg.idle.Seconds())).Scan(&s.AdminID, &s.Username, &s.CSRF)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return s, nil
}

func (a *App) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: a.authCfg.cookieName(), Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: a.authCfg.secure, SameSite: http.SameSiteStrictMode})
}

// requireAdmin guards every /api/admin/ route except login and session.
func (a *App) requireAdmin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, e := a.currentSession(r.Context(), r)
		if e != nil {
			fail(w, 500, "Không kiểm tra được phiên đăng nhập")
			return
		}
		if s == nil {
			fail(w, 401, "Cần đăng nhập quản trị")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1 {
				fail(w, 403, "Yêu cầu không hợp lệ, hãy tải lại trang")
				return
			}
			if (r.Method == http.MethodPost || r.Method == http.MethodPatch) && !isJSON(r) {
				fail(w, 415, "Yêu cầu cần dữ liệu JSON")
				return
			}
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, s)))
	})
}

func (a *App) sessionInfo(s *adminSession) map[string]any {
	return map[string]any{"authenticated": true, "username": s.Username, "csrf_token": s.CSRF, "report_to": a.reportTo}
}

// handleSession tells the page whether this browser holds an admin session.
// Guests only learn {"authenticated": false}.
func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	s, e := a.currentSession(r.Context(), r)
	if e != nil {
		fail(w, 500, "Không kiểm tra được phiên đăng nhập")
		return
	}
	if s == nil {
		send(w, 200, map[string]bool{"authenticated": false})
		return
	}
	send(w, 200, a.sessionInfo(s))
}

const loginError = "Tên đăng nhập hoặc mật khẩu không đúng"

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) || !isJSON(r) {
		fail(w, 403, "Yêu cầu không hợp lệ, hãy tải lại trang")
		return
	}
	client, now := clientAddr(r), time.Now()
	if a.logins.blocked(client, loginFailures, now) || a.logins.blocked(loginAllKey, loginFailuresAll, now) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
		fail(w, 429, "Đăng nhập sai quá nhiều lần, hãy thử lại sau 15 phút")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Username == "" || in.Password == "" || len(in.Username) > 64 || len(in.Password) > 4*maxPasswordLen {
		fail(w, 400, "Hãy nhập tên đăng nhập và mật khẩu")
		return
	}
	var id int64
	var hash string
	e := a.db.QueryRow(r.Context(), `SELECT id,password_hash FROM admin_users WHERE username=$1`, in.Username).Scan(&id, &hash)
	if errors.Is(e, pgx.ErrNoRows) {
		id, hash = 0, dummyHash()
	} else if e != nil {
		fail(w, 500, "Không đăng nhập được, hãy thử lại")
		return
	}
	if !checkPassword(hash, in.Password) || id == 0 {
		a.logins.fail(client, now)
		a.logins.fail(loginAllKey, now)
		log.Printf("admin login failed from %s", client)
		fail(w, 401, loginError)
		return
	}
	// A new token on every login (no session fixation); the session the
	// browser held before, if any, ends.
	if old, e := r.Cookie(a.authCfg.cookieName()); e == nil && len(old.Value) <= sessionCookieSize {
		a.db.Exec(r.Context(), `DELETE FROM admin_sessions WHERE token_hash=$1`, tokenHash(old.Value))
	}
	a.cleanSessions(r.Context())
	token, csrf := randomToken(), randomToken()
	if _, e = a.db.Exec(r.Context(), `INSERT INTO admin_sessions(token_hash,admin_id,csrf_token,expires_at) VALUES($1,$2,$3,now()+$4*interval '1 second')`, tokenHash(token), id, csrf, int64(a.authCfg.ttl.Seconds())); e != nil {
		fail(w, 500, "Không đăng nhập được, hãy thử lại")
		return
	}
	a.setSessionCookie(w, token, int(a.authCfg.ttl.Seconds()))
	log.Printf("admin login from %s", client)
	send(w, 200, a.sessionInfo(&adminSession{Username: in.Username, CSRF: csrf}))
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	s := r.Context().Value(sessionKey{}).(*adminSession)
	if _, e := a.db.Exec(r.Context(), `DELETE FROM admin_sessions WHERE token_hash=$1`, s.hash); e != nil {
		fail(w, 500, "Không đăng xuất được, hãy thử lại")
		return
	}
	a.setSessionCookie(w, "", -1)
	send(w, 200, map[string]bool{"authenticated": false})
}

// cleanSessions removes expired and idle sessions.
func (a *App) cleanSessions(ctx context.Context) {
	if _, e := a.db.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at<=now() OR last_seen<=now()-$1*interval '1 second'`, int64(a.authCfg.idle.Seconds())); e != nil {
		log.Printf("clean sessions: %v", e)
	}
}
