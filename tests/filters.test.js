// Run: node --test tests/
const test = require('node:test');
const assert = require('node:assert');
const { listParams, countryOptions, countryName, sourcesForCountry, keepSource, UNKNOWN } = require('../web/filters.js');
const { create, loadAndMark } = require('../web/seen.js');

const sources = [
  { id: 1, name: 'VnExpress', adapter: 'vnexpress', country: 'VN' },
  { id: 2, name: 'BBC News', adapter: 'bbc', country: 'GB' },
  { id: 3, name: 'Tuổi Trẻ', adapter: 'tuoitre', country: 'VN' },
  { id: 4, name: 'Nguồn mới', adapter: 'x', country: '' },
];
const countries = { countries: [{ code: 'VN', name: 'Việt Nam', in_use: true }, { code: 'GB', name: 'Vương quốc Anh', in_use: true }, { code: 'TH', name: 'Thái Lan', in_use: false }], unknown_in_use: true };

test('list query combines page, category, source, country and keyword', () => {
  const p = listParams({ page: 3, category: 'the-gioi', source: '2', country: 'GB', query: 'bầu cử' });
  assert.deepStrictEqual(Object.fromEntries(p), { page: '3', source: '2', q: 'bầu cử', category: 'the-gioi', country: 'GB' });
  assert.deepStrictEqual(Object.fromEntries(listParams({})), { page: '1', source: '', q: '', category: '', country: '' });
});

test('reader offers only countries in use, then "Chưa xác định"', () => {
  assert.deepStrictEqual(countryOptions(countries).map(o => o.value), ['', 'VN', 'GB', UNKNOWN]);
  assert.strictEqual(countryOptions(countries).at(-1).label, 'Chưa xác định');
  assert.deepStrictEqual(countryOptions({ countries: [], unknown_in_use: false }).map(o => o.value), ['']);
  assert.strictEqual(countryName('', countries.countries), 'Chưa xác định', 'no country is never guessed');
  assert.strictEqual(countryName('TH', countries.countries), 'Thái Lan');
});

test('source filter follows the country; a source of another country is dropped', () => {
  assert.deepStrictEqual(sourcesForCountry(sources, 'VN').map(s => s.id), [1, 3]);
  assert.deepStrictEqual(sourcesForCountry(sources, UNKNOWN).map(s => s.id), [4]);
  assert.strictEqual(sourcesForCountry(sources, '').length, 4);
  assert.strictEqual(keepSource(sources, 'VN', '3'), '3');
  assert.strictEqual(keepSource(sources, 'GB', '3'), '');
  assert.strictEqual(keepSource(sources, '', '3'), '3');
});

test('opening a category while a country is selected does not mark it seen', async () => {
  const s = create();
  s.sync(['the-gioi', 'thoi-su'], 100);
  const ok = async () => ({ cursor: 250, items: [] });
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, country: 'VN' });
  assert.strictEqual(s.cursor('the-gioi'), 100);
  await loadAndMark(s, ok, 'the-gioi', { userAction: true, country: '' });
  assert.strictEqual(s.cursor('the-gioi'), 250, 'without filters it is marked as before');
});
