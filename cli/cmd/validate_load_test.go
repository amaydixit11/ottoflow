/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

// A document that declares `kind: Workflow` but whose body does not parse as one must be
// reported as a broken Workflow. The permissive TypeMeta parse succeeds for any body, so
// probing Workflow first and falling back to TypeMeta recorded it as an unsupported kind --
// which made runValidate print SKIP and exit 0 on a structurally-broken workflow.
func TestLoadWorkflowFromFileFailsOnMistypedWorkflowBody(t *testing.T) {
	path := writeManifest(t, "broken.yaml", `apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: broken
spec:
  steps: "this-should-be-a-list"
`)

	wf, err := loadWorkflowFromFile(path)
	if err == nil {
		t.Fatalf("expected an error for a Workflow with a mistyped body, got workflow %+v", wf)
	}
	var noWorkflow *noWorkflowInFileError
	if errors.As(err, &noWorkflow) {
		t.Fatalf("error must not be noWorkflowInFileError (that is the skippable case): %v", err)
	}
	if !strings.Contains(err.Error(), "parse Workflow") {
		t.Errorf("error should name the failed Workflow parse, got: %v", err)
	}
}

// A file holding only sibling kinds is still skippable -- that is the behaviour the
// stricter Workflow parse must not regress.
func TestLoadWorkflowFromFileSkipsSiblingKindsOnly(t *testing.T) {
	path := writeManifest(t, "agent.yaml", `apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Agent
metadata:
  name: sidekick
spec:
  provider: local
  modelName: qwen3:8b
`)

	if _, err := loadWorkflowFromFile(path); err == nil {
		t.Fatal("expected noWorkflowInFileError for a file with no Workflow")
	} else {
		var noWorkflow *noWorkflowInFileError
		if !errors.As(err, &noWorkflow) {
			t.Fatalf("expected noWorkflowInFileError, got %T: %v", err, err)
		}
		if len(noWorkflow.otherKinds) != 1 || noWorkflow.otherKinds[0] != "Agent" {
			t.Errorf("expected otherKinds=[Agent], got %v", noWorkflow.otherKinds)
		}
		if noWorkflow.parseErr != nil {
			t.Errorf("a well-formed sibling manifest must not record a parse error: %v", noWorkflow.parseErr)
		}
	}
}

// A Workflow declared after a sibling kind in the same file is still found.
func TestLoadWorkflowFromFileFindsWorkflowAfterSibling(t *testing.T) {
	path := writeManifest(t, "pair.yaml", `apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Agent
metadata:
  name: sidekick
spec:
  provider: local
  modelName: qwen3:8b
---
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: real
spec:
  steps:
    - name: s1
      expressions:
        - name: x
          expression: "1 + 1"
`)

	wf, err := loadWorkflowFromFile(path)
	if err != nil {
		t.Fatalf("expected to find the Workflow, got error: %v", err)
	}
	if wf.Name != "real" {
		t.Errorf("expected workflow %q, got %q", "real", wf.Name)
	}
}
