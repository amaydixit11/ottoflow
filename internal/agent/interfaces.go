/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// AgentTokenUsage holds token counts from an agent execution.
// Zero value means no usage data was available.
type AgentTokenUsage struct {
	InputTokens  int64
	OutputTokens int64
}

// AgentExecutor defines the interface for executing agent steps
type AgentExecutor interface {
	// ExecuteAgent executes an agent step with the given prompt and context.
	// namespace is used for MCP server/tool lookup when the agent has mcpTools configured.
	// Returns the agent response, token usage, and any error.
	ExecuteAgent(ctx context.Context, agent *ottoflowv1alpha1.Agent, prompt string, workflowContext map[string]interface{}, namespace string) (string, AgentTokenUsage, error)
}

// MCPToolMeta describes one MCP tool (name, description, input schema) for registration with the LLM.
type MCPToolMeta struct {
	Name        string
	Description string
	InputSchema *gollm.Schema
}

// MCPClient defines the interface for MCP tool calls
type MCPClient interface {
	// ListTools returns metadata for all tools exposed by the MCP server (for LLM tool registration).
	ListTools(ctx context.Context) ([]MCPToolMeta, error)

	// CallTool calls an MCP tool with the given arguments
	// Returns the tool result and any error
	CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (interface{}, error)

	// Close closes the MCP client connection
	Close() error
}

// MCPClientProvider supplies MCP clients by server name and namespace (e.g. for agent MCP tool registration).
type MCPClientProvider interface {
	GetClient(ctx context.Context, serverName string, namespace string) (MCPClient, error)
}

// MCPClientFactory creates MCP clients from MCPServer configurations
type MCPClientFactory interface {
	// CreateClient creates a new MCP client from an MCPServer CRD
	CreateClient(ctx context.Context, mcpServer *ottoflowv1alpha1.MCPServer) (MCPClient, error)
}

// RealMCPClientBuilder builds a real MCP client from config (used by DefaultMCPClientFactory).
// When nil, DefaultMCPClientFactory uses kubectl-ai mcp.NewClient. Inject in tests to avoid real MCP.
type RealMCPClientBuilder interface {
	// Build returns an MCPClient for the given config (e.g. a realMCPClient or test double).
	Build(ctx context.Context, serverName string, connectionTimeout time.Duration, cfg interface{}) (MCPClient, error)
}

// OutputExtractor extracts structured outputs from agent responses
type OutputExtractor interface {
	// Extract extracts outputs from the agent response according to the extraction config
	Extract(response string, config *ottoflowv1alpha1.OutputExtraction) (map[string]interface{}, error)
}

// LLMClientFactory creates gollm clients for a given provider and options.
// Used for testing; when nil, DefaultAgentExecutor uses gollm.NewClient.
type LLMClientFactory interface {
	// NewClient creates an LLM client. providerID is the gollm provider id; opts match gollm.Option.
	NewClient(ctx context.Context, providerID string, opts ...gollm.Option) (gollm.Client, error)
}
