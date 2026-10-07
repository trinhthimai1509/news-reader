package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"personal.news/reader/internal/news"
)

func newTestApp(db *pgxpool.Pool) *App {
	return &App{db: db, authCfg: defaultAuthConfig(), logins: newLoginLimiter(), reportTo: defaultReportEmail}
}

const (
	testAdmin    = "quantri"
	testPassword = "mat-khau-thu-nghiem-dai"
)

// adminClient sends requests the way the page does: session cookie, CSRF
// header, same-origin, JSON.
type adminClient struct {
	a      *App
	cookie *http.Cookie
	csrf   string
}

var (
	adminsMu sync.Mutex
	admins   = map[*App]*adminClient{}
)

func createAdmin(t *testing.T, a *App) {
	h, e := hashPassword(testPassword)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.db.Exec(context.Background(), `INSERT INTO admin_users(username,password_hash) VALUES($1,$2) ON CONFLICT DO NOTHING`, testAdmin, h); e != nil {
		t.Fatal(e)
	}
}

func loginReq(a *App, user, pw, peer string, hdr map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	r := httptest.NewRequest("POST", "/api/admin/login", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://example.com")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if peer != "" {
		r.RemoteAddr = peer
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == "nr_admin" || c.Name == "__Host-nr_admin" {
			return c
		}
	}
	return nil
}

func login(t *testing.T, a *App) *adminClient {
	w := loginReq(a, testAdmin, testPassword, "", nil)
	var b struct {
		CSRF string `json:"csrf_token"`
	}
	json.Unmarshal(w.Body.Bytes(), &b)
	c := sessionCookie(w)
	if w.Code != 200 || c == nil || b.CSRF == "" {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	return &adminClient{a: a, cookie: c, csrf: b.CSRF}
}

// adminFor returns a logged-in client for a (one session per App).
func adminFor(t *testing.T, a *App) *adminClient {
	adminsMu.Lock()
	defer adminsMu.Unlock()
	if c := admins[a]; c != nil {
		return c
	}
	createAdmin(t, a)
	c := login(t, a)
	admins[a] = c
	t.Cleanup(func() { adminsMu.Lock(); delete(admins, a); adminsMu.Unlock() })
	return c
}

func (c *adminClient) req(method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://example.com")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	c.a.routes().ServeHTTP(w, r)
	return w
}
func (c *adminClient) do(method, path, body string) *httptest.ResponseRecorder {
	return c.req(method, path, body, nil)
}

func adminGet(t *testing.T, a *App, path string, v any) int {
	w := adminFor(t, a).do("GET", path, "")
	if v != nil && w.Code == 200 {
		if e := json.Unmarshal(w.Body.Bytes(), v); e != nil {
			t.Fatal(e)
		}
	}
	return w.Code
}

// Every route that changes data or shows operational details.
var adminRoutes = [][2]string{
	{"GET", "/api/admin/sources"},
	{"PATCH", "/api/admin/sources/1"},
	{"DELETE", "/api/admin/sources/1"},
	{"POST", "/api/admin/sources/import"},
	{"GET", "/api/admin/status"},
	{"GET", "/api/admin/publishers"},
	{"PATCH", "/api/admin/publishers/bbc"},
	{"POST", "/api/admin/logout"},
	{"PUT", "/api/admin/sources/1"},
	{"GET", "/api/admin/anything"},
}

func TestPasswordHash(t *testing.T) {
	h, e := hashPassword(testPassword)
	if e != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") || strings.Contains(h, testPassword) {
		t.Fatal(h, e)
	}
	h2, _ := hashPassword(testPassword)
	if h == h2 {
		t.Fatal("salt not random")
	}
	if !verifyPassword(h, testPassword) || verifyPassword(h, testPassword+"x") || verifyPassword(h, "") {
		t.Fatal("verify")
	}
	for _, bad := range []string{"", testPassword, strings.Replace(h, "argon2id", "argon2i", 1), strings.Replace(h, "m=65536", "m=99999999", 1), strings.Replace(h, "t=3", "t=0", 1), h + "$x"} {
		if verifyPassword(bad, testPassword) {
			t.Errorf("accepted %q", bad)
		}
	}
	if validPassword("short") == nil || validPassword(strings.Repeat("x", 129)) == nil || validPassword("mười-hai-ký-tự") != nil {
		t.Fatal("password length rule")
	}
}

