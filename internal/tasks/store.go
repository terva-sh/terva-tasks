// Package tasks holds the terva-tasks store: the task model, the create/update
// logic (including the one-active invariant), and session-keyed JSON
// persistence. It depends only on the standard library — no terva SDK — so it
// compiles and is fully unit-testable on its own, independent of the ext glue.
package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status is a task's lifecycle state.
type Status string

const (
	StatusPending   Status = "pending"
	StatusActive    Status = "active"
	StatusBlocked   Status = "blocked"
	StatusDone      Status = "done"
	StatusCancelled Status = "cancelled"
)

// ValidStatus reports whether s is one of the five known statuses.
func ValidStatus(s Status) bool {
	switch s {
	case StatusPending, StatusActive, StatusBlocked, StatusDone, StatusCancelled:
		return true
	}
	return false
}

// IsTerminal reports whether the status is a finished state (collapsed in the
// panel).
func (s Status) IsTerminal() bool { return s == StatusDone || s == StatusCancelled }

// Task is a single tracked unit of work. IDs are harness-generated and never
// model-supplied.
type Task struct {
	ID         string `json:"id"`
	Title      string `json:"title"`       // imperative
	ActiveForm string `json:"active_form"` // present-continuous; falls back to Title
	Status     Status `json:"status"`
	Owner      string `json:"owner,omitempty"`
	Evidence   string `json:"evidence,omitempty"`
	Note       string `json:"note,omitempty"`
	CreatedAt  string `json:"created_at"` // RFC3339
	UpdatedAt  string `json:"updated_at"`
}

// CreateSpec is the model-supplied shape for one new task. It has no ID field
// by design.
type CreateSpec struct {
	Title      string
	ActiveForm string
	Status     Status // "" => pending
	Note       string
}

// UpdatePatch patches an existing task by ID. A nil pointer means "leave
// unchanged"; this distinguishes absent from set-to-empty.
type UpdatePatch struct {
	ID         string
	Title      *string
	ActiveForm *string
	Status     *Status
	Evidence   *string
	Note       *string
}

// storeFile is the on-disk shape.
type storeFile struct {
	Tasks  []Task `json:"tasks"`
	NextID int    `json:"next_id"`
}

// Store is the session-scoped task list. The zero session id ("") means "no
// active session": the list is held in memory and never written to disk. A
// non-empty session id keys persistence to <dataDir>/tasks-<id>.json.
type Store struct {
	mu        sync.Mutex
	dataDir   string
	owner     string
	sessionID string
	path      string // "" => in-memory only
	now       func() time.Time
	tasks     []Task
	nextID    int
}

// NewStore constructs an in-memory store. dataDir may be empty at construction
// (it often isn't known until after the host handshake); set it later with
// SetDataDir before the first Rebind to a real session.
func NewStore(dataDir, owner string) *Store {
	if owner == "" {
		owner = "agent"
	}
	return &Store{
		dataDir: dataDir,
		owner:   owner,
		now:     time.Now,
		nextID:  1,
	}
}

// SetDataDir sets the directory used for session files. Safe to call before the
// first Rebind.
func (s *Store) SetDataDir(dir string) {
	s.mu.Lock()
	s.dataDir = dir
	s.mu.Unlock()
}

// SessionID returns the currently bound session id ("" = none).
func (s *Store) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionID
}

func pathFor(dataDir, sessionID string) string {
	if sessionID == "" || dataDir == "" {
		return ""
	}
	// sessionFileName contains no path separators or "..", so the join can never
	// escape dataDir even for a hostile session id.
	return filepath.Join(dataDir, sessionFileName(sessionID))
}

