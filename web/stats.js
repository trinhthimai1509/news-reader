// Reading statistics (see cmd/server/stats.go). The page reports:
//   - a "read" when an article opened and its content loaded; the server
//     counts it once per article per stats session, so opening it again,
//     reloading or another tab add nothing, and retries are harmless;
//   - "heartbeat"s while the page is visible and the reader interacted in the
//     last 5 minutes, at most once a minute for the whole browser ("Đang hoạt
//     động").
// Tabs of one browser share the stats session through localStorage; without
// storage the session lasts as long as the page (never one per heartbeat).
// Ids are random (no fingerprinting). A session unused for 30 minutes is
// replaced, like the server ends it. Nothing here is shared with the admin
// login, and failures are ignored: statistics never get in the way of reading.
(function (root) {
  const KEY = 'news-stats-v1';
  const MINUTE = 60000;
  const HEARTBEAT_EVERY = MINUTE;
  const ACTIVE_FOR = 5 * MINUTE;
  const SESSION_IDLE = 30 * MINUTE;
  const RETRY_AFTER = [5000, 20000, 60000];
  const ID = /^[0-9a-f]{32}$/;

  function randomId(fill) {
    const b = new Uint8Array(16);
    fill(b);
    return Array.from(b, x => x.toString(16).padStart(2, '0')).join('');
  }
  // Network failure (0), rate limit and server errors are worth a retry;
  // anything else (400, 403, 404, 415) will not get better.
  const retryable = status => status === 0 || status === 429 || status >= 500;

  // env: storage (or undefined), now(), fill(bytes), later(fn, ms), visible(),
  // send(body) -> Promise<{status, data}> (status 0 on network error).
  function create(env) {
    let memory = null, lastInteraction = env.now(), lastPost = Promise.resolve();
    function load() {
      if (env.storage) {
        try {
          const v = JSON.parse(env.storage.getItem(KEY) || 'null');
          if (v && ID.test(v.sid) && Number.isFinite(v.sent)) return v;
        } catch { /* unreadable: fall back to memory */ }
      }
      return memory;
    }
    function save(v) {
      memory = v;
      if (env.storage) try { env.storage.setItem(KEY, JSON.stringify(v)); } catch { /* blocked: memory only */ }
    }
    // The shared session. `sent` is when any tab last reported it; a clock
    // jump backwards also starts a new session.
    function use(t) {
      let s = load();
      if (!s || t - s.sent > SESSION_IDLE || s.sent - t > MINUTE) s = { sid: randomId(env.fill), sent: -Infinity };
      return s;
    }
    const post = body => {
      try { return Promise.resolve(env.send(body)).catch(() => ({ status: 0 })); } catch { return Promise.resolve({ status: 0 }); }
    };

    return {
      // An article opened and loaded. Resolves with the server's answer to
      // the first attempt ({status, data: {reads}}); retries go on in the
      // background with the same session.
      read(articleId) {
        const t = env.now(), s = use(t);
        s.sent = t;
        save(s);
        lastInteraction = t;
        const body = { type: 'read', session_id: s.sid, article_id: articleId };
        let tries = 0;
        const attempt = () => post(body).then(res => {
          if (retryable(res.status) && tries < RETRY_AFTER.length) env.later(attempt, RETRY_AFTER[tries++]);
          return res;
        });
        return attempt();
      },
      // Called periodically: sends a heartbeat when the page is visible, the
      // reader interacted recently and no tab reported in the last minute.
      tick() {
        const t = env.now();
        if (!env.visible() || t - lastInteraction >= ACTIVE_FOR) return false;
        const s = use(t);
        if (t - s.sent < HEARTBEAT_EVERY) return false;
        s.sent = t;
        save(s);
        lastPost = post({ type: 'heartbeat', session_id: s.sid });
        return true;
      },
      // Input from the reader. Coming back after 5 idle minutes reports at once
      // (still at most once a minute).
      interaction() {
        const t = env.now(), idle = t - lastInteraction >= ACTIVE_FOR;
        lastInteraction = t;
        return idle ? this.tick() : false;
      },
      // The page was opened, the tab became visible again or the page came
      // back from the back/forward cache: that is activity too.
      resumed() {
        lastInteraction = env.now();
        return this.tick();
      },
      // Settles once the last heartbeat got its answer (or failed), so a
      // count read afterwards includes this browser.
      settled() { return lastPost.then(() => {}, () => {}); },
    };
  }

  // Text for a count; anything unexpected (error, missing data) is "—",
  // never 0.
  const count = v => (Number.isSafeInteger(v) && v >= 0 ? v.toLocaleString('vi-VN') : '—');

  const api = { create, count, KEY, HEARTBEAT_EVERY, ACTIVE_FOR, SESSION_IDLE, RETRY_AFTER };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.VisitStats = api;
})(typeof window !== 'undefined' ? window : globalThis);
