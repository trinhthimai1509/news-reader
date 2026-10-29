#!/bin/sh
# Runs one measured scenario against the test stack and prints a summary.
#   sh scripts/vps-test/run.sh OUTDIR NAME SESSIONS SECONDS
# SESSIONS=0 measures without load (idle, worker only) for SECONDS.
# THINK=0 in the environment: no pauses between actions (stress test).
# Writes OUTDIR/NAME.samples.csv, NAME.load.json, NAME.dbconn.txt, NAME.summary.json.
set -eu
DIR=$1 NAME=$2 N=$3 SECS=$4
HERE=$(dirname "$0")
mkdir -p "$DIR"
sh "$HERE/sample.sh" "$DIR/$NAME.samples.csv" & SAMPLER=$!
# One long-lived psql counts client connections every 5 s (itself excluded).
printf "SELECT now()::time(0), count(*) FILTER (WHERE backend_type='client backend' AND pid<>pg_backend_pid()) FROM pg_stat_activity \\\\watch 5\n" |
  docker exec -i nr-vps-db psql -U news -d news -At > "$DIR/$NAME.dbconn.txt" 2>/dev/null & WATCH=$!
sleep 3
FROM=$(date +%s)
if [ "$N" -gt 0 ]; then node "$HERE/load.mjs" "$N" "$SECS" http://127.0.0.1:8083 "$DIR/$NAME.load.json" > /dev/null
else sleep "$SECS"; echo '{}' > "$DIR/$NAME.load.json"; fi
TO=$(date +%s)
sleep 2
kill $SAMPLER $WATCH 2>/dev/null || true
pkill -f "colima ssh -- sh -c" 2>/dev/null || true
docker exec nr-vps-db pkill -f "psql -U news -d news -At" 2>/dev/null || true
CONN=$(awk -F'|' '{print $2}' "$DIR/$NAME.dbconn.txt" | sort -n | tail -1)
RESTARTS=$(docker inspect -f '{{.RestartCount}}/{{.State.OOMKilled}}/{{.State.Status}}' nr-vps-app)
node "$HERE/summarize.mjs" "$DIR/$NAME.samples.csv" "$FROM" "$TO" > "$DIR/$NAME.res.json"
node -e '
const [l,r,conn,restarts,name]=process.argv.slice(1);
const load=JSON.parse(require("fs").readFileSync(l)),res=JSON.parse(require("fs").readFileSync(r));
const s={name,load:load.all?{sessions:load.sessions,seconds:load.seconds,requests:load.all.requests,rps:load.rps,errors:load.all.errors,stats429:load.all.stats_rate_limited_429,p50:load.all.p50,p95:load.all.p95,p99:load.all.p99,max:load.all.max}:null,db_conn_max:+conn||0,app_restart_oom_state:restarts,...res};
require("fs").writeFileSync(l.replace(".load.json",".summary.json"),JSON.stringify(s));console.log(JSON.stringify(s));' \
  "$DIR/$NAME.load.json" "$DIR/$NAME.res.json" "${CONN:-0}" "$RESTARTS" "$NAME"