// Rebind points the store at a session. It is the single keying entry point:
//
//   - same id: no-op.
//   - to "": flush the outgoing file, then reset to a fresh in-memory list.
//   - to a real id: flush the outgoing file, then load tasks-<id>.json. If that
//     file doesn't exist yet and we're binding for the FIRST time out of the
//     pre-bind in-memory state, carry the in-memory tasks into the new session
//     (so work created before the session opened isn't lost). A session SWITCH
//     (real id -> real id) never carries tasks across.
func (s *Store) Rebind(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == s.sessionID {
		return nil
	}
	carry := s.sessionID == "" && sessionID != ""
	prevTasks := s.tasks
	prevNextID := s.nextID

	if s.path != "" {
		_ = s.saveLocked() // best-effort flush of the outgoing session
	}

	newPath := pathFor(s.dataDir, sessionID)

	// Load the incoming session's state into locals first; commit to the store
	// only after a clean load. A failed or corrupt load must never leave the new
	// session showing the previous session's tasks.
	var newTasks []Task
	newNextID := 1
	var warn error

	if newPath != "" {
		sf, found, err := readStoreFile(newPath)
		if err != nil {
			// Corrupt file: move it aside and start this session empty rather
			// than failing or leaking prior state.
			s.backupCorruptLocked(newPath)
			warn = fmt.Errorf("corrupt session file %s moved aside; started empty: %w", newPath, err)
			found = false
		}
		switch {
		case found:
			newTasks = sf.Tasks
			newNextID = sf.NextID
			if newNextID < 1 {
				newNextID = deriveNextID(newTasks)
			}
		case carry && len(prevTasks) > 0:
			// First binding out of pre-session in-memory state: carry that work
			// into the new (empty) session file.
			newTasks = prevTasks
			newNextID = prevNextID
		}
	}

	// Commit atomically.
	s.sessionID = sessionID
	s.path = newPath
	s.tasks = newTasks
	s.nextID = newNextID

	// Persist carried work immediately; a brand-new empty session materializes
	// on its first mutation.
	if s.path != "" && carry && len(s.tasks) > 0 {
		_ = s.saveLocked()
	}
	return warn
}

// readStoreFile reads a session file. found is false (no error) when the file is
// missing or empty; a malformed file returns an error so the caller can quarantine
// it instead of trusting partial state.
func readStoreFile(path string) (storeFile, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return storeFile{}, false, nil
		}
		return storeFile{}, false, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return storeFile{}, false, nil
	}
	var sf storeFile
	if err := json.Unmarshal(b, &sf); err != nil {
		return storeFile{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return sf, true, nil
}

// backupCorruptLocked renames a malformed session file aside so it isn't
// overwritten and can be inspected, leaving the path free for a fresh start.
func (s *Store) backupCorruptLocked(path string) {
	bak := path + ".corrupt-" + s.now().UTC().Format("20060102T150405Z")
	_ = os.Rename(path, bak) // best-effort
}

// deriveNextID recovers a monotonic counter from existing ids when a legacy
// file lacks next_id.
func deriveNextID(tasks []Task) int {
	max := 0
	for _, t := range tasks {
		if strings.HasPrefix(t.ID, "task-") {
			if v, err := strconv.Atoi(t.ID[len("task-"):]); err == nil && v > max {
				max = v
			}
		}
	}
	return max + 1
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil // in-memory only
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(storeFile{Tasks: s.tasks, NextID: s.nextID}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Store) nowStr() string { return s.now().UTC().Format(time.RFC3339) }

func (s *Store) findLocked(id string) (*Task, int) {
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			return &s.tasks[i], i
		}
	}
	return nil, -1
}

// applyOneActiveLocked enforces the one-active invariant: any OTHER task that is
// active is demoted to pending. Returns the demoted task (by value) if there was
// one. At most one other can be active given the invariant is maintained.
func (s *Store) applyOneActiveLocked(targetIdx int) (Task, bool) {
	for i := range s.tasks {
		if i != targetIdx && s.tasks[i].Status == StatusActive {
			s.tasks[i].Status = StatusPending
			s.tasks[i].UpdatedAt = s.nowStr()
			return s.tasks[i], true
		}
	}
	return Task{}, false
}

