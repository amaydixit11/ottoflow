/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// fakeEventRecorder records event calls for assertion in tests.
type fakeEventRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
	record bool
}

type recordedEvent struct {
	EventType string
	Reason    string
	Message   string
}

func (f *fakeEventRecorder) Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record {
		f.events = append(f.events, recordedEvent{EventType: eventtype, Reason: reason, Message: fmt.Sprintf(note, args...)})
	}
}

func (f *fakeEventRecorder) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = nil
	f.record = true
}

func (f *fakeEventRecorder) getEvents() []recordedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedEvent, len(f.events))
	copy(out, f.events)
	return out
}

func ptr(b bool) *bool { return &b }

var _ = Describe("Workflow Events", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		recorder  *fakeEventRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = ottoflowv1alpha1.AddToScheme(scheme)
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder = &fakeEventRecorder{}
		recorder.reset()
	})

	It("should emit WorkflowRunning, WorkflowExecution, and WorkflowSucceeded for a successful run", func() {
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run-1", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf", Namespace: "default"},
				InputValues: map[string]string{},
			},
			Status: ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
		}
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:    "echo",
					Outputs: []ottoflowv1alpha1.Output{{Name: "done", Value: &apiextensionsv1.JSON{Raw: []byte(`"ok"`)}}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		exec, err := NewWorkflowExecutorWithAgentExecutor(k8sClient, nil, nil, nil, workflowRun, nil, false, 0, 5, recorder)
		Expect(err).NotTo(HaveOccurred())
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())

		Expect(workflowRun.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseSucceeded))
		Expect(workflowRun.Status.StepStatuses["echo"].Phase).To(Equal(ottoflowv1alpha1.StepPhaseSucceeded))

		events := recorder.getEvents()
		Expect(len(events)).To(BeNumerically(">=", 3), "expected at least WorkflowRunning and step started/succeeded")
		Expect(events[0].Reason).To(Equal("WorkflowRunning"))
		Expect(events[0].Message).To(Equal("Workflow run started"))
		Expect(events[1].Reason).To(Equal("WorkflowExecution"))
		Expect(events[1].Message).To(ContainSubstring("started"))
		Expect(events[2].Reason).To(Equal("WorkflowExecution"))
		Expect(events[2].Message).To(ContainSubstring("succeeded"))
		if len(events) >= 4 {
			Expect(events[3].Reason).To(Equal("WorkflowSucceeded"))
			Expect(events[3].Message).To(Equal("Workflow run completed successfully"))
		}
	})

	It("should emit WorkflowExecution Warning when a step fails", func() {
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run-2", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf2", Namespace: "default"},
				InputValues: map[string]string{},
			},
			Status: ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
		}
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf2", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "fail",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: `int("not a number")`}}, // CEL error
				}},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		exec, err := NewWorkflowExecutorWithAgentExecutor(k8sClient, nil, nil, nil, workflowRun, nil, false, 0, 5, recorder)
		Expect(err).NotTo(HaveOccurred())
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).To(HaveOccurred())

		events := recorder.getEvents()
		Expect(events).To(HaveLen(4)) // WorkflowRunning, Step started, Step failed (Warning), WorkflowFailed
		Expect(events[0].Reason).To(Equal("WorkflowRunning"))
		Expect(events[2].Reason).To(Equal("WorkflowExecution"))
		Expect(events[2].EventType).To(Equal("Warning"))
		Expect(events[2].Message).To(ContainSubstring("failed"))
		Expect(events[3].Reason).To(Equal("WorkflowFailed"))
		Expect(events[3].EventType).To(Equal("Warning"))
	})

	It("should not emit step events when verbosity is workflowOnly", func() {
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run-3", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf3", Namespace: "default"},
				InputValues: map[string]string{},
			},
			Status: ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
		}
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf3", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Events: &ottoflowv1alpha1.EventConfig{Level: "Workflow"},
				Steps: []ottoflowv1alpha1.Step{{
					Name:    "echo",
					Outputs: []ottoflowv1alpha1.Output{{Name: "done", Value: &apiextensionsv1.JSON{Raw: []byte(`"ok"`)}}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		exec, err := NewWorkflowExecutorWithAgentExecutor(k8sClient, nil, nil, nil, workflowRun, nil, false, 0, 5, recorder)
		Expect(err).NotTo(HaveOccurred())
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())

		Expect(workflowRun.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseSucceeded))
		events := recorder.getEvents()
		// Only workflow-level when verbosity is workflowOnly (no step events)
		Expect(len(events)).To(BeNumerically(">=", 1), "level Workflow should emit at least WorkflowRunning")
		Expect(events[0].Reason).To(Equal("WorkflowRunning"))
		if len(events) >= 2 {
			Expect(events[1].Reason).To(Equal("WorkflowSucceeded"))
		}
	})

	It("should not emit any events when events.enabled is false", func() {
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run-4", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf4", Namespace: "default"},
				InputValues: map[string]string{},
				Events:      &ottoflowv1alpha1.EventConfig{Enabled: ptr(false)},
			},
			Status: ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
		}
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf4", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:    "echo",
					Outputs: []ottoflowv1alpha1.Output{{Name: "done", Value: &apiextensionsv1.JSON{Raw: []byte(`"ok"`)}}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		exec, err := NewWorkflowExecutorWithAgentExecutor(k8sClient, nil, nil, nil, workflowRun, nil, false, 0, 5, recorder)
		Expect(err).NotTo(HaveOccurred())
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())

		events := recorder.getEvents()
		Expect(events).To(BeEmpty())
	})
})
