/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"fmt"
	"sync"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// MockAgentExecutor is a mock implementation of AgentExecutor for testing
// It allows configuring responses for different agents and prompts
type MockAgentExecutor struct {
	mu sync.RWMutex
	// responses maps agent name -> prompt -> response
	responses map[string]map[string]string
	// errors maps agent name -> prompt -> error
	errors map[string]map[string]error
	// defaultResponse is returned when no specific response is configured
	defaultResponse string
	// defaultError is returned when no specific error is configured
	defaultError error
	// callHistory records all ExecuteAgent calls for verification
	callHistory []MockAgentCall
}

// MockAgentCall represents a call to ExecuteAgent
type MockAgentCall struct {
	AgentName string
	Agent     *ottoflowv1alpha1.Agent
	Prompt    string
	Context   map[string]interface{}
	Namespace string
	Response  string
	Error     error
}

// NewMockAgentExecutor creates a new mock agent executor
func NewMockAgentExecutor() *MockAgentExecutor {
	return &MockAgentExecutor{
		responses:   make(map[string]map[string]string),
		errors:      make(map[string]map[string]error),
		callHistory: []MockAgentCall{},
	}
}

// ExecuteAgent implements the AgentExecutor interface
func (m *MockAgentExecutor) ExecuteAgent(ctx context.Context, agent *ottoflowv1alpha1.Agent, prompt string, workflowContext map[string]interface{}, namespace string) (string, AgentTokenUsage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agentName := agent.Name
	if agent.Namespace != "" {
		agentName = fmt.Sprintf("%s/%s", agent.Namespace, agent.Name)
	}

	// Check for specific error
	if agentErrors, ok := m.errors[agentName]; ok {
		if err, ok := agentErrors[prompt]; ok {
			call := MockAgentCall{
				AgentName: agentName,
				Agent:     agent,
				Prompt:    prompt,
				Context:   workflowContext,
				Namespace: namespace,
				Error:     err,
			}
			m.callHistory = append(m.callHistory, call)
			return "", AgentTokenUsage{}, err
		}
	}

	// Check for specific response
	if agentResponses, ok := m.responses[agentName]; ok {
		if response, ok := agentResponses[prompt]; ok {
			call := MockAgentCall{
				AgentName: agentName,
				Agent:     agent,
				Prompt:    prompt,
				Context:   workflowContext,
				Namespace: namespace,
				Response:  response,
			}
			m.callHistory = append(m.callHistory, call)
			return response, AgentTokenUsage{}, nil
		}
	}

	// Check for default error
	if m.defaultError != nil {
		call := MockAgentCall{
			AgentName: agentName,
			Agent:     agent,
			Prompt:    prompt,
			Context:   workflowContext,
			Namespace: namespace,
			Error:     m.defaultError,
		}
		m.callHistory = append(m.callHistory, call)
		return "", AgentTokenUsage{}, m.defaultError
	}

	// Use default response or generate one
	response := m.defaultResponse
	if response == "" {
		response = fmt.Sprintf("Mock response for agent %s with prompt: %s", agentName, prompt)
	}

	call := MockAgentCall{
		AgentName: agentName,
		Agent:     agent,
		Prompt:    prompt,
		Context:   workflowContext,
		Namespace: namespace,
		Response:  response,
	}
	m.callHistory = append(m.callHistory, call)
	return response, AgentTokenUsage{}, nil
}

// SetResponse sets a specific response for an agent and prompt combination
func (m *MockAgentExecutor) SetResponse(agentName, prompt, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.responses[agentName] == nil {
		m.responses[agentName] = make(map[string]string)
	}
	m.responses[agentName][prompt] = response
}

// SetError sets a specific error for an agent and prompt combination
func (m *MockAgentExecutor) SetError(agentName, prompt string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errors[agentName] == nil {
		m.errors[agentName] = make(map[string]error)
	}
	m.errors[agentName][prompt] = err
}

// SetDefaultResponse sets the default response when no specific response is configured
func (m *MockAgentExecutor) SetDefaultResponse(response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultResponse = response
}

// SetDefaultError sets the default error when no specific error is configured
func (m *MockAgentExecutor) SetDefaultError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultError = err
}

// Reset clears all configured responses, errors, and call history
func (m *MockAgentExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.responses = make(map[string]map[string]string)
	m.errors = make(map[string]map[string]error)
	m.defaultResponse = ""
	m.defaultError = nil
	m.callHistory = []MockAgentCall{}
}

// GetCallHistory returns all calls made to ExecuteAgent
func (m *MockAgentExecutor) GetCallHistory() []MockAgentCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make([]MockAgentCall, len(m.callHistory))
	copy(history, m.callHistory)
	return history
}

// GetCallCount returns the number of times ExecuteAgent was called
func (m *MockAgentExecutor) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.callHistory)
}

// WasCalledWith checks if ExecuteAgent was called with specific agent and prompt
func (m *MockAgentExecutor) WasCalledWith(agentName, prompt string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, call := range m.callHistory {
		if call.AgentName == agentName && call.Prompt == prompt {
			return true
		}
	}
	return false
}

// GetCallsForAgent returns all calls for a specific agent
func (m *MockAgentExecutor) GetCallsForAgent(agentName string) []MockAgentCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var calls []MockAgentCall
	for _, call := range m.callHistory {
		if call.AgentName == agentName {
			calls = append(calls, call)
		}
	}
	return calls
}
