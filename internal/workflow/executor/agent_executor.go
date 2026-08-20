/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/agent"
	"github.com/nirmata/ottoflow/internal/tracing"
)

// executeAgentStep executes an agent step
func (e *WorkflowExecutor) executeAgentStep(ctx context.Context, workflowRun *ottoflowv1alpha1.WorkflowRun, step ottoflowv1alpha1.Step) (map[string]interface{}, error) {
	agentRef := step.AgentRef

	// Determine namespace for Agent CRD
	agentNamespace := agentRef.Namespace
	if agentNamespace == "" {
		agentNamespace = workflowRun.Namespace
	}

	// Get the Agent CRD
	agentCRD := &ottoflowv1alpha1.Agent{}
	agentKey := client.ObjectKey{
		Name:      agentRef.Name,
		Namespace: agentNamespace,
	}
	if err := e.controlClient.Get(ctx, agentKey, agentCRD); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"agent %q not found in namespace %q"+
					" (if running locally, ensure the Agent YAML is in --workflow-dir)",
				agentRef.Name, agentNamespace,
			)
		}
		return nil, fmt.Errorf("failed to get Agent %s/%s: %w", agentNamespace, agentRef.Name, err)
	}

	if e.providerOverride != "" || e.modelOverride != "" {
		agentCRD = agentCRD.DeepCopy()
		if e.providerOverride != "" {
			agentCRD.Spec.ModelProvider = e.providerOverride
		}
		if e.modelOverride != "" {
			agentCRD.Spec.ModelName = e.modelOverride
		}
	}

	// Start invoke_agent span. Token usage from the LLM call is returned by ExecuteAgent
	// and set on this span below, in addition to the inner chat CLIENT span.
	provider := agentCRD.Spec.ModelProvider
	if provider == "" {
		// Empty is retained only for backward compatibility with Agent objects
		// stored before modelProvider became required (CRD required is
		// enforced at admission, not retroactively).
		provider = nirmataProvider
	}
	ctx, agentSpan := otel.Tracer("ottoflow").Start(ctx, "invoke_agent",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "invoke_agent"),
			attribute.String("gen_ai.system", provider),
			attribute.String("gen_ai.request.model", agentCRD.Spec.ModelName),
			attribute.String("gen_ai.agent.name", agentRef.Name),
		))
	defer func() { agentSpan.End() }()

	// Read current context for prompt evaluation
	contextData, err := e.contextManager.ReadContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read context: %w", err)
	}

	// Build variable map for CEL evaluation
	vars := e.celEvaluator.BuildVariableMap(contextData)

	// Start with agent's base prompt (static, no CEL evaluation)
	promptParts := []string{agentCRD.Spec.Prompt}

	// Evaluate and append additional prompts from step (can contain CEL expressions)
	if len(agentRef.AdditionalPrompts) > 0 {
		for _, additionalPromptTemplate := range agentRef.AdditionalPrompts {
			// Evaluate each additional prompt with workflow context
			evaluatedPrompt, err := e.celEvaluator.EvaluateExpression(ctx, additionalPromptTemplate, vars)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate additional prompt: %w", err)
			}
			// Convert to string — use JSON for maps/slices so the LLM gets
			// structured data instead of Go's fmt %v format.
			promptParts = append(promptParts, formatValueForPrompt(evaluatedPrompt))
		}
	}

	// Apply token budget to additional prompts if set (rough heuristic: ~4 runes per token)
	var promptStr string
	if len(promptParts) > 1 && agentRef.MaxAdditionalPromptTokens != nil && *agentRef.MaxAdditionalPromptTokens > 0 {
		additionalText := strings.Join(promptParts[1:], "\n\n")
		tokenBudget := *agentRef.MaxAdditionalPromptTokens * 4
		if int32(utf8.RuneCountInString(additionalText)) > tokenBudget {
			runes := []rune(additionalText)
			additionalText = string(runes[:tokenBudget]) + "..."
		}
		promptStr = promptParts[0] + "\n\n" + additionalText
	} else {
		promptStr = strings.Join(promptParts, "\n\n")
	}

	if err := e.waitOutboundRateLimit(ctx); err != nil {
		return nil, err
	}

	var response string
	var tokenUsage agent.AgentTokenUsage
	var extractedOutputs map[string]interface{}

	if e.localExecutionMode {
		// Local execution (CLI with --workflow-dir): call agent in-process
		response, tokenUsage, extractedOutputs, err = executeAndExtract(ctx, e.agentExecutor, agentCRD, promptStr, contextData, workflowRun.Namespace)
		if err != nil {
			return nil, fmt.Errorf("agent local execution failed: %w", err)
		}
	} else {
		// Cluster: execute via lightweight exec HTTP endpoint.
		serviceName := agentCRD.Spec.ServiceName
		if serviceName == "" {
			serviceName = "ottoflow-agent-executor"
		}
		serviceNamespace := agentCRD.Spec.ServiceNamespace
		if serviceNamespace == "" {
			// AGENT_EXECUTOR_NAMESPACE is injected by the controller into the runner Job
			// so the runner knows which namespace the agent-executor is deployed in.
			serviceNamespace = os.Getenv("AGENT_EXECUTOR_NAMESPACE")
		}
		if serviceNamespace == "" {
			serviceNamespace = "ottoflow"
		}
		response, tokenUsage, extractedOutputs, err = e.executeAgentViaExecHTTP(ctx, workflowRun, agentCRD, agentRef, promptStr, contextData, serviceName, serviceNamespace)
		if err != nil {
			return nil, fmt.Errorf("agent execution failed: %w", err)
		}
	}

	if tokenUsage.InputTokens > 0 || tokenUsage.OutputTokens > 0 {
		agentSpan.SetAttributes(
			tracing.GenAIUsageInputTokens.Int64(tokenUsage.InputTokens),
			tracing.GenAIUsageOutputTokens.Int64(tokenUsage.OutputTokens),
		)
	}

	// Write agent response to steps map in context for access via steps.step-name.response
	if err := e.writeStepResponseToContext(ctx, step.Name, response, extractedOutputs); err != nil {
		return nil, fmt.Errorf("failed to write step response to context: %w", err)
	}

	var outputs map[string]interface{}
	// If step has output definitions, evaluate them with agent response in context
	if len(step.Outputs) > 0 {
		// Read updated context (now includes steps map)
		contextData, err = e.contextManager.ReadContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read context: %w", err)
		}

		// Add agent response to context for output evaluation
		outputContext := make(map[string]interface{})
		for k, v := range contextData {
			outputContext[k] = v
		}
		outputContext["agentResponse"] = response
		outputContext["agentOutputs"] = extractedOutputs

		// Build variable map for output evaluation
		outputVars := e.celEvaluator.BuildVariableMap(outputContext)

		// Evaluate step outputs
		outputs, err = e.celEvaluator.EvaluateStepOutputs(ctx, step, outputVars)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate step outputs: %w", err)
		}

		// Write outputs to context
		if err := e.contextManager.WriteStepOutputs(ctx, step.Name, outputs); err != nil {
			return nil, fmt.Errorf("failed to write step outputs: %w", err)
		}
	} else {
		// No output definitions - use extracted outputs directly
		outputs = extractedOutputs
		if err := e.contextManager.WriteStepOutputs(ctx, step.Name, extractedOutputs); err != nil {
			return nil, fmt.Errorf("failed to write step outputs: %w", err)
		}
	}

	return outputs, nil
}

// writeStepResponseToContext writes the agent response to the steps map in context
func (e *WorkflowExecutor) writeStepResponseToContext(ctx context.Context, stepName string, response string, outputs map[string]interface{}) error {
	// Use GetContextFrom so forEach child steps write to scoped context
	inMemoryContext := e.contextManager.GetContextFrom(ctx)
	if inMemoryContext == nil {
		return fmt.Errorf("context not initialized")
	}

	// Get or create steps map
	stepsMap, ok := inMemoryContext["steps"].(map[string]interface{})
	if !ok {
		stepsMap = make(map[string]interface{})
		inMemoryContext["steps"] = stepsMap
	}

	// Write step response
	stepData := map[string]interface{}{
		"response": response,
	}
	if outputs != nil {
		stepData["outputs"] = outputs
	}
	stepsMap[stepName] = stepData

	return nil
}
