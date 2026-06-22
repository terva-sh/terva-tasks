// Command terva-tasks is a terva extension that gives the agent a native-style
// task list: three LLM tools (task_list / task_create / task_update), a live
// context card + status segment injected into the model's context each turn, and
// an interactive panel. It is a terva-only, protocol-v2 extension — it requires
// session identity (per-session lists) and the host context surface.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"terva-tasks/internal/tasks"

	"terva.sh/terva/packages/agent/ext"
)

func main() {
	e := ext.New("tasks", "0.2.4")
	// Require protocol 2: a host that can't deliver session identity or the
	// context surface (upstream zot, or a pre-v2 terva) refuses to load this
	// extension with a clear message rather than misbehaving.
	e.RequireProtocol(2)

	// Standing task-discipline policy, folded into the cached system prompt by
	// the host. This is the primary policy vector now; the tool descriptions
	// keep only a minimal fallback (a user/project can opt out of context
	// injection via disable_context_extensions, which keeps the tools working).
	e.ContributeContext(contextPolicy)

	store := tasks.NewStore(nil, "agent") // FileStore set on the first session event
	a := newApp(e, store)

	e.Command("tasks", "open the tasks panel", a.handleCommand)

	// Effect-class each tool so the host can auto-admit them without prompting:
	// task_list only inspects (ReadOnly), and create/update read+write nothing but
	// this extension's own data dir (local-data). Without these, a read-only or
	// workspace approval mode would gate the task tools on every call.
	e.Tool("task_list", descList, schemaList(), a.handleList, ext.ReadOnly())
	e.Tool("task_create", descCreate, schemaCreate(), a.handleCreate, ext.WithAuthority(ext.AuthorityLocalData))
	e.Tool("task_update", descUpdate, schemaUpdate(), a.handleUpdate, ext.WithAuthority(ext.AuthorityLocalData))

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

// contextPolicy is the standing guidance the host folds into the cached system
// prompt (ContributeContext). It is the primary policy vector; the tool
// descriptions below carry only a minimal restatement as an opt-out fallback.
const contextPolicy = "You have a task list (task_create / task_update / task_list). " +
	"Its current state is shown to you each turn as a Tasks context card — consult it " +
	"to stay oriented and to decide what remains.\n" +
	"\n" +
	"WHEN: Use tasks for work that is meaningfully multi-step, long-running, risky, or " +
	"interruptible. Skip them for a simple factual answer, a single-file edit, or one " +
	"command.\n" +
	"\n" +
	"PLAN UP FRONT (before you start editing):\n" +
	"- Break the work into its distinct steps and create ONE task per step — in a single " +
	"task_create call, as separate array items. Investigate / implement / test / document " +
	"is four tasks, not one.\n" +
	"- Never create a single umbrella task (\"develop\", \"implement the feature\", \"do " +
	"the work\") and run everything under it. If a title doesn't name a specific, checkable " +
	"outcome, split it.\n" +
	"- Title each task as a specific outcome. GOOD: [\"Add CSV serializer\", \"Wire export " +
	"button to serializer\", \"Add export integration test\", \"Document the export flag\"]. " +
	"BAD: [\"Develop the export feature\"].\n" +
	"\n" +
	"WHILE WORKING:\n" +
	"- Keep exactly one task 'active' at a time — mark a task active before working it.\n" +
	"- Record short evidence when you complete or block a task (a passing test command, " +
	"an edited path, a user clarification).\n" +
	"- Do NOT mark a task 'done' while its tests fail, the work is partial, or errors are " +
	"unresolved — use 'blocked' and say why."

// The tool descriptions are deliberately terse — the policy lives in
// contextPolicy. They keep only the essential rules so the tools remain usable
// if a user opts out of context injection.

const descList = "Return the current task list (id, status, title). Use it to " +
	"reorient or to decide what remains before finishing."

const descCreate = "Create tasks for multi-step work. Decompose the job and pass each " +
	"step as a separate array item in one call — one task per step, not a single " +
	"'develop' / 'implement everything' task. Each needs an imperative `title` naming a " +
	"specific, checkable outcome; optional `active_form` (present-continuous), `status` " +
	"(default 'pending'), and `note`. Ids are system-assigned — never supply your own. " +
	"Don't create tasks for trivial one-step requests."

const descUpdate = "Update a task by `id` — mainly status transitions. Mark a task " +
	"'active' before working it; only one task is active at a time. Provide " +
	"`evidence` when setting 'done' or 'blocked', and use 'blocked' (not 'done') if " +
	"the work is failing or incomplete. May also patch `title`, `active_form`, `note`."

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
