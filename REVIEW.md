# Review guide

Every PR is reviewed before it is merged (rule in `agent.md`): by PR-Agent
(owner, 22.10.2026); a new Claude agent checks the **Wichtig** of earlier
rounds, and reviews when PR-Agent fails. The
reviewer knows nothing of the session that wrote the change: it gets the
PR's intent, its diff and the repository, and this file. The owner does not read code; this review is the second reader.

## What failure looks like here

Hansei proposes edits to the user's Obsidian notes; the user reviews every
change as a diff, and only accepted changes are written. A bug that
matters is one that touches the vault or private data wrongly, or one the
user sees:

- **Writes wrong:** a change written that was not accepted, written to a
  folder that is not allowed, or written without a journal entry (no
  undo); an undo that does not restore the file.
- **Leaks:** a blocked folder read (also for the index), a local-only
  folder sent to an external provider, a secret leaving the machine
  without its placeholder or shown unmasked in the diff, a key outside
  KWallet or `HANSEI_KEY_<NAME>`.
- **Shows wrong:** a diff that does not match the change that will be
  written, a decision applied to the wrong change, file or batch, a
  version or thread that loses feedback.
- **Breaks a front end:** the KDE app, the TUI or a script out of step
  with `core/service` (JSON-RPC 2.0), a daemon that hangs or crashes.
- **Ships the demo:** demo code or data in a build without `-tags demo` or
  `-DHANSEI_DEMO=ON`.
- **Lies:** README, `ROADMAP.md`, help texts or comments the code
  contradicts.

## What to check in particular

- Layers: the front ends (KDE app, TUI, scripts) go through
  `core/service` only; each layer talks only to the one below it.
- Scope: every read goes through the vault's scope check; every write is
  an accepted change in an allowed folder and gets a journal entry.
- Texts: German strings in `kde/po/de/hansei.po` (`kde/Messages.sh`).
- GUI rule: Kante as it is (`agent.md`), no raw colours or sizes; System
  stays the default style.
- Bug fixes: a test that failed before the fix, and the cause removed,
  not one case hidden.

## Skip

- `kde/qml/Kante/` and `tui/kante/palette.json` (vendored by
  `scripts/sync-kante.sh`; check only that they match Kante).
- `go.sum`, images in `docs/`; style that `gofmt` and `go vet` enforce.

## How to report

- Every finding names file and line and the concrete input or state that
  goes wrong. No guess without evidence; if unsure, say so.
- Each finding gets one label: **Wichtig** (a bug from the list above,
  must be fixed or answered before the merge), **Kleinigkeit** (optional,
  at most five) or **Bestand** (the bug was there before this PR; not
  blocking).
- The report starts with the count, e.g. "2 Wichtig, 1 Kleinigkeit" (the
  session puts the heading above it). The count of **Wichtig** holds every
  one still open, also from an earlier round, e.g. "1 Wichtig (offen aus
  Runde 1)". Only when none is open it says plainly: "Nichts zu beheben."
- In a later round: say for each open **Wichtig** of the last round whether
  it is fixed, or whether its answer is accepted, with the reason. PR-Agent
  sees no earlier round: it reports only its own findings with their labels,
  and a new Claude agent closes the open ones (`agent.md`).
- Findings in German, code names as they are.
