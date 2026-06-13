# terva-tasks

A native-style task/todo system for the [terva](https://github.com/terva-sh/terva)
agent harness, shipped as an extension. It gives the agent three tools and a live
panel, with a separate task list per session:

- `task_list` — return the current list (reorient / resume / decide what's left).
- `task_create` — create one or more tasks for multi-step work.
- `task_update` — status transitions, with a one-active invariant and evidence.
- `/tasks` — open or focus the task panel.

## Requirements

terva **v0.105.0+** (extension protocol v2). terva-tasks keeps a separate task
list per session, which relies on the session identity the host delivers in
protocol v2. It calls `RequireProtocol(2)` and refuses to load on an older host
with a clear message rather than mis-keying state. Building needs Go 1.22+.

## Install

From a clone of this repo:

```bash
terva ext install .
```

The manifest runs the extension from source (`go run .`), which resolves the
terva SDK and compiles on first launch. For faster startup, build a binary and
point the manifest at it:

```bash
go build -o terva-tasks .
# then set extension.json: "exec": "./terva-tasks", "args": []
```

Or load it for a single session without installing:

```bash
terva --ext /path/to/terva-tasks
```

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
- The list reaches the model only through tool results (an extension can't edit
  the system prompt) — the argument for going native if the trial proves out.

## Development

A standard Go module. To build against a local terva checkout instead of the
pinned SDK release, add a replace (and drop it before releasing):

```bash
go mod edit -replace terva.sh/terva=../terva     # develop against ../terva
go mod edit -dropreplace terva.sh/terva          # back to the pinned release
```

```bash
just test   # go test -race ./internal/...
just lint   # go vet + gofmt check
just build  # build the binary
```
