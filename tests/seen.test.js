// Run: node --test tests/
const test = require('node:test');
const assert = require('node:assert');
const { create, loadAndMark, isNew, KEY } = require('../web/seen.js');

function storage() {
  const m = new Map();
  return { getItem: k => (m.has(k) ? m.get(k) : null), setItem: (k, v) => m.set(k, v), m };
}
const slugs = ['thoi-su', 'the-gioi', 'khac'];

test('first visit uses current data as baseline, not all history', () => {
  const s = create(storage());
  assert.strictEqual(s.param(), '');
  s.sync(slugs, 500);
  assert.deepStrictEqual(s.snapshot(), { 'thoi-su': 500, 'the-gioi': 500, khac: 500 });
});

test('seen state survives reload (same storage)', () => {
  const st = storage();
  const a = create(st);
  a.sync(slugs, 500);
  a.markSeen('the-gioi', 520, { userAction: true });
  const b = create(st);
  assert.strictEqual(b.cursor('the-gioi'), 520);
  assert.match(b.param(), /the-gioi:520/);
  b.sync(slugs, 600); // sync never moves an existing cursor forward
  assert.strictEqual(b.cursor('the-gioi'), 520);
});

test('failed request does not mark seen', async () => {
  const s = create(storage());
  s.sync(slugs, 500);
  await assert.rejects(loadAndMark(s, async () => { throw new Error('HTTP 500'); }, 'the-gioi', { userAction: true }));
  assert.strictEqual(s.cursor('the-gioi'), 500);
});

test('background refresh, all view and filtered lists do not mark seen', async () => {
  const s = create(storage());
  s.sync(slugs, 500);
  const ok = async () => ({ cursor: 900, items: [] });
  await loadAndMark(s, ok, 'the-gioi', { userAction: false });
  await loadAndMark(s, ok, '', { userAction: true });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, query: 'bão' });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, source: '3' });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, country: 'GB' });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, country: 'unknown' });
  assert.deepStrictEqual(s.snapshot(), { 'thoi-su': 500, 'the-gioi': 500, khac: 500 });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true });
  assert.strictEqual(s.cursor('the-gioi'), 900);
  assert.strictEqual(s.cursor('thoi-su'), 500, 'other categories keep their own cursor');
});

test('articles after the seen cursor stay new; late published articles count by id', () => {
  const s = create(storage());
  s.sync(slugs, 500);
  s.markSeen('the-gioi', 700, { userAction: true });
  const c = s.snapshot();
  assert.strictEqual(isNew({ id: 701, categories: ['the-gioi'], published_at: '2020-01-01T00:00:00Z' }, c), true);
  assert.strictEqual(isNew({ id: 650, categories: ['the-gioi'] }, c), false);
  assert.strictEqual(isNew({ id: 650, categories: ['the-gioi', 'thoi-su'] }, c), true, 'new in a category not yet opened');
});

test('cursor never moves backwards; restored database clamps stored cursors', () => {
  const s = create(storage());
  s.sync(slugs, 500);
  s.markSeen('the-gioi', 800, { userAction: true });
  s.markSeen('the-gioi', 600, { userAction: true });
  assert.strictEqual(s.cursor('the-gioi'), 800);
  s.sync(slugs, 300);
  assert.strictEqual(s.cursor('the-gioi'), 300);
});

test('corrupt or unavailable storage falls back safely', () => {
  const st = storage();
  st.setItem(KEY, '{not json');
  const s = create(st);
  assert.strictEqual(s.param(), '');
  const broken = { getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); } };
  const b = create(broken);
  b.sync(slugs, 7);
  assert.strictEqual(b.cursor('khac'), 7, 'works in memory when storage is blocked');
  const quota = { getItem: () => null, setItem() { throw new Error('quota'); } };
  const q = create(quota);
  q.sync(slugs, 5);
  assert.strictEqual(q.cursor('khac'), 5, 'kept in memory when saving fails');
});
