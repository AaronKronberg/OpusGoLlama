// model_info.go defines the list_models tool types.
package main

// ListModelsArgs is the input for the list_models tool. No arguments needed.
type ListModelsArgs struct{}

// ListModelsOutput lists all models available in the local Ollama instance.
type ListModelsOutput struct {
	Models []ModelInfo `json:"models"`
}

// ModelInfo describes a single Ollama model's capabilities.
type ModelInfo struct {
	Name              string `json:"name"`
	Size              int64  `json:"size"`                // size in bytes
	ParameterSize     string `json:"parameter_size"`      // e.g. "14B", "7B"
	QuantizationLevel string `json:"quantization_level"`  // e.g. "Q4_K_M"
	Family            string `json:"family"`              // e.g. "qwen2"
}
