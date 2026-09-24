#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
db_path="$repo_root/.e2e-library.db"
state_path="$repo_root/.e2e-state"

rm -f -- "$db_path" "$db_path-shm" "$db_path-wal"
rm -rf -- "$state_path"

cd "$repo_root"
exec go run ./cmd/kino serve \
  --addr 127.0.0.1:18080 \
  --library ./testdata \
  --db "$db_path" \
  --state "$state_path"
