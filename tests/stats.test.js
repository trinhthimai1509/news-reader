const test = require('node:test');
const assert = require('node:assert');
const { create, count, KEY, SESSION_IDLE } = require('../web/stats.js');

function memoryStorage() {
  const m = new Map();
  return { getItem: k => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, String(v)), map: m };
}
const blockedStorage = { getItem() { throw Error('blocked'); }, setItem() { throw Error('blocked'); } };

// A browser: shared clock and storage; each tab() is one page.
function browser(storage) {
  let clock = 1_000_000, seq = 0;
  const sent = [], timers = [];
  const world = {
    sent, timers, statuses: [],
    advance(ms) { clock += ms; },
    tab({ visible = true } = {}) {
      const page = { visible };
      page.stats = create({
        storage,
        now: () => clock,
        fill: b => { seq++; for (let i = 0; i < b.length; i++) b[i] = (seq * 31 + i) & 255; },
        later: (fn, ms) => timers.push({ fn, ms }),
        visible: () => page.visible,
        send: body => { sent.push(body); const status = world.statuses.length ? world.statuses.shift() : 200; return Promise.resolve({ status, data: { reads: 1 } }); },
      });
      return page;
    },
  };
  return world;
}
const flush = () => new Promise(r => setImmediate(r));
const ofType = (sent, type) => sent.filter(b => b.type === type);

test('a read carries the article and the session; retries keep both, then stop', async () => {
  const b = browser(memoryStorage());
  b.statuses.push(0, 503, 429, 500);
  const tab = b.tab();
  assert.strictEqual((await tab.stats.read(42)).status, 0);
  while (b.timers.length) { b.timers.shift().fn(); await flush(); }
  const r = ofType(b.sent, 'read');
  assert.strictEqual(r.length, 4, 'first try + 3 retries');
  assert.ok(r.every(x => x.article_id === 42 && x.session_id === r[0].session_id));
  assert.deepStrictEqual(Object.keys(r[0]).sort(), ['article_id', 'session_id', 'type']);
});

test('refused reads (400/403/404/415) are not retried', async () => {
  for (const status of [400, 403, 404, 415]) {
    const b = browser(memoryStorage());
    b.statuses.push(status);
    await b.tab().stats.read(1);
    assert.strictEqual(b.timers.length, 0, `status ${status}`);
  }
});

test('reload and other tabs reuse the session; it is replaced after 30 idle minutes', async () => {
  const storage = memoryStorage();
  const b = browser(storage);
  await b.tab().stats.read(1);
  b.advance(10 * 60_000);
  await b.tab().stats.read(1); // reload, or another tab
  const [first, again] = ofType(b.sent, 'read');
  assert.strictEqual(again.session_id, first.session_id);
  b.advance(SESSION_IDLE + 1);
  await b.tab().stats.read(1);
  assert.notStrictEqual(ofType(b.sent, 'read')[2].session_id, first.session_id);
  assert.deepStrictEqual(Object.keys(JSON.parse(storage.map.get(KEY))).sort(), ['sent', 'sid'], 'only the id and a time are stored');
});

test('heartbeats never carry an article and are at most one a minute', async () => {
  const b = browser(memoryStorage());
  const tab = b.tab();
  assert.strictEqual(tab.stats.resumed(), true, 'page opened');
  let answered = false;
  b.sent.length && tab.stats.settled().then(() => { answered = true; });
  assert.strictEqual(tab.stats.tick(), false);
  b.advance(59_000);
  tab.stats.interaction();
  assert.strictEqual(tab.stats.tick(), false);
  b.advance(1_000);
  assert.strictEqual(tab.stats.tick(), true);
  await flush();
  assert.ok(answered, 'settled() resolves after the heartbeat');
  const hb = ofType(b.sent, 'heartbeat');
  assert.strictEqual(hb.length, 2);
  assert.deepStrictEqual(Object.keys(hb[0]).sort(), ['session_id', 'type']);
});

test('tabs of one browser share the session and the one-a-minute limit', () => {
  const b = browser(memoryStorage());
  const t1 = b.tab(), t2 = b.tab();
  assert.strictEqual(t1.stats.resumed(), true);
  assert.strictEqual(t2.stats.resumed(), false, 'the other tab just reported for both');
  b.advance(60_000);
  t2.stats.interaction();
  assert.strictEqual(t2.stats.tick(), true);
  assert.strictEqual(new Set(b.sent.map(x => x.session_id)).size, 1);
});

test('hidden tab or 5 minutes without interaction: no heartbeat', () => {
  const b = browser(memoryStorage());
  const tab = b.tab();
  tab.stats.resumed();
  b.advance(60_000);
  tab.visible = false;
  assert.strictEqual(tab.stats.tick(), false, 'hidden');
  tab.visible = true;
  b.advance(4 * 60_000);
  assert.strictEqual(tab.stats.tick(), false, 'idle');
  assert.strictEqual(tab.stats.interaction(), true, 'interacting again reports at once');
  b.advance(6 * 60_000);
  assert.strictEqual(tab.stats.resumed(), true, 'coming back to the tab');
  assert.strictEqual(tab.stats.resumed(), false);
  assert.strictEqual(ofType(b.sent, 'heartbeat').length, 3);
});

test('blocked storage: reading works, one session per page, not per event', async () => {
  const b = browser(blockedStorage);
  const tab = b.tab();
  tab.stats.resumed();
  for (let i = 0; i < 5; i++) { b.advance(60_000); tab.stats.interaction(); tab.stats.tick(); await tab.stats.read(i + 1); }
  assert.strictEqual(new Set(b.sent.map(x => x.session_id)).size, 1);
});

test('corrupt storage is ignored; a throwing send never escapes', async () => {
  const storage = memoryStorage();
  storage.setItem(KEY, '{"sid":"<script>","sent":"x"}');
  const b = browser(storage);
  await b.tab().stats.read(3);
  assert.match(b.sent[0].session_id, /^[0-9a-f]{32}$/);
  const stats = create({ storage: undefined, now: () => 0, fill: x => x.fill(1), later() {}, visible: () => true, send: () => { throw Error('offline'); } });
  assert.strictEqual((await stats.read(1)).status, 0);
});

test('count: formatted numbers, errors as a dash, never 0', () => {
  assert.strictEqual(count(1234567), '1.234.567');
  assert.strictEqual(count(0), '0');
  for (const bad of [undefined, null, -1, 1.5, NaN, '3']) assert.strictEqual(count(bad), '—');
});
