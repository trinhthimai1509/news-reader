// Search box behaviour. The keyword filter changes only on Enter/submit, or
// when the box is emptied (native clear "X", Backspace/Delete, select-all +
// delete, or whitespace only). Events fired together by one user action
// (input + search + submit) produce a single reload.
(function (root) {
  const normalize = v => String(v == null ? '' : v).trim();

  function create(run, defer) {
    defer = defer || (fn => setTimeout(fn, 0));
    let applied = '', pending = null;
    function request(q, explicit) {
      if (pending) { pending.q = q; pending.explicit = pending.explicit || explicit; return; }
      pending = { q, explicit };
      defer(() => { const p = pending; pending = null; run(p.q, { explicit: p.explicit }); });
    }
    return {
      // The keyword the list is currently filtered by.
      get applied() { return applied; },
      // Enter or the "Tìm" button: always reloads, even with the same keyword.
      submit(value) { applied = normalize(value); request(applied, true); },
      // input / native "search" events: react only when the box became empty
      // while a keyword filter was applied. Not an explicit view of the
      // category, so it must not mark it seen.
      changed(value) {
        if (normalize(value) !== '' || applied === '') return false;
        applied = '';
        request('', false);
        return true;
      },
    };
  }

  // latest() hands out tickets; only the newest ticket's response may render.
  function latest() {
    let seq = 0;
    return { next() { const mine = ++seq; return () => mine === seq; } };
  }

  const api = { create, latest, normalize };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.SearchBox = api;
})(typeof window !== 'undefined' ? window : globalThis);
