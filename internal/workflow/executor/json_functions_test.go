/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("JSON Marshal Functions", func() {
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
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-json-marshal",
				Namespace: "default",
			},
		}
		var err error
		evaluator, err = NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("json.marshal (from SDK)", func() {
		It("should marshal a map to compact JSON", func() {
			expr := `json.marshal({"key": "value"})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue(), "json.marshal should return a string")
			// Parse to verify valid JSON, compare as structures (key order is non-deterministic)
			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(resultStr), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKeyWithValue("key", "value"))
		})

		It("should marshal a list to JSON array", func() {
			expr := `json.marshal([1, 2, 3])`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue())
			var parsed []interface{}
			Expect(json.Unmarshal([]byte(resultStr), &parsed)).To(Succeed())
			Expect(parsed).To(HaveLen(3))
		})

		It("should marshal nested map literals", func() {
			expr := `json.marshal({"outer": {"inner": "value"}})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`{"outer":{"inner":"value"}}`))
		})

		It("should marshal nested structures from json.unmarshal (native Go types)", func() {
			// When nested data comes from json.unmarshal (not CEL map literals),
			// the values are native Go types and json.marshal works correctly.
			expr := `json.marshal(json.unmarshal("{\"outer\": {\"inner\": [1, 2]}}"))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue())
			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(resultStr), &parsed)).To(Succeed())
			inner, ok := parsed["outer"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(inner["inner"]).To(HaveLen(2))
		})

		It("should marshal scalar string", func() {
			expr := `json.marshal("hello")`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`"hello"`))
		})

		It("should marshal scalar int", func() {
			expr := `json.marshal(42)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("42"))
		})

		It("should marshal scalar float", func() {
			expr := `json.marshal(3.14)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("3.14"))
		})

		It("should marshal scalar bool", func() {
			expr := `json.marshal(true)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("true"))
		})

		It("should marshal empty map", func() {
			expr := `json.marshal({})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("{}"))
		})

		It("should marshal empty list", func() {
			expr := `json.marshal([])`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("[]"))
		})

		It("should round-trip with json.unmarshal for a simple map", func() {
			// CEL type checker rejects mixed-type map literals (e.g., int and list),
			// so we round-trip a uniform-type map instead.
			expr := `json.unmarshal(json.marshal({"a": "one", "b": "two"}))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(resultMap["a"]).To(Equal("one"))
			Expect(resultMap["b"]).To(Equal("two"))
		})

		It("should properly escape unicode and special characters", func() {
			expr := `json.marshal({"msg": "hello \"world\" \n\ttab"})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue())
			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(resultStr), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("msg"))
		})

		It("should marshal null value as JSON null", func() {
			expr := `json.marshal(null)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("null"))
		})

		It("should marshal large int64", func() {
			expr := `json.marshal(9223372036854775807)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("9223372036854775807"))
		})
	})

})
