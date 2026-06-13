package tasks

import (
	"strings"
	"testing"
)

func TestRenderCompactEmpty(t *testing.T) {
	if got := RenderCompact(nil); got != "No tasks." {
		t.Errorf("empty render: %q", got)
	}
}

func TestRenderCompactUsesActiveFormAndEvidence(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "Patch parser", ActiveForm: "Patching parser", Status: StatusActive},
		{ID: "task-2", Title: "Add test", Status: StatusPending},
		{ID: "task-3", Title: "Repro", Status: StatusDone, Evidence: "cargo test failed"},
	}
	out := RenderCompact(tasks)
	if !strings.Contains(out, "task-1  active   Patching parser") {
		t.Errorf("active line:\n%s", out)
	}
	if !strings.Contains(out, "task-2  pending  Add test") {
		t.Errorf("pending line:\n%s", out)
	}
	if !strings.Contains(out, "task-3  done     Repro — cargo test failed") {
		t.Errorf("evidence line:\n%s", out)
	}
}

func TestPanelTitle(t *testing.T) {
	one := []Task{
		{ID: "task-1", Title: "Patch", ActiveForm: "Patching", Status: StatusActive},
		{ID: "task-2", Title: "Test", Status: StatusBlocked},
	}
	got := PanelTitle(one, "")
	if !strings.Contains(got, "Tasks · Patching") || !strings.Contains(got, "1 blocked") {
		t.Errorf("title with one active + blocked: %q", got)
	}
	none := []Task{{ID: "task-1", Title: "A", Status: StatusPending}}
	if got := PanelTitle(none, "my session"); !strings.Contains(got, "Tasks (1)") || !strings.Contains(got, "my session") {
		t.Errorf("title with no active + session title: %q", got)
	}
}

func TestPanelLinesCollapseAndExpand(t *testing.T) {
	tasks := []Task{
		{ID: "task-1", Title: "A", Status: StatusActive, ActiveForm: "Aing"},
		{ID: "task-2", Title: "B", Status: StatusBlocked},
		{ID: "task-3", Title: "C", Status: StatusDone},
		{ID: "task-4", Title: "D", Status: StatusDone},
	}
	collapsed := PanelLines(tasks, false)
	if !strings.Contains(collapsed[0], "blocked") {
		t.Errorf("blocked should sort first: %v", collapsed)
	}
	joined := strings.Join(collapsed, "\n")
	if !strings.Contains(joined, "done (2)") {
		t.Errorf("done should collapse: %v", collapsed)
	}
	expanded := strings.Join(PanelLines(tasks, true), "\n")
	if strings.Contains(expanded, "done (2)") {
		t.Errorf("expanded should not summarize: %v", expanded)
	}
}

func TestPanelLinesAllTerminal(t *testing.T) {
	tasks := []Task{{ID: "task-1", Title: "A", Status: StatusDone}}
	joined := strings.Join(PanelLines(tasks, false), "\n")
	if !strings.Contains(joined, "All tasks complete.") {
		t.Errorf("missing completion line: %v", joined)
	}
}

func TestPanelLinesEmpty(t *testing.T) {
	joined := strings.Join(PanelLines(nil, false), "\n")
	if !strings.Contains(joined, "No tasks yet") {
		t.Errorf("empty panel body: %v", joined)
	}
}
