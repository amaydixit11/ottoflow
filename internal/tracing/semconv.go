/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package tracing

import "go.opentelemetry.io/otel/attribute"

// OTel GenAI Semantic Conventions attribute keys.
// Based on https://opentelemetry.io/docs/specs/semconv/gen-ai/ (development status as of 2026).
// No stable Go contrib package exists yet (opentelemetry-go-contrib v1.43.0), so keys are
// defined here directly. Replace with the upstream package once it reaches stable status.
var (
	// GenAI core
	GenAISystem        = attribute.Key("gen_ai.system")
	GenAIOperationName = attribute.Key("gen_ai.operation.name")
	GenAIRequestModel  = attribute.Key("gen_ai.request.model")

	// GenAI tool use
	GenAIToolName = attribute.Key("gen_ai.tool.name")

	// GenAI token usage — set on both the chat CLIENT span (internal/agent/executor.go) and
	// the invoke_agent INTERNAL span (internal/workflow/executor/agent_executor.go).
	GenAIUsageInputTokens  = attribute.Key("gen_ai.usage.input_tokens")
	GenAIUsageOutputTokens = attribute.Key("gen_ai.usage.output_tokens")

	// MCP transport
	MCPServerName    = attribute.Key("mcp.server.name")
	MCPTransportType = attribute.Key("mcp.transport.type")

	// OttoFlow workflow attributes (custom, not in GenAI SemConv)
	WorkflowRunName      = attribute.Key("workflow.run.name")
	WorkflowRunNamespace = attribute.Key("workflow.run.namespace")
	WorkflowName         = attribute.Key("workflow.name")
	WorkflowStepName     = attribute.Key("workflow.step.name")
	WorkflowStepType     = attribute.Key("workflow.step.type")

	// A2A protocol
	A2AAgentURL = attribute.Key("a2a.agent.url")
)
