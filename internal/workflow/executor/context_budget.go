/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"

// applyContextBudget filters contextData according to agentRef.ContextBudgetMode.
// Applied before BuildVariableMap so no materialization cost is paid for pruned entries.
// Returns contextData unchanged (no allocation) for "full" or empty mode.
func applyContextBudget(
	contextData map[string]interface{},
	agentRef *ottoflowv1alpha1.StepAgentRef,
	completionOrder []string,
) map[string]interface{} {
	switch agentRef.ContextBudgetMode {
	case "lastN":
		n := 5
		if agentRef.ContextBudgetLastN != nil && *agentRef.ContextBudgetLastN > 0 {
			n = int(*agentRef.ContextBudgetLastN)
		}
		return applyLastNBudget(contextData, n, completionOrder)
	case "omit":
		return applyOmitBudget(contextData)
	default:
		return contextData
	}
}

// applyLastNBudget returns a copy of contextData where "steps" only contains entries
// for the last n steps in completionOrder. inputs, variables, and expressions are preserved.
func applyLastNBudget(contextData map[string]interface{}, n int, completionOrder []string) map[string]interface{} {
	start := len(completionOrder) - n
	if start < 0 {
		start = 0
	}
	keepSet := make(map[string]bool, n)
	for _, name := range completionOrder[start:] {
		keepSet[name] = true
	}

	stepsMap, ok := contextData["steps"].(map[string]interface{})
	if !ok || len(stepsMap) == 0 {
		return copyContextWithSteps(contextData, stepsMap)
	}
	filteredSteps := make(map[string]interface{}, len(keepSet))
	for name, data := range stepsMap {
		if keepSet[name] {
			filteredSteps[name] = data
		}
	}
	return copyContextWithSteps(contextData, filteredSteps)
}

// applyOmitBudget returns a copy of contextData where "steps" is replaced with an empty map.
// inputs, variables, and expressions are preserved.
func applyOmitBudget(contextData map[string]interface{}) map[string]interface{} {
	return copyContextWithSteps(contextData, map[string]interface{}{})
}

// copyContextWithSteps returns a shallow copy of contextData with "steps" replaced by filteredSteps.
func copyContextWithSteps(contextData map[string]interface{}, filteredSteps map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(contextData))
	for k, v := range contextData {
		out[k] = v
	}
	out["steps"] = filteredSteps
	return out
}
