# SQLite catalog

Kino uses a local SQLite database with foreign keys, WAL journaling and a busy
timeout. The database stores metadata and file references; ROM and media bytes
are stored separately. Keep the database and state directory on a local
filesystem rather than SMB or NFS.

## Library relationships

- Games contain platform identifiers and default titles.
- Editions belong to games and carry version, language and region metadata.
- Artifacts belong to editions and describe ROM paths, roles, sizes and hashes.
- Media belongs to a game or edition and records content identity and status.
- Series group games with explicit membership types and ordering.
- Sources and scans record import configuration and reviewed scan results.
- Custom platforms describe additional platform names, aliases and formats.
- HashPack tables retain published reference identities independently of local ROMs.

Localized titles supplement the default game, edition and series titles.
Foreign keys and transactional writes preserve relationships. Artifact hashes
prevent duplicate content identities. Source paths remain relative to the
configured library root.

## State and backups

| Path | Contents |
|---|---|
| `/data/library.db` | Catalog database |
| `/data/roms/` | Managed ROM copies |
| `/data/media/` | Content-addressed media |
| `/data/recovery/` | Quarantined files and recovery records |
| `/library/` | Externally managed ROM library |

Use `kino backup` for a consistent database snapshot, or `kino backup-state`
for a complete state backup. Do not copy a running database without its WAL
state. Back up externally referenced ROMs separately. Validate snapshots with
`kino check-state`; restore into a separate directory before switching service
configuration.

Schema migrations run transactionally at startup. A database newer than the
program supports is rejected. Restore a matching snapshot before rolling back
to an older binary. `kino db-check` checks database integrity and catalog
constraints without changing ROM files.
