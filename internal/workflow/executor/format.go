/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"encoding/json"
	"fmt"
)

// formatValueForPrompt converts a value to its string representation for use
// in agent prompts. Strings are returned as-is. Complex types (maps, slices,
// structs) are serialized as JSON so the LLM receives structured data instead
// of Go's fmt %v format (e.g. {"key":"value"} instead of map[key:value]).
func formatValueForPrompt(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