// Create adds tasks. It validates every spec before mutating (no partial
// create). A spec with status "active" applies the one-active invariant; within
// a batch, the last active spec wins.
func (s *Store) Create(specs []CreateSpec) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(specs) == 0 {
		return nil, fmt.Errorf("no tasks to create")
	}
	if len(specs) > MaxBatch {
		return nil, fmt.Errorf("too many tasks in one batch (%d > %d max)", len(specs), MaxBatch)
	}
	if len(s.tasks)+len(specs) > MaxTasksPerSession {
		return nil, fmt.Errorf("task limit reached (%d max per session)", MaxTasksPerSession)
	}
	// Validate + normalize every spec before mutating (no partial create).
	type prepared struct {
		title, activeForm, note string
		status                  Status
	}
	prep := make([]prepared, len(specs))
	for i, sp := range specs {
		title := CleanOneLine(sp.Title, MaxTitleLen)
		if title == "" {
			return nil, fmt.Errorf("task %d: title is required", i+1)
		}
		if sp.Status != "" && !ValidStatus(sp.Status) {
			return nil, fmt.Errorf("task %d: invalid status %q", i+1, sp.Status)
		}
		af := CleanOneLine(sp.ActiveForm, MaxActiveFormLen)
		if af == "" {
			af = title
		}
		st := sp.Status
		if st == "" {
			st = StatusPending
		}
		prep[i] = prepared{title: title, activeForm: af, note: CleanOneLine(sp.Note, MaxNoteLen), status: st}
	}
	now := s.nowStr()
	ids := make([]string, 0, len(prep))
	for _, p := range prep {
		t := Task{
			ID:         fmt.Sprintf("task-%d", s.nextID),
			Title:      p.title,
			ActiveForm: p.activeForm,
			Status:     p.status,
			Owner:      s.owner,
			Note:       p.note,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		s.nextID++
		s.tasks = append(s.tasks, t)
		if p.status == StatusActive {
			s.applyOneActiveLocked(len(s.tasks) - 1)
		}
		ids = append(ids, t.ID)
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	// Re-read so earlier active specs reflect any demotion by later ones.
	out := make([]Task, 0, len(ids))
	for _, id := range ids {
		if _, idx := s.findLocked(id); idx >= 0 {
			out = append(out, s.tasks[idx])
		}
	}
	return out, nil
}

// Update patches a task by id. It returns the updated task and, when activating
// a task demoted another, the deactivated task. Unknown/blank ids are rejected
// (Update never creates).
func (s *Store) Update(p UpdatePatch) (Task, *Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(p.ID) == "" {
		return Task{}, nil, fmt.Errorf("id is required")
	}
	_, idx := s.findLocked(p.ID)
	if idx < 0 {
		return Task{}, nil, fmt.Errorf("no task with id %q", p.ID)
	}
	if p.Status != nil && !ValidStatus(*p.Status) {
		return Task{}, nil, fmt.Errorf("invalid status %q", *p.Status)
	}
	t := &s.tasks[idx]
	if p.Title != nil {
		// An empty/whitespace title patch is a no-op (a title can never be
		// blank), so a status-only update that incidentally carries an empty
		// title isn't rejected.
		if title := CleanOneLine(*p.Title, MaxTitleLen); title != "" {
			t.Title = title
		}
	}
	if p.ActiveForm != nil {
		af := CleanOneLine(*p.ActiveForm, MaxActiveFormLen)
		if af == "" {
			af = t.Title
		}
		t.ActiveForm = af
	}
	if p.Note != nil {
		t.Note = CleanOneLine(*p.Note, MaxNoteLen)
	}
	if p.Evidence != nil {
		t.Evidence = CleanOneLine(*p.Evidence, MaxEvidenceLen)
	}
	if p.Status != nil {
		t.Status = *p.Status
	}
	t.UpdatedAt = s.nowStr()

	var deactivated *Task
	if t.Status == StatusActive {
		if d, ok := s.applyOneActiveLocked(idx); ok {
			dc := d
			deactivated = &dc
		}
	}
	if err := s.saveLocked(); err != nil {
		return Task{}, nil, err
	}
	return s.tasks[idx], deactivated, nil
}

// List returns a copy of the current tasks.
func (s *Store) List() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Task(nil), s.tasks...)
}
