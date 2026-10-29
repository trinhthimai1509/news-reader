#!/bin/sh
# Samples memory and CPU of the test containers every second, read directly
# from the kernel (cgroup v2) inside the Colima VM, plus the VM's available
# memory. Runs in the VM, outside the containers' budgets.
#
#   sh scripts/vps-test/sample.sh OUT.csv &      # stop with kill
#
# Columns: unix time, then per container (app, db): memory.current (all
# charged memory incl. page cache), anon (process memory), file (page cache
# + PostgreSQL shared buffers), cpu usage µs (cumulative), oom_kill count;
# then VM MemAvailable (kB).
set -eu
OUT=$1
APP=$(docker inspect -f '{{.Id}}' nr-vps-app)
DB=$(docker inspect -f '{{.Id}}' nr-vps-db)
echo "t,app_cur,app_anon,app_file,app_cpu_us,app_oom,db_cur,db_anon,db_file,db_cpu_us,db_oom,vm_avail_kb" > "$OUT"
colima ssh -- sh -c '
  r() { d=/sys/fs/cgroup/docker/$1
    printf "%s,%s,%s,%s,%s" "$(cat $d/memory.current)" "$(awk "/^anon /{print \$2}" $d/memory.stat)" "$(awk "/^file /{print \$2}" $d/memory.stat)" "$(awk "/^usage_usec/{print \$2}" $d/cpu.stat)" "$(awk "/^oom_kill /{print \$2}" $d/memory.events)"; }
  while true; do
    printf "%s,%s,%s,%s\n" "$(date +%s)" "$(r '"$APP"')" "$(r '"$DB"')" "$(awk "/MemAvailable/{print \$2}" /proc/meminfo)"
    sleep 1
  done' >> "$OUT"
