const test = require('node:test');
const assert = require('node:assert');
const { reportHref, outcome } = require('../web/importer.js');

test('mailto has only the trimmed URL, encoded', () => {
  const url = 'https://example.com/tin tức?a=1&b=2#x+y%20';
  const href = reportHref(`  ${url} \n`);
  assert.ok(href.startsWith('mailto:trinhthimai1509@gmail.com?subject='));
  const q = new URLSearchParams(href.slice(href.indexOf('?') + 1).replace(/\+/g, '%2B'));
  assert.strictEqual(q.get('subject'), 'Lỗi import');
  assert.strictEqual(q.get('body'), url);
  assert.deepStrictEqual([...q.keys()], ['subject', 'body'], 'no other fields');
  assert.ok(!/token|Bearer|localStorage/i.test(href));
  assert.strictEqual(decodeURIComponent(href.split('&body=')[1]), url);
});

test('report button only for failures', () => {
  assert.strictEqual(outcome(200, { status: 'ok', message: 'x', added: [{ name: 'A' }] }).report, false);
  assert.strictEqual(outcome(200, { status: 'exists', message: 'x', existing: [{ name: 'A' }] }).report, false);
  assert.strictEqual(outcome(200, { status: 'partial', message: 'x', added: [{ name: 'A' }], failed: [{ name: 'B', error: '404' }] }).report, true);
  assert.strictEqual(outcome(200, { status: 'failed', message: 'x', failed: [{ name: 'B' }] }).report, true);
  assert.strictEqual(outcome(422, { status: 'rejected', message: 'Nguồn này chưa được hỗ trợ.' }).report, true);
  const auth = outcome(401, { error: 'Mã truy cập không đúng' });
  assert.strictEqual(auth.report, false);
  assert.strictEqual(auth.tone, 'auth');
  assert.match(auth.text, /đăng nhập lại/);
  assert.strictEqual(outcome(429, { error: 'Đang có một lượt import khác' }).report, false);
  const p = outcome(200, { status: 'partial', message: 'm', added: [{ name: 'A' }], existing: [{ name: 'C', note: 'đã bật lại' }], failed: [{ name: 'B', error: 'HTTP 404' }] });
  assert.deepStrictEqual(p.details, ['Đã thêm: A', 'Đã có: C (đã bật lại)', 'Lỗi: B — HTTP 404']);
});
