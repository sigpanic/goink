package seeddata

import "encoding/json"

// SeedData is the complete input written after a released Run creates a fresh schema.
// Only this data type is compiled with historical source; assertions stay in current code.
type SeedData struct {
	Rows      []SeedRow         `json:"rows"`
	Files     map[string]string `json:"files"`
	LLMConfig json.RawMessage   `json:"llm_config"`
}

type SeedRow struct {
	Table  string         `json:"table"`
	Values map[string]any `json:"values"`
}
