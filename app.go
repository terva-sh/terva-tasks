package main

import (
	"encoding/json"
	"strings"
	"sync"

	"terva-tasks/internal/handlers"
	"terva-tasks/internal/tasks"

	"terva.sh/terva/packages/agent/ext"
)

const (
	panelID  = "terva-tasks-main"
	cardID   = "tasks" // model-facing context card + status segment id
	statusID = "tasks"
)

// app wires the pure task store to the terva extension SDK: tool handlers,
// panel rendering, and the session re-key hook.
type app struct {
	e     *ext.Extension
	store *tasks.Store

	mu           sync.Mutex
	panelOpen    bool
	showDone     bool
	sessionTitle string
}

func newApp(e *ext.Extension, store *tasks.Store) *app {
	return &app{e: e, store: store}
}

// ---- tool handlers ----
//
// The parsing + store + result-text logic lives in internal/handlers (pure,
// unit-tested). These methods add the panel side effects and wrap the text in
// an ext.ToolResult.

func (a *app) handleList(_ json.RawMessage) ext.ToolResult {
	text := handlers.List(a.store)
	a.refresh()
	return ext.TextResult(text)
}

func (a *app) handleCreate(raw json.RawMessage) ext.ToolResult {
	text, isErr := handlers.Create(a.store, raw)
	if isErr {
		return ext.TextErrorResult(text)
	}
	a.ensurePanel()
	a.refresh()
	return ext.TextResult(text)
}

func (a *app) handleUpdate(raw json.RawMessage) ext.ToolResult {
	text, isErr := handlers.Update(a.store, raw)
	if isErr {
		return ext.TextErrorResult(text)
	}
	a.refresh()
	return ext.TextResult(text)
}

// ---- panel ----

func (a *app) ensurePanel() {
	a.mu.Lock()
	already := a.panelOpen
	a.panelOpen = true
	showDone := a.showDone
	title := a.sessionTitle
	a.mu.Unlock()
	if already {
		return
	}
	list := a.store.List()
	a.e.OpenPanel(panelID, tasks.PanelTitle(list, title), tasks.PanelLines(list, showDone), tasks.PanelFooter())
}

// refresh updates every surface from current state: the model-facing context
// card (always — it's injected each turn whether or not the panel is open), the
// TUI status segment (always), and the panel (only when open). Called after each
// mutation and on session change.
func (a *app) refresh() {
	list := a.store.List()

	// Model-facing context card: inject the live list, or clear it when empty.
	if len(list) == 0 {
		a.e.ClearContextCard(cardID)
	} else {
		a.e.PushContextCard(ext.Card{
			ID:       cardID,
			Label:    "Tasks",
			Text:     tasks.RenderCard(list),
			Blocking: tasks.AnyOpen(list),
		})
	}
	// TUI status segment (not model-facing); empty text clears it.
	a.e.SetStatus(statusID, tasks.StatusGlance(list))

	// Panel, only when open.
	a.mu.Lock()
	open := a.panelOpen
	showDone := a.showDone
	title := a.sessionTitle
	a.mu.Unlock()
	if open {
		a.e.RenderPanel(panelID, tasks.PanelTitle(list, title), tasks.PanelLines(list, showDone), tasks.PanelFooter())
	}
}

// handleCommand backs the /tasks slash command: open or focus the panel.
func (a *app) handleCommand(_ string) ext.Response {
	a.mu.Lock()
	a.panelOpen = true
	showDone := a.showDone
	title := a.sessionTitle
	a.mu.Unlock()
	list := a.store.List()
	return ext.OpenPanel(panelID, tasks.PanelTitle(list, title), tasks.PanelLines(list, showDone), tasks.PanelFooter())
}

func (a *app) handleKey(key, text string) {
	switch {
	case key == "rune" && strings.EqualFold(text, "d"):
		a.mu.Lock()
		a.showDone = !a.showDone
		a.mu.Unlock()
		a.refresh()
	case key == "rune" && strings.EqualFold(text, "r"):
		a.refresh()
	}
}

func (a *app) handleClose() {
	a.mu.Lock()
	a.panelOpen = false
	a.mu.Unlock()
}

// onSession re-keys persistence when the active session opens or changes
// (resume / fork / /new). id == "" means no active session (in-memory only).
func (a *app) onSession(id, title string) {
	// DataFS layers the writable DataDir over the read-only install dir, so
	// boards written under the old (pre-split) DataDir keep loading and migrate
	// forward on their next save. It's stable across sessions; setting it on
	// each session event is idempotent.
	a.store.SetFS(a.e.Host().DataFS())
	if err := a.store.Rebind(id); err != nil {
		a.e.Logf("rebind session %q: %v", id, err)
	}
	a.mu.Lock()
	a.sessionTitle = title
	a.mu.Unlock()
	a.refresh()
}
