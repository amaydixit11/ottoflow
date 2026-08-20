/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// `item` is declared in the CEL env but only bound inside a forEach. Binding it to nil
// everywhere else degraded the error a stray reference produces: "no such key: name"
// points at the field rather than at the missing loop, and a bare `item` evaluated to
// null with no error at all.
var _ = Describe("CELEvaluator item binding", func() {
	var (
		ctx       context.Context
		evaluator *CELEvaluator
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(ottoflowv1alpha1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "default"},
		}
		var err error
		evaluator, err = NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("leaves item unbound outside a forEach so the error names item itself", func() {
		vars := evaluator.BuildVariableMap(map[string]interface{}{})
		Expect(vars).NotTo(HaveKey("item"), "item must stay unbound outside a forEach")

		_, err := evaluator.EvaluateExpression(ctx, "item.name", vars)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("item"),
			"the error should name the missing item binding, not the field being read off it")
	})

	It("binds item inside a forEach", func() {
		vars := evaluator.BuildVariableMap(map[string]interface{}{
			"item": map[string]interface{}{"name": "pod-a"},
		})
		Expect(vars).To(HaveKey("item"))

		result, err := evaluator.EvaluateExpression(ctx, "item.name", vars)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("pod-a"))
	})
})
