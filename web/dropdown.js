// Custom dropdown over a native <select>. The menu of a native select is
// drawn by the browser/OS (it may cover the field), so the page shows its own
// "select-only combobox" (WAI-ARIA APG) instead:
//   - a <button role="combobox"> with the chosen text and a chevron, and
//   - a listbox opened below it (6 px gap, same width, own scroll), attached
//     to <body> so no container clips it or draws over it.
// The <select> stays in the page, hidden, and remains the source of truth:
// app code keeps reading/setting .value, adding options and listening to
// "change", which this component fires when the user picks an option.
// Keys (focus stays on the button): ArrowDown/ArrowUp/Alt+ArrowDown open or
// move, Home/End, PageUp/PageDown, Enter/Space choose, Escape closes, Tab
// closes, typing letters jumps to a matching option (accents ignored).
(function (root) {
  const GAP = 6, MAX_HEIGHT = 320, MIN_HEIGHT = 160, PAGE = 8, TYPE_RESET = 700;

  // Index to move to for a key, skipping disabled options; -1 if none.
  function move(opts, i, key) {
    const ok = j => j >= 0 && j < opts.length && !opts[j].disabled;
    const scan = (from, step) => { for (let j = from; j >= 0 && j < opts.length; j += step) if (ok(j)) return j; return -1; };
    switch (key) {
      case 'ArrowDown': return i < 0 ? scan(0, 1) : (scan(i + 1, 1) >= 0 ? scan(i + 1, 1) : i);
      case 'ArrowUp': return i < 0 ? scan(opts.length - 1, -1) : (scan(i - 1, -1) >= 0 ? scan(i - 1, -1) : i);
      case 'Home': return scan(0, 1);
      case 'End': return scan(opts.length - 1, -1);
      case 'PageDown': { const j = scan(Math.min(Math.max(i, 0) + PAGE, opts.length - 1), -1); return j > i ? j : move(opts, i, 'End'); }
      case 'PageUp': { const j = scan(Math.max(i - PAGE, 0), 1); return j >= 0 && j < i ? j : move(opts, i, 'Home'); }
      default: return i;
    }
  }

  const fold = s => String(s).normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/đ/g, 'd').replace(/Đ/g, 'D').toLowerCase();
  // Type-ahead: the next enabled option whose label starts with text,
  // searching after the current one first (repeating a letter cycles).
  function typeahead(opts, i, text) {
    const t = fold(text);
    if (!t) return -1;
    const same = [...t].every(c => c === t[0]);
    const n = opts.length;
    for (let k = 0; k < n; k++) {
      const j = (i + (same || t.length === 1 ? 1 : 0) + k + n) % n;
      if (!opts[j].disabled && fold(opts[j].label).startsWith(same ? t[0] : t)) return j;
    }
    return -1;
  }

  let seq = 0, openOne = null;

  function enhance(select) {
    if (select.dataset.dropdown) return select;
    const doc = select.ownerDocument, id = `dd${++seq}`;
    select.dataset.dropdown = id;
    const wrap = doc.createElement('div');
    wrap.className = 'dd';
    const button = doc.createElement('button');
    button.type = 'button';
    button.className = 'dd-button';
    button.id = `${id}-button`;
    button.setAttribute('role', 'combobox');
    button.setAttribute('aria-haspopup', 'listbox');
    button.setAttribute('aria-expanded', 'false');
    const text = doc.createElement('span');
    text.className = 'dd-value';
    const arrow = doc.createElement('span');
    arrow.className = 'dd-arrow';
    arrow.setAttribute('aria-hidden', 'true');
    button.append(text, arrow);
    // Found before the button is added: it then becomes the label's control.
    const label = select.closest('label') || (select.labels && select.labels[0]);
    select.before(wrap);
    // The button comes first, so a wrapping <label> labels it.
    wrap.append(button, select);
    select.classList.add('dd-native');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');
    // Name: the label's own text (not the option texts inside it).
    if (label) {
      let name = label.querySelector(':scope > .dd-label');
      if (!name) {
        const own = [...label.childNodes].filter(n => n.nodeType === 3 && n.textContent.trim());
        if (own.length) { name = doc.createElement('span'); name.className = 'dd-label'; own[0].before(name); name.append(...own); }
      }
      if (name) { name.id = name.id || `${id}-label`; button.setAttribute('aria-labelledby', name.id); }
      // Clicking the label text focuses the field, like a native label; it
      // does not toggle the list.
      label.addEventListener('click', e => { if (!wrap.contains(e.target)) { e.preventDefault(); button.focus(); } });
    }

    let list = null, active = -1, typed = '', typedAt = 0, pressing = false;
    const opts = () => [...select.options].map(o => ({ label: o.text, value: o.value, disabled: o.disabled }));
    function sync() {
      const o = select.options[select.selectedIndex];
      text.textContent = o ? o.text : '';
      button.disabled = select.disabled;
      if (select.disabled && list) close(false);
      if (list) render();
    }
    // Programmatic changes (.value = …, .selectedIndex = …) update the button.
    for (const prop of ['value', 'selectedIndex']) {
      const d = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(select), prop) || Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, prop);
      Object.defineProperty(select, prop, { configurable: true, get() { return d.get.call(this); }, set(v) { d.set.call(this, v); sync(); } });
    }
    new MutationObserver(sync).observe(select, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled'], characterData: true });

    function render() {
      list.replaceChildren();
      const all = opts();
      all.forEach((o, i) => {
        const li = doc.createElement('li');
        li.id = `${id}-o${i}`;
        li.className = 'dd-option';
        li.setAttribute('role', 'option');
        li.setAttribute('aria-selected', String(i === select.selectedIndex));
        if (o.disabled) li.setAttribute('aria-disabled', 'true');
        li.textContent = o.label;
        li.addEventListener('click', () => { if (!o.disabled) choose(i); });
        li.addEventListener('pointermove', () => { if (!o.disabled && active !== i) setActive(i, false); });
        list.append(li);
      });
      if (active >= all.length) active = -1;
      setActive(active, false);
    }
    function setActive(i, scroll = true) {
      active = i;
      if (!list) return;
      for (const li of list.children) li.classList.toggle('active', li.id === `${id}-o${i}`);
      const li = i >= 0 ? list.children[i] : null;
      if (li) { button.setAttribute('aria-activedescendant', li.id); if (scroll) li.scrollIntoView({ block: 'nearest' }); }
      else button.removeAttribute('aria-activedescendant');
    }
    // Below the button, same width; the height fits the room below (at least
    // MIN_HEIGHT: the page scrolls to show it rather than opening upwards).
    function place() {
      if (!list) return;
      const r = button.getBoundingClientRect(), view = doc.documentElement.clientHeight;
      list.style.left = `${r.left + root.scrollX}px`;
      list.style.top = `${r.bottom + GAP + root.scrollY}px`;
      list.style.width = `${r.width}px`;
      list.style.maxHeight = `${Math.max(MIN_HEIGHT, Math.min(MAX_HEIGHT, view - r.bottom - GAP - 8))}px`;
    }
    function open() {
      if (list || select.disabled) return;
      if (openOne) openOne.close(false);
      list = doc.createElement('ul');
      list.className = 'dd-list';
      list.id = `${id}-list`;
      list.setAttribute('role', 'listbox');
      // Pressing in the list (an option or its scrollbar) keeps the focus on
      // the button.
      list.addEventListener('mousedown', e => e.preventDefault());
      list.addEventListener('pointerdown', () => { pressing = true; });
      if (button.hasAttribute('aria-labelledby')) list.setAttribute('aria-labelledby', button.getAttribute('aria-labelledby'));
      doc.body.append(list);
      button.setAttribute('aria-controls', list.id);
      button.setAttribute('aria-expanded', 'true');
      wrap.classList.add('open');
      active = select.selectedIndex;
      render();
      place();
      setActive(active);
      // Not enough room below: scroll the page so the whole list shows.
      const lr = list.getBoundingClientRect();
      if (lr.bottom > doc.documentElement.clientHeight) list.scrollIntoView({ block: 'nearest' });
      openOne = api;
      doc.addEventListener('pointerdown', outside, true);
      doc.addEventListener('pointerup', released, true);
      root.addEventListener('resize', place);
    }
    function close(focus = true) {
      if (!list) return;
      list.remove();
      list = null;
      button.setAttribute('aria-expanded', 'false');
      button.removeAttribute('aria-activedescendant');
      button.removeAttribute('aria-controls');
      wrap.classList.remove('open');
      if (openOne === api) openOne = null;
      doc.removeEventListener('pointerdown', outside, true);
      doc.removeEventListener('pointerup', released, true);
      root.removeEventListener('resize', place);
      pressing = false;
      if (focus) button.focus();
    }
    function released() { if (pressing) { pressing = false; if (list && doc.activeElement !== button) button.focus(); } }
    function outside(e) { if (!wrap.contains(e.target) && !(list && list.contains(e.target))) close(false); }
    function choose(i) {
      const changed = i !== select.selectedIndex;
      if (i >= 0) select.selectedIndex = i;
      close();
      if (changed) {
        select.dispatchEvent(new Event('input', { bubbles: true }));
        select.dispatchEvent(new Event('change', { bubbles: true }));
      }
    }

    button.addEventListener('click', () => (list ? close() : open()));
    button.addEventListener('keydown', e => {
      const k = e.key;
      if (!list) {
        if (k === 'ArrowDown' || k === 'ArrowUp' || ((k === 'Home' || k === 'End') && !e.altKey)) {
          e.preventDefault(); open();
          if (k !== 'ArrowDown' || e.altKey) setActive(k === 'ArrowUp' ? Math.max(active, 0) : move(opts(), active, k));
          return;
        }
        // Enter/Space: the button's own click opens the list.
      } else if (k === 'Escape' || (k === 'ArrowUp' && e.altKey)) { e.preventDefault(); close(); return; }
      else if (k === 'Enter' || k === ' ' && !typed) { e.preventDefault(); choose(active); return; }
      else if (k === 'Tab') { close(false); return; }
      else if (['ArrowDown', 'ArrowUp', 'Home', 'End', 'PageDown', 'PageUp'].includes(k)) { e.preventDefault(); setActive(move(opts(), active, k)); return; }
      if (k.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey && (k !== ' ' || typed)) {
        const now = Date.now();
        typed = now - typedAt > TYPE_RESET ? k : typed + k;
        typedAt = now;
        const j = typeahead(opts(), list ? active : select.selectedIndex, typed);
        if (j < 0) return;
        e.preventDefault();
        if (list) setActive(j);
        else if (j !== select.selectedIndex) choose(j); // like a native select: typing on the closed field picks
        if (!list) button.focus();
        setTimeout(() => { if (Date.now() - typedAt >= TYPE_RESET) typed = ''; }, TYPE_RESET + 10);
      }
    });
    button.addEventListener('blur', () => { if (list && !pressing) setTimeout(() => { if (list && !pressing && doc.activeElement !== button) close(false); }, 0); });
    const api = { close, open, sync, button, get list() { return list; } };
    select.dropdown = api;
    sync();
    return select;
  }

  const api = { enhance, move, typeahead, fold };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Dropdown = api;
})(typeof window !== 'undefined' ? window : globalThis);
