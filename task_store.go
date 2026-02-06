// store.go implements a thread-safe, in-memory task store.
//
// All MCP tool handlers and worker goroutines access tasks through this store.
// The mutex ensures safe concurrent access. State is ephemeral — it lives only
// for the duration of the MCP server process (i.e. one Claude Code session).
package main

import (
	"sync"
	"time"
)

// TaskStore holds all tasks in memory, protected by a mutex. Tasks are stored
// in a map for O(1) lookup and a separate slice to preserve insertion order
// for stable iteration in List/Summary.
type TaskStore struct {
	mu    sync.Mutex
	tasks map[string]*Task
	order []string // insertion order for stable iteration
}

// NewTaskStore creates an empty task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*Task),
	}
}

// Add inserts a batch of tasks into the store. Called by submit_tasks.
func (s *TaskStore) Add(tasks []*Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tasks {
		s.tasks[t.ID] = t
		s.order = append(s.order, t.ID)
	}
}

// Get returns a single task by ID, or nil if not found.
//
// WARNING: Returns a raw *Task pointer. Reading fields on the returned pointer
// without holding the store lock is a data race if a worker goroutine is
// concurrently mutating the task. For safe reads from outside the store, use
// Results() or Summary() which return copies under the lock.
func (s *TaskStore) Get(id string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[id]
}

// List returns tasks matching the filter criteria, in insertion order.
//   - If ids is non-empty, only tasks with those IDs are included.
//   - If tag is non-empty, only tasks with that tag are included.
//   - Both filters can be combined (AND logic).
//   - If both are empty, all tasks are returned.
//
// WARNING: Like Get(), this returns raw *Task pointers. Reading mutable fields
// on the returned pointers without holding the store lock is a data race if a
// worker goroutine is concurrently mutating the task. The only safe field to
// read without the lock is ID (immutable after creation). For safe reads of
// mutable fields, use Results() or Summary() which return copies under the lock.
func (s *TaskStore) List(ids []string, tag string) []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var result []*Task
	for _, id := range s.order {
		t := s.tasks[id]
		if len(idSet) > 0 && !idSet[t.ID] {
			continue
		}
		if tag != "" && t.Tag != tag {
			continue
		}
		result = append(result, t)
	}
	return result
}

// Summary returns aggregate counts and per-task statuses for the check_tasks
// tool. This is intentionally lightweight — no result content is included.
// The lock is held for the entire operation to avoid races with worker
// goroutines that mutate task status concurrently.
func (s *TaskStore) Summary(ids []string, tag string) (TaskSummary, []TaskStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	var summary TaskSummary
	var statuses []TaskStatus
	now := time.Now()

	for _, id := range s.order {
		t := s.tasks[id]
		if len(idSet) > 0 && !idSet[t.ID] {
			continue
		}
		if tag != "" && t.Tag != tag {
			continue
		}
		summary.Total++
		switch t.Status {
		case "pending":
			summary.Pending++
		case "running":
			summary.Running++
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		case "cancelled":
			summary.Cancelled++
		}
		statuses = append(statuses, TaskStatus{
			ID:             t.ID,
			Tag:            t.Tag,
			Status:         t.Status,
			Error:          t.Error,
			OutputFile:     t.OutputFile,
			ElapsedSeconds: taskElapsedSeconds(t, now),
		})
	}
	return summary, statuses
}

// taskElapsedSeconds computes wall-clock seconds for a task based on its state.
//   - pending: seconds since created (queue wait time)
//   - running: seconds since started (inference time so far)
//   - completed/failed: seconds from start to completion (actual work duration)
//   - cancelled: seconds from start to completion if it ran, else 0
func taskElapsedSeconds(t *Task, now time.Time) int {
	switch t.Status {
	case "pending":
		return int(now.Sub(t.CreatedAt).Seconds())
	case "running":
		return int(now.Sub(t.StartedAt).Seconds())
	case "completed", "failed":
		return int(t.CompletedAt.Sub(t.StartedAt).Seconds())
	case "cancelled":
		if !t.StartedAt.IsZero() {
			return int(t.CompletedAt.Sub(t.StartedAt).Seconds())
		}
		return 0
	}
	return 0
}

