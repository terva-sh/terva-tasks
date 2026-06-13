// Command terva-tasks is a terva extension that gives the agent a native-style
// task list: three LLM tools (task_list / task_create / task_update) plus a
// persistent panel. It is a terva-only, protocol-v2 extension — it requires the
// session-identity protocol so each session keeps its own task list.
//
// Build note: the session wiring below uses the protocol-v2 SDK surface
// (e.OnSession). Until that lands in the terva checkout this file won't compile;
// the pure logic in internal/tasks builds and is fully tested today.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"terva-tasks/internal/tasks"

	"terva.sh/terva/packages/agent/ext"
)

func main() {
	e := ext.New("terva-tasks", "0.1.0")
	// Require protocol 2: a host that can't deliver session identity (upstream
	// zot, or a pre-v2 terva) refuses to load this extension with a clear
	// message rather than mis-keying per-session state.
	e.RequireProtocol(2)

	store := tasks.NewStore("", "agent") // dataDir set on the first session event
	a := newApp(e, store)

	e.Command("tasks", "open the terva-tasks panel", a.handleCommand)

	e.Tool("task_list", descList, schemaList(), a.handleList)
	e.Tool("task_create", descCreate, schemaCreate(), a.handleCreate)
	e.Tool("task_update", descUpdate, schemaUpdate(), a.handleUpdate)

	e.OnPanelKey(panelID, a.handleKey, a.handleClose)

	// v2: learn the active session (id, path, title) when it opens, and re-key
	// on every change (resume / fork / /new). Empty id => no active session.
	// Struct-shaped callback per the locked v2 spec, so future session fields
	// never break this signature.
	e.OnSession(func(s ext.Session) { a.onSession(s.ID, s.Title) })

	if err := e.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Tool descriptions are the only system-prompt vector an extension has, so they
// carry the usage policy (when to use, the one-active rule, the evidence rule).

const descList = "Return the current task list with each task's id, status, and " +
	"title. Call this to reorient after a long sequence of tool calls, to resume " +
	"work after an interruption, or to decide what remains before giving a final " +
	"answer. Tasks are referenced by the id shown here."

const descCreate = "Create one or more tasks for tracking multi-step work. " +
	"Provide a `tasks` array; each task needs an imperative `title` (e.g. 'Patch " +
	"the parser bug') and may include `active_form` (present-continuous, e.g. " +
	"'Patching the parser bug', shown while the task is active), an initial " +
	"`status` (defaults to 'pending'), and a short `note`. Ids are assigned by the " +
	"system and returned — never supply your own. Create tasks when the work is " +
	"meaningfully multi-step, long-running, risky, interruptible, or delegated: " +
	"investigate → implement → test → document, multi-file refactors, debugging, " +
	"releases. Do NOT create tasks for a simple factual answer, a single-file " +
	"edit, or one command — that is just noise."

const descUpdate = "Update a task by `id`. Primary use is status transitions: " +
	"mark a task 'active' before you start working on it. At most one task is " +
	"active at a time — moving a task to 'active' automatically returns any other " +
	"active task to 'pending', and the result tells you which. Provide `evidence` " +
	"when setting status to 'done' or 'blocked': a passing test command, an edited " +
	"file path, a user clarification. Do NOT mark a task 'done' if its tests are " +
	"failing, the implementation is partial, or errors are unresolved — use " +
	"'blocked' instead and say why in `evidence`. You may also patch `title`, " +
	"`active_form`, or `note`. Reference the task by the id returned from " +
	"task_create or task_list."

func schemaList() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func schemaCreate() json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":       map[string]any{"type": "string"},
						"active_form": map[string]any{"type": "string"},
						"status":      map[string]any{"type": "string", "enum": statusEnum},
						"note":        map[string]any{"type": "string"},
					},
					"required": []string{"title"},
				},
			},
		},
		"required": []string{"tasks"},
	})
	return b
}

func schemaUpdate() json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string"},
			"title":       map[string]any{"type": "string"},
			"active_form": map[string]any{"type": "string"},
			"status":      map[string]any{"type": "string", "enum": statusEnum},
			"evidence":    map[string]any{"type": "string"},
			"note":        map[string]any{"type": "string"},
		},
		"required": []string{"id"},
	})
	return b
}

var statusEnum = []string{"pending", "active", "blocked", "done", "cancelled"}
