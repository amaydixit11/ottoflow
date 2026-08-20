/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// A bare "---" split also matches a markdown table separator inside a value, truncating
// the document mid-input. The truncated fragment still parses as a WorkflowRun, so the run
// proceeds with silently missing inputs rather than failing.
func TestLoadWorkflowRunFromFileKeepsInputsAfterMarkdownTable(t *testing.T) {
	manifest := `apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  name: split-regression
  namespace: ottoflow
spec:
  workflowRef:
    name: hello-world
  inputValues:
    report: |
      | col | col |
      |---|---|
    afterTable: "must-survive"
`
	path := filepath.Join(t.TempDir(), "run.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	run, err := loadWorkflowRunFromFile(path)
	if err != nil {
		t.Fatalf("loadWorkflowRunFromFile: %v", err)
	}
	if got, ok := run.Spec.InputValues["afterTable"]; !ok || got != "must-survive" {
		t.Errorf("input after the markdown table was lost: InputValues = %v", run.Spec.InputValues)
	}
}
