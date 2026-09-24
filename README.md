# Kino

Self-hosted ROM library management. Organize games and editions, scan ROMs,
review imports, manage covers and metadata, and export to Pegasus or ES-DE.

## Run

```sh
./scripts/build-local.sh
./bin/kino version --json
./bin/kino serve --library /path/to/roms --db /path/to/data/library.db --state /path/to/data
```

The default listener is loopback. Set `GAME_LIBRARY_TOKEN` to a random secret
before listening on a LAN address. The web interface is currently Chinese;
ROM metadata supports multilingual titles.

For a published container, copy `.env.ghcr.example` to `.env`, configure existing
absolute ROM/data directories and a private token, then run:

```sh
docker compose -f compose.ghcr.yaml pull
docker compose -f compose.ghcr.yaml up -d
```

`ghcr.io/kocomic/kino:edge` and `docker.io/kocomic/kino:edge` follow verified
main builds. Pin the published digest for controlled updates. Containers use
UID/GID 10001, mount ROMs read-only, and store mutable state in `/data`.

## Updates and backups

Back up the database and complete state directory before updating. Keep the
same `/data` and `/library` mounts, pull the image and recreate the container.
Updating a registry tag does not restart an installed container.

See [deployment](docs/DEPLOYMENT.md), [API](docs/API.md),
[portable formats](docs/PORTABLE_FORMATS.md), and
[third-party notices](docs/THIRD_PARTY_NOTICES.md).

## Development

```sh
go test -race ./...
go vet ./...
npm ci
npx playwright install chromium
npm run test:e2e
```

Apache-2.0. See [LICENSE](LICENSE).