// Guests (no cookie) are refused on every admin route; neither the old
// Bearer token nor a "local" request opens them, and the old write routes
// are gone. No database is needed: the refusal comes first.
func TestGuestCannotReachAdminAPI(t *testing.T) {
	app := newTestApp(nil)
	for _, rt := range adminRoutes {
		for _, hdr := range []map[string]string{
			nil,
			{"Authorization": "Bearer " + strings.Repeat("a", 32)},
			{"X-Forwarded-For": "127.0.0.1", "Host": "localhost:8080"},
			{"Cookie": "nr_admin="},
		} {
			r := httptest.NewRequest(rt[0], rt[1], strings.NewReader(`{"enabled":false}`))
			r.RemoteAddr = "127.0.0.1:5000"
			r.Header.Set("Content-Type", "application/json")
			for k, v := range hdr {
				if k == "Host" {
					r.Host = v
				} else {
					r.Header.Set(k, v)
				}
			}
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, r)
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "feed_url") {
				t.Errorf("%v %v: %d %s", rt, hdr, w.Code, w.Body)
			}
		}
	}
	for _, rt := range [][2]string{{"PATCH", "/api/sources/1"}, {"DELETE", "/api/sources/1"}, {"POST", "/api/sources/import"}, {"POST", "/api/sources"}, {"GET", "/api/status"}, {"GET", "/api/session"}} {
		r := httptest.NewRequest(rt[0], rt[1], strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
		w := httptest.NewRecorder()
		app.routes().ServeHTTP(w, r)
		if w.Code != 404 && w.Code != 405 {
			t.Errorf("old route %v still served: %d", rt, w.Code)
		}
	}
}

// The public API is read-only: no write method reaches a handler.
func TestPublicRoutesAreReadOnly(t *testing.T) {
	app := newTestApp(nil)
	for _, p := range []string{"/api/articles", "/api/articles/1", "/api/sources", "/api/sources/1", "/api/categories", "/api/countries", "/api/admin/session", "/healthz"} {
		for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			w := httptest.NewRecorder()
			app.routes().ServeHTTP(w, httptest.NewRequest(m, p, strings.NewReader(`{}`)))
			if w.Code != 404 && w.Code != 405 && !(w.Code == 401 && strings.HasPrefix(p, "/api/admin/")) {
				t.Errorf("%s %s: %d", m, p, w.Code)
			}
		}
	}
}

func TestLoginRefusesCrossSiteAndThrottles(t *testing.T) {
	app := newTestApp(nil)
	for _, hdr := range []map[string]string{
		{"Origin": "https://evil.example"},
		{"Origin": "null"},
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Content-Type": "text/plain"},
		{"Content-Type": "application/x-www-form-urlencoded"},
	} {
		if w := loginReq(app, testAdmin, testPassword, "", hdr); w.Code != 403 {
			t.Errorf("%v: %d", hdr, w.Code)
		}
	}
	// Once blocked, the limit answers before the database or the password
	// check, even for a correct password.
	now := time.Now()
	for i := 0; i < loginFailures; i++ {
		app.logins.fail("192.0.2.1", now)
	}
	w := loginReq(app, testAdmin, testPassword, "192.0.2.1:1", nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("per-address limit: %d", w.Code)
	}
	for i := 0; i < loginFailuresAll; i++ {
		app.logins.fail(loginAllKey, now)
	}
	if w := loginReq(app, testAdmin, testPassword, "198.51.100.7:1", nil); w.Code != 429 {
		t.Fatalf("global limit: %d", w.Code)
	}
	if app.logins.blocked(loginAllKey, loginFailuresAll, now.Add(loginWindow+time.Second)) {
		t.Fatal("limit does not expire")
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app := newTestApp(nil)
		app.authCfg.secure = secure
		w := httptest.NewRecorder()
		app.setSessionCookie(w, "v", 3600)
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure != secure || c.MaxAge != 3600 {
			t.Errorf("secure=%v: %+v", secure, c)
		}
		if secure != strings.HasPrefix(c.Name, "__Host-") {
			t.Errorf("cookie name %s", c.Name)
		}
	}
}

