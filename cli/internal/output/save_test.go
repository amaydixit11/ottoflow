/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

func newTestWorkflowRun(phase ottoflowv1alpha1.WorkflowRunPhase, outputs map[string]interface{}) *ottoflowv1alpha1.WorkflowRun {
	now := metav1.NewTime(time.Date(2026, 4, 8, 18, 30, 0, 0, time.UTC))
	later := metav1.NewTime(now.Add(45 * time.Second))

	wr := &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run-abc",
			Namespace: "default",
		},
		Spec: ottoflowv1alpha1.WorkflowRunSpec{
			WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "my-workflow", Namespace: "default"},
			InputValues: map[string]string{"key": "value"},
		},
		Status: ottoflowv1alpha1.WorkflowRunStatus{
			Phase:   phase,
			Message: "completed",
			StepStatuses: map[string]ottoflowv1alpha1.StepStatus{
				"step1": {
					Phase:          ottoflowv1alpha1.StepPhaseSucceeded,
					StartTime:      &now,
					CompletionTime: &later,
					Message:        "done",
				},
			},
			StartTime:      &now,
			CompletionTime: &later,
		},
	}

	if outputs != nil {
		wr.Status.Outputs = make(map[string]apiextensionsv1.JSON)
		for k, v := range outputs {
			raw, _ := json.Marshal(v)
			wr.Status.Outputs[k] = apiextensionsv1.JSON{Raw: raw}
		}
	}

	return wr
}

func TestSaveRunOutput_JSONContainsExpectedFields(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, map[string]interface{}{
		"greeting": "hello world",
	})

	jsonPath, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	for _, field := range []string{"name", "namespace", "phase", "steps", "outputs", "startTime", "completionTime"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing field %q in JSON output", field)
		}
	}
	if _, ok := m["inputValues"]; ok {
		t.Error("inputValues should not be present when includeInputs=false")
	}
	if m["phase"] != "Succeeded" {
		t.Errorf("expected phase Succeeded, got %v", m["phase"])
	}
}

func TestSaveRunOutput_JSONIncludesInputsWhenRequested(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)

	jsonPath, _, err := SaveRunOutput(wr, dir, true)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	if _, ok := m["inputValues"]; !ok {
		t.Error("inputValues should be present when includeInputs=true")
	}
}

func TestSaveRunOutput_MarkdownDetection(t *testing.T) {
	dir := t.TempDir()
	markdownContent := "# Cost Report\n\n## Summary\n\nTotal savings: $500"
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, map[string]interface{}{
		"report": markdownContent,
	})

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "my-workflow") {
		t.Error("markdown should contain workflow name in header")
	}
	if !strings.Contains(content, markdownContent) {
		t.Error("markdown should contain the report output content")
	}
}

func TestSaveRunOutput_MarkdownDetectionByContent(t *testing.T) {
	dir := t.TempDir()
	markdownContent := "# Analysis Results\n\n## Findings\n\nSeveral issues detected."
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, map[string]interface{}{
		"analysis": markdownContent, // not a known markdown output name — detected by content
	})

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, markdownContent) {
		t.Error("markdown should contain content-detected markdown output")
	}
	if strings.Contains(content, "## Steps") {
		t.Error("should use detected markdown, not structured report")
	}
}

func TestSaveRunOutput_NonStringReportFallsBack(t *testing.T) {
	dir := t.TempDir()
	// "report" key with a JSON object, not a string — should fall through to generated report
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, map[string]interface{}{
		"report": map[string]interface{}{"total": 500},
	})

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	// Should have the structured report with Steps table
	if !strings.Contains(content, "## Steps") {
		t.Error("expected structured report with Steps section when report output is not a string")
	}
}

func TestSaveRunOutput_FallbackStructuredReport(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, map[string]interface{}{
		"count": 42,
	})

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## Steps") {
		t.Error("expected structured report with Steps section")
	}
	if !strings.Contains(content, "## Outputs") {
		t.Error("expected structured report with Outputs section")
	}
	if !strings.Contains(content, "step1") {
		t.Error("expected step1 in report")
	}
}

