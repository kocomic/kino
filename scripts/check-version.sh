#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

version_file="internal/buildinfo/VERSION"
test -f "$version_file"
version="$(tr -d '\r\n' < "$version_file")"

if [[ ! "$version" =~ ^0\.[0-9]+\.[0-9]+-preview\.[0-9]+$ ]]; then
  echo "invalid canonical preview version: $version" >&2
  exit 1
fi

check_equal() {
  local label="$1"
  local actual="$2"
  if [[ "$actual" != "$version" ]]; then
    echo "$label version drifted: got '$actual', want '$version'" >&2
    exit 1
  fi
}

package_version="$(sed -n 's/^[[:space:]]*"version": "\([^"]*\)",*$/\1/p' package.json | head -n 1)"
lock_root_version="$(sed -n 's/^[[:space:]]\{2\}"version": "\([^"]*\)",*$/\1/p' package-lock.json | head -n 1)"
lock_package_version="$(sed -n 's/^[[:space:]]\{6\}"version": "\([^"]*\)",*$/\1/p' package-lock.json | head -n 1)"
openapi_version="$(sed -n 's/^[[:space:]]*version: \(.*\)$/\1/p' internal/server/openapi.yaml | head -n 1)"
compose_version="$(sed -n 's/^[[:space:]]*image: "${KINO_IMAGE:-kino:\([^}]*\)}"$/\1/p' compose.yaml | head -n 1)"
synology_compose_version="$(sed -n 's/^[[:space:]]*image: "${KINO_IMAGE:-kino:\([^}]*\)}"$/\1/p' compose.synology.yaml | head -n 1)"

check_equal "package.json" "$package_version"
check_equal "package-lock root" "$lock_root_version"
check_equal "package-lock package" "$lock_package_version"
check_equal "OpenAPI" "$openapi_version"
check_equal "Compose image" "$compose_version"
check_equal "Synology Compose image" "$synology_compose_version"

echo "version_identity=passed version=$version"
