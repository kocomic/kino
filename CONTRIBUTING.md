# Contributing

This repository covers ROM library management. Follow the development checks
in README.md.
Do not commit ROMs, user state, credentials or generated databases. Existing
synthetic fixtures are checked by `scripts/check-source-hygiene.sh`.

Keep portable metadata backward compatible, validate paths before file access,
and preserve data during migrations. Include focused regression tests for
behavior changes. See docs/CONTRIBUTION_RIGHTS.md and LICENSE.
