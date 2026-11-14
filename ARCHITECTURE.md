# Architecture

How this repository is meant to be built. Where the code differs today is listed under
"Known deviations". Change this file in the PR that changes the structure.

## What it does

Hansei lets an AI propose changes to the notes of an Obsidian vault and lets the user
review them change by change: accept, reject with a reason, edit, give feedback. It
also checks notes against rules that need no AI. One daemon does the work; a KDE app,
a terminal interface and scripts are its front ends.

## Building blocks

| Path | Its one task |
|---|---|
| `core/service/` | The one door for every front end (`API`): tasks and AI runs, review decisions, findings, start page, settings. |
| `core/rpc/` | The same `API` over the daemon's Unix socket (JSON-RPC 2.0): server, client, the list of methods. |
| `core/vault/` | The vault: which folders are allowed or blocked, the index, every read and write of a note. |
| `core/journal/` | The record of every write to the vault, for undo. |
| `core/batch/` | Proposals grouped by topic: files, versions, decisions per change, the thread with the AI; their store on disk. |
| `core/diff/` | Comparing two texts into changes and rows a front end can draw. |
| `core/agent/`, `core/llm/` | Turning a task or feedback into proposals; the providers (Claude, OpenAI-compatible, the scripted one of the demo). The agent reads the vault, never writes. |
| `core/rules/`, `core/redact/` | The checks without AI and the rulebook notes; finding and masking secrets before text leaves the machine. |
| `core/config/`, `core/secrets/`, `core/stats/`, `core/i18n/` | Settings file; API keys in the keyring; the daily history; the few texts the core itself produces. |
| `core/testvault/` | Small vaults for tests. |
| `tui/` | The terminal interface: screens, keys, drawing. `tui/kante/` is a synced copy of the palette. |
| `kde/src/`, `kde/qml/` | The KDE app: the C++ client of the socket, the pages and their components. `kde/qml/Kante/` is a synced copy of the design system, never edited here. |
| `cmd/hansei/`, `internal/cli/` | The program `hansei`: daemon, terminal interface and the commands for scripts; the hook for extra commands. |
| `internal/demo/`, `demo/`, `kde/demo/` | The demo world with a scripted AI, only in a demo build; its start and screenshots. |
| `contrib/`, `kde/dolphin/`, `scripts/`, `docs/`, `tools/` | systemd units; the Dolphin action; release gate and Kante sync; landing page; the structure check (synced copy). |

## Layers

```
 kde/ (C++ client, QML)      tui/           cmd/hansei (commands for scripts)
        │ socket               │ in process or socket        │
        ▼                      ▼                             ▼
 core/rpc  ─────────────►  core/service  (API: the one door)
                               │
      ┌────────────┬───────────┼─────────────┬──────────────┐
      ▼            ▼           ▼             ▼              ▼
 core/agent    core/batch   core/rules   core/journal   config, secrets,
      │        (uses diff)  (uses redact)               stats, i18n
      ▼            │           │
 core/llm          └─────┬─────┘
      │                  ▼
      ▼             core/vault ─► the notes on disk
 the AI provider
 (agent reads core/vault too, through its tools)
```

- A front end knows the core through `service.API` only, in process or over the
  socket. The types of `core/batch` and `core/diff` are the words both sides share;
  a front end calls none of their functions that read or write.
- A front end displays and takes input. Every decision about the review (which
  change comes next, which file opens, what is folded, which reasons exist) is made
  in the core and comes with the answer; the two front ends do not each rebuild it.
- Every read and write of the vault goes through `core/vault`, which keeps blocked
  folders out. The service records every write in the journal, with the text before
  and after.
- The agent proposes and never writes; only an accepted change reaches the vault,
  through the service.
- Text is redacted before it leaves the machine; only `core/llm` talks to a provider.
- `API`, the method list of `core/rpc` and the client stay equal: a new call is added
  to all three.
- An error while saving is reported to the front end and never dropped.
- A setting is carried over at one place when it is saved.
- One file per screen in a front end; a file's name says its topic.
- Demo code is in the build only with the tag `demo`; the release gate checks it.

## Where new things go

| Change | Touch |
|---|---|
| A new call for the front ends | `core/service/api.go` and its implementation, `core/rpc/methods.go` and `client.go`; then `kde/src/client.*` or the QML store, and `tui/` |
| A rule of the review (order, folding, reasons) | `core/service/` or `core/batch/`, with a test; the front ends only show it |
| A new check without AI | `core/rules/`, with a test |
| A new AI provider | `core/llm/` |
| A new tool the AI may use | `core/agent/tools.go`; it reads through `core/vault` |
| A new setting | `core/config/`, `core/service/settings.go`, the settings page in `kde/qml/` and `tui/settings.go` |
| A new screen | A page in `kde/qml/` and a file in `tui/` |
| A new command for scripts | `cmd/hansei/` |
| A design element that is missing | Kante, then `scripts/sync-kante.sh`; never the copies here |

## Known deviations

- Both front ends decide themselves which change and file come next and what is
  folded; they already differ (`AUDIT.md`, finding 1).
- The quick reasons stand in three places, differently; the core keeps no list
  (`AUDIT.md`, finding 2).
- Saving the settings carries every value over twice by hand (`AUDIT.md`, finding 3).
- The folder tree of the settings reads the vault from disk itself, past `core/vault`
  (`AUDIT.md`, finding 4).
- Errors while saving a batch are dropped in seven places (`AUDIT.md`, finding 5).
- `tui/update.go` holds the keys of all screens, `tui/parity.go` parts of five
  screens (`AUDIT.md`, finding 7).
- `core/service/tasks.go` holds AI runs and, beside them, findings, start page and
  import (`AUDIT.md`, finding 8).
