# Hansei

反省, reflection. An AI proposes edits to your Obsidian notes in batches by topic; you review every
change as a diff, give feedback, and only what you accept is written. Every write can be undone.

Built for the homelab/IT documentation in an Obsidian vault, usable for any folder you allow.

- **Review line:** batches on the left, the diff side by side (old and new scroll as one, deletions
  face a hatched filler), the conversation with the AI on the right. Accept, reject, edit or give
  feedback per change, per file or per batch. Keyboard first.
- **Workbench:** the same batches as a board, from rule findings to “done”.
- **Feedback loop:** every round is a version; repeated feedback becomes a rulebook suggestion.
- **Checks without AI:** plain-text secrets, old code names, required frontmatter, old review
  dates, dead wikilinks, compose copies. Code names with a known replacement are fixed without AI.
- **Scope:** only allowed folders are read; blocked folders are never read, not even for the index;
  local-only folders never reach external providers. Secrets are replaced by placeholders before
  anything leaves the machine, and masked in the diff.
- **Providers:** Claude (official SDK, server-side refusal fallback) and any OpenAI-compatible
  endpoint (Ollama, LM Studio). Keys live in KWallet (Secret Service) or `HANSEI_KEY_<NAME>`.
- **Three front ends, one core:** KDE app (Kirigami), terminal UI, scripts. System style is the
  default; Kante and Kante Light are opt-in.

```
hansei-kde (C++/QML) ─┐                    ┌─ vault (scope, index, writes) ─ journal (undo)
hansei (TUI) ─────────┼─ core/service ─────┼─ batches (versions, decisions, thread)
scripts, andon ───────┘   JSON-RPC 2.0     ├─ rules (checks, rulebooks)
                          Unix socket       └─ agent ─ llm (Claude, OpenAI-compatible)
```

## Install

```sh
make build && install -m755 bin/hansei ~/.local/bin/     # Go 1.26+
make kde && sudo cmake --install kde/build                 # Qt 6.6+, KF 6.8+, Kirigami
hansei init --vault ~/Obsidian/Vault --allow IT --block "Diary,Therapie Journal"
```

The KDE app starts the daemon (`hansei daemon`) when needed and offers the same first-run setup.
Keys: `hansei key set claude` (stdin) or in the settings.

## Use

| | |
|---|---|
| `hansei-kde` | the app; `hansei-kde FOLDER` opens a task for that folder (Dolphin: *Review with Hansei…*) |
| `hansei` | terminal UI, same keys as the app (`?` for help) |
| `hansei befunde [--json]` | rule findings |
| `hansei auftrag [--in IT/Netzwerk] [--wait] TEXT` | start a task, e.g. from `contrib/hansei-befunde.timer` |
| `hansei batches [--json]` | list batches |
| `hansei import` | read the old `review-queue/` (`import_queue` in the config) |

Keys in the review line: `A` accept · `R` reject (with reason) · `E` edit · `F` feedback ·
`J/K` next/previous change · `Shift+J/K` file · `Shift+A/X` whole file · `V` version ·
`Shift+V` history · `M` layout · `U` undo · `Shift+R` propose again · `N` new task · `W` workbench.

Config: `~/.config/hansei/config.yaml` (`HANSEI_CONFIG`), data: `~/.local/share/hansei` (`HANSEI_DATA`),
socket: `$XDG_RUNTIME_DIR/hansei.sock` (`HANSEI_SOCKET`).

## Develop

```sh
make check          # gofmt, vet, go test -race
make release        # scripts/check-release.sh: builds what ships, fails on demo leftovers
scripts/sync-kante.sh   # vendors the Kante QML module and palette
kde/Messages.sh     # extracts strings; German in kde/po/de/hansei.po
```

The demo (internal, for screenshots, never shipped) uses the Studio Weber IT docs from the shared
demo world: `demo/start.sh [de|en] [app|tui]`. It exists only with `go build -tags demo` and
`-DHANSEI_DEMO=ON`; `demo/shots.json` drives `shrippen.github.io/demo/tools/screenshots.py`.

Working rules: [agent.md](agent.md). Plan: [ROADMAP.md](ROADMAP.md).