// Results returns the full content for specific task IDs. Used by get_result.
// If a task ID is not found, a "not_found" entry is returned for that ID.
func (s *TaskStore) Results(ids []string) []TaskResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]TaskResult, 0, len(ids))
	for _, id := range ids {
		t, ok := s.tasks[id]
		if !ok {
			results = append(results, TaskResult{
				ID:     id,
				Status: "not_found",
				Error:  "task not found",
			})
			continue
		}
		results = append(results, TaskResult{
			ID:         t.ID,
			Tag:        t.Tag,
			Status:     t.Status,
			Content:    t.Result,
			Error:      t.Error,
			OutputFile: t.OutputFile,
		})
	}
	return results
}

// SetRunning marks a task as running. Returns false if the task doesn't exist
// or isn't pending (e.g. it was already cancelled). Called by the worker pool
// when a goroutine acquires a semaphore slot and begins processing.
func (s *TaskStore) SetRunning(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[id]; ok && t.Status == "pending" {
		t.Status = "running"
		t.StartedAt = time.Now()
		return true
	}
	return false
}

// SetCompleted marks a task as completed and stores the Ollama response.
// Only transitions from "running" — a cancelled task won't be overwritten.
// Input fields are cleared to free memory since they're no longer needed.
// If the task wrote its output to a file (FileWritten), the Result is also
// cleared since the content is on disk.
func (s *TaskStore) SetCompleted(id string, result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[id]; ok && t.Status == "running" {
		t.Status = "completed"
		t.Result = result
		t.CompletedAt = time.Now()
		t.SystemPrompt = ""
		t.Prompt = ""
		t.InputFile = ""
		t.PostWriteCmd = ""
		t.Cancel = nil
		// If the result was written to a file, clear it from memory
		if t.FileWritten {
			t.Result = ""
		}
	}
}

// SetFailed marks a task as failed and stores the error message.
// Only transitions from "running" — a cancelled task won't be overwritten.
// Input fields (SystemPrompt, Prompt, InputFile, PostWriteCmd) are cleared
// to free memory since they're no longer needed.
func (s *TaskStore) SetFailed(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[id]; ok && t.Status == "running" {
		t.Status = "failed"
		t.Error = errMsg
		t.CompletedAt = time.Now()
		t.SystemPrompt = ""
		t.Prompt = ""
		t.InputFile = ""
		t.PostWriteCmd = ""
		t.Cancel = nil
	}
}

// SetFailedWithResult marks a task as failed but also stores the Ollama result.
// Used when Ollama succeeded but a subsequent step (file write, post-command)
// failed — the result is preserved so get_result can return it.
func (s *TaskStore) SetFailedWithResult(id string, result string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[id]; ok && t.Status == "running" {
		t.Status = "failed"
		t.Result = result
		t.Error = errMsg
		t.CompletedAt = time.Now()
		t.SystemPrompt = ""
		t.Prompt = ""
		t.InputFile = ""
		t.PostWriteCmd = ""
		t.Cancel = nil
	}
}

// SetFileWritten marks a task as having its output written to disk.
// Called by the worker after a successful file write, before SetCompleted.
func (s *TaskStore) SetFileWritten(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tasks[id]; ok {
		t.FileWritten = true
	}
}

// SetCancelled marks a single task as cancelled and calls its cancel function
// to abort any in-flight Ollama request. Only affects pending/running tasks.
// Returns true if the task was actually cancelled.
func (s *TaskStore) SetCancelled(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok || (t.Status != "pending" && t.Status != "running") {
		return false
	}
	prev := t.Status
	t.Status = "cancelled"
	t.CompletedAt = time.Now()
	if t.Cancel != nil {
		t.Cancel()
	}
	t.Cancel = nil
	// Only clear input fields for pending tasks. Running tasks may have a
	// worker goroutine concurrently reading these fields in callOllama.
	if prev == "pending" {
		t.SystemPrompt = ""
		t.Prompt = ""
		t.InputFile = ""
		t.PostWriteCmd = ""
	}
	return true
}

// Cancel cancels all tasks matching the filter and returns the count.
// If both ids and tag are empty, all pending/running tasks are cancelled.
func (s *TaskStore) Cancel(ids []string, tag string) int {
	targets := s.List(ids, tag)
	count := 0
	for _, t := range targets {
		if s.SetCancelled(t.ID) {
			count++
		}
	}
	return count
}
