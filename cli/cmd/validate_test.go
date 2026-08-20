/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"strings"
	"testing"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/webhook"
	"github.com/nirmata/ottoflow/internal/workflow/executor"
)

func TestBuildDAG_CycleDetected(t *testing.T) {
	steps := []ottoflowv1alpha1.Step{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}
	_, err := executor.BuildDAG(steps)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error, got: %v", err)
	}
}

func TestBuildDAG_MissingDependsOn(t *testing.T) {
	steps := []ottoflowv1alpha1.Step{
		{Name: "a", DependsOn: []string{"nonexistent"}},
	}
	_, err := executor.BuildDAG(steps)
	if err == nil {
		t.Fatal("expected missing-step error, got nil")
	}
	if !strings.Contains(err.Error(), "non-existent") {
		t.Errorf("expected 'non-existent' in error, got: %v", err)
	}
}

func TestValidateStepDependencies_MissingDependsOn(t *testing.T) {
	spec := &ottoflowv1alpha1.WorkflowSpec{
		Steps: []ottoflowv1alpha1.Step{
			{Name: "collect", Outputs: []ottoflowv1alpha1.Output{{Name: "data"}}},
			{
				Name: "report",
				// references steps.collect but missing dependsOn
				Expressions: []ottoflowv1alpha1.Expression{{Expression: "steps.collect.data"}},
			},
		},
	}
	err := webhook.ValidateStepDependencies(spec)
	if err == nil {
		t.Fatal("expected dependency error, got nil")
	}
}

func TestCollectCELExpressions_ExcludesAgentPrompts(t *testing.T) {
	step := &ottoflowv1alpha1.Step{
		Name: "analyze",
		AgentRef: &ottoflowv1alpha1.StepAgentRef{
			Name:              "my-agent",
			AdditionalPrompts: []string{"Please analyze this cluster. It has many pods."},
		},
		Expressions: []ottoflowv1alpha1.Expression{
			{Expression: "inputs.count > 0"},
		},
	}
	exprs := collectCELExpressions(step)
	if len(exprs) != 1 || exprs[0] != "inputs.count > 0" {
		t.Errorf("expected only CEL expression, got: %v", exprs)
	}
}

func TestCELSyntaxCheck_InvalidExpression(t *testing.T) {
	celEnv, err := executor.NewValidationCELEnv()
	if err != nil {
		t.Fatalf("NewValidationCELEnv: %v", err)
	}
	// '!!!' is syntactically invalid CEL
	_, iss := celEnv.Compile("!!!")
	if iss == nil || iss.Err() == nil {
		t.Error("expected compile error for '!!!', got none")
	}
}

func TestCELSyntaxCheck_ValidExpression(t *testing.T) {
	celEnv, err := executor.NewValidationCELEnv()
	if err != nil {
		t.Fatalf("NewValidationCELEnv: %v", err)
	}
	_, iss := celEnv.Compile("inputs.name + ' world'")
	if iss != nil && iss.Err() != nil {
		t.Errorf("expected valid expression to compile, got: %v", iss.Err())
	}
}

// TestCostAnalyzerCELExpressions loads the cost-analyzer workflow YAML and compiles
// every step expression through the validation CEL environment, including type-check
// errors. This catches "expected type 'string' but found 'dyn'" mistakes — the class
// of runtime compilation failure that isCELTypeOnlyError normally suppresses.
func TestCostAnalyzerCELExpressions(t *testing.T) {
	wf, err := loadWorkflowFromFile("../../samples/workflows/production/cost-analyzer.yaml")
	if err != nil {
		t.Fatalf("load cost-analyzer.yaml: %v", err)
	}

	celEnv, err := executor.NewValidationCELEnv()
	if err != nil {
		t.Fatalf("NewValidationCELEnv: %v", err)
	}

	for i := range wf.Spec.Steps {
		step := &wf.Spec.Steps[i]
		for _, expr := range collectCELExpressions(step) {
			_, iss := celEnv.Compile(expr)
			if iss == nil || iss.Err() == nil {
				continue
			}
			// Report ALL compile errors — including type-check errors. Unlike the
			// main validate path (which skips type-only errors as potential false
			// positives), this test enforces that every expression in cost-analyzer
			// compiles cleanly. Runtime failures like "expected type 'string' but
			// found 'dyn'" are compile-time errors and must be fixed with string().
			t.Errorf("step %q: expr compile error: %s\n  expr: %s",
				step.Name, iss.Err(), expr)
		}
	}
}
