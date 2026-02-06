package main

import (
	"sync"
	"testing"
	"time"
)

// helper to create a minimal task with the given id, tag, and status.
func makeTask(id, tag, status string) *Task {
	return &Task{
		ID:           id,
		Tag:          tag,
		SystemPrompt: "sys:" + id,
		Prompt:       "prompt:" + id,
		InputFile:    "input:" + id,
		Status:       status,
		CreatedAt:    time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Add / Get / List basics
// ---------------------------------------------------------------------------

func TestAddAndGet(t *testing.T) {
	s := NewTaskStore()
	task := makeTask("t1", "", "pending")
	s.Add([]*Task{task})

	got := s.Get("t1")
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.ID != "t1" {
		t.Fatalf("expected ID t1, got %s", got.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewTaskStore()
	if s.Get("nope") != nil {
		t.Fatal("expected nil for missing task")
	}
}

func TestListAll(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending"), makeTask("b", "", "pending")})

	all := s.List(nil, "")
	if len(all) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(all))
	}
	// insertion order preserved
	if all[0].ID != "a" || all[1].ID != "b" {
		t.Fatalf("unexpected order: %s, %s", all[0].ID, all[1].ID)
	}
}

// ---------------------------------------------------------------------------
// Status transition guards
// ---------------------------------------------------------------------------

func TestSetRunningOnlyFromPending(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})

	if !s.SetRunning("t1") {
		t.Fatal("SetRunning should succeed from pending")
	}
	if s.Get("t1").Status != "running" {
		t.Fatal("status should be running")
	}
	// second call should fail (already running)
	if s.SetRunning("t1") {
		t.Fatal("SetRunning should fail from running")
	}
}

func TestSetRunningFailsFromCompleted(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetCompleted("t1", "done")

	if s.SetRunning("t1") {
		t.Fatal("SetRunning should fail from completed")
	}
}

func TestSetCompletedOnlyFromRunning(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})

	// cannot complete a pending task
	s.SetCompleted("t1", "result")
	if s.Get("t1").Status != "pending" {
		t.Fatal("SetCompleted should not affect pending task")
	}

	s.SetRunning("t1")
	s.SetCompleted("t1", "result")
	if s.Get("t1").Status != "completed" {
		t.Fatal("status should be completed")
	}
}

func TestSetFailedOnlyFromRunning(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})

	s.SetFailed("t1", "err")
	if s.Get("t1").Status != "pending" {
		t.Fatal("SetFailed should not affect pending task")
	}

	s.SetRunning("t1")
	s.SetFailed("t1", "err")
	if s.Get("t1").Status != "failed" {
		t.Fatal("status should be failed")
	}
}

func TestSetCancelledFromPending(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	if !s.SetCancelled("t1") {
		t.Fatal("should cancel pending task")
	}
	if s.Get("t1").Status != "cancelled" {
		t.Fatal("status should be cancelled")
	}
}

func TestSetCancelledFromRunning(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	if !s.SetCancelled("t1") {
		t.Fatal("should cancel running task")
	}
	if s.Get("t1").Status != "cancelled" {
		t.Fatal("status should be cancelled")
	}
}

// ---------------------------------------------------------------------------
// Cancel doesn't overwrite completed — the race condition fix
// ---------------------------------------------------------------------------

func TestCancelDoesNotOverwriteCompleted(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetCompleted("t1", "done")

	if s.SetCancelled("t1") {
		t.Fatal("cancel should return false for completed task")
	}
	if s.Get("t1").Status != "completed" {
		t.Fatal("completed status should not change")
	}
}

func TestCancelDoesNotOverwriteFailed(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetFailed("t1", "err")

	if s.SetCancelled("t1") {
		t.Fatal("cancel should return false for failed task")
	}
	if s.Get("t1").Status != "failed" {
		t.Fatal("failed status should not change")
	}
}

// ---------------------------------------------------------------------------
// Memory cleanup on terminal states
// ---------------------------------------------------------------------------

func TestMemoryCleanupOnCompleted(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetCompleted("t1", "result")

	task := s.Get("t1")
	if task.InputFile != "" || task.SystemPrompt != "" || task.Prompt != "" || task.PostWriteCmd != "" {
		t.Fatal("input fields should be cleared on completion")
	}
	if task.Cancel != nil {
		t.Fatal("Cancel func should be nil on completion")
	}
}

func TestMemoryCleanupOnFailed(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetFailed("t1", "err")

	task := s.Get("t1")
	if task.InputFile != "" || task.SystemPrompt != "" || task.Prompt != "" || task.PostWriteCmd != "" {
		t.Fatal("input fields should be cleared on failure")
	}
}

