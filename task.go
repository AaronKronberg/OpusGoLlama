// task.go defines the internal task representation used by the store and worker pool.
// Not exposed via MCP.
package main

import (
	"context"
	"time"
)

// Task is the internal representation of a work item. It tracks everything
// from the original request through to the final result.
//
// Lifecycle: pending -> running -> completed | failed
//
//	pending/running -> cancelled (via cancel_tasks)
type Task struct {
	ID           string
	Tag          string
	SystemPrompt string
	Prompt       string
	Model        string
	ResponseHint string

	InputFile           string
	OutputFile          string
	StripMarkdownFences bool   // plain bool — handler resolves default from *bool
	PostWriteCmd        string
	FileWritten         bool   // set by worker after successful file write

	TimeoutSeconds int              // per-task timeout; 0 means use default

	Status      string             // pending, running, completed, failed, cancelled
	Result      string             // full Ollama response (populated on completion)
	Error       string             // error message (populated on failure)
	Cancel      context.CancelFunc // cancels this task's context, aborting the Ollama call
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
}
