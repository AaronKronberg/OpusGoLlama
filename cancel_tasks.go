// cancel_tasks.go defines the cancel_tasks tool types.
package main

// CancelTasksArgs is the input for the cancel_tasks tool.
type CancelTasksArgs struct {
	// TaskIDs cancels specific tasks. If both TaskIDs and Tag are empty,
	// all pending/running tasks are cancelled.
	TaskIDs []string `json:"task_ids,omitempty" jsonschema:"Specific task IDs to cancel. Empty with no tag cancels all."`
	Tag     string   `json:"tag,omitempty"      jsonschema:"Cancel all tasks with this tag"`
}

// CancelTasksOutput reports how many tasks were actually cancelled.
type CancelTasksOutput struct {
	Cancelled int `json:"cancelled"`
}
