// Package handlers turns the raw JSON tool arguments into store operations and
// the compact text the model sees in each tool result. It depends only on the
// task store (no terva SDK), so the wire parsing, the update pointer semantics
// (absent vs set-to-empty), and the result formatting are all unit-testable
// without spinning up the extension. The ext glue in app.go is a thin wrapper
// over these functions that adds panel side effects and the ToolResult type.
package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"terva-tasks/internal/tasks"
)

type createArgs struct {
	Tasks []struct {
		Title      string `json:"title"`
		ActiveForm string `json:"active_form"`
		Status     string `json:"status"`
		Note       string `json:"note"`
	} `json:"tasks"`
}

type updateArgs struct {
	ID         string  `json:"id"`
	Title      *string `json:"title"`
	ActiveForm *string `json:"active_form"`
	Status     *string `json:"status"`
	Evidence   *string `json:"evidence"`
	Note       *string `json:"note"`
}

// List returns the compact rendering of the current task list.
func List(s *tasks.Store) string {
	return tasks.RenderCompact(s.List())
}

// Create parses task_create args, creates the tasks, and returns the result
// text plus an isError flag.
func Create(s *tasks.Store, raw json.RawMessage) (string, bool) {
	var in createArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return "invalid args: " + err.Error(), true
	}
	if len(in.Tasks) == 0 {
		return "task_create requires at least one task in `tasks`", true
	}
	specs := make([]tasks.CreateSpec, 0, len(in.Tasks))
	for _, t := range in.Tasks {
		specs = append(specs, tasks.CreateSpec{
			Title:      t.Title,
			ActiveForm: t.ActiveForm,
			Status:     tasks.Status(strings.TrimSpace(t.Status)),
			Note:       t.Note,
		})
	}
	created, err := s.Create(specs)
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Created %d task(s):\n", len(created))
	for _, t := range created {
		fmt.Fprintf(&b, "  %s  %s  %s\n", t.ID, t.Status, t.Title)
	}
	b.WriteString("\n")
	b.WriteString(tasks.RenderCompact(s.List()))
	return b.String(), false
}

// Update parses task_update args, applies the patch (enforcing the one-active
// invariant), and returns the result text plus an isError flag.
func Update(s *tasks.Store, raw json.RawMessage) (string, bool) {
	var in updateArgs
	if err := json.Unmarshal(raw, &in); err != nil {
		return "invalid args: " + err.Error(), true
	}
	patch := tasks.UpdatePatch{
		ID:         in.ID,
		Title:      in.Title,
		ActiveForm: in.ActiveForm,
		Evidence:   in.Evidence,
		Note:       in.Note,
	}
	if in.Status != nil {
		st := tasks.Status(strings.TrimSpace(*in.Status))
		patch.Status = &st
	}
	updated, deactivated, err := s.Update(patch)
	if err != nil {
		return err.Error(), true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Updated %s → %s: %s\n", updated.ID, updated.Status, updated.Title)
	if deactivated != nil {
		fmt.Fprintf(&b, "Deactivated %s (was active): %s\n", deactivated.ID, deactivated.Title)
	}
	// Soft evidence nudge: name the task and the status so the recommendation is
	// actionable, not generic. Stays a note (never isError) — evidence is
	// encouraged, not mechanically required.
	if (updated.Status == tasks.StatusDone || updated.Status == tasks.StatusBlocked) && strings.TrimSpace(updated.Evidence) == "" {
		fmt.Fprintf(&b, "note: %s marked %s without evidence — add a passing test command, an edited path, or a short reason so %q is checkable.\n",
			updated.ID, updated.Status, string(updated.Status))
	}
	// List once: reused for the closing-the-list warning and the inline render.
	list := s.List()
	// Soft closing-the-list warning: marking a task done/cancelled when real work
	// (pending/active) remains and nothing is currently active means the agent
	// closed its focus and left work behind. Observational — the transition still
	// applied. Blocked tasks are acknowledged parks (excluded by OpenSummary).
	if updated.Status == tasks.StatusDone || updated.Status == tasks.StatusCancelled {
		if open, active, names := tasks.OpenSummary(list); open > 0 && active == 0 {
			label := strings.Join(names, ", ")
			if open > len(names) {
				label += fmt.Sprintf(", +%d more", open-len(names))
			}
			fmt.Fprintf(&b, "note: %s %s, but %d task(s) still open (%s) and none active — mark the next one active or finish them before wrapping up.\n",
				updated.ID, updated.Status, open, label)
		}
	}
	b.WriteString("\n")
	b.WriteString(tasks.RenderCompact(list))
	return b.String(), false
}
