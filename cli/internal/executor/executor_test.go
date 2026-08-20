/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const defaultNamespace = "default"

func TestNewWorkflowExecutor(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	exec := NewWorkflowExecutor(fakeClient)
	if exec == nil || exec.client == nil {
		t.Fatal("NewWorkflowExecutor should return non-nil executor with client")
	}
}

func TestCreateWorkflowRun(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	exec := NewWorkflowExecutor(fakeClient)

	run, err := exec.CreateWorkflowRun(ctx, "my-wf", defaultNamespace, map[string]string{"x": "y"})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.Namespace != defaultNamespace || run.Spec.WorkflowRef.Name != "my-wf" {
		t.Errorf("expected default/my-wf, got %s/%s", run.Namespace, run.Spec.WorkflowRef.Name)
	}
	if run.Spec.InputValues["x"] != "y" {
		t.Errorf("expected input x=y, got %v", run.Spec.InputValues)
	}

	key := types.NamespacedName{Name: run.Name, Namespace: run.Namespace}
	got := &ottoflowv1alpha1.WorkflowRun{}
	if err := fakeClient.Get(ctx, key, got); err != nil {
		t.Fatalf("Get created run: %v", err)
	}
	if got.Name != run.Name {
		t.Errorf("expected created run name %s, got %s", run.Name, got.Name)
	}
}

func TestLoadWorkflowRunFromFile_Create(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	exec := NewWorkflowExecutor(fakeClient)

	dir := t.TempDir()
	yaml := `
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  name: from-file
  namespace: default
spec:
  workflowRef:
    name: some-workflow
  inputValues: {}
`
	path := filepath.Join(dir, "run.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	run, err := exec.LoadWorkflowRunFromFile(ctx, path)
	if err != nil {
		t.Fatalf("LoadWorkflowRunFromFile: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.Name != "from-file" || run.Namespace != defaultNamespace || run.Spec.WorkflowRef.Name != "some-workflow" {
		t.Errorf("unexpected run: name=%s ns=%s ref=%s", run.Name, run.Namespace, run.Spec.WorkflowRef.Name)
	}

	key := types.NamespacedName{Name: "from-file", Namespace: defaultNamespace}
	got := &ottoflowv1alpha1.WorkflowRun{}
	if err := fakeClient.Get(ctx, key, got); err != nil {
		t.Fatalf("Get created run: %v", err)
	}
}

func TestLoadWorkflowRunFromFile_Existing(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	existing := &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-run", Namespace: defaultNamespace},
		Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf"}},
		Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseSucceeded},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	exec := NewWorkflowExecutor(fakeClient)

	dir := t.TempDir()
	yaml := `
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  name: existing-run
  namespace: default
spec:
  workflowRef:
    name: other
  inputValues: {}
`
	path := filepath.Join(dir, "run.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	run, err := exec.LoadWorkflowRunFromFile(ctx, path)
	if err != nil {
		t.Fatalf("LoadWorkflowRunFromFile: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.Name != "existing-run" || run.Status.Phase != ottoflowv1alpha1.WorkflowRunPhaseSucceeded {
		t.Errorf("expected existing run with Succeeded phase, got name=%s phase=%s", run.Name, run.Status.Phase)
	}
}

func TestLoadWorkflowRunFromFile_NoName(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	exec := NewWorkflowExecutor(fakeClient)

	dir := t.TempDir()
	yaml := `
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  namespace: default
spec:
  workflowRef:
    name: wf
`
	path := filepath.Join(dir, "run.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	run, err := exec.LoadWorkflowRunFromFile(ctx, path)
	if err == nil {
		t.Fatal("expected error when name is missing")
	}
	if run != nil {
		t.Error("expected nil run on error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
	}
}

func TestLoadWorkflowRunFromFile_ReadError(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	exec := NewWorkflowExecutor(fakeClient)

	_, err := exec.LoadWorkflowRunFromFile(ctx, filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Errorf("expected 'read file' in error, got: %v", err)
	}
}

func TestWatchWorkflow_AlreadySucceeded(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	run := &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "done", Namespace: defaultNamespace},
		Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseSucceeded},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	exec := NewWorkflowExecutor(fakeClient)

	err := exec.WatchWorkflow(ctx, run, 5*time.Second, "table", false)
	if err != nil {
		t.Errorf("WatchWorkflow: %v", err)
	}
}

func TestWatchWorkflow_AlreadyFailed(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := ottoflowv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	run := &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Name: "failed", Namespace: defaultNamespace},
		Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseFailed},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(run).Build()
	exec := NewWorkflowExecutor(fakeClient)

	err := exec.WatchWorkflow(ctx, run, 5*time.Second, "table", false)
	if err != nil {
		t.Errorf("WatchWorkflow: %v", err)
	}
}
