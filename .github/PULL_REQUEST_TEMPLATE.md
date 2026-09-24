## What changed

Describe the user-visible behavior and why this change belongs in Kino.

## Compatibility and data safety

- [ ] I described any API, database, manifest, package, or client compatibility impact.
- [ ] This change does not move, rename, overwrite, or delete user ROMs, media, or backups without an explicit reviewed workflow.
- [ ] New migrations are one-way, tested from an older database, and preserve existing records.

## Source and license

- [ ] I have the right to submit every code, documentation, data, and visual change under Apache-2.0.
- [ ] I identified all third-party or generated material that affects provenance review.
- [ ] This change contains no commercial ROM, BIOS, firmware, key, credential, private path, or unapproved media.

Relevant source or license notes:

<!-- Write “None” when this change is entirely original and has no external material. -->

## Verification

List the commands and manual flows actually run. Distinguish automated checks and manual validation.

- [ ] `./scripts/check-source-hygiene.sh`
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] Relevant browser, container, migration, import/export, or NAS acceptance was run and reported above.

## Screenshots

For UI changes, include desktop and 390 px captures in every affected theme. Remove tokens, paths, ROM names, library identifiers, and other private content first.
