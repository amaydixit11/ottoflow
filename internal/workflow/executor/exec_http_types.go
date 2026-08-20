/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

// ExecRequest is the JSON body sent by the workflow runner to
// POST /api/exec/{namespace}/{agentName}.
type ExecRequest struct {
	Prompt  string                 `json:"prompt"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// ExecResponse is the JSON body returned on a successful execution.
type ExecResponse struct {
	Content          string                 `json:"content"`
	ExtractedOutputs map[string]interface{} `json:"extractedOutputs,omitempty"`
	InputTokens      int64                  `json:"inputTokens,omitempty"`
	OutputTokens     int64                  `json:"outputTokens,omitempty"`
}

// ExecErrorResponse is the JSON body returned on a failed execution (HTTP 4xx/5xx).
type ExecErrorResponse struct {
	Error string `json:"error"`
}
