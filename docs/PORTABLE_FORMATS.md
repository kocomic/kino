# Portable library format

## Library manifest v6

`library-manifest.json` describes games, editions, artifacts, media, series,
and any non-built-in platform definitions needed to interpret the package. New
writers emit `format_version: 6` and SHOULD validate against
[`library-manifest-v6.schema.json`](../schemas/library-manifest-v6.schema.json).

Kino retains v4/v5 readers for migration. That is not permission for new
producers to omit v6 integrity and platform fields.

### Layout and identity

- `entries` contains at most 50,000 editions. `game_id` groups editions of the
  same game; each `edition_id` remains an independently hashed/playable item.
- `series` is an optional cross-platform grouping layer. Member relation types
  are `mainline`, `port`, `remake`, `spinoff`, `collection`, or `other`.
- `platform` MUST resolve to a Kino built-in platform or to an exact definition
  in `custom_platforms`. At most 256 custom platforms may appear. Each supplied
  custom platform MUST be used, MUST NOT shadow a built-in or collide with
  another registry key, and MUST be enabled. An existing local definition is
  reusable only when its portable fields match exactly and it remains enabled.
- `game_titles` and `edition_titles` are locale-to-title maps. Default titles
  are portable fallbacks; if only one default title is populated, Kino uses
  it for the other. `edition_type` defaults to `original` when omitted by an
  older producer, but canonical v6 exports include it.

### Artifacts and media

Every entry contains 1 through 64 paths in `artifacts`. V6 keeps this legacy
path list and pairs it positionally with an equally sized `artifact_records`
array. Each record repeats the same path and adds role, disc index, original
name, size, and SHA-256. Artifact roles are `rom`, `disc`, `executable`,
`patch`, `dlc`, `update`, or `other`; `disc_index` is 0 through 64.

Each entry may contain at most 256 media records. `owner_type` is `game` or
`edition`. Supported kinds are `cover`, `box_front`, `box_back`, `box_spine`,
`logo`, `screenshot`, `title_screen`, `background`, `fanart`, `marquee`,
`bezel`, `manual`, `video`, `music`, `cartridge`, `poster`, `banner`, `tile`,
and `other`.

All paths are portable `/`-separated paths resolved beneath the explicitly
configured library root. The manifest itself must be a regular non-symlink file
inside that root and is limited to 16 MiB. Kino rejects traversal and symlink
escapes. Present files are rehashed and their size/hash MUST match the record.
Missing media is skipped. Missing ROM artifacts remain visible during preview
but the affected game is skipped at commit: metadata alone cannot create a
library entry because no content hash can be verified.

The schema covers JSON shape. Kino additionally enforces positional
`artifacts`/`artifact_records` equality, path containment, filesystem integrity,
custom-platform registry identity, series referential integrity, and atomic
catalogue commit.
