/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"encoding/json"
	"fmt"
)

// MockAgentHelper provides convenience functions for common mock scenarios
type MockAgentHelper struct {
	executor *MockAgentExecutor
}

// NewMockAgentHelper creates a new mock agent helper
func NewMockAgentHelper(executor *MockAgentExecutor) *MockAgentHelper {
	return &MockAgentHelper{executor: executor}
}

// SetSuccessResponse sets a successful response for an agent
// agentName can be "name" or "namespace/name"
func (h *MockAgentHelper) SetSuccessResponse(agentName, prompt, response string) {
	h.executor.SetResponse(agentName, prompt, response)
}

// SetErrorResponse sets an error response for an agent
func (h *MockAgentHelper) SetErrorResponse(agentName, prompt string, err error) {
	h.executor.SetError(agentName, prompt, err)
}

// SetJSONResponse sets a JSON response for an agent (automatically marshals the value)
func (h *MockAgentHelper) SetJSONResponse(agentName, prompt string, value interface{}) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON response: %w", err)
	}
	h.executor.SetResponse(agentName, prompt, string(jsonBytes))
	return nil
}

// SetDefaultSuccessResponse sets a default successful response for all agents
func (h *MockAgentHelper) SetDefaultSuccessResponse(response string) {
	h.executor.SetDefaultResponse(response)
}

// SetDefaultErrorResponse sets a default error response for all agents
func (h *MockAgentHelper) SetDefaultErrorResponse(err error) {
	h.executor.SetDefaultError(err)
}

// Reset clears all mock configurations
func (h *MockAgentHelper) Reset() {
	h.executor.Reset()
}

// GetCallHistory returns the call history
func (h *MockAgentHelper) GetCallHistory() []MockAgentCall {
	return h.executor.GetCallHistory()
}

// GetCallCount returns the number of calls made
func (h *MockAgentHelper) GetCallCount() int {
	return h.executor.GetCallCount()
}

// WasCalledWith checks if the executor was called with specific parameters
func (h *MockAgentHelper) WasCalledWith(agentName, prompt string) bool {
	return h.executor.WasCalledWith(agentName, prompt)
}

// GetCallsForAgent returns all calls for a specific agent
func (h *MockAgentHelper) GetCallsForAgent(agentName string) []MockAgentCall {
	return h.executor.GetCallsForAgent(agentName)
}

// CommonMockScenarios provides pre-configured mock scenarios for testing

// SetScenario_SuccessfulAnalysis sets up a mock for a successful analysis agent
func (h *MockAgentHelper) SetScenario_SuccessfulAnalysis(agentName string) {
	h.SetSuccessResponse(agentName, "", `{
		"analysis": "successful",
		"recommendations": ["recommendation1", "recommendation2"]
	}`)
}

// SetScenario_FailedExecution sets up a mock for a failed agent execution
func (h *MockAgentHelper) SetScenario_FailedExecution(agentName string, err error) {
	if err == nil {
		err = fmt.Errorf("agent execution failed")
	}
	h.SetErrorResponse(agentName, "", err)
}

// SetScenario_Timeout sets up a mock for a timeout scenario
func (h *MockAgentHelper) SetScenario_Timeout(agentName string) {
	h.SetErrorResponse(agentName, "", fmt.Errorf("agent execution timeout"))
}

// SetScenario_EmptyResponse sets up a mock for an empty response
func (h *MockAgentHelper) SetScenario_EmptyResponse(agentName string) {
	h.SetSuccessResponse(agentName, "", "")
}

// SetScenario_LargeResponse sets up a mock for a large response
func (h *MockAgentHelper) SetScenario_LargeResponse(agentName string, sizeKB int) {
	largeResponse := make([]byte, sizeKB*1024)
	for i := range largeResponse {
		largeResponse[i] = 'A'
	}
	h.SetSuccessResponse(agentName, "", string(largeResponse))
}
