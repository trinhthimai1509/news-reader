const test = require('node:test');
const assert = require('node:assert');
const { move, typeahead, fold } = require('../web/dropdown.js');

const opts = (...labels) => labels.map(l => ({ label: l.replace(/^-/, ''), disabled: l.startsWith('-') }));

test('arrow keys move one step, skip disabled options and stop at the ends', () => {
  const o = opts('A', '-B', 'C', 'D');
  assert.strictEqual(move(o, 0, 'ArrowDown'), 2);
  assert.strictEqual(move(o, 2, 'ArrowUp'), 0);
  assert.strictEqual(move(o, 3, 'ArrowDown'), 3, 'stays on the last');
  assert.strictEqual(move(o, 0, 'ArrowUp'), 0, 'stays on the first');
  assert.strictEqual(move(o, -1, 'ArrowDown'), 0, 'nothing active: first enabled');
  assert.strictEqual(move(o, -1, 'ArrowUp'), 3);
});

test('Home, End, PageUp, PageDown', () => {
  const o = opts(...Array.from({ length: 20 }, (_, i) => `O${i}`));
  assert.strictEqual(move(o, 5, 'Home'), 0);
  assert.strictEqual(move(o, 5, 'End'), 19);
  assert.strictEqual(move(o, 5, 'PageDown'), 13);
  assert.strictEqual(move(o, 15, 'PageDown'), 19);
  assert.strictEqual(move(o, 13, 'PageUp'), 5);
  assert.strictEqual(move(o, 3, 'PageUp'), 0);
  assert.strictEqual(move(opts('-A', 'B', '-C'), 1, 'Home'), 1);
});

test('type-ahead ignores Vietnamese accents and case, cycles on a repeated letter', () => {
  const o = opts('Tất cả quốc gia', 'Việt Nam', 'Vương quốc Anh', 'Đức', 'Thái Lan');
  assert.strictEqual(typeahead(o, 0, 'viet'), 1);
  assert.strictEqual(typeahead(o, 0, 'VUONG'), 2);
  assert.strictEqual(typeahead(o, 0, 'd'), 3, 'đ matches d');
  assert.strictEqual(typeahead(o, 1, 'v'), 2, 'next V after the current one');
  assert.strictEqual(typeahead(o, 2, 'vv'), 1, 'repeating cycles');
  assert.strictEqual(typeahead(o, 0, 'th'), 4);
  assert.strictEqual(typeahead(o, 0, 'xyz'), -1);
  assert.strictEqual(typeahead(opts('-Việt Nam', 'Vương quốc Anh'), -1, 'v'), 1, 'disabled skipped');
  assert.strictEqual(fold('Khoa học công nghệ'), 'khoa hoc cong nghe');
});
