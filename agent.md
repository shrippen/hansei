# Working rules for this repository

The rules for all shrippen repositories are in `RULES.md` (writing, code, commits, bug
fixes, repository, GUI). It is a synced copy from the rules repository; do not edit it
here. This file holds only what is particular to this repository. A Claude Code session on
the owner's machine has the rules through the global `CLAUDE.md` already and reads
`RULES.md` only for its GUI rule, before work on a user interface.

## GUI (this project)

- Hansei: the KDE app vendors `kde/qml/Kante` and the TUI `tui/kante/palette.json` via
  `scripts/sync-kante.sh`. System is the default style; Kante and Kante Light are opt-in.
  The landing page `docs/index.html` links `shrippen.css` and `shrippen.js`.

## Repository (this project)

- CI runs on the GitHub mirror (`.github/workflows/ci.yml`, since 16.10.2026) and reports back to Gitea as the commit status `github / test`.
- The merge waits for: CI is green on the PR's latest commit: the newest `github / test` status there (`.github/workflows/ci.yml` on the GitHub mirror: `make check` and the release gate `scripts/check-release.sh`, on every push, no path filter) is success.
- The owner also confirms: a PR that changes `scripts/check-release.sh`.
- Everything else: `RULES.md` (Repository) and the skill `pr-review`.
