# ROM API

The authoritative contract is `internal/server/openapi.yaml`, served at
`/api/v1/openapi.yaml`. Protected requests require the configured owner token.
`/api/v1/health`, `/health/live`, `/health/ready`, capabilities and discovery
are public. Collections use `data` and `pagination` with `limit` / `offset`.

Domains: games, editions, artifacts, platforms, series, sources, import review,
hash packs, media and managed-storage maintenance. Preview/commit tokens bind
reviewed import candidates to the current source state. Unsupported methods
return 405 with Allow; unknown endpoints return 404 after authentication.

The `/api` library aliases are deprecated; use `/api/v1`.
Pegasus and ES-DE export are administrator CLI operations.

Run `scripts/build-api-docs.sh NEW_DIRECTORY` to render the reference.
