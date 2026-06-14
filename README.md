# terva-tasks

A native-style task/todo system for the [terva](https://github.com/terva-sh/terva)
agent harness, shipped as an extension. It gives the agent three tools and a live
panel, with a separate task list per session:

- `task_list` — return the current list (reorient / resume / decide what's left).
- `task_create` — create one or more tasks for multi-step work.
- `task_update` — status transitions, with a one-active invariant and evidence.
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
(a passing test command, an edited file path, a user clarification).

## Persistence and limitations

- Tasks persist to `<data-dir>/tasks-<session-id>.json`, saved on every change
  and reloaded when a session opens or is resumed.
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
