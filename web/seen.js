// Per-browser "seen" cursors for each category. A cursor is the highest
// article id the browser had loaded when the user last opened the category;
// the server counts readable articles above it as new. Article ids are
// assigned in the order articles are first received, so a late article (old
// published date) is still new.
(function (root) {
  const KEY = 'news-seen-v1';
  const VERSION = 2;
  // Category list v2 (migration 015). Renamed categories keep their cursor.
  // The dropped ones were merged into "khac", which takes the highest cursor
  // among them and the old "khac": an article above it was above the cursor
  // of every old category it belonged to, so it already had a red dot — the
  // change creates no false dot. The price: an article still unseen in the
  // old Khác but below a newer cursor of a merged category loses its dot.
  const MOVED = { 'kinh-doanh': 'kinh-te', 'cong-nghe': 'khoa-hoc-cong-nghe', 'giai-tri': 'khac', 'the-thao': 'khac', 'suc-khoe': 'khac' };

  // Upgrades stored cursors of an older version (no "v") in place.
  function upgrade(state) {
    if (state.v === VERSION) return false;
    const c = state.cursors;
    for (const [from, to] of Object.entries(MOVED)) {
      if (!(from in c)) continue;
      if (Number.isSafeInteger(c[from]) && c[from] >= 0 && !(Number.isSafeInteger(c[to]) && c[to] >= c[from])) c[to] = c[from];
      delete c[from];
    }
    state.v = VERSION;
    return true;
  }

  function memoryStorage() {
    const m = new Map();
    return { getItem: k => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, String(v)), removeItem: k => m.delete(k) };
  }

  function create(storage) {
    storage = storage || memoryStorage();
    let state = null;
    try {
      const v = JSON.parse(storage.getItem(KEY) || 'null');
      if (v && typeof v.cursors === 'object' && v.cursors) state = v;
    } catch { state = null; }
    const save = () => { try { storage.setItem(KEY, JSON.stringify(state)); } catch { /* private mode: keep in memory */ } };
    if (state && upgrade(state)) save();
    const valid = n => Number.isSafeInteger(n) && n >= 0;

    return {
      // Query value for GET /api/categories ("slug:cursor,...").
      param() {
        return state ? Object.entries(state.cursors).map(([k, v]) => `${k}:${v}`).join(',') : '';
      },
      cursor(slug) { return state && slug in state.cursors ? state.cursors[slug] : undefined; },
      snapshot() { return state ? { ...state.cursors } : {}; },
      // Called with every successful categories response. A category this
      // browser has no cursor for (first use, or a new category) starts at the
      // current cursor, so existing history is not shown as new. A stored
      // cursor above the server's (database restored) is clamped.
      sync(slugs, cursor) {
        if (!valid(cursor)) return;
        if (!state) state = { v: VERSION, cursors: {} };
        let changed = false;
        for (const s of slugs) {
          if (!(s in state.cursors) || state.cursors[s] > cursor) { state.cursors[s] = cursor; changed = true; }
        }
        if (changed) save();
      },
      // Mark a category seen up to the cursor of a list the user opened and
      // that loaded successfully. Background refreshes, the "all" view and
      // lists narrowed by source, country or search do not count as seeing
      // the category: the user only saw part of it.
      markSeen(slug, cursor, { userAction, query, source, country } = {}) {
        if (!state || !slug || !userAction || query || source || country || !valid(cursor)) return false;
        if (!(slug in state.cursors) || cursor > state.cursors[slug]) { state.cursors[slug] = cursor; save(); }
        return true;
      },
    };
  }

  // Loads a list and marks it seen only after the request succeeded.
  async function loadAndMark(store, fetchList, slug, opts) {
    const res = await fetchList();
    store.markSeen(slug, res.cursor, opts);
    return res;
  }

  // An article is new for this browser when its id is above the cursor of at
  // least one of its categories, as stored before the current list was marked.
  function isNew(article, cursors) {
    return (article.categories || []).some(c => cursors[c] !== undefined && article.id > cursors[c]);
  }

  const api = { create, loadAndMark, isNew, upgrade, KEY, VERSION };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.SeenStore = api;
})(typeof window !== 'undefined' ? window : globalThis);
