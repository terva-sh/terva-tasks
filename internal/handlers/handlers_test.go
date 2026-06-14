package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"terva-tasks/internal/tasks"
)

func boundStore(t *testing.T) *tasks.Store {
	t.Helper()
	s := tasks.NewStore(tasks.NewDirFS(t.TempDir()), "test-agent")
	if err := s.Rebind("sess-1"); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	return s
}

func TestCreateValid(t *testing.T) {
	s := boundStore(t)
	text, isErr := Create(s, json.RawMessage(`{"tasks":[
		{"title":"Patch the parser bug"},
		{"title":"Add regression test","active_form":"Adding regression test"}
	]}`))
	if isErr {
		t.Fatalf("unexpected error: %s", text)
	}
	if !strings.Contains(text, "Created 2 task(s):") {
		t.Errorf("missing count header:\n%s", text)
	}
	if !strings.Contains(text, "task-1") || !strings.Contains(text, "task-2") {
		t.Errorf("missing ids:\n%s", text)
	}
	// The full current list is appended so the model sees inline state.
	if !strings.Contains(text, "task-1  pending  Patch the parser bug") {
		t.Errorf("missing inline list:\n%s", text)
	}
}

func TestCreateErrors(t *testing.T) {
	s := boundStore(t)
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"bad json", `{`, "invalid args"},
		{"empty tasks", `{"tasks":[]}`, "at least one task"},
		{"blank title", `{"tasks":[{"title":"  "}]}`, "title is required"},
		{"bad status", `{"tasks":[{"title":"x","status":"in_progress"}]}`, "invalid status"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, isErr := Create(s, json.RawMessage(c.raw))
			if !isErr {
				t.Fatalf("expected error, got: %s", text)
			}
			if !strings.Contains(text, c.want) {
				t.Errorf("want %q in %q", c.want, text)
			}
		})
	}
	if n := len(s.List()); n != 0 {
		t.Errorf("failed creates must not mutate; have %d", n)
	}
}

func TestUpdateActivationReportsDeactivation(t *testing.T) {
	s := boundStore(t)
	if _, isErr := Create(s, json.RawMessage(`{"tasks":[{"title":"A"},{"title":"B"}]}`)); isErr {
		t.Fatal("seed create failed")
	}
	if text, isErr := Update(s, json.RawMessage(`{"id":"task-1","status":"active"}`)); isErr {
		t.Fatalf("activate A: %s", text)
	}
	text, isErr := Update(s, json.RawMessage(`{"id":"task-2","status":"active"}`))
	if isErr {
		t.Fatalf("activate B: %s", text)
	}
	if !strings.Contains(text, "Updated task-2 → active") {
		t.Errorf("missing update line:\n%s", text)
	}
	if !strings.Contains(text, "Deactivated task-1 (was active)") {
		t.Errorf("missing deactivation line:\n%s", text)
	}
}

func TestUpdateUnknownID(t *testing.T) {
	s := boundStore(t)
	Create(s, json.RawMessage(`{"tasks":[{"title":"A"}]}`))
	text, isErr := Update(s, json.RawMessage(`{"id":"task-9","status":"done"}`))
	if !isErr {
		t.Fatalf("expected error, got: %s", text)
	}
	if !strings.Contains(text, `no task with id "task-9"`) {
		t.Errorf("unexpected error text: %s", text)
	}
}

func TestUpdateEvidenceNudge(t *testing.T) {
	s := boundStore(t)
	Create(s, json.RawMessage(`{"tasks":[{"title":"A"},{"title":"B"}]}`))

	// done without evidence -> nudge.
	text, _ := Update(s, json.RawMessage(`{"id":"task-1","status":"done"}`))
	if !strings.Contains(text, "evidence is recommended") {
		t.Errorf("expected evidence nudge:\n%s", text)
	}
	// done with evidence -> no nudge, evidence shown inline.
	text, _ = Update(s, json.RawMessage(`{"id":"task-2","status":"done","evidence":"go test ./... passed"}`))
	if strings.Contains(text, "evidence is recommended") {
		t.Errorf("unexpected nudge when evidence given:\n%s", text)
	}
	if !strings.Contains(text, "go test ./... passed") {
		t.Errorf("evidence not shown inline:\n%s", text)
	}
}

// TestUpdateAbsentVsEmpty pins the pointer semantics: a field omitted from the
// JSON is left unchanged, while a field present as "" is applied.
func TestUpdateAbsentVsEmpty(t *testing.T) {
	s := boundStore(t)
	Create(s, json.RawMessage(`{"tasks":[{"title":"A","note":"keep me"}]}`))

	// Status-only update omits note -> note preserved.
	if _, isErr := Update(s, json.RawMessage(`{"id":"task-1","status":"active"}`)); isErr {
		t.Fatal("update failed")
	}
	if got := s.List()[0].Note; got != "keep me" {
		t.Errorf("absent note should be preserved, got %q", got)
	}
	// Explicit empty note -> cleared.
	if _, isErr := Update(s, json.RawMessage(`{"id":"task-1","note":""}`)); isErr {
		t.Fatal("update failed")
	}
	if got := s.List()[0].Note; got != "" {
		t.Errorf("explicit empty note should clear, got %q", got)
	}
}

func TestListEmptyAndPopulated(t *testing.T) {
	s := boundStore(t)
	if got := List(s); got != "No tasks." {
		t.Errorf("empty list: %q", got)
	}
	Create(s, json.RawMessage(`{"tasks":[{"title":"A"}]}`))
	if got := List(s); !strings.Contains(got, "task-1  pending  A") {
		t.Errorf("populated list: %q", got)
	}
}
