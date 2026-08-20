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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("CELEvaluator EvaluateOutputValue and tryEvaluateAsCEL", func() {
	var (
		ctx         context.Context
		evaluator   *CELEvaluator
		fakeClient  client.Client
		workflowRun *ottoflowv1alpha1.WorkflowRun
		scheme      *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(ottoflowv1alpha1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		workflowRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run", Namespace: "default"},
		}
		var err error
		evaluator, err = NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("EvaluateOutputValue", func() {
		It("returns literal string when no CEL indicators", func() {
			vars := map[string]interface{}{}
			result, err := evaluator.EvaluateOutputValue(ctx, "plain literal", vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("plain literal"))
		})

		It("evaluates string that looks like CEL expression", func() {
			vars := map[string]interface{}{
				"variables": map[string]interface{}{"x": int64(10)},
			}
			result, err := evaluator.EvaluateOutputValue(ctx, "variables.x + 2", vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(12)))
		})

		It("returns literal when CEL evaluation fails", func() {
			vars := map[string]interface{}{}
			result, err := evaluator.EvaluateOutputValue(ctx, "undefined.ref", vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("undefined.ref"))
		})

		It("recursively evaluates map values", func() {
			vars := map[string]interface{}{
				"variables": map[string]interface{}{"a": "foo"},
			}
			value := map[string]interface{}{
				"key1": "literal",
				"key2": "variables.a",
			}
			result, err := evaluator.EvaluateOutputValue(ctx, value, vars)
			Expect(err).NotTo(HaveOccurred())
			rm := result.(map[string]interface{})
			Expect(rm["key1"]).To(Equal("literal"))
			Expect(rm["key2"]).To(Equal("foo"))
		})

		It("recursively evaluates slice elements", func() {
			vars := map[string]interface{}{
				"variables": map[string]interface{}{"n": int64(3)},
			}
			// Use expressions that contain CEL indicators (e.g. '.') so tryEvaluateAsCEL runs
			value := []interface{}{"[1,2,3].size()", "variables.n"}
			result, err := evaluator.EvaluateOutputValue(ctx, value, vars)
			Expect(err).NotTo(HaveOccurred())
			rl := result.([]interface{})
			Expect(rl[0]).To(Equal(int64(3)))
			Expect(rl[1]).To(Equal(int64(3)))
		})

		It("returns numbers and booleans as-is", func() {
			vars := map[string]interface{}{}
			result, err := evaluator.EvaluateOutputValue(ctx, 42, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(42))
			result, err = evaluator.EvaluateOutputValue(ctx, true, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})
})
