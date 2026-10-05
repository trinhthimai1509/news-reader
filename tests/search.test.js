const test = require('node:test');
const assert = require('node:assert');
const { create, latest } = require('../web/search.js');

function harness() {
  const runs = [], queue = [];
  const s = create((q, o) => runs.push({ q, ...o }), fn => queue.push(fn));
  const flush = () => { while (queue.length) queue.shift()(); };
  return { s, runs, flush };
}

test('native X: input + search events give one reload with empty query', () => {
  const { s, runs, flush } = harness();
  s.submit('bão'); flush();
  s.changed(''); s.changed(''); flush(); // input event, then "search" event
  assert.deepStrictEqual(runs, [{ q: 'bão', explicit: true }, { q: '', explicit: false }]);
  assert.strictEqual(s.applied, '');
});

test('Backspace/Delete to empty and whitespace-only count as cleared', () => {
  const { s, runs, flush } = harness();
  s.submit('kinh tế'); flush();
  s.changed('kinh t'); s.changed('k'); flush();
  assert.strictEqual(runs.length, 1, 'no live search while typing');
  s.changed('   '); flush();
  assert.deepStrictEqual(runs[1], { q: '', explicit: false });
  s.submit('  '); flush();
  assert.deepStrictEqual(runs[2], { q: '', explicit: true }, 'whitespace submit searches with q empty');
});

test('clearing an already empty box does nothing', () => {
  const { s, runs, flush } = harness();
  s.changed(''); flush();
  assert.strictEqual(runs.length, 0);
});

test('Enter still searches; events of one keypress coalesce', () => {
  const { s, runs, flush } = harness();
  s.submit('a'); flush();
  s.changed(''); s.submit(''); flush(); // Enter on emptied box: search event + submit
  assert.deepStrictEqual(runs.slice(1), [{ q: '', explicit: true }]);
  s.submit('a'); s.submit('a'); flush();
  assert.strictEqual(runs.length, 3);
});

test('a stale response cannot overwrite a newer one', async () => {
  const l = latest();
  const shown = [];
  const load = (label, ms) => { const fresh = l.next(); return new Promise(r => setTimeout(r, ms)).then(() => { if (fresh()) shown.push(label); }); };
  await Promise.all([load('old q=bão', 30), load('new q=', 5)]);
  assert.deepStrictEqual(shown, ['new q=']);
});
