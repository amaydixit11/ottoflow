/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/workflow/executor"
)

var _ = Describe("Workflow References", func() {
	var (
		ctx            context.Context
		parentWorkflow *ottoflowv1alpha1.Workflow
		childWorkflow  *ottoflowv1alpha1.Workflow
		parentRun      *ottoflowv1alpha1.WorkflowRun
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create child workflow (sub-workflow)
		childWorkflow = &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "child-workflow",
				Namespace: "default",
			},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Inputs: []ottoflowv1alpha1.Input{
					{Name: "message"},
				},
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "echo",
						Expressions: []ottoflowv1alpha1.Expression{
							{
								Name:       "result",
								Expression: `inputs.message`,
							},
						},
					},
				},
				Outputs: []ottoflowv1alpha1.Output{
					{
						Name:       "result",
						Expression: "steps.echo.outputs.result",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, childWorkflow)).To(Succeed())

		// Create parent workflow with workflow reference
		parentWorkflow = &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "parent-workflow",
				Namespace: "default",
			},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "callChild",
						WorkflowRef: &ottoflowv1alpha1.StepWorkflowRef{
							Name:      "child-workflow",
							Namespace: "default",
							Inputs: map[string]string{
								"message": `"Hello from parent"`,
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, parentWorkflow)).To(Succeed())

		// Create parent workflow run
		parentRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "parent-run",
				Namespace: "default",
			},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{
					Name:      "parent-workflow",
					Namespace: "default",
				},
			},
		}
		Expect(k8sClient.Create(ctx, parentRun)).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup
		if parentRun != nil {
			// Cleanup any child runs first
			childRunList := &ottoflowv1alpha1.WorkflowRunList{}
			_ = k8sClient.List(ctx, childRunList, client.InNamespace("default"))
			for _, run := range childRunList.Items {
				if run.Labels != nil && run.Labels["ottoflow.nirmata.io/parent-workflow"] == "parent-workflow" {
					_ = k8sClient.Delete(ctx, &run)
				}
			}
			_ = k8sClient.Delete(ctx, parentRun)
		}
		if parentWorkflow != nil {
			_ = k8sClient.Delete(ctx, parentWorkflow)
		}
		if childWorkflow != nil {
			_ = k8sClient.Delete(ctx, childWorkflow)
		}
	})

	Context("When executing a workflow with a workflow reference", func() {
		It("should run sub-workflow inline and complete in one execution", func() {
			exec, err := executor.NewWorkflowExecutorWithMetrics(k8sClient, nil, &executor.NoOpCustomMetricsClient{}, &executor.NoOpPrometheusClient{}, parentRun, 0, 5, nil)
			Expect(err).NotTo(HaveOccurred())

			err = exec.ExecuteWorkflow(ctx, parentWorkflow, parentRun)
			Expect(err).NotTo(HaveOccurred())

			// Sub-workflow runs inline (no separate WorkflowRun/Job); parent completes in one go
			Expect(parentRun.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseSucceeded))
			Expect(parentRun.Status.StepStatuses["callChild"].Phase).To(Equal(ottoflowv1alpha1.StepPhaseSucceeded))

			// No child WorkflowRun is created in the cluster
			childRunList := &ottoflowv1alpha1.WorkflowRunList{}
			Expect(k8sClient.List(ctx, childRunList, client.InNamespace("default"))).To(Succeed())
			childCount := 0
			for i := range childRunList.Items {
				if childRunList.Items[i].Spec.WorkflowRef.Name == "child-workflow" {
					childCount++
				}
			}
			Expect(childCount).To(Equal(0))
		})
	})
})
