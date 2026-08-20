/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package logging

import (
	"testing"
)

func TestKeysForRun(t *testing.T) {
	got := KeysForRun("my-workflow", "default", "run-1")
	if len(got) != 6 {
		t.Fatalf("KeysForRun: expected 6 elements, got %d", len(got))
	}
	// key, value pairs: workflow, my-workflow, namespace, default, workflowRun, run-1
	expectPairs := map[string]interface{}{
		"workflow":    "my-workflow",
		"namespace":   "default",
		"workflowRun": "run-1",
	}
	for i := 0; i < len(got); i += 2 {
		k, ok := got[i].(string)
		if !ok {
			t.Errorf("key at %d is not string: %T", i, got[i])
			continue
		}
		if v, want := got[i+1], expectPairs[k]; v != want {
			t.Errorf("KeysForRun: key %q = %v, want %v", k, v, want)
		}
	}
}

func TestKeysForStep(t *testing.T) {
	got := KeysForStep("step-a")
	if len(got) != 2 {
		t.Fatalf("KeysForStep: expected 2 elements, got %d", len(got))
	}
	if got[0] != KeyStep || got[1] != "step-a" {
		t.Errorf("KeysForStep: got %v, want [KeyStep, \"step-a\"]", got)
	}
}

func TestConstants(t *testing.T) {
	if KeyWorkflow != "workflow" {
		t.Errorf("KeyWorkflow = %q, want \"workflow\"", KeyWorkflow)
	}
	if KeyWorkflowRun != "workflowRun" {
		t.Errorf("KeyWorkflowRun = %q, want \"workflowRun\"", KeyWorkflowRun)
	}
	if KeyNamespace != "namespace" {
		t.Errorf("KeyNamespace = %q, want \"namespace\"", KeyNamespace)
	}
	if KeyStep != "step" {
		t.Errorf("KeyStep = %q, want \"step\"", KeyStep)
	}
	if KeyPhase != "phase" {
		t.Errorf("KeyPhase = %q, want \"phase\"", KeyPhase)
	}
}
