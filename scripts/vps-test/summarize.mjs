// Summarises a sample.sh CSV for a time window.
//   node scripts/vps-test/summarize.mjs SAMPLES.csv [FROM_UNIX] [TO_UNIX]
// Memory in MiB. "cur" is everything the kernel charges to the container
// (process memory + page cache + PostgreSQL shared buffers); "anon" is process
// memory only; "file" is page cache + shared buffers (mostly reclaimable,
// except shared buffers in use). CPU in % of one core (200% = both cores).
import { readFileSync } from 'node:fs';
const [file, from = 0, to = Infinity] = process.argv.slice(2);
const rows = readFileSync(file, 'utf8').trim().split('\n').slice(1).map(l => l.split(',').map(Number))
  .filter(r => r.length === 12 && r[0] >= +from && r[0] <= +to);
if (rows.length < 2) { console.log(JSON.stringify({ samples: rows.length })); process.exit(); }
const MiB = b => +(b / 1048576).toFixed(1);
const col = i => rows.map(r => r[i]);
const max = a => Math.max(...a), avg = a => a.reduce((x, y) => x + y, 0) / a.length;
function cpu(i) {
  const pct = [];
  for (let k = 1; k < rows.length; k++) { const dt = rows[k][0] - rows[k - 1][0]; if (dt > 0) pct.push((rows[k][i] - rows[k - 1][i]) / 1e6 / dt * 100); }
  return { avg: +avg(pct).toFixed(1), max: +max(pct).toFixed(1) };
}
const out = {
  seconds: rows.at(-1)[0] - rows[0][0], samples: rows.length,
  app: { cur_max: MiB(max(col(1))), anon_max: MiB(max(col(2))), file_max: MiB(max(col(3))), cur_end: MiB(rows.at(-1)[1]), cpu: cpu(4), oom_kills: rows.at(-1)[5] - rows[0][5] },
  db: { cur_max: MiB(max(col(6))), anon_max: MiB(max(col(7))), file_max: MiB(max(col(8))), cur_end: MiB(rows.at(-1)[6]), cpu: cpu(9), oom_kills: rows.at(-1)[10] - rows[0][10] },
  vm_avail_min_mib: +(Math.min(...col(11)) / 1024).toFixed(0),
};
console.log(JSON.stringify(out));
