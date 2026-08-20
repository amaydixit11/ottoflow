/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const defaultNamespace = "default"

var _ = Describe("WorkflowRun Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		workflowrun := &ottoflowv1alpha1.WorkflowRun{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind WorkflowRun")
			err := k8sClient.Get(ctx, typeNamespacedName, workflowrun)
			if err != nil && errors.IsNotFound(err) {
				// Create a Workflow first
				workflow := &ottoflowv1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: ottoflowv1alpha1.WorkflowSpec{
						Steps: []ottoflowv1alpha1.Step{
							{
								Name: "testStep",
								Expressions: []ottoflowv1alpha1.Expression{
									{
										Name:       "result",
										Expression: `"test"`,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

				resource := &ottoflowv1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: ottoflowv1alpha1.WorkflowRunSpec{
						WorkflowRef: ottoflowv1alpha1.WorkflowRef{
							Name:      "test-workflow",
							Namespace: "default",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup WorkflowRun
			resource := &ottoflowv1alpha1.WorkflowRun{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance WorkflowRun")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Cleanup Workflow
			workflow := &ottoflowv1alpha1.Workflow{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-workflow", Namespace: "default"}, workflow)
			if err == nil {
				Expect(k8sClient.Delete(ctx, workflow)).To(Succeed())
			}
		})
		It("should create a runner Job for the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &WorkflowRunReconciler{
				Client:              k8sClient,
				Scheme:              k8sClient.Scheme(),
				MetricsClient:       nil,
				CustomMetricsClient: nil,
				PrometheusClient:    nil,
				RunnerConfig:        RunnerConfig{RunnerClusterRole: "ottoflow-runner-role"},
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying WorkflowRun status and step propagation")
			updated := &ottoflowv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhasePending))
			Expect(updated.Status.Execution).NotTo(BeNil())
			Expect(updated.Status.Execution.JobName).To(Equal("test-resource-runner"))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			job := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-resource-runner", Namespace: "default"}, job)).To(Succeed())
		})
	})

	Context("Run policy (retention and maxAllowed)", func() {
		ctx := context.Background()
		ns := defaultNamespace

		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		It("should delete completed runs when retentionMinutes is set", func() {
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "retention-wf", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{Name: "s1", Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: `"ok"`}}},
					},
					Run: &ottoflowv1alpha1.RunPolicy{RetentionMinutes: 1},
				},
			}
			oldCompletion := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			wr := &ottoflowv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: "run-old", Namespace: ns},
				Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "retention-wf", Namespace: ns}},
				Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseSucceeded, CompletionTime: &oldCompletion},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wr).WithObjects(workflow, wr).Build()
			reconciler := &WorkflowRunReconciler{Client: fakeClient, Scheme: scheme}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: wr.Name, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			err = fakeClient.Get(ctx, types.NamespacedName{Name: wr.Name, Namespace: ns}, &ottoflowv1alpha1.WorkflowRun{})
			Expect(errors.IsNotFound(err)).To(BeTrue(), "old run should be deleted by retention")
		})

		It("should delete oldest completed runs when maxAllowed is exceeded", func() {
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "max-wf", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{Name: "s1", Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: `"ok"`}}},
					},
					Run: &ottoflowv1alpha1.RunPolicy{MaxAllowed: 2},
				},
			}
			baseTime := time.Now()
			objs := []client.Object{workflow}
			for i := 0; i < 3; i++ {
				t := metav1.NewTime(baseTime.Add(time.Duration(i) * time.Minute))
				name := "run-max-" + string(rune('a'+i))
				wr := &ottoflowv1alpha1.WorkflowRun{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
					Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "max-wf", Namespace: ns}},
					Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhaseSucceeded, CompletionTime: &t},
				}
				objs = append(objs, wr)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(objs[1], objs[2], objs[3]).WithObjects(objs...).Build()
			reconciler := &WorkflowRunReconciler{Client: fakeClient, Scheme: scheme}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "run-max-a", Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			var list ottoflowv1alpha1.WorkflowRunList
			Expect(fakeClient.List(ctx, &list, client.InNamespace(ns))).To(Succeed())
			completed := 0
			for _, r := range list.Items {
				if r.Spec.WorkflowRef.Name == "max-wf" && (r.Status.Phase == ottoflowv1alpha1.WorkflowRunPhaseSucceeded || r.Status.Phase == ottoflowv1alpha1.WorkflowRunPhaseFailed) {
					completed++
				}
			}
			Expect(completed).To(Equal(2), "maxAllowed=2 should leave exactly 2 completed runs")
		})
	})

	Context("Runner Job execution", func() {
		ctx := context.Background()
		ns := defaultNamespace

		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		It("should create a runner Job by default", func() {
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "restart-wf", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{Name: "restartStep", Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"done"`}}},
					},
				},
			}
			wr := &ottoflowv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: "restart-run", Namespace: ns},
				Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "restart-wf", Namespace: ns}},
				Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wr).WithObjects(workflow, wr).Build()
			reconciler := &WorkflowRunReconciler{Client: fakeClient, Scheme: scheme, RunnerConfig: RunnerConfig{RunnerClusterRole: "ottoflow-runner-role"}}

			By("Reconciling a run")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: wr.Name, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: wr.Name, Namespace: ns}, wr)).To(Succeed())
			Expect(wr.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhasePending))
			Expect(wr.Status.Execution).NotTo(BeNil())
			Expect(wr.Status.Execution.JobName).To(Equal("restart-run-runner"))

			job := &batchv1.Job{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "restart-run-runner", Namespace: ns}, job)).To(Succeed())
		})

		It("should mark WorkflowRun failed when the runner Job fails", func() {
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "restart-fail-wf", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{Name: "restartStep", Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"done"`}}},
					},
				},
			}
			wr := &ottoflowv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: "restart-fail-run", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowRunSpec{
					WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "restart-fail-wf", Namespace: ns},
				},
				Status: ottoflowv1alpha1.WorkflowRunStatus{
					Phase: ottoflowv1alpha1.WorkflowRunPhasePending,
					Execution: &ottoflowv1alpha1.WorkflowRunExecutionStatus{
						JobName: "restart-fail-run-runner",
					},
				},
			}
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "restart-fail-run-runner", Namespace: ns},
				Status:     batchv1.JobStatus{Failed: 1},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wr).WithObjects(workflow, wr, job).Build()
			reconciler := &WorkflowRunReconciler{Client: fakeClient, Scheme: scheme}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: wr.Name, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: wr.Name, Namespace: ns}, wr)).To(Succeed())
			Expect(wr.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseFailed))
			Expect(wr.Status.Execution).NotTo(BeNil())
			Expect(wr.Status.Execution.JobName).To(Equal("restart-fail-run-runner"))
			Expect(wr.Status.Execution.Phase).To(Equal(string(ottoflowv1alpha1.WorkflowRunPhaseFailed)))
		})
	})

	Context("SetupWithManager", func() {
		It("should register WorkflowRunReconciler with manager", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			r := &WorkflowRunReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.SetupWithManager(mgr)).To(Succeed())
		})
	})

	Context("Runner RBAC (least-privilege)", func() {
		ctx := context.Background()
		ns := defaultNamespace

		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		It("binds a default runner Job to a per-workflow ServiceAccount and the narrowed runner ClusterRole", func() {
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "rbac-wf", Namespace: ns},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{Name: "s1", Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: `"ok"`}}},
					},
				},
			}
			wr := &ottoflowv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{Name: "rbac-run", Namespace: ns},
				Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "rbac-wf", Namespace: ns}},
				Status:     ottoflowv1alpha1.WorkflowRunStatus{Phase: ottoflowv1alpha1.WorkflowRunPhasePending},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(wr).WithObjects(workflow, wr).Build()
			// RunnerServiceAccount and RunnerClusterRole reflect the chart's new defaults:
			// no controller-SA fallback, and runner Jobs bind to the narrowed runner role.
			reconciler := &WorkflowRunReconciler{
				Client: fakeClient,
				Scheme: scheme,
				RunnerConfig: RunnerConfig{
					RunnerServiceAccount: "",
					RunnerClusterRole:    "ottoflow-runner-role",
				},
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: wr.Name, Namespace: ns}})
			Expect(err).NotTo(HaveOccurred())

			job := &batchv1.Job{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rbac-run-runner", Namespace: ns}, job)).To(Succeed())
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("rbac-wf-runner"))
			Expect(job.Spec.Template.Spec.ServiceAccountName).NotTo(Equal("controller-manager"))

			crbName := workflowRunnerRoleBindingName(ns, job.Spec.Template.Spec.ServiceAccountName)
			crb := &rbacv1.ClusterRoleBinding{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: crbName}, crb)).To(Succeed())
			Expect(crb.RoleRef.Name).To(Equal("ottoflow-runner-role"))
			Expect(crb.RoleRef.Name).NotTo(Equal("ottoflow-role"))
		})

		It("migrates an existing runner ClusterRoleBinding to the narrowed role via delete-and-recreate", func() {
			saName := "rbac-migrate-sa"

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: ns, Labels: map[string]string{"app.kubernetes.io/part-of": "ottoflow"}},
			}
			Expect(k8sClient.Create(ctx, sa)).To(Succeed())

			crbName := workflowRunnerRoleBindingName(ns, saName)
			crb := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: crbName, Labels: map[string]string{"app.kubernetes.io/part-of": "ottoflow"}},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "ottoflow-role"},
				Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: ns}},
			}
			Expect(k8sClient.Create(ctx, crb)).To(Succeed())

			defer func() {
				_ = k8sClient.Delete(ctx, crb)
				_ = k8sClient.Delete(ctx, sa)
			}()

			reconciler := &WorkflowRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				RunnerConfig: RunnerConfig{
					RunnerClusterRole: "ottoflow-runner-role",
				},
			}
			wr := &ottoflowv1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Namespace: ns}}

			// This runs against the envtest admin k8sClient, which unconditionally holds every
			// RBAC verb, so it validates the delete/recreate migration LOGIC only — not that the
			// controller's own ServiceAccount actually has the "delete" and "bind" grants added
			// to -role:core. Those are covered by the helm-template rule assertions and the e2e
			// suite, which run under the real, narrower controller-manager identity.
			Expect(reconciler.ensureRunnerAccess(ctx, wr, nil, saName, false)).To(Succeed())

			reloaded := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, reloaded)).To(Succeed())
			Expect(reloaded.RoleRef.Name).To(Equal("ottoflow-runner-role"))
		})
	})
})
