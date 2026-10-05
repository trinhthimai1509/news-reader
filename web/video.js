// Video blocks. The browser plays media straight from the source CDN; the
// server never downloads or proxies it. Nothing loads until the reader presses
// play: before that only the poster image is shown. A video that cannot play
// here (unsupported format, source error, link-only source) keeps its poster,
// caption and a "Xem video tại nguồn" link to the article page.
(function (root) {
  // Media hosts/paths verified per kind (mirrors news.VideoSrc on the server).
  function allowed(kind, src) {
    let u;
    try { u = new URL(src); } catch { return false; }
    if (u.protocol !== 'https:' || u.username || u.password || u.port || u.search || u.hash) return false;
    if (kind === 'mp4') return u.hostname === 'cdn2.tuoitre.vn' && u.pathname.endsWith('.mp4') && !u.pathname.startsWith('/1.1/');
    return false;
  }
  const types = { mp4: ['video/mp4'] };
  function supported(kind, probe) {
    return (types[kind] || []).some(t => probe.canPlayType(t) !== '');
  }
  const duration = s => s > 0 ? `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}` : '';
  const LOAD_TIMEOUT = 20000;

  function create(doc, opts = {}) {
    const timeout = opts.timeout || LOAD_TIMEOUT;
    const active = new Set();
    const el = (tag, text, cls) => { const n = doc.createElement(tag); if (text !== undefined) n.textContent = text; if (cls) n.className = cls; return n; };

    // stopAll pauses every player and releases its media request (empty src +
    // load() aborts the download), then restores the poster.
    function stopAll() { for (const p of [...active]) p.stop(); }

    function figure(v, { articleTitle = '', sourceName = '', posterOK = () => true } = {}) {
      const fig = el('figure', undefined, 'media video');
      const frame = el('div', undefined, 'video-frame');
      if (v.width > 0 && v.height > 0) frame.style.aspectRatio = `${v.width} / ${v.height}`;
      const poster = () => {
        if (!v.poster || !posterOK(v.poster)) return null;
        const img = el('img'); img.alt = ''; img.loading = 'lazy'; img.decoding = 'async'; img.referrerPolicy = 'no-referrer'; img.src = v.poster;
        img.onerror = () => img.remove();
        return img;
      };
      const status = el('span', '', 'video-status'); status.setAttribute('role', 'status');
      const playable = v.kind === 'mp4' && allowed(v.kind, v.src);
      let player = null, timer = null;
      const showPoster = () => {
        frame.replaceChildren();
        const img = poster(); if (img) frame.append(img);
        if (playable) {
          const btn = el('button', undefined, 'video-play'); btn.type = 'button';
          btn.append(el('span', '▶', 'video-icon'), el('span', 'Xem video'));
          if (v.duration) btn.append(el('span', duration(v.duration), 'video-duration'));
          btn.setAttribute('aria-label', `Phát video${v.title ? ': ' + v.title : ''}`);
          btn.onclick = play;
          frame.append(btn);
        } else {
          frame.append(el('span', `Video này chỉ xem được tại ${sourceName || 'trang nguồn'}.`, 'video-note'));
        }
      };
      const handle = {
        stop() {
          clearTimeout(timer); timer = null;
          if (player) { player.pause(); player.removeAttribute('src'); player.load(); player.remove(); player = null; }
          active.delete(handle);
          status.textContent = '';
          showPoster();
        },
      };
      function fail(msg) {
        handle.stop();
        // No automatic retry: the reader can press play again or use the link.
        status.textContent = msg;
        fig.classList.add('failed');
      }
      function play() {
        const probe = el('video');
        if (!supported(v.kind, probe)) { fail('Trình duyệt này không phát được định dạng video của nguồn. Hãy xem tại nguồn.'); return; }
        stopAll();
        fig.classList.remove('failed');
        const me = player = probe;
        // Events of a player that was already stopped (article changed) are ignored.
        const mine = f => () => { if (player === me) f(); };
        me.controls = true; me.playsInline = true; me.preload = 'metadata';
        if (v.poster && posterOK(v.poster)) me.poster = v.poster;
        me.addEventListener('playing', mine(() => { clearTimeout(timer); status.textContent = ''; }));
        me.addEventListener('loadeddata', mine(() => clearTimeout(timer)));
        me.addEventListener('error', mine(() => fail('Không phát được video từ nguồn (video có thể đã bị gỡ hoặc bị chặn).')));
        timer = setTimeout(mine(() => fail('Video tải quá lâu nên đã dừng. Hãy thử lại hoặc xem tại nguồn.')), timeout);
        status.textContent = 'Đang tải video…';
        player.src = v.src;
        frame.replaceChildren(player);
        active.add(handle);
        const p = player.play();
        if (p && p.catch) p.catch(() => {}); // the error event reports real failures
      }
      showPoster();
      fig.append(frame);
      const caption = v.caption || (v.title && v.title !== articleTitle ? v.title : '');
      if (caption || v.credit) {
        const fc = el('figcaption');
        if (caption) fc.append(el('span', caption, 'caption'));
        if (v.credit) fc.append(el('span', `Video: ${v.credit}`, 'credit'));
        fig.append(fc);
      }
      const actions = el('p', undefined, 'video-actions');
      let page = null;
      try { page = new URL(v.page_url); } catch { /* no link */ }
      if (page && page.protocol === 'https:') {
        const a = el('a', 'Xem video tại nguồn'); a.href = page.href; a.target = '_blank'; a.rel = 'noopener noreferrer';
        actions.append(a);
      }
      actions.append(status);
      fig.append(actions);
      fig.videoHandle = handle;
      return fig;
    }
    return { figure, stopAll, active };
  }

  const api = { create, allowed, supported, duration };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.VideoBlocks = api;
})(typeof window !== 'undefined' ? window : globalThis);
