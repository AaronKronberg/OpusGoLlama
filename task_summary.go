// task_summary.go defines the check_tasks tool types: lightweight status
// polling with aggregate counts and per-task status (no result content).
package main

// CheckTasksArgs is the input for the check_tasks tool.
type CheckTasksArgs struct {
	// TaskIDs filters to specific tasks. Empty returns all tasks.
	TaskIDs []string `json:"task_ids,omitempty" jsonschema:"Filter to specific task IDs. Empty returns all."`
	// Tag filters to tasks with a matching tag.
	Tag string `json:"tag,omitempty" jsonschema:"Filter tasks by tag"`
}

// CheckTasksOutput contains a compact summary plus individual task statuses.
type CheckTasksOutput struct {
	Summary TaskSummary  `json:"summary"`
	Tasks   []TaskStatus `json:"tasks"`
}

// TaskSummary provides aggregate counts across all matched tasks.
type TaskSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
}

// TaskStatus is the per-task view in check_tasks. Intentionally omits the
// full result content — use get_result for that.
type TaskStatus struct {
	ID             string `json:"id"`
	Tag            string `json:"tag,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`       // brief error message if failed
	OutputFile     string `json:"output_file,omitempty"` // path where output was written (if applicable)
	ElapsedSeconds int    `json:"elapsed_seconds"`       // wall-clock seconds (meaning varies by status)
}