func TestAdminLoginCSRFAndLogout(t *testing.T) {
	a, ctx := migratedApp(t)
	createAdmin(t, a)
	// Wrong password and unknown user get the same answer.
	w1 := loginReq(a, testAdmin, "sai-mat-khau-hoan-toan", "", nil)
	w2 := loginReq(a, "khong-ton-tai", testPassword, "", nil)
	if w1.Code != 401 || w2.Code != 401 || w1.Body.String() != w2.Body.String() || sessionCookie(w1) != nil {
		t.Fatalf("%d %s / %d %s", w1.Code, w1.Body, w2.Code, w2.Body)
	}
	c := login(t, a)
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", c.cookie)
	}
	var stored string
	a.db.QueryRow(ctx, `SELECT encode(token_hash,'hex') FROM admin_sessions`).Scan(&stored)
	if stored == "" || strings.Contains(stored, c.cookie.Value) {
		t.Fatal("session token must be stored hashed")
	}
	var info map[string]any
	r := httptest.NewRequest("GET", "/api/admin/session", nil)
	r.AddCookie(c.cookie)
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	json.Unmarshal(w.Body.Bytes(), &info)
	if info["authenticated"] != true || info["csrf_token"] != c.csrf || info["report_to"] != defaultReportEmail {
		t.Fatalf("session: %s", w.Body)
	}
	s := addSource(t, a, "BBC thử", "https://feeds.bbci.co.uk/news/t-auth.xml", "khac")
	path := fmt.Sprintf("/api/admin/sources/%d", s.ID)
	if w := c.do("GET", "/api/admin/sources", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "feed_url") {
		t.Fatalf("admin sources: %d", w.Code)
	}
	for name, hdr := range map[string]map[string]string{
		"no csrf":         {"X-CSRF-Token": ""},
		"wrong csrf":      {"X-CSRF-Token": "x" + c.csrf},
		"cross-site":      {"Origin": "https://evil.example"},
		"fetch metadata":  {"Sec-Fetch-Site": "cross-site"},
		"text/plain body": {"Content-Type": "text/plain"},
	} {
		w := c.req("PATCH", path, `{"enabled":false}`, hdr)
		if w.Code != 403 && w.Code != 415 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	var enabled bool
	a.db.QueryRow(ctx, `SELECT enabled FROM sources WHERE id=$1`, s.ID).Scan(&enabled)
	if !enabled {
		t.Fatal("a refused request changed data")
	}
	if w := c.do("PATCH", path, `{"enabled":false}`); w.Code != 200 {
		t.Fatalf("valid patch: %d", w.Code)
	}
	// Logout ends the session server-side: the old cookie no longer writes.
	w = c.do("POST", "/api/admin/logout", "")
	if w.Code != 200 || sessionCookie(w) == nil || sessionCookie(w).MaxAge >= 0 {
		t.Fatalf("logout: %d", w.Code)
	}
	for _, rt := range adminRoutes {
		if w := c.do(rt[0], strings.Replace(rt[1], "/1", fmt.Sprintf("/%d", s.ID), 1), `{"enabled":true}`); w.Code != 401 {
			t.Errorf("after logout %v: %d", rt, w.Code)
		}
	}
	var n int
	a.db.QueryRow(ctx, `SELECT count(*) FROM admin_sessions`).Scan(&n)
	a.db.QueryRow(ctx, `SELECT enabled FROM sources WHERE id=$1`, s.ID).Scan(&enabled)
	if n != 0 || enabled {
		t.Fatalf("sessions=%d enabled=%v", n, enabled)
	}
}

