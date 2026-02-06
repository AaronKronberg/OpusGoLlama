// task_result.go defines the get_result tool types: full Ollama response
// retrieval for specific completed or failed tasks.
package main

// GetResultArgs is the input for the get_result tool.
type GetResultArgs struct {
	TaskIDs []string `json:"task_ids" jsonschema:"Task IDs to retrieve full results for"`
}

// GetResultOutput contains the full content for each requested task.
type GetResultOutput struct {
	Results []TaskResult `json:"results"`
}

// TaskResult includes the full Ollama response text for a single task.
type TaskResult struct {
	ID         string `json:"id"`
	Tag        string `json:"tag,omitempty"`
	Status     string `json:"status"`
	Content    string `json:"content,omitempty"`     // full Ollama response (empty if written to output_file)
	Error      string `json:"error,omitempty"`
	OutputFile string `json:"output_file,omitempty"` // path where output was written (if applicable)
}
