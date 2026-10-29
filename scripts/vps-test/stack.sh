#!/bin/sh
# Separate test stack for the VPS sizing measurements (docs/DANH-GIA-VPS.md).
# It never touches the real stack: its own network (nr-vps), volume
# (nr-vps-pgdata), containers (nr-vps-db, nr-vps-app) and port 8083.
#
#   sh scripts/vps-test/stack.sh up 1g|2g DUMPFILE   start with a RAM budget, restore DUMPFILE
#   sh scripts/vps-test/stack.sh app 1g|2g           (re)create only the app with that budget
#   sh scripts/vps-test/stack.sh down                remove containers, network and volume
#
# RAM budgets (container limits, no swap: --memory-swap = --memory):
#   1g: PostgreSQL 384 MiB, app 192 MiB  (≈ 448 MiB left for OS, Docker, proxy)
#   2g: PostgreSQL 768 MiB, app 384 MiB  (≈ 896 MiB left)
# The app runs like compose.yaml (read-only, no capabilities, init).
set -eu
IMAGE=${IMAGE:-news-reader-app:latest}
PW=tam-thoi-chi-de-thu   # throw-away password of the test database only
budget() {
  case "$1" in
    1g) DB_MEM=384m APP_MEM=192m ;;
    2g) DB_MEM=768m APP_MEM=384m ;;
    *) echo "budget must be 1g or 2g" >&2; exit 2 ;;
  esac
}
start_app() {
  docker rm -f nr-vps-app >/dev/null 2>&1 || true
  docker run -d --name nr-vps-app --network nr-vps -p 127.0.0.1:8083:8080 \
    --memory "$APP_MEM" --memory-swap "$APP_MEM" \
    --read-only --cap-drop ALL --security-opt no-new-privileges:true --init \
    -e DATABASE_URL="postgres://news:$PW@nr-vps-db:5432/news?sslmode=disable" \
    -e LISTEN_ADDR=:8080 -e POLL_INTERVAL="${POLL_INTERVAL:-1m}" -e FULL_TEXT_ENABLED=true -e COOKIE_SECURE=false \
    "$IMAGE" >/dev/null
  for i in $(seq 1 60); do
    if curl -fs -o /dev/null http://127.0.0.1:8083/healthz; then echo "app ready after ~${i}x0.5s"; return; fi
    sleep 0.5
  done
  echo "app not healthy" >&2; exit 1
}
case "${1:-}" in
  up)
    budget "$2"; DUMP=$3
    docker network create nr-vps >/dev/null
    docker volume create nr-vps-pgdata >/dev/null
    docker run -d --name nr-vps-db --network nr-vps --memory "$DB_MEM" --memory-swap "$DB_MEM" \
      -e POSTGRES_DB=news -e POSTGRES_USER=news -e POSTGRES_PASSWORD=$PW \
      -v nr-vps-pgdata:/var/lib/postgresql/data postgres:17-alpine >/dev/null
    until docker exec nr-vps-db pg_isready -U news -d news >/dev/null 2>&1; do sleep 1; done
    sleep 2
    docker exec -i nr-vps-db pg_restore -U news -d news --no-owner < "$DUMP"
    # No real crawling by default: every feed off, so the test worker does not
    # fetch the news sites the real worker already reads.
    docker exec nr-vps-db psql -U news -d news -qc "UPDATE sources SET enabled=false"
    start_app ;;
  app) budget "$2"; start_app ;;
  down)
    docker rm -f nr-vps-app nr-vps-db >/dev/null 2>&1 || true
    docker network rm nr-vps >/dev/null 2>&1 || true
    docker volume rm nr-vps-pgdata >/dev/null 2>&1 || true ;;
  *) sed -n '2,15p' "$0"; exit 2 ;;
esac