func TestMemoryCleanupOnCancelled(t *testing.T) {
	s := NewTaskStore()
	task := makeTask("t1", "", "pending")
	task.Cancel = func() {} // set a cancel func
	s.Add([]*Task{task})
	s.SetCancelled("t1")

	got := s.Get("t1")
	if got.InputFile != "" || got.SystemPrompt != "" || got.Prompt != "" || got.PostWriteCmd != "" {
		t.Fatal("input fields should be cleared on cancellation")
	}
	if got.Cancel != nil {
		t.Fatal("Cancel func should be nil on cancellation")
	}
}

// ---------------------------------------------------------------------------
// List filtering
// ---------------------------------------------------------------------------

func TestListFilterByIDs(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "x", "pending"), makeTask("b", "y", "pending"), makeTask("c", "x", "pending")})

	got := s.List([]string{"a", "c"}, "")
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("unexpected IDs: %s, %s", got[0].ID, got[1].ID)
	}
}

func TestListFilterByTag(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "x", "pending"), makeTask("b", "y", "pending"), makeTask("c", "x", "pending")})

	got := s.List(nil, "x")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestListFilterByIDsAndTag(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "x", "pending"), makeTask("b", "y", "pending"), makeTask("c", "x", "pending")})

	got := s.List([]string{"a", "b"}, "x")
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("expected only task a, got %v", got)
	}
}

func TestListEmpty(t *testing.T) {
	s := NewTaskStore()
	got := s.List(nil, "")
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Summary counts
// ---------------------------------------------------------------------------

func TestSummaryCounts(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{
		makeTask("a", "", "pending"),
		makeTask("b", "", "pending"),
		makeTask("c", "", "pending"),
		makeTask("d", "", "pending"),
		makeTask("e", "", "pending"),
	})
	s.SetRunning("b")
	s.SetRunning("c")
	s.SetCompleted("c", "ok")
	s.SetRunning("d")
	s.SetFailed("d", "err")
	s.SetCancelled("e")

	summary, statuses := s.Summary(nil, "")
	if summary.Total != 5 {
		t.Fatalf("total: want 5, got %d", summary.Total)
	}
	if summary.Pending != 1 {
		t.Fatalf("pending: want 1, got %d", summary.Pending)
	}
	if summary.Running != 1 {
		t.Fatalf("running: want 1, got %d", summary.Running)
	}
	if summary.Completed != 1 {
		t.Fatalf("completed: want 1, got %d", summary.Completed)
	}
	if summary.Failed != 1 {
		t.Fatalf("failed: want 1, got %d", summary.Failed)
	}
	if summary.Cancelled != 1 {
		t.Fatalf("cancelled: want 1, got %d", summary.Cancelled)
	}
	if len(statuses) != 5 {
		t.Fatalf("statuses: want 5, got %d", len(statuses))
	}
}

func TestSummaryFiltered(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "x", "pending"), makeTask("b", "y", "pending")})
	summary, statuses := s.Summary(nil, "x")
	if summary.Total != 1 || len(statuses) != 1 {
		t.Fatalf("expected 1 task for tag x, got total=%d statuses=%d", summary.Total, len(statuses))
	}
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

