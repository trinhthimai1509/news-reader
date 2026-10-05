const test = require('node:test');
const assert = require('node:assert');
const { allowed, supported, duration, create } = require('../web/video.js');

test('only verified media hosts and paths are accepted', () => {
  assert.ok(allowed('mp4', 'https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/x.mp4'));
  // VnExpress HLS is hotlink-protected, so no HLS URL is playable.
  for (const [k, u] of [['hls', 'https://d1.vnecdn.net/thethao/video/video/web/mp4/,360p,480p,/2026/10/05/x/vne/master.m3u8'], ['hls', 'http://d1.vnecdn.net/vnexpress/video/a.m3u8'], ['hls', 'https://evil.example/vnexpress/video/a.m3u8'],
    ['hls', 'https://d1.vnecdn.net/other/a.m3u8'], ['hls', 'https://d1.vnecdn.net/vnexpress/video/a.m3u8'], ['hls', 'https://d1.vnecdn.net/vnexpress/video/a.mp4'], ['mp4', 'https://cdn2.tuoitre.vn/1.1/?vid=x.mp4'],
    ['mp4', 'https://cdn2.tuoitre.vn/a.mp4?token=1'], ['mp4', 'https://cdn2.tuoitre.vn:8443/a.mp4'], ['link', 'https://cdn2.tuoitre.vn/a.mp4'], ['mp4', 'javascript:alert(1)']]) {
    assert.strictEqual(allowed(k, u), false, `${k} ${u}`);
  }
});

test('format support comes from the browser', () => {
  const none = { canPlayType: () => '' }, mp4 = { canPlayType: t => t === 'video/mp4' ? 'maybe' : '' };
  assert.strictEqual(supported('mp4', none), false);
  assert.strictEqual(supported('mp4', mp4), true);
  assert.strictEqual(supported('hls', mp4), false);
  assert.strictEqual(supported('link', mp4), false);
  assert.strictEqual(duration(113), '1:53');
});

// Minimal DOM stand-in: enough for the player's state handling.
function fakeDoc({ canPlay = 'maybe' } = {}) {
  const made = [];
  const node = tag => {
    const n = { tag, children: [], attrs: {}, className: '', textContent: '', style: {}, listeners: {}, classList: { add() {}, remove() {} },
      append(...c) { this.children.push(...c); }, prepend(...c) { this.children.unshift(...c); }, replaceChildren(...c) { this.children = c; },
      remove() { this.removed = true; }, setAttribute(k, v) { this.attrs[k] = v; }, removeAttribute(k) { if (k === 'src') { this.src = undefined; } },
      addEventListener(e, f) { (this.listeners[e] = this.listeners[e] || []).push(f); }, emit(e) { (this.listeners[e] || []).forEach(f => f()); } };
    if (tag === 'video') Object.assign(n, { canPlayType: () => canPlay, paused: true, loads: 0, play() { this.paused = false; return Promise.resolve(); }, pause() { this.paused = true; }, load() { this.loads++; } });
    made.push(n);
    return n;
  };
  return { createElement: node, made };
}
const find = (n, f) => f(n) ? n : (n.children || []).map(c => find(c, f)).find(Boolean);
const vid = { kind: 'mp4', src: 'https://cdn2.tuoitre.vn/471584752817336320/2026/10/5/x.mp4', poster: 'https://cdn2.tuoitre.vn/thumb_w/1200/p.jpg', page_url: 'https://tuoitre.vn/video/a-203231.htm', duration: 60 };

test('nothing loads before play; stopAll pauses and releases the request', () => {
  const doc = fakeDoc(); const v = create(doc);
  const fig = v.figure(vid);
  assert.ok(!find(fig, n => n.tag === 'video'), 'no player before the click');
  assert.ok(find(fig, n => n.tag === 'a' && n.textContent === 'Xem video tại nguồn'));
  find(fig, n => n.className === 'video-play').onclick();
  const player = find(fig, n => n.tag === 'video');
  assert.strictEqual(player.src, vid.src);
  assert.strictEqual(player.preload, 'metadata');
  assert.strictEqual(player.paused, false);
  v.stopAll();
  assert.ok(player.paused && player.src === undefined && player.loads === 1 && player.removed);
  assert.ok(!find(fig, n => n.tag === 'video') && find(fig, n => n.className === 'video-play'), 'poster and button back');
  player.emit('error'); // late event of the stopped player
  assert.strictEqual(find(fig, n => n.className === 'video-status').textContent, '');
});

test('errors and timeouts fall back to the poster and the source link, without retrying', async () => {
  const doc = fakeDoc(); const v = create(doc, { timeout: 20 });
  const fig = v.figure(vid);
  find(fig, n => n.className === 'video-play').onclick();
  find(fig, n => n.tag === 'video').emit('error');
  assert.match(find(fig, n => n.className === 'video-status').textContent, /Không phát được/);
  assert.strictEqual(v.active.size, 0);
  find(fig, n => n.className === 'video-play').onclick();
  await new Promise(r => setTimeout(r, 40));
  assert.match(find(fig, n => n.className === 'video-status').textContent, /quá lâu/);
  const players = doc.made.filter(n => n.tag === 'video' && n.src !== undefined);
  assert.strictEqual(players.length, 0, 'no player left loading');
});

test('unsupported format and link-only videos never create a player', () => {
  const doc = fakeDoc({ canPlay: '' }); const v = create(doc);
  const fig = v.figure(vid);
  find(fig, n => n.className === 'video-play').onclick();
  assert.match(find(fig, n => n.className === 'video-status').textContent, /không phát được định dạng/);
  const link = v.figure({ kind: 'link', poster: 'https://ichef.bbci.co.uk/p.jpg', page_url: 'https://www.bbc.co.uk/news/videos/x' }, { sourceName: 'BBC News' });
  assert.ok(!find(link, n => n.className === 'video-play'));
  assert.match(find(link, n => n.className === 'video-note').textContent, /chỉ xem được tại BBC News/);
  // An unverified src is treated as link-only.
  const bad = v.figure({ kind: 'mp4', src: 'https://evil.example/a.mp4', page_url: 'https://tuoitre.vn/video/a-1.htm' });
  assert.ok(!find(bad, n => n.className === 'video-play'));
});
