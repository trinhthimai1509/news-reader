// Load generator for the VPS sizing test (docs/DANH-GIA-VPS.md). Runs on the
// developer machine, outside the test stack's RAM budget, and only calls the
// local test stack (never a news site or CDN; images are not fetched).
//
//   node scripts/vps-test/load.mjs SESSIONS SECONDS [BASE] [OUT.json]
//
// Each session acts like a reader: opens the page (HTML, CSS, JS, start-up
// API calls), then every 2–5 s does one action: list, category, country or
// source filter, search, next page, or open an article (+ "read" event).
// Every 60 s it sends a heartbeat and reads /api/stats like the page does.
// THINK=0 (environment) removes the 2–5 s pauses: a stress test that shows
// the peak the stack reaches, not a number of real readers.
import { writeFileSync } from 'node:fs';
import { randomBytes } from 'node:crypto';

const [sessions = 5, seconds = 60] = process.argv.slice(2, 4).map(Number);
const BASE = (process.argv[4] || 'http://127.0.0.1:8083').replace(/\/$/, '');
const OUT = process.argv[5];
const STATIC = ['/', '/style.css', '/seen.js', '/filters.js', '/search.js', '/video.js', '/importer.js', '/stats.js', '/dropdown.js', '/app.js'];
const sleep = ms => new Promise(r => setTimeout(r, ms));
const pick = a => a[Math.floor(Math.random() * a.length)];
const THINK = process.env.THINK !== '0';
const results = []; // [kind, status, ms]
let stop = false;

async function req(kind, path, opts = {}) {
  const t = performance.now();
  let status = 0;
  try {
    const r = await fetch(BASE + path, { ...opts, signal: AbortSignal.timeout(30000) });
    status = r.status;
    await r.arrayBuffer();
  } catch { status = 0; }
  results.push([kind, status, performance.now() - t]);
  return status;
}
async function json(path) { const r = await fetch(BASE + path); return r.json(); }

// Data to choose from: real ids from the copied database.
const cats = (await json('/api/categories')).categories.map(c => c.slug);
const countries = (await json('/api/countries')).countries.filter(c => c.in_use).map(c => c.code);
const sources = (await json('/api/sources')).map(s => s.id);
const ids = [], words = new Set();
for (let p = 1; p <= 5; p++) {
  for (const a of (await json(`/api/articles?page=${p}`)).items) {
    ids.push(a.id);
    for (const w of a.title.split(/\s+/)) if (w.length >= 4 && /^[\p{L}]+$/u.test(w)) words.add(w.toLowerCase());
  }
}
const terms = [...words];
const qs = o => new URLSearchParams({ page: '1', source: '', q: '', category: '', country: '', ...o }).toString();
const post = body => ({ method: 'POST', headers: { 'Content-Type': 'application/json', Origin: BASE }, body: JSON.stringify(body) });

async function session(n) {
  await sleep(Math.random() * Math.min(10000, seconds * 200)); // ramp up
  const sid = randomBytes(16).toString('hex');
  for (const f of STATIC) await req('static', f);
  await Promise.all([req('session', '/api/admin/session'), req('categories', '/api/categories?seen='), req('countries', '/api/countries'), req('sources', '/api/sources')]);
  await req('heartbeat', '/api/stats/events', post({ type: 'heartbeat', session_id: sid }));
  await req('stats', '/api/stats');
  await req('list', '/api/articles?' + qs({}));
  let beat = Date.now(), page = 1;
  while (!stop) {
    await sleep(THINK ? 2000 + Math.random() * 3000 : 0);
    if (stop) break;
    if (Date.now() - beat >= 60000) {
      beat = Date.now();
      await req('heartbeat', '/api/stats/events', post({ type: 'heartbeat', session_id: sid }));
      await req('stats', '/api/stats');
    }
    const x = Math.random();
    if (x < 0.30) {
      const id = pick(ids);
      if ((await req('article', `/api/articles/${id}`)) === 200) await req('read', '/api/stats/events', post({ type: 'read', session_id: sid, article_id: id }));
    } else if (x < 0.50) { page = 1; await req('list-category', '/api/articles?' + qs({ category: pick(cats) })); await req('categories', '/api/categories?seen='); }
    else if (x < 0.60) { page = 1; await req('list-country', '/api/articles?' + qs({ country: pick(countries) })); }
    else if (x < 0.70) { page = 1; await req('list-source', '/api/articles?' + qs({ source: String(pick(sources)) })); }
    else if (x < 0.85) { page = 1; await req('search', '/api/articles?' + qs({ q: pick(terms) })); }
    else if (x < 0.95) { page++; await req('list-page', '/api/articles?' + qs({ page: String(Math.min(page, 20)) })); }
    else { await req('list', '/api/articles?' + qs({})); await req('categories', '/api/categories?seen='); }
  }
}

const started = performance.now();
const running = Array.from({ length: sessions }, (_, i) => session(i));
await sleep(seconds * 1000);
stop = true;
await Promise.all(running);
const elapsed = (performance.now() - started) / 1000;

const pct = (a, p) => { if (!a.length) return null; const s = [...a].sort((x, y) => x - y); return +s[Math.min(s.length - 1, Math.floor(p / 100 * s.length))].toFixed(1); };
function summary(rows) {
  const ms = rows.map(r => r[2]);
  const limited = rows.filter(r => r[1] === 429 && (r[0] === 'read' || r[0] === 'heartbeat')).length;
  const errors = rows.filter(r => !(r[1] >= 200 && r[1] < 400) && !(r[1] === 429 && (r[0] === 'read' || r[0] === 'heartbeat'))).length;
  return { requests: rows.length, errors, stats_rate_limited_429: limited, p50: pct(ms, 50), p95: pct(ms, 95), p99: pct(ms, 99), max: pct(ms, 100) };
}
const byKind = {};
for (const k of new Set(results.map(r => r[0]))) byKind[k] = summary(results.filter(r => r[0] === k));
const statuses = {};
for (const r of results) statuses[r[1]] = (statuses[r[1]] || 0) + 1;
const out = { sessions, think: THINK, seconds: +elapsed.toFixed(1), rps: +(results.length / elapsed).toFixed(1), all: summary(results), statuses, byKind };
console.log(JSON.stringify(out));
if (OUT) writeFileSync(OUT, JSON.stringify(out, null, 1));
