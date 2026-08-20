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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("Workflow Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		workflow := &ottoflowv1alpha1.Workflow{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workflow")
			err := k8sClient.Get(ctx, typeNamespacedName, workflow)
			if err != nil && errors.IsNotFound(err) {
				resource := &ottoflowv1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
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
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &ottoflowv1alpha1.Workflow{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workflow")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &WorkflowReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("SetupWithManager", func() {
		It("should register WorkflowReconciler with manager", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			r := &WorkflowReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(r.SetupWithManager(mgr)).To(Succeed())
		})
	})
})
