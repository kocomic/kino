#!/usr/bin/env bash
set -euo pipefail
image=${1:?image required}
name="kino-rom-smoke-$$"
cleanup() { docker rm -fv "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT
# Use the real entrypoint, auth, health handler, and embedded UI.
docker run -d --name "$name" -e GAME_LIBRARY_TOKEN=smoke-private-token "$image" >/dev/null
ready=false
for attempt in {1..30}; do
  if docker exec "$name" wget -qO- http://127.0.0.1:8080/api/v1/health/ready | grep -q '"ready"'; then ready=true; break; fi
  sleep 1
done
$ready || { docker logs "$name"; exit 1; }
docker exec "$name" wget -qO- --header='Authorization: Bearer smoke-private-token' http://127.0.0.1:8080/api/v1/games | grep -q '"pagination"'
docker exec "$name" wget -qO- http://127.0.0.1:8080/ | grep -q 'ROM Library'
for path in save-streams sync/sessions devices web-emulation/readiness web-netplay/readiness packages; do
  output=$(docker exec "$name" wget -S -O /dev/null --header='Authorization: Bearer smoke-private-token' "http://127.0.0.1:8080/api/v1/$path" 2>&1 || true)
  printf '%s' "$output" | grep -q '404 Not Found'
done
printf 'rom_container_smoke=passed\n'