func TestSessionExpiry(t *testing.T) {
	a, ctx := migratedApp(t)
	c := adminFor(t, a)
	if w := c.do("GET", "/api/admin/status", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	// Absolute lifetime over.
	a.db.Exec(ctx, `UPDATE admin_sessions SET expires_at=now()-interval '1 second'`)
	if w := c.do("GET", "/api/admin/status", ""); w.Code != 401 {
		t.Fatalf("expired: %d", w.Code)
	}
	// Idle too long.
	c2 := login(t, a)
	a.db.Exec(ctx, `UPDATE admin_sessions SET last_seen=now()-$1*interval '1 second' WHERE expires_at>now()`, int64(a.authCfg.idle.Seconds())+1)
	if w := c2.do("GET", "/api/admin/status", ""); w.Code != 401 {
		t.Fatalf("idle: %d", w.Code)
	}
	a.cleanSessions(ctx)
	var n int
	a.db.QueryRow(ctx, `SELECT count(*) FROM admin_sessions`).Scan(&n)
	if n != 0 {
		t.Fatalf("expired sessions kept: %d", n)
	}
	// A short configured lifetime is honoured.
	a.authCfg.ttl = time.Second
	c3 := login(t, a)
	if c3.cookie.MaxAge != 1 {
		t.Fatalf("max-age %d", c3.cookie.MaxAge)
	}
	time.Sleep(2500 * time.Millisecond)
	if w := c3.do("GET", "/api/admin/status", ""); w.Code != 401 {
		t.Fatalf("ttl: %d", w.Code)
	}
}

func TestLoginLimitWithDatabase(t *testing.T) {
	a, _ := migratedApp(t)
	createAdmin(t, a)
	for i := 0; i < loginFailures; i++ {
		if w := loginReq(a, testAdmin, "sai-mat-khau-hoan-toan", "192.0.2.9:1", nil); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	if w := loginReq(a, testAdmin, testPassword, "192.0.2.9:1", nil); w.Code != 429 {
		t.Fatalf("blocked address with correct password: %d", w.Code)
	}
	if w := loginReq(a, testAdmin, testPassword, "192.0.2.10:1", nil); w.Code != 200 {
		t.Fatalf("other address: %d", w.Code)
	}
}

func TestAdminCLI(t *testing.T) {
	a, ctx := migratedApp(t)
	pw := func(p string) func() (string, error) { return func() (string, error) { return p, nil } }
	var out bytes.Buffer
	if e := adminCommand(ctx, a.db, []string{"reset-password"}, pw(testPassword), &out); e == nil {
		t.Fatal("reset without account")
	}
	if e := adminCommand(ctx, a.db, []string{"create", "x"}, pw(testPassword), &out); e == nil {
		t.Fatal("bad username accepted")
	}
	if e := adminCommand(ctx, a.db, []string{"create", testAdmin}, pw(testPassword), &out); e != nil {
		t.Fatal(e)
	}
	if e := adminCommand(ctx, a.db, []string{"create", "nguoi-khac"}, pw(testPassword), &out); e == nil {
		t.Fatal("second admin created")
	}
	old := login(t, a)
	if e := adminCommand(ctx, a.db, []string{"reset-password"}, pw("mat-khau-moi-rat-dai"), &out); e != nil {
		t.Fatal(e)
	}
	if w := old.do("GET", "/api/admin/sources", ""); w.Code != 401 {
		t.Fatalf("session survived reset: %d", w.Code)
	}
	if w := loginReq(a, testAdmin, testPassword, "", nil); w.Code != 401 {
		t.Fatalf("old password: %d", w.Code)
	}
	if w := loginReq(a, testAdmin, "mat-khau-moi-rat-dai", "", nil); w.Code != 200 {
		t.Fatalf("new password: %d", w.Code)
	}
	if e := adminCommand(ctx, a.db, []string{"status"}, nil, &out); e != nil {
		t.Fatal(e)
	}
	if e := adminCommand(ctx, a.db, []string{"logout-all"}, nil, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), testPassword) || strings.Contains(out.String(), "mat-khau-moi") || strings.Contains(out.String(), "argon2") {
		t.Fatalf("CLI printed a secret: %s", out.String())
	}
	var hash string
	a.db.QueryRow(ctx, `SELECT password_hash FROM admin_users`).Scan(&hash)
	if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, "mat-khau") {
		t.Fatal("password not stored as argon2id hash")
	}
}

// Public responses carry what a reader needs and nothing operational.
func TestPublicAPIHidesAdminData(t *testing.T) {
	a, ctx := migratedApp(t)
	s := addSource(t, a, "World", "https://feeds.bbci.co.uk/news/t-world.xml", "the-gioi")
	now := time.Now()
	a.ingest(ctx, s, []news.Item{item("p1", "Public 1", now)}, now)
	a.db.Exec(ctx, `UPDATE sources SET last_error='dial tcp 10.0.0.5: secret-internal-error' WHERE id=$1`, s.ID)
	a.db.Exec(ctx, `UPDATE articles SET content_error='secret-content-error',enrich_error='secret-enrich-error'`)
	adminFor(t, a) // an admin session exists
	var id int64
	a.db.QueryRow(ctx, `SELECT id FROM articles LIMIT 1`).Scan(&id)
	for _, p := range []string{"/api/sources", "/api/articles", fmt.Sprintf("/api/articles/%d", id), "/api/categories", "/api/countries", "/api/admin/session"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		body := w.Body.String()
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d", p, w.Code)
		}
		for _, secret := range []string{"feed_url", "last_error", "secret-", "etag", "csrf", "password", "token", "report_to", "@", testAdmin, "10.0.0.5"} {
			if strings.Contains(body, secret) {
				t.Errorf("%s leaks %q: %s", p, secret, body)
			}
		}
	}
	var src []map[string]any
	get(t, a, "/api/sources", &src)
	if len(src) == 0 || len(src[0]) != 4 {
		t.Fatalf("public source fields: %v", src)
	}
}