func TestSaveRunOutput_FilenameCollision(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)

	jsonPath1, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	jsonPath2, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}

	if jsonPath1 == jsonPath2 {
		t.Errorf("expected different filenames, both got %s", jsonPath1)
	}
	if !strings.Contains(filepath.Base(jsonPath2), "-1") {
		t.Errorf("expected -1 suffix in second filename, got %s", filepath.Base(jsonPath2))
	}
}

func TestSaveRunOutput_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "output")
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)

	jsonPath, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("JSON file not created: %v", err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("MD file not created: %v", err)
	}
}

func TestSaveRunOutput_EmptyOutputs(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "my-workflow") {
		t.Error("markdown should contain workflow name even with no outputs")
	}
}

func TestSaveRunOutput_FailedWorkflow(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseFailed, nil)
	wr.Status.StepStatuses["step1"] = ottoflowv1alpha1.StepStatus{
		Phase:   ottoflowv1alpha1.StepPhaseFailed,
		Message: "something went wrong",
		Error:   "connection refused",
	}

	jsonPath, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	if m["phase"] != "Failed" {
		t.Errorf("expected phase Failed, got %v", m["phase"])
	}

	mdData, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}
	if !strings.Contains(string(mdData), "Failed") {
		t.Error("expected Failed in markdown header")
	}
}

func TestSaveRunOutput_FallbackWorkflowName(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)
	wr.Spec.WorkflowRef.Name = "" // clear WorkflowRef.Name to test fallback
	// Should fall back to WorkflowRun.Name ("test-run-abc")

	jsonPath, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	base := filepath.Base(jsonPath)
	if !strings.HasPrefix(base, "test-run-abc-") {
		t.Errorf("expected filename to start with 'test-run-abc-', got %s", base)
	}
}

func TestSaveRunOutput_DefaultWorkflowName(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)
	wr.Spec.WorkflowRef.Name = ""
	wr.Name = "" // both empty — should fall back to "workflow"

	jsonPath, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	base := filepath.Base(jsonPath)
	if !strings.HasPrefix(base, "workflow-") {
		t.Errorf("expected filename to start with 'workflow-', got %s", base)
	}
}

func TestSaveRunOutput_PipeCharsInStepMessage(t *testing.T) {
	dir := t.TempDir()
	now := metav1.NewTime(time.Date(2026, 4, 8, 18, 30, 0, 0, time.UTC))
	later := metav1.NewTime(now.Add(10 * time.Second))
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)
	wr.Status.StepStatuses["step-with-pipe"] = ottoflowv1alpha1.StepStatus{
		Phase:          ottoflowv1alpha1.StepPhaseFailed,
		StartTime:      &now,
		CompletionTime: &later,
		Error:          "cmd | grep foo | wc -l returned error",
	}

	_, mdPath, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read MD: %v", err)
	}

	content := string(data)
	// Pipes should be escaped in markdown table
	if strings.Contains(content, "| cmd | grep") {
		t.Error("pipe characters in step message should be escaped in markdown table")
	}
	if !strings.Contains(content, "cmd \\| grep") {
		t.Error("expected escaped pipe characters in step message")
	}
}

func TestSaveRunOutput_UsesWorkflowRefName(t *testing.T) {
	dir := t.TempDir()
	wr := newTestWorkflowRun(ottoflowv1alpha1.WorkflowRunPhaseSucceeded, nil)
	// WorkflowRun.Name is "test-run-abc" but WorkflowRef.Name is "my-workflow"
	// Filename should use WorkflowRef.Name

	jsonPath, _, err := SaveRunOutput(wr, dir, false)
	if err != nil {
		t.Fatalf("SaveRunOutput: %v", err)
	}

	base := filepath.Base(jsonPath)
	if !strings.HasPrefix(base, "my-workflow-") {
		t.Errorf("expected filename to start with 'my-workflow-', got %s", base)
	}
}
