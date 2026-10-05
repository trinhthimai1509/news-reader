// Per-browser "seen" cursors for each category. A cursor is the highest
// article id the browser had loaded when the user last opened the category;
// the server counts readable articles above it as new. Article ids are
// assigned in the order articles are first received, so a late article (old
// published date) is still new.
(function (root) {
  const KEY = 'news-seen-v1';

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
        if (!state) state = { cursors: {} };
        let changed = false;
        for (const s of slugs) {
          if (!(s in state.cursors) || state.cursors[s] > cursor) { state.cursors[s] = cursor; changed = true; }
        }
        if (changed) save();
      },
      // Mark a category seen up to the cursor of a list the user opened and
      // that loaded successfully. Background refreshes, the "all" view and
      // lists narrowed by source or search do not count as seeing the category.
      markSeen(slug, cursor, { userAction, query, source } = {}) {
        if (!state || !slug || !userAction || query || source || !valid(cursor)) return false;
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

  const api = { create, loadAndMark, isNew, KEY };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.SeenStore = api;
})(typeof window !== 'undefined' ? window : globalThis);
