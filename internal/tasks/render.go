package tasks

import (
	"fmt"
	"sort"
	"strings"
)

// RenderCompact is the plain-text rendering returned inline in tool results, so
// the model always sees current state. Empty list => "No tasks."
func RenderCompact(tasks []Task) string {
	if len(tasks) == 0 {
		return "No tasks."
	}
	lines := make([]string, 0, len(tasks))
	for _, t := range tasks {
		lines = append(lines, compactLine(t))
	}
	return strings.Join(lines, "\n")
}

func compactLine(t Task) string {
	line := fmt.Sprintf("%s  %-8s %s", t.ID, t.Status, displayLabel(t))
	if ev := strings.TrimSpace(t.Evidence); ev != "" {
		line += " — " + ev
	} else if nt := strings.TrimSpace(t.Note); nt != "" {
		line += " — " + nt
	}
	return line
}

// displayLabel shows the present-continuous form for an active task, else the
// imperative title.
func displayLabel(t Task) string {
	if t.Status == StatusActive && strings.TrimSpace(t.ActiveForm) != "" {
		return t.ActiveForm
	}
	return t.Title
}

// PanelTitle builds the panel header. With exactly one active task it surfaces
// that task's active form; otherwise it shows the open count. A blocked count
// and an optional session title are appended.
func PanelTitle(tasks []Task, sessionTitle string) string {
	var active string
	activeCount, blocked, open := 0, 0, 0
	for _, t := range tasks {
		switch t.Status {
		case StatusActive:
			activeCount++
			active = displayLabel(t)
		case StatusBlocked:
			blocked++
		}
		if !t.Status.IsTerminal() {
			open++
		}
	}
	var title string
	if activeCount == 1 {
		title = "Tasks · " + active
	} else {
		title = fmt.Sprintf("Tasks (%d)", open)
	}
	if blocked > 0 {
		title += fmt.Sprintf(" · %d blocked", blocked)
	}
	if strings.TrimSpace(sessionTitle) != "" {
		title += " — " + sessionTitle
	}
	return title
}

func statusRank(s Status) int {
	switch s {
	case StatusBlocked:
		return 0
	case StatusActive:
		return 1
	default: // pending
		return 2
	}
}

// PanelLines renders the panel body. Open tasks are listed with blocked first;
// done/cancelled collapse to summary lines unless showDone is set.
func PanelLines(tasks []Task, showDone bool) []string {
	if len(tasks) == 0 {
		return []string{"  No tasks yet — the agent will populate this."}
	}
	var open, done, cancelled []Task
	for _, t := range tasks {
		switch t.Status {
		case StatusDone:
			done = append(done, t)
		case StatusCancelled:
			cancelled = append(cancelled, t)
		default:
			open = append(open, t)
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		return statusRank(open[i].Status) < statusRank(open[j].Status)
	})

	lines := make([]string, 0, len(open)+len(done)+len(cancelled)+2)
	for _, t := range open {
		lines = append(lines, "  "+panelRow(t))
	}
	if len(open) == 0 {
		lines = append(lines, "  All tasks complete.")
	}
	if showDone {
		for _, t := range done {
			lines = append(lines, "  "+panelRow(t))
		}
		for _, t := range cancelled {
			lines = append(lines, "  "+panelRow(t))
		}
		return lines
	}
	if len(done) > 0 {
		lines = append(lines, fmt.Sprintf("  done (%d)", len(done)))
	}
	if len(cancelled) > 0 {
		lines = append(lines, fmt.Sprintf("  cancelled (%d)", len(cancelled)))
	}
	return lines
}

func panelRow(t Task) string {
	row := fmt.Sprintf("%-9s %s", string(t.Status), displayLabel(t))
	if ev := strings.TrimSpace(t.Evidence); ev != "" {
		row += " — " + ev
	}
	return row
}

// PanelFooter is the static key hint line.
func PanelFooter() string {
	return "d expand/collapse done · r refresh · esc close"
}
