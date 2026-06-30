# terva-tasks

A native-style task/todo system for the [terva](https://github.com/terva-sh/terva)
agent harness, shipped as an extension. It gives the agent four tools and a live
panel, with a separate task list per session:

- `task_list` — return the current list (reorient / resume / decide what's left),
  or browse archived lists (`archived: true` / `generation: N`).
- `task_create` — create one or more tasks for multi-step work.
- `task_update` — status transitions, with a one-active invariant and evidence.
- `task_archive` — roll the current list off into a read-only archive at a phase
  boundary so the next phase starts on a clean board.
- `/tasks` — open or focus the task panel.

The live list is also injected into the model's context each turn (a host context
card), and the active task shows in the status line — so the agent stays oriented
without re-querying.

## Requirements

terva **v0.105.1+** (protocol v2 with extension context cards). terva-tasks keeps a
per-session task list and injects it into the model's context, so it needs both
session identity and the context surface — both shipped in v0.105.1. It calls
`RequireProtocol(2)` and refuses to load on an older host with a clear message
rather than misbehaving. Building needs Go 1.22+.

## Install

```bash
terva ext install https://github.com/terva-sh/terva-tasks.git
```

terva clones the repo and the `run.sh` launcher builds it on first launch
(rebuilding only when sources change) — so installing needs only a Go 1.22+
toolchain, with no platform binary committed. You can also install from a local
clone (`terva ext install .`), or load it for a single session without
installing:

```bash
terva --ext /path/to/terva-tasks
```

During local development, `just install` builds and (re)installs in one step —
and copies the binary in, since `terva ext install` skips git-ignored files.

## Usage

The agent creates and updates tasks itself as it works through multi-step jobs;
`/tasks` opens the panel to watch progress. At most one task is `active` at a
time — activating a task returns any previously active one to `pending`.
Marking a task `done` or `blocked` is encouraged to carry short `evidence`
(a passing test command, an edited file path, a user clarification). When you
step away from a task and the successor is known, `task_update` accepts
`activate_next` (the next task's id, valid with `status` `"done"`, `"cancelled"`,
or `"blocked"`) to close/park the current task and focus the next one in a single
step — keeping exactly one task active across the hand-off.

When a distinct phase of work is finished, `task_archive` parks the current list
as a numbered, read-only *generation* and clears the board for the next phase —
which keeps a long session from accumulating stale tasks in the panel and the
model's context. By default it archives **everything**, including open tasks, and
leaves the current list empty; pass `keep_open: true` to archive only finished
(done/cancelled) tasks and keep the open ones. Archived lists stay browsable with
`task_list` (`archived: true` for the index, `generation: N` for one list). They
are not resumable in-place yet — an open task carried into an archive must be
recreated if still needed (the archive note names it for you).

## Persistence and limitations

- Tasks persist to `tasks-<session-id>.json` in the host's per-extension data
  dir, saved on every change and reloaded when a session opens or is resumed.
  Archived generations live in that same file (a bounded history — the oldest
  are dropped once the cap is reached), so they're session-scoped and travel with
  it on resume.
  On terva **v0.105.2+** that data dir is a dedicated writable location separate
  from the install dir (`$TERVA_HOME/ext-data/tasks/`); boards written by
  an earlier version under the old in-install location are read through and
  migrate forward automatically on their next save, so upgrading loses nothing.
- With no active session (`--no-session`), tasks are held in memory only and are
  lost on exit.
- The current list is injected into the model's context every turn as a host
  context card (kept out of the transcript, so auto-compaction can't drop it), and
  the task-discipline policy rides the cached system prompt — so the model stays
  oriented across the session and after `/clear`. A user or project can opt out via
  `disable_context_extensions` (the tools and panel still work).

## Development

A standard Go module. To build against a local terva checkout instead of the
pinned SDK release, add a replace (and drop it before releasing):

```bash
go mod edit -replace terva.sh/terva=../terva     # develop against ../terva
go mod edit -dropreplace terva.sh/terva          # back to the pinned release
```

```bash
just test     # go test -race ./internal/...
just lint     # go vet + gofmt check
just build    # build ./terva-tasks
just install  # build + (re)install into terva for dogfooding
just try DIR  # build + launch terva with the extension (cwd = DIR)
just ci       # lint + race tests
```
