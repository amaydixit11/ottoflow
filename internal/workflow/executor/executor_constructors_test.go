/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/agent"
)

var _ = Describe("Executor constructors and progress callback", func() {
	var (
		ctx         context.Context
		scheme      *runtime.Scheme
		workflowRun *ottoflowv1alpha1.WorkflowRun
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		workflowRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "default"},
			Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf"}},
		}
	})

	It("NewWorkflowExecutor creates executor with default agent", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec, err := NewWorkflowExecutor(k8sClient, workflowRun)
		Expect(err).NotTo(HaveOccurred())
		Expect(exec).NotTo(BeNil())
		Expect(exec.GetContextManager()).NotTo(BeNil())
	})

	It("defaults the agent executor to RoutingAgentExecutor, not the Nirmata delegate directly", func() {
		// Regression guard: a nil agentExecutor must resolve to the router (which
		// dispatches nirmata/empty to the Nirmata delegate and everything else to
		// DefaultAgentExecutor), not straight to the Nirmata delegate — that would
		// silently drop DefaultAgentExecutor from the wiring.
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec, err := NewWorkflowExecutorWithClientsAndAgentExecutor(
			k8sClient, k8sClient, nil, nil, nil,
			workflowRun, nil, nil, false, 0, 5, nil, nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(exec.agentExecutor).To(BeAssignableToTypeOf(&agent.RoutingAgentExecutor{}))
	})

	It("NewWorkflowExecutorWithMetrics creates executor with nil metrics", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec, err := NewWorkflowExecutorWithMetrics(k8sClient, nil, nil, nil, workflowRun, 0, 0, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(exec).NotTo(BeNil())
	})

	It("SetProgressCallback sets callback invoked during execution", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		exec, err := NewWorkflowExecutorWithAgentExecutor(
			k8sClient, nil, nil, nil, workflowRun, agent.NewMockAgentExecutor(), false, 0, 5, nil)
		Expect(err).NotTo(HaveOccurred())

		var mu sync.Mutex
		var callCount int
		exec.SetProgressCallback(func(wr *ottoflowv1alpha1.WorkflowRun, w *ottoflowv1alpha1.Workflow) {
			mu.Lock()
			callCount++
			mu.Unlock()
		})

		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "s1", Outputs: []ottoflowv1alpha1.Output{{Name: "x", Expression: `"ok"`}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())

		mu.Lock()
		n := callCount
		mu.Unlock()
		Expect(n).To(BeNumerically(">=", 1), "progress callback should be invoked at least once")
	})

	It("runs normally when Phase is Running but checkpointing is disabled (controller-restart scenario, no checkpoint)", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "default"},
			Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf"}},
			Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseRunning},
		}
		exec, err := NewWorkflowExecutorWithMetrics(k8sClient, nil, nil, nil, workflowRun, 0, 5, nil)
		Expect(err).NotTo(HaveOccurred())
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "default"},
			Spec:       ottoflowv1alpha1.WorkflowSpec{Steps: []ottoflowv1alpha1.Step{{Name: "s1", Outputs: []ottoflowv1alpha1.Output{{Name: "x", Expression: `"ok"`}}}}},
		}
		err = exec.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())
		Expect(workflowRun.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseSucceeded))
	})
})
