# Storage

ROM references stay beneath the configured library root. Managed copies live
in `state/roms`; covers and other media live in `state/media`. Path validation
rejects escapes. Cleanup uses reviewed quarantine and explicit restore APIs.
The database and state directory must reside on a local filesystem.

Use complete-state backups for the SQLite catalog, managed ROMs, media and
recovery records. Referenced external ROMs require a separate backup.
