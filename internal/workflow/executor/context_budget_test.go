/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"testing"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

func makeTestContext(stepNames ...string) map[string]interface{} {
	steps := make(map[string]interface{})
	for _, name := range stepNames {
		steps[name] = map[string]interface{}{
			"response": "response-" + name,
			"outputs":  map[string]interface{}{"key": "val-" + name},
		}
	}
	return map[string]interface{}{
		"inputs":      map[string]interface{}{"env": "test"},
		"variables":   map[string]interface{}{"foo": "bar"},
		"expressions": map[string]interface{}{},
		"steps":       steps,
	}
}

func stepsIn(ctx map[string]interface{}) map[string]interface{} {
	if m, ok := ctx["steps"].(map[string]interface{}); ok {
		return m
	}
	return nil
}

// applyContextBudget tests

func TestApplyContextBudget_FullMode(t *testing.T) {
	ctx := makeTestContext("a", "b", "c")
	agentRef := &ottoflowv1alpha1.StepAgentRef{ContextBudgetMode: "full"}
	result := applyContextBudget(ctx, agentRef, []string{"a", "b", "c"})
	// full mode: all steps must be present, nothing pruned
	steps := stepsIn(result)
	if len(steps) != 3 {
		t.Errorf("full mode must preserve all steps, got %d", len(steps))
	}
}

func TestApplyContextBudget_EmptyMode(t *testing.T) {
	ctx := makeTestContext("a", "b")
	agentRef := &ottoflowv1alpha1.StepAgentRef{} // empty ContextBudgetMode
	result := applyContextBudget(ctx, agentRef, []string{"a", "b"})
	// empty mode behaves as full: all steps preserved
	steps := stepsIn(result)
	if len(steps) != 2 {
		t.Errorf("empty mode must preserve all steps, got %d", len(steps))
	}
}

func TestApplyContextBudget_LastNMode(t *testing.T) {
	ctx := makeTestContext("a", "b", "c", "d", "e")
	n := int32(2)
	agentRef := &ottoflowv1alpha1.StepAgentRef{ContextBudgetMode: "lastN", ContextBudgetLastN: &n}
	result := applyContextBudget(ctx, agentRef, []string{"a", "b", "c", "d", "e"})
	steps := stepsIn(result)
	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}
	if _, ok := steps["d"]; !ok {
		t.Error("expected step d to be present")
	}
	if _, ok := steps["e"]; !ok {
		t.Error("expected step e to be present")
	}
	if _, ok := steps["a"]; ok {
		t.Error("expected step a to be pruned")
	}
}

func TestApplyContextBudget_OmitMode(t *testing.T) {
	ctx := makeTestContext("a", "b", "c")
	agentRef := &ottoflowv1alpha1.StepAgentRef{ContextBudgetMode: "omit"}
	result := applyContextBudget(ctx, agentRef, []string{"a", "b", "c"})
	steps := stepsIn(result)
	if len(steps) != 0 {
		t.Errorf("omit mode must produce empty steps map, got %d entries", len(steps))
	}
}

// applyLastNBudget tests

func TestApplyLastNBudget_KeepsLastN(t *testing.T) {
	ctx := makeTestContext("s1", "s2", "s3", "s4", "s5")
	order := []string{"s1", "s2", "s3", "s4", "s5"}
	result := applyLastNBudget(ctx, 2, order)
	steps := stepsIn(result)
	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}
	if _, ok := steps["s4"]; !ok {
		t.Error("s4 should be present")
	}
	if _, ok := steps["s5"]; !ok {
		t.Error("s5 should be present")
	}
}

func TestApplyLastNBudget_NGreaterThanTotal(t *testing.T) {
	ctx := makeTestContext("s1", "s2", "s3")
	order := []string{"s1", "s2", "s3"}
	result := applyLastNBudget(ctx, 10, order)
	steps := stepsIn(result)
	if len(steps) != 3 {
		t.Errorf("N > total should return all steps, got %d", len(steps))
	}
}

func TestApplyLastNBudget_NEqualsTotal(t *testing.T) {
	ctx := makeTestContext("s1", "s2", "s3")
	order := []string{"s1", "s2", "s3"}
	result := applyLastNBudget(ctx, 3, order)
	steps := stepsIn(result)
	if len(steps) != 3 {
		t.Errorf("N = total should return all steps, got %d", len(steps))
	}
}

func TestApplyLastNBudget_EmptyCompletionOrder(t *testing.T) {
	ctx := makeTestContext("s1", "s2")
	result := applyLastNBudget(ctx, 5, []string{})
	steps := stepsIn(result)
	if len(steps) != 0 {
		t.Errorf("empty completion order should produce empty steps, got %d", len(steps))
	}
}

func TestApplyLastNBudget_PreservesNonStepKeys(t *testing.T) {
	ctx := makeTestContext("s1", "s2", "s3")
	order := []string{"s1", "s2", "s3"}
	result := applyLastNBudget(ctx, 1, order)
	if result["inputs"] == nil {
		t.Error("inputs must be preserved")
	}
	if result["variables"] == nil {
		t.Error("variables must be preserved")
	}
	if result["expressions"] == nil {
		t.Error("expressions must be preserved")
	}
}

func TestApplyLastNBudget_EmptyStepsMap(t *testing.T) {
	ctx := map[string]interface{}{
		"inputs":      map[string]interface{}{"k": "v"},
		"steps":       map[string]interface{}{},
		"variables":   map[string]interface{}{},
		"expressions": map[string]interface{}{},
	}
	result := applyLastNBudget(ctx, 3, []string{"s1"})
	steps := stepsIn(result)
	if len(steps) != 0 {
		t.Errorf("empty steps map should stay empty, got %d", len(steps))
	}
}

// applyOmitBudget tests

func TestApplyOmitBudget_EmptiesSteps(t *testing.T) {
	ctx := makeTestContext("s1", "s2", "s3")
	result := applyOmitBudget(ctx)
	steps := stepsIn(result)
	if len(steps) != 0 {
		t.Errorf("omit must produce empty steps map, got %d", len(steps))
	}
}

func TestApplyOmitBudget_PreservesNonStepKeys(t *testing.T) {
	ctx := makeTestContext("s1")
	result := applyOmitBudget(ctx)
	if result["inputs"] == nil {
		t.Error("inputs must be preserved")
	}
	if result["variables"] == nil {
		t.Error("variables must be preserved")
	}
	if result["expressions"] == nil {
		t.Error("expressions must be preserved")
	}
}

func TestApplyOmitBudget_AlreadyEmptySteps(t *testing.T) {
	ctx := map[string]interface{}{
		"inputs":      map[string]interface{}{"k": "v"},
		"steps":       map[string]interface{}{},
		"variables":   map[string]interface{}{},
		"expressions": map[string]interface{}{},
	}
	result := applyOmitBudget(ctx)
	steps := stepsIn(result)
	if len(steps) != 0 {
		t.Error("omit on already-empty steps should stay empty")
	}
}