func TestResultsFound(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending")})
	s.SetRunning("a")
	s.SetCompleted("a", "hello world")

	results := s.Results([]string{"a"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "completed" || results[0].Content != "hello world" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestResultsNotFound(t *testing.T) {
	s := NewTaskStore()
	results := s.Results([]string{"missing"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "not_found" {
		t.Fatalf("expected not_found, got %s", results[0].Status)
	}
}

func TestResultsMixed(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending")})
	s.SetRunning("a")
	s.SetCompleted("a", "ok")

	results := s.Results([]string{"a", "missing"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "completed" {
		t.Fatalf("first result: want completed, got %s", results[0].Status)
	}
	if results[1].Status != "not_found" {
		t.Fatalf("second result: want not_found, got %s", results[1].Status)
	}
}

// ---------------------------------------------------------------------------
// Results error propagation
// ---------------------------------------------------------------------------

func TestResultsErrorField(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "grp", "pending")})
	s.SetRunning("a")
	s.SetFailed("a", "connection refused")

	results := s.Results([]string{"a"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Fatalf("expected failed, got %s", results[0].Status)
	}
	if results[0].Error != "connection refused" {
		t.Fatalf("expected error 'connection refused', got %q", results[0].Error)
	}
	if results[0].Tag != "grp" {
		t.Fatalf("expected tag 'grp', got %q", results[0].Tag)
	}
}

// ---------------------------------------------------------------------------
// Summary error field in TaskStatus
// ---------------------------------------------------------------------------

func TestSummaryErrorField(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending")})
	s.SetRunning("a")
	s.SetFailed("a", "out of memory")

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Error != "out of memory" {
		t.Fatalf("expected error in summary status, got %q", statuses[0].Error)
	}
}

// ---------------------------------------------------------------------------
// Transitions on non-existent task IDs
// ---------------------------------------------------------------------------

func TestSetRunningNonExistent(t *testing.T) {
	s := NewTaskStore()
	if s.SetRunning("nope") {
		t.Fatal("SetRunning should return false for non-existent ID")
	}
}

func TestSetCompletedNonExistent(t *testing.T) {
	s := NewTaskStore()
	// Should not panic
	s.SetCompleted("nope", "result")
}

func TestSetFailedNonExistent(t *testing.T) {
	s := NewTaskStore()
	// Should not panic
	s.SetFailed("nope", "err")
}

func TestSetCancelledNonExistent(t *testing.T) {
	s := NewTaskStore()
	if s.SetCancelled("nope") {
		t.Fatal("SetCancelled should return false for non-existent ID")
	}
}

// ---------------------------------------------------------------------------
// Cancel with both IDs and tag (AND logic)
// ---------------------------------------------------------------------------

func TestCancelByIDsAndTag(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{
		makeTask("a", "x", "pending"),
		makeTask("b", "y", "pending"),
		makeTask("c", "x", "pending"),
	})
	// Only "a" matches both the ID list and the tag
	count := s.Cancel([]string{"a", "b"}, "x")
	if count != 1 {
		t.Fatalf("expected 1 cancelled (AND logic), got %d", count)
	}
	if s.Get("a").Status != "cancelled" {
		t.Fatal("task a should be cancelled")
	}
	if s.Get("b").Status != "pending" {
		t.Fatal("task b should still be pending (wrong tag)")
	}
	if s.Get("c").Status != "pending" {
		t.Fatal("task c should still be pending (not in ID list)")
	}
}

// ---------------------------------------------------------------------------
// Cancel variants
// ---------------------------------------------------------------------------

func TestCancelByTag(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "x", "pending"), makeTask("b", "y", "pending"), makeTask("c", "x", "pending")})
	count := s.Cancel(nil, "x")
	if count != 2 {
		t.Fatalf("expected 2 cancelled, got %d", count)
	}
	if s.Get("b").Status != "pending" {
		t.Fatal("task b should still be pending")
	}
}

func TestCancelByIDs(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending"), makeTask("b", "", "pending"), makeTask("c", "", "pending")})
	count := s.Cancel([]string{"a", "c"}, "")
	if count != 2 {
		t.Fatalf("expected 2 cancelled, got %d", count)
	}
	if s.Get("b").Status != "pending" {
		t.Fatal("task b should still be pending")
	}
}

func TestCancelAll(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending"), makeTask("b", "", "pending")})
	s.SetRunning("b")
	count := s.Cancel(nil, "")
	if count != 2 {
		t.Fatalf("expected 2 cancelled, got %d", count)
	}
}

func TestCancelCallsCancelFunc(t *testing.T) {
	s := NewTaskStore()
	called := false
	task := makeTask("t1", "", "pending")
	task.Cancel = func() { called = true }
	s.Add([]*Task{task})
	s.SetCancelled("t1")
	if !called {
		t.Fatal("Cancel func should have been called")
	}
}

// ---------------------------------------------------------------------------
// Concurrent access (designed to catch races with -race flag)
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	s := NewTaskStore()
	const n = 100
	tasks := make([]*Task, n)
	for i := range tasks {
		id := "t" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		tasks[i] = makeTask(id, "concurrent", "pending")
	}
	s.Add(tasks)

	var wg sync.WaitGroup
	// Goroutines racing to transition tasks
	for _, task := range tasks {
		wg.Add(3)
		id := task.ID
		go func() {
			defer wg.Done()
			s.SetRunning(id)
		}()
		go func() {
			defer wg.Done()
			s.SetCompleted(id, "done")
		}()
		go func() {
			defer wg.Done()
			s.SetCancelled(id)
		}()
	}
	wg.Wait()

	// Every task should be in a terminal or running state — no panics, no data races.
	for _, task := range tasks {
		got := s.Get(task.ID)
		switch got.Status {
		case "pending", "running", "completed", "cancelled":
			// all valid
		default:
			t.Fatalf("unexpected status %q for task %s", got.Status, task.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// Cancelling a running task preserves input fields (avoids racing with worker)
// ---------------------------------------------------------------------------

func TestMemoryPreservedOnCancelledRunning(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("t1", "", "pending")})
	s.SetRunning("t1")
	s.SetCancelled("t1")

	task := s.Get("t1")
	if task.Status != "cancelled" {
		t.Fatal("status should be cancelled")
	}
	// Input fields are NOT cleared for running tasks to avoid racing
	// with worker goroutines that may be reading them in callOllama.
	if task.InputFile == "" {
		t.Fatal("InputFile should be preserved when cancelling a running task")
	}
	if task.SystemPrompt == "" {
		t.Fatal("system prompt should be preserved when cancelling a running task")
	}
	if task.Cancel != nil {
		t.Fatal("Cancel func should be nil on cancellation")
	}
}

// ---------------------------------------------------------------------------
// Summary on empty store
// ---------------------------------------------------------------------------

func TestSummaryEmpty(t *testing.T) {
	s := NewTaskStore()
	summary, statuses := s.Summary(nil, "")
	if summary.Total != 0 {
		t.Fatalf("expected 0 total, got %d", summary.Total)
	}
	if len(statuses) != 0 {
		t.Fatalf("expected 0 statuses, got %d", len(statuses))
	}
}

// ---------------------------------------------------------------------------
// Cancel on empty store
// ---------------------------------------------------------------------------

func TestCancelEmpty(t *testing.T) {
	s := NewTaskStore()
	count := s.Cancel(nil, "")
	if count != 0 {
		t.Fatalf("expected 0 cancelled on empty store, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Duplicate IDs in Results
// ---------------------------------------------------------------------------

func TestResultsDuplicateIDs(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{makeTask("a", "", "pending")})
	s.SetRunning("a")
	s.SetCompleted("a", "result")

	results := s.Results([]string{"a", "a"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results for duplicate IDs, got %d", len(results))
	}
	for _, r := range results {
		if r.ID != "a" || r.Status != "completed" || r.Content != "result" {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}

// ---------------------------------------------------------------------------
// Summary tag propagation
// ---------------------------------------------------------------------------

func TestSummaryTagPropagation(t *testing.T) {
	s := NewTaskStore()
	s.Add([]*Task{
		makeTask("a", "batch1", "pending"),
		makeTask("b", "batch2", "pending"),
	})

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Tag != "batch1" {
		t.Fatalf("expected tag 'batch1', got %q", statuses[0].Tag)
	}
	if statuses[1].Tag != "batch2" {
		t.Fatalf("expected tag 'batch2', got %q", statuses[1].Tag)
	}
}

// ---------------------------------------------------------------------------
// Results include OutputFile
// ---------------------------------------------------------------------------

func TestResultsIncludeOutputFile(t *testing.T) {
	s := NewTaskStore()
	task := &Task{
		ID:         "a",
		Status:     "pending",
		OutputFile: "/tmp/out.go",
		CreatedAt:  time.Now(),
	}
	s.Add([]*Task{task})
	s.SetRunning("a")
	s.SetCompleted("a", "result")

	results := s.Results([]string{"a"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].OutputFile != "/tmp/out.go" {
		t.Fatalf("expected OutputFile '/tmp/out.go', got %q", results[0].OutputFile)
	}
}

// ---------------------------------------------------------------------------
// Summary includes OutputFile
// ---------------------------------------------------------------------------

func TestSummaryIncludesOutputFile(t *testing.T) {
	s := NewTaskStore()
	task := &Task{
		ID:         "a",
		Status:     "pending",
		OutputFile: "/tmp/out.go",
		CreatedAt:  time.Now(),
	}
	s.Add([]*Task{task})

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].OutputFile != "/tmp/out.go" {
		t.Fatalf("expected OutputFile '/tmp/out.go', got %q", statuses[0].OutputFile)
	}
}

// ---------------------------------------------------------------------------
// SetCompleted clears Result when FileWritten is true
// ---------------------------------------------------------------------------

func TestSetCompletedClearsResultWhenFileWritten(t *testing.T) {
	s := NewTaskStore()
	task := &Task{
		ID:         "a",
		Status:     "pending",
		OutputFile: "/tmp/out.go",
		CreatedAt:  time.Now(),
	}
	s.Add([]*Task{task})
	s.SetRunning("a")
	s.SetFileWritten("a")
	s.SetCompleted("a", "the result")

	got := s.Get("a")
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Result != "" {
		t.Fatal("Result should be cleared when FileWritten is true")
	}
	if got.OutputFile != "/tmp/out.go" {
		t.Fatal("OutputFile should be preserved")
	}
}

// ---------------------------------------------------------------------------
// SetCompleted keeps Result when no OutputFile
// ---------------------------------------------------------------------------

func TestSetCompletedKeepsResultWhenNoOutputFile(t *testing.T) {
	s := NewTaskStore()
	task := &Task{
		ID:        "a",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	s.Add([]*Task{task})
	s.SetRunning("a")
	s.SetCompleted("a", "the result")

	got := s.Get("a")
	if got.Result != "the result" {
		t.Fatalf("expected Result 'the result', got %q", got.Result)
	}
}

// ---------------------------------------------------------------------------
// SetFailedWithResult
// ---------------------------------------------------------------------------

func TestSetFailedWithResult(t *testing.T) {
	s := NewTaskStore()
	task := &Task{
		ID:        "a",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	s.Add([]*Task{task})
	s.SetRunning("a")
	s.SetFailedWithResult("a", "ollama output", "write failed")

	got := s.Get("a")
	if got.Status != "failed" {
		t.Fatalf("expected failed, got %s", got.Status)
	}
	if got.Result != "ollama output" {
		t.Fatalf("expected Result 'ollama output', got %q", got.Result)
	}
	if got.Error != "write failed" {
		t.Fatalf("expected Error 'write failed', got %q", got.Error)
	}
	// Input fields should still be cleared
	if got.SystemPrompt != "" || got.Prompt != "" || got.InputFile != "" || got.PostWriteCmd != "" {
		t.Fatal("input fields should be cleared on SetFailedWithResult")
	}
}

func TestSetFailedWithResultOnlyFromRunning(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now()}
	s.Add([]*Task{task})

	// Should not transition from pending
	s.SetFailedWithResult("a", "result", "err")
	if s.Get("a").Status != "pending" {
		t.Fatal("SetFailedWithResult should not affect pending task")
	}
}

// ---------------------------------------------------------------------------
// SetFileWritten
// ---------------------------------------------------------------------------

func TestSetFileWritten(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now()}
	s.Add([]*Task{task})

	s.SetFileWritten("a")
	if !s.Get("a").FileWritten {
		t.Fatal("FileWritten should be true after SetFileWritten")
	}
}

func TestSetFileWrittenNonExistent(t *testing.T) {
	s := NewTaskStore()
	// Should not panic
	s.SetFileWritten("nope")
}

// ---------------------------------------------------------------------------
// ElapsedSeconds in Summary
// ---------------------------------------------------------------------------

func TestSummaryElapsedPending(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now().Add(-2 * time.Second)}
	s.Add([]*Task{task})

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].ElapsedSeconds < 1 {
		t.Fatalf("pending elapsed_seconds should be > 0, got %d", statuses[0].ElapsedSeconds)
	}
}

func TestSummaryElapsedRunning(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now()}
	s.Add([]*Task{task})
	s.SetRunning("a")

	// Backdate StartedAt to simulate time passing
	s.mu.Lock()
	s.tasks["a"].StartedAt = time.Now().Add(-3 * time.Second)
	s.mu.Unlock()

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].ElapsedSeconds < 2 {
		t.Fatalf("running elapsed_seconds should be >= 2, got %d", statuses[0].ElapsedSeconds)
	}
}

func TestSummaryElapsedCompleted(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now()}
	s.Add([]*Task{task})
	s.SetRunning("a")

	// Backdate StartedAt so completion has a measurable duration
	s.mu.Lock()
	s.tasks["a"].StartedAt = time.Now().Add(-2 * time.Second)
	s.mu.Unlock()

	s.SetCompleted("a", "done")

	_, statuses1 := s.Summary(nil, "")
	if len(statuses1) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses1))
	}
	elapsed1 := statuses1[0].ElapsedSeconds
	if elapsed1 < 1 {
		t.Fatalf("completed elapsed_seconds should be > 0, got %d", elapsed1)
	}

	// Verify stable — doesn't grow on subsequent calls
	time.Sleep(10 * time.Millisecond)
	_, statuses2 := s.Summary(nil, "")
	elapsed2 := statuses2[0].ElapsedSeconds
	if elapsed2 != elapsed1 {
		t.Fatalf("completed elapsed_seconds should be stable, got %d then %d", elapsed1, elapsed2)
	}
}

func TestSummaryElapsedCancelledFromPending(t *testing.T) {
	s := NewTaskStore()
	task := &Task{ID: "a", Status: "pending", CreatedAt: time.Now()}
	s.Add([]*Task{task})
	s.SetCancelled("a")

	_, statuses := s.Summary(nil, "")
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].ElapsedSeconds != 0 {
		t.Fatalf("cancelled-from-pending elapsed_seconds should be 0, got %d", statuses[0].ElapsedSeconds)
	}
}
