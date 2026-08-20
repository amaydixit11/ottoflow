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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("OpenReport Step Execution", func() {
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
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "test-workflow"},
			},
		}
	})

	minimalWorkflow := &ottoflowv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
	}

	newExec := func(k8sClient client.Client) *WorkflowExecutor {
		exec, err := NewWorkflowExecutorWithClientsAndAgentExecutor(
			k8sClient, k8sClient, nil, nil, nil,
			workflowRun, nil, nil, false, 0, 5, nil, nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(exec.contextManager.InitializeContext(ctx, minimalWorkflow, nil)).To(Succeed())
		return exec
	}

	// newExecutorWithCRD returns an executor whose controlClient has the
	// reports.openreports.io CRD pre-created (simulates OpenReports installed).
	newExecutorWithCRD := func() *WorkflowExecutor {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		crdObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"metadata":   map[string]interface{}{"name": openReportsCRDName},
			},
		}
		Expect(k8sClient.Create(ctx, crdObj)).To(Succeed())
		return newExec(k8sClient)
	}

	// newExecutorNoCRD returns an executor whose controlClient has no CRD
	// (simulates OpenReports not installed).
	newExecutorNoCRD := func() *WorkflowExecutor {
		return newExec(fake.NewClientBuilder().WithScheme(scheme).Build())
	}

	// CEL literal expressions — compact single-line to avoid parser issues.
	// resultsExpr produces 4 results: 1 fail, 2 pass, 1 warn.
	const resultsExpr = `[{"policy":"require-labels","result":"fail"},{"policy":"resource-limits","result":"pass"},{"policy":"no-privileged","result":"pass"},{"policy":"read-only-fs","result":"warn"}]`
	const summaryExpr = `{"pass":99,"fail":1,"warn":0,"error":0,"skip":0}`

	mkStep := func(ref ottoflowv1alpha1.StepOpenReport) ottoflowv1alpha1.Step {
		return ottoflowv1alpha1.Step{Name: "emitReport", OpenReport: &ref}
	}

	Describe("data mode (OpenReports CRD absent)", func() {
		It("step succeeds and reportResult.mode is 'data'", func() {
			exec := newExecutorNoCRD()

			outputs, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			rr, ok := outputs["reportResult"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "reportResult should be a map")
			Expect(rr["mode"]).To(Equal("data"))
			Expect(rr["name"]).To(Equal(""))
			Expect(rr["namespace"]).To(Equal(""))
			Expect(rr["data"]).To(HaveLen(4))
		})

		It("no Report CRD is created in the cluster", func() {
			exec := newExecutorNoCRD()

			_, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			reportList := &unstructured.UnstructuredList{}
			reportList.SetGroupVersionKind(openReportsGVK)
			_ = exec.client.List(ctx, reportList)
			Expect(reportList.Items).To(BeEmpty())
		})
	})

	Describe("CRD mode (OpenReports CRD present)", func() {
		It("step succeeds and reportResult.mode is 'crd'", func() {
			exec := newExecutorWithCRD()

			outputs, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			rr, ok := outputs["reportResult"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(rr["mode"]).To(Equal("crd"))
			Expect(rr["name"]).To(Equal("compliance-report"))
			Expect(rr["namespace"]).To(Equal("default"))
		})

		It("creates the Report CRD object with the correct source", func() {
			exec := newExecutorWithCRD()

			_, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
				Source:            "test-source",
			}))
			Expect(err).NotTo(HaveOccurred())

			created := &unstructured.Unstructured{}
			created.SetGroupVersionKind(openReportsGVK)
			Expect(exec.client.Get(ctx, types.NamespacedName{Name: "compliance-report", Namespace: "default"}, created)).To(Succeed())
			Expect(created.Object["source"]).To(Equal("test-source"))
		})

		It("defaults source to 'ottoflow' when not specified", func() {
			exec := newExecutorWithCRD()

			_, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			created := &unstructured.Unstructured{}
			created.SetGroupVersionKind(openReportsGVK)
			Expect(exec.client.Get(ctx, types.NamespacedName{Name: "compliance-report", Namespace: "default"}, created)).To(Succeed())
			Expect(created.Object["source"]).To(Equal("ottoflow"))
		})

		It("is idempotent: second run updates the existing Report CRD", func() {
			exec := newExecutorWithCRD()
			ref := ottoflowv1alpha1.StepOpenReport{
				ReportName:        "compliance-report",
				ResultsExpression: resultsExpr,
			}

			_, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ref))
			Expect(err).NotTo(HaveOccurred())

			// Reset Once so the second run re-checks availability
			exec.openReportsCRDOnce = sync.Once{}
			_, err = exec.executeOpenReportStep(ctx, workflowRun, mkStep(ref))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("auto-computed summary", func() {
		It("correctly counts pass/fail/warn/error/skip from results", func() {
			exec := newExecutorNoCRD()

			outputs, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "report",
				ResultsExpression: resultsExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			rr := outputs["reportResult"].(map[string]interface{})
			summary := rr["summary"].(map[string]interface{})
			Expect(summary["pass"]).To(Equal(int64(2)))
			Expect(summary["fail"]).To(Equal(int64(1)))
			Expect(summary["warn"]).To(Equal(int64(1)))
			Expect(summary["error"]).To(Equal(int64(0)))
			Expect(summary["skip"]).To(Equal(int64(0)))
		})
	})

	Describe("summaryExpression override", func() {
		It("uses the CEL-provided summary instead of auto-computing", func() {
			exec := newExecutorNoCRD()

			outputs, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "report",
				ResultsExpression: resultsExpr,
				SummaryExpression: summaryExpr,
			}))
			Expect(err).NotTo(HaveOccurred())

			rr := outputs["reportResult"].(map[string]interface{})
			summary := rr["summary"].(map[string]interface{})
			Expect(summary["pass"]).To(Equal(int64(99)))
		})
	})

	Describe("sync.Once caches the CRD check", func() {
		It("openReportsCRDAvail is set after first call and stays consistent", func() {
			exec := newExecutorNoCRD()

			for i := 0; i < 3; i++ {
				_, err := exec.executeOpenReportStep(ctx, workflowRun, mkStep(ottoflowv1alpha1.StepOpenReport{
					ReportName:        "report",
					ResultsExpression: resultsExpr,
				}))
				Expect(err).NotTo(HaveOccurred())
			}

			// CRD was never installed, so all runs land in data mode.
			// openReportsCRDAvail should be false (set once, never changed).
			Expect(exec.openReportsCRDAvail).To(BeFalse())
			Expect(exec.openReportsCRDErr).To(BeNil())
		})
	})

	Describe("user-defined outputs alongside reportResult", func() {
		It("reportResult is always written even when step.Outputs is non-empty", func() {
			exec := newExecutorNoCRD()
			step := mkStep(ottoflowv1alpha1.StepOpenReport{
				ReportName:        "report",
				ResultsExpression: resultsExpr,
			})
			step.Outputs = []ottoflowv1alpha1.Output{
				{Name: "failCount", Expression: "reportResult.summary.fail"},
			}

			outputs, err := exec.executeOpenReportStep(ctx, workflowRun, step)
			Expect(err).NotTo(HaveOccurred())

			// User output evaluated correctly
			Expect(outputs["failCount"]).To(Equal(int64(1)))

			// reportResult must always be present regardless of user outputs
			rr, ok := outputs["reportResult"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "reportResult must always be present in step outputs")
			Expect(rr["mode"]).To(Equal("data"))
			Expect(rr["data"]).To(HaveLen(4))
		})
	})
})
