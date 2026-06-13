package main

import (
	"encoding/json"
	"strings"
	"sync"

	"terva-tasks/internal/handlers"
	"terva-tasks/internal/tasks"

	"terva.sh/terva/packages/agent/ext"
)

const panelID = "terva-tasks-main"

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
	a.rerender()
	return ext.TextResult(text)
}

func (a *app) handleCreate(raw json.RawMessage) ext.ToolResult {
	text, isErr := handlers.Create(a.store, raw)
	if isErr {
		return ext.TextErrorResult(text)
	}
	a.ensurePanel()
	a.rerender()
	return ext.TextResult(text)
}

func (a *app) handleUpdate(raw json.RawMessage) ext.ToolResult {
	text, isErr := handlers.Update(a.store, raw)
	if isErr {
		return ext.TextErrorResult(text)
	}
	a.rerender()
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

func (a *app) rerender() {
	a.mu.Lock()
	open := a.panelOpen
	showDone := a.showDone
	title := a.sessionTitle
	a.mu.Unlock()
	if !open {
		return
	}
	list := a.store.List()
	a.e.RenderPanel(panelID, tasks.PanelTitle(list, title), tasks.PanelLines(list, showDone), tasks.PanelFooter())
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
		a.rerender()
	case key == "rune" && strings.EqualFold(text, "r"):
		a.rerender()
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
	a.store.SetDataDir(a.e.Host().DataDir)
	if err := a.store.Rebind(id); err != nil {
		a.e.Logf("rebind session %q: %v", id, err)
	}
	a.mu.Lock()
	a.sessionTitle = title
	a.mu.Unlock()
	a.rerender()
}
