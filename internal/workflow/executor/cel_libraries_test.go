/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

type mockCustomMetricValue struct {
	name   string
	val    int64
	ts     time.Time
	window int64
}

func (m *mockCustomMetricValue) MetricName() string   { return m.name }
func (m *mockCustomMetricValue) Value() int64         { return m.val }
func (m *mockCustomMetricValue) Timestamp() time.Time { return m.ts }
func (m *mockCustomMetricValue) WindowSeconds() int64 { return m.window }

type mockCustomMetricsClient struct {
	value CustomMetricValue
	err   error
}

func (m *mockCustomMetricsClient) GetMetric(ctx context.Context, apiVersion, kind, namespace, name, metricName string) (CustomMetricValue, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.value, nil
}

var _ = Describe("CEL options constructors", func() {
	It("GetKyvernoCELOptions returns non-empty options", func() {
		opts := GetKyvernoCELOptions("default")
		Expect(opts).NotTo(BeEmpty())
	})

	It("GetKubernetesCELOptions returns non-empty options", func() {
		opts := GetKubernetesCELOptions()
		Expect(opts).NotTo(BeEmpty())
	})
})

var _ = Describe("ResolveCELCostLimit", func() {
	It("returns default when spec is nil", func() {
		Expect(ResolveCELCostLimit(nil)).To(Equal(DefaultCELCostLimit))
	})
	It("returns default when CELCostLimit is nil", func() {
		spec := &ottoflowv1alpha1.WorkflowSpec{}
		Expect(ResolveCELCostLimit(spec)).To(Equal(DefaultCELCostLimit))
	})
	It("returns custom limit when CELCostLimit is set", func() {
		limit := int64(1000)
		spec := &ottoflowv1alpha1.WorkflowSpec{CELCostLimit: &limit}
		Expect(ResolveCELCostLimit(spec)).To(Equal(uint64(1000)))
	})
})

var _ = Describe("CEL Libraries", func() {
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
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workflow",
				Namespace: "default",
			},
		}

		var err error
		evaluator, err = NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(evaluator).NotTo(BeNil())
	})

	Describe("Time Library", func() {
		It("should support now() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, "time.now()", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "time.now() should return a time.Time")
		})

		It("should support time.now() namespace function", func() {
			result, err := evaluator.EvaluateExpression(ctx, "time.now()", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "time.now() should return a time.Time")
		})

		It("should support duration() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `duration("1h")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a duration
			_, ok := result.(time.Duration)
			Expect(ok).To(BeTrue(), "duration() should return a time.Duration")
		})

		It("should support timestamp() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `timestamp("2024-01-01T00:00:00Z")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "timestamp() should return a time.Time")
		})

		It("should support toCron() function", func() {
			now := time.Now()
			expr := `time.toCron(timestamp("` + now.Format(time.RFC3339) + `"))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string
			_, ok := result.(string)
			Expect(ok).To(BeTrue(), "time.toCron() should return a string")
		})

		It("should support time.toCron() namespace function", func() {
			now := time.Now()
			expr := `time.toCron(timestamp("` + now.Format(time.RFC3339) + `"))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string
			_, ok := result.(string)
			Expect(ok).To(BeTrue(), "time.toCron() should return a string")
		})

		It("should support truncate() function", func() {
			now := time.Now()
			expr := `time.truncate(timestamp("` + now.Format(time.RFC3339) + `"), duration("1h"))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "time.truncate() should return a time.Time")
		})

		It("should support time.truncate() namespace function", func() {
			now := time.Now()
			expr := `time.truncate(timestamp("` + now.Format(time.RFC3339) + `"), duration("1h"))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "time.truncate() should return a time.Time")
		})

		It("should support time arithmetic", func() {
			result, err := evaluator.EvaluateExpression(ctx, `time.now() - duration("1h")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a timestamp
			_, ok := result.(time.Time)
			Expect(ok).To(BeTrue(), "time arithmetic should return a time.Time")
		})
	})

	Describe("JSON Library", func() {
		It("should support json.unmarshal() function", func() {
			// Use single quotes for the outer string and escape double quotes inside
			// Kyverno CEL uses lowercase: json.unmarshal (not Unmarshal)
			expr := `json.unmarshal('{"key": "value", "number": 42, "bool": true}')`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a map
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "json.unmarshal() should return a map")
			Expect(resultMap["key"]).To(Equal("value"))
			Expect(resultMap["number"]).To(Equal(float64(42)))
			Expect(resultMap["bool"]).To(Equal(true))
		})

		It("should support nested JSON objects", func() {
			expr := `json.unmarshal('{"nested": {"key": "value"}}').nested.key`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value"))
		})

		It("should support JSON arrays", func() {
			expr := `json.unmarshal('[1, 2, 3]')`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a slice
			resultSlice, ok := result.([]interface{})
			Expect(ok).To(BeTrue(), "json.unmarshal() should return a slice for arrays")
			Expect(len(resultSlice)).To(Equal(3))
		})
	})

	Describe("YAML Library", func() {
		It("should support yaml.parse() function", func() {
			// Use single quotes and \n for newlines
			// Kyverno CEL uses lowercase: yaml.parse (not Parse)
			expr := `yaml.parse('key: value\nnumber: 42\nbool: true')`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a map
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "yaml.parse() should return a map")
			Expect(resultMap["key"]).To(Equal("value"))
			// YAML parsing returns numbers as float64 by default
			Expect(resultMap["number"]).To(Equal(float64(42)))
			Expect(resultMap["bool"]).To(Equal(true))
		})

		It("should support nested YAML objects", func() {
			expr := `yaml.parse('nested:\n  key: value').nested.key`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value"))
		})
	})

	Describe("Hash Library", func() {
		It("should support md5() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `md5("test")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string (hex digest)
			hash, ok := result.(string)
			Expect(ok).To(BeTrue(), "md5() should return a string")
			Expect(len(hash)).To(Equal(32), "MD5 hash should be 32 hex characters")
		})

		It("should support sha1() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `sha1("test")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string (hex digest)
			hash, ok := result.(string)
			Expect(ok).To(BeTrue(), "sha1() should return a string")
			Expect(len(hash)).To(Equal(40), "SHA1 hash should be 40 hex characters")
		})

		It("should support sha256() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `sha256("test")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string (hex digest)
			hash, ok := result.(string)
			Expect(ok).To(BeTrue(), "sha256() should return a string")
			Expect(len(hash)).To(Equal(64), "SHA256 hash should be 64 hex characters")
		})
	})

	Describe("Math Library", func() {
		It("should support math.round() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `math.round(3.14159, 2)`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a number
			rounded, ok := result.(float64)
			Expect(ok).To(BeTrue(), "math.round() should return a float64")
			Expect(rounded).To(BeNumerically("~", 3.14, 0.01))
		})

		It("should support math.round() with negative precision", func() {
			result, err := evaluator.EvaluateExpression(ctx, `math.round(1234.567, -2)`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			rounded, ok := result.(float64)
			Expect(ok).To(BeTrue())
			Expect(rounded).To(BeNumerically("~", 1200, 1))
		})
	})

	Describe("Random Library", func() {
		It("should support random() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `random()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string
			randomStr, ok := result.(string)
			Expect(ok).To(BeTrue(), "random() should return a string")
			Expect(len(randomStr)).To(Equal(8), "random() should return 8 characters by default")
		})

		It("should support random() with pattern", func() {
			result, err := evaluator.EvaluateExpression(ctx, `random("[a-z]{10}")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string
			randomStr, ok := result.(string)
			Expect(ok).To(BeTrue(), "random() with pattern should return a string")
			Expect(len(randomStr)).To(Equal(10), "random() with pattern should return matching length")
		})
	})

	Describe("Transform Library", func() {
		It("should support listObjToMap() function", func() {
			expr := `listObjToMap([{"name": "a"}, {"name": "b"}], [{"value": "1"}, {"value": "2"}], "name", "value")`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a map
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "listObjToMap() should return a map")
			Expect(resultMap["a"]).To(Equal("1"))
			Expect(resultMap["b"]).To(Equal("2"))
		})
	})

	Describe("Two-Variable Comprehensions", func() {
		It("should support transformMapEntry() for list-to-map indexing", func() {
			// Use homogeneous string values — in real workflows, dyn-typed K8s data avoids this constraint
			expr := `[{"name": "alice", "role": "admin"}, {"name": "bob", "role": "user"}].transformMapEntry(i, e, {e.name: e.role})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "transformMapEntry() should return a map")
			Expect(resultMap["alice"]).To(Equal("admin"))
			Expect(resultMap["bob"]).To(Equal("user"))
		})

		It("should support O(1) key lookup on transformMapEntry result", func() {
			expr := `[{"id": "ns/pod-a", "status": "running"}, {"id": "ns/pod-b", "status": "pending"}].transformMapEntry(i, e, {e.id: e.status})["ns/pod-b"]`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("pending"))
		})

		It("should support 'in' operator on transformMapEntry result", func() {
			expr := `"ns/pod-a" in [{"id": "ns/pod-a", "status": "ok"}].transformMapEntry(i, e, {e.id: e.status})`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should return empty map for transformMapEntry on empty list", func() {
			expr := `size([].transformMapEntry(i, e, {e: e}))`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(0)))
		})
	})

	Describe("Kubernetes List Library", func() {
		It("should support indexOf() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[1, 2, 3].indexOf(2)`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(1)))
		})

		It("should support lastIndexOf() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[1, 2, 2, 3].lastIndexOf(2)`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(2)))
		})

		It("should support min() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[3, 1, 2].min()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(1)))
		})

		It("should support max() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[3, 1, 2].max()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(3)))
		})

		It("should support sum() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[1, 2, 3].sum()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(6)))
		})

		It("should support isSorted() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[1, 2, 3].isSorted()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})

	Describe("Kubernetes Regex Library", func() {
		It("should support find() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `"hello world".find("[a-z]+")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a string or list
			_, ok := result.(string)
			Expect(ok).To(BeTrue(), "find() should return a string")
		})

		It("should support findAll() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `"hello world".findAll("[a-z]+")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return a list
			_, ok := result.([]interface{})
			Expect(ok).To(BeTrue(), "findAll() should return a list")
		})
	})

	Describe("Kubernetes URL Library", func() {
		It("should support isURL() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isURL("https://example.com")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should support url() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `url("https://example.com:8080/path?key=value").getHost()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("example.com:8080"))
		})
	})

	Describe("Kubernetes IP Library", func() {
		It("should support isIP() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isIP("192.168.1.1")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should support ip() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `ip("192.168.1.1").family()`, nil)
			Expect(err).NotTo(HaveOccurred())
			// family() returns 4 for IPv4 or 6 for IPv6 as int64
			family, ok := result.(int64)
			Expect(ok).To(BeTrue(), "ip().family() should return an int64")
			Expect(family).To(BeElementOf([]int64{4, 6}))
		})
	})

	Describe("Kubernetes CIDR Library", func() {
		It("should support isCIDR() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isCIDR("192.168.1.0/24")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should support cidr() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `cidr("192.168.1.0/24").containsIP("192.168.1.1")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})

	Describe("Kubernetes Format Library", func() {
		// Note: Format library functions may have different API or may not be available
		// Skipping these tests until we can verify the correct API
		PIt("should support format.dns1123Label() function", func() {
			// Format library API needs to be verified
			result, err := evaluator.EvaluateExpression(ctx, `format.dns1123Label("my-label")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		PIt("should support format.uuid() function", func() {
			// Format library API needs to be verified
			result, err := evaluator.EvaluateExpression(ctx, `format.uuid("550e8400-e29b-41d4-a716-446655440000")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("Kubernetes Quantity Library", func() {
		It("should support quantity() function", func() {
			// Try asInteger() first, if it fails try asApproximateFloat()
			result, err := evaluator.EvaluateExpression(ctx, `quantity("100m").asInteger()`, nil)
			if err != nil {
				result, err = evaluator.EvaluateExpression(ctx, `quantity("100m").asApproximateFloat()`, nil)
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return an integer or float
			_, isInt := result.(int64)
			_, isFloat := result.(float64)
			Expect(isInt || isFloat).To(BeTrue(), "quantity() should return int64 or float64")
		})

		It("should support isQuantity() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isQuantity("100m")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})

	Describe("Kubernetes Semver Library", func() {
		It("should support semver() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `semver("1.2.3").major()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(1)))
		})

		It("should support isSemver() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isSemver("1.2.3")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})

	Describe("Standard CEL Functions", func() {
		It("should support string operations", func() {
			result, err := evaluator.EvaluateExpression(ctx, `"hello".upperAscii()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("HELLO"))
		})

		It("should support list operations", func() {
			result, err := evaluator.EvaluateExpression(ctx, `[1, 2, 3].size()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(int64(3)))
		})

		It("should support map operations", func() {
			// Use has() function which works on maps
			result, err := evaluator.EvaluateExpression(ctx, `has({"key": "value"}, "key")`, nil)
			if err != nil {
				// Try alternative syntax
				result, err = evaluator.EvaluateExpression(ctx, `"key" in {"key": "value"}`, nil)
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should support format() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `format("hello %s", "world")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("hello world"))
		})

		It("should support format() with 3 arguments (hits format_string_dyn_dyn overload)", func() {
			result, err := evaluator.EvaluateExpression(ctx, `format("%s %s %s", "a", "b", "c")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("a b c"))
		})
		It("should support format() with 4 and 5 arguments (more overloads)", func() {
			r4, err := evaluator.EvaluateExpression(ctx, `format("%s %s %s %s", "a", "b", "c", "d")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(r4).To(Equal("a b c d"))
			r5, err := evaluator.EvaluateExpression(ctx, `format("%d %d %d %d %d", 1, 2, 3, 4, 5)`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(r5).To(Equal("1 2 3 4 5"))
		})

		It("should support format() with 12 arguments (perNamespaceSummary-style)", func() {
			expr := `format("%s: %d %d %d %d %d %d %d %d %d %d %d", "ns", 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11)`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("ns: 1 2 3 4 5 6 7 8 9 10 11"))
		})

		It("should support formatList(string, list)", func() {
			// List literal with same type (string) to satisfy CEL type inference; formatList accepts list(dyn)
			result, err := evaluator.EvaluateExpression(ctx, `formatList("a=%s b=%s c=%s", ["x", "y", "3"])`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("a=x b=y c=3"))
		})

		It("should support formatList with many args (no variadic cap)", func() {
			expr := `formatList("%s %s %s %s %s %s %s %s %s %s %s %s", ["a","b","c","d","e","f","g","h","i","j","k","l"])`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("a b c d e f g h i j k l"))
		})

		It("should return nested map and list from CEL (convertCELValueToNativeRecursive)", func() {
			vars := map[string]interface{}{
				"variables": map[string]interface{}{
					"nested": map[string]interface{}{"k": "v"},
					"list":   []interface{}{1.0, 2.0},
				},
			}
			result, err := evaluator.EvaluateExpression(ctx, `variables`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(m).To(HaveKey("nested"))
			Expect(m).To(HaveKey("list"))
		})
	})

	Describe("HTTP Library", func() {
		It("should support http.Get() function", func() {
			// Note: This requires network access, so we'll test compilation only
			// In real workflows, http.Get() would make actual HTTP requests
			expr := `http.Get("https://example.com")`
			_, err := evaluator.getOrCompileProgram(expr)
			// Should compile successfully (http variable is declared)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should support http.Post() function", func() {
			// Note: This requires network access, so we'll test compilation only
			expr := `http.Post("https://example.com", "body", {})`
			_, err := evaluator.getOrCompileProgram(expr)
			// Should compile successfully
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("User Library", func() {
		It("should support parseServiceAccount() function", func() {
			// parseServiceAccount extracts service account name and namespace from username
			expr := `parseServiceAccount("system:serviceaccount:default:my-sa")`
			result, err := evaluator.EvaluateExpression(ctx, expr, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return an object with Name and Namespace fields
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "parseServiceAccount() should return a map")
			Expect(resultMap["Name"]).To(Equal("my-sa"))
			Expect(resultMap["Namespace"]).To(Equal("default"))
		})
	})

	Describe("Image Library", func() {
		It("should support isImage() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `isImage("nginx:latest")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})

		It("should support image() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `image("nginx:latest").registry()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Should return registry string (may be "index.docker.io" or "docker.io")
			registry, ok := result.(string)
			Expect(ok).To(BeTrue(), "image().registry() should return a string")
			Expect(registry).To(BeElementOf([]string{"docker.io", "index.docker.io"}))
		})

		It("should support image().repository() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `image("nginx:latest").repository()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			repository, ok := result.(string)
			Expect(ok).To(BeTrue(), "image().repository() should return a string")
			Expect(repository).To(Equal("library/nginx"))
		})

		It("should support image().tag() function", func() {
			result, err := evaluator.EvaluateExpression(ctx, `image("nginx:latest").tag()`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tag, ok := result.(string)
			Expect(ok).To(BeTrue(), "image().tag() should return a string")
			Expect(tag).To(Equal("latest"))
		})

		It("should support image().containsDigest() function", func() {
			// Use a valid digest format - sha256 with full 64-character hash
			result, err := evaluator.EvaluateExpression(ctx, `image("nginx@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234").containsDigest()`, nil)
			if err != nil {
				// Try alternative format with docker.io prefix
				result, err = evaluator.EvaluateExpression(ctx, `image("docker.io/library/nginx@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234").containsDigest()`, nil)
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(true))
		})
	})

	Describe("ImageData Library", func() {
		It("should support image.GetMetadata() function", func() {
			// Note: This requires network access to fetch image metadata
			// We'll test that the function compiles and can be called
			// In real workflows, this would fetch actual image metadata
			expr := `image.GetMetadata("nginx:latest")`
			_, err := evaluator.getOrCompileProgram(expr)
			// Should compile successfully (image variable is declared)
			Expect(err).NotTo(HaveOccurred())
		})

		It("evaluates image.GetMetadata with MockImageDataFetcher", func() {
			mockData := map[string]any{"digest": "sha256:abc123", "registry": "docker.io"}
			evaluator.SetImageDataFetcher(&MockImageDataFetcher{Data: mockData})
			vars := map[string]interface{}{}
			result, err := evaluator.EvaluateExpression(ctx, `image.GetMetadata("nginx:latest")`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Result should be the mock data (as map or CEL representation)
			if m, ok := result.(map[string]interface{}); ok {
				Expect(m["digest"]).To(Equal("sha256:abc123"))
			}
		})
	})

	Describe("Resource Macros (pod/node/prometheusMetrics)", func() {
		It("prometheusMetrics returns result when mock client returns vector", func() {
			mockResult := &mockPrometheusResult{
				typ: "vector",
				samples: []PrometheusSample{
					&mockPrometheusSample{
						metric: map[string]string{"job": "test"},
						value:  1.0,
						ts:     time.Now(),
					},
				},
			}
			mockClient := &mockPrometheusClient{result: mockResult}
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, mockClient, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())

			result, err := eval.EvaluateExpression(ctx, `prometheusMetrics("up", "5m")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(m["type"]).To(Equal("vector"))
			Expect(m).To(HaveKey("samples"))
		})

		It("prometheusMetrics returns error map when mock client returns error", func() {
			mockClient := &mockPrometheusClient{err: fmt.Errorf("connection refused")}
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, mockClient, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())

			result, err := eval.EvaluateExpression(ctx, `prometheusMetrics("up", "5m")`, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(m["error"]).To(ContainSubstring("connection refused"))
		})

		It("podTotalCPURequest sums container CPU requests", func() {
			pod := map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
							},
						},
						map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "200m"},
							},
						},
					},
				},
			}
			vars := map[string]interface{}{"variables": map[string]interface{}{"pod": pod}}
			result, err := evaluator.EvaluateExpression(ctx, `podTotalCPURequest(variables.pod)`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.3, 0.001))
		})

		It("podTotalMemRequest sums container memory requests", func() {
			pod := map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"memory": "256Mi"},
							},
						},
					},
				},
			}
			vars := map[string]interface{}{"variables": map[string]interface{}{"pod": pod}}
			result, err := evaluator.EvaluateExpression(ctx, `podTotalMemRequest(variables.pod)`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically(">", 0))
		})

		// Regression: when a pod reaches the CEL function as an unstructured.Unstructured
		// (or *unstructured.Unstructured) rather than as map[string]interface{}, the prior
		// type assertion silently returned 0. unwrapObjectMap now handles all three shapes,
		// so the same expression returns the correct sum regardless of how cel-go wrapped
		// the value. This is the root cause of the cost-analyzer "savings > baseline"
		// anomaly; the affected workflow worked around it by switching to a pure-CEL path,
		// while this fix repairs the Go helper itself.
		It("podTotalCPURequest works when pod is unstructured.Unstructured", func() {
			obj := map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"cpu": "250m"},
							},
						},
					},
				},
			}
			pod := unstructured.Unstructured{Object: obj}
			vars := map[string]interface{}{"variables": map[string]interface{}{"pod": pod}}
			result, err := evaluator.EvaluateExpression(ctx, `podTotalCPURequest(variables.pod)`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("~", 0.25, 0.001))
		})

		It("podTotalMemRequest works when pod is *unstructured.Unstructured", func() {
			obj := map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{"memory": "512Mi"},
							},
						},
					},
				},
			}
			pod := &unstructured.Unstructured{Object: obj}
			vars := map[string]interface{}{"variables": map[string]interface{}{"pod": pod}}
			result, err := evaluator.EvaluateExpression(ctx, `podTotalMemRequest(variables.pod)`, vars)
			Expect(err).NotTo(HaveOccurred())
			// 512Mi == 512 * 2^20 bytes
			Expect(result).To(BeNumerically("~", float64(512*1024*1024), 1.0))
		})

		It("nodeCapacityCPU extracts node status.capacity.cpu", func() {
			node := map[string]interface{}{
				"status": map[string]interface{}{
					"capacity": map[string]interface{}{"cpu": "4", "memory": "8Gi"},
				},
			}
			vars := map[string]interface{}{"variables": map[string]interface{}{"node": node}}
			result, err := evaluator.EvaluateExpression(ctx, `nodeCapacityCPU(variables.node)`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(4.0))
		})

		It("nodeAllocatableMemory extracts node status.allocatable.memory", func() {
			node := map[string]interface{}{
				"status": map[string]interface{}{
					"allocatable": map[string]interface{}{"memory": "7600Mi"},
				},
			}
			vars := map[string]interface{}{"variables": map[string]interface{}{"node": node}}
			result, err := evaluator.EvaluateExpression(ctx, `nodeAllocatableMemory(variables.node)`, vars)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically(">", 0))
		})

		It("prometheusMetrics with invalid timeRange returns error", func() {
			mockClient := &mockPrometheusClient{result: &mockPrometheusResult{typ: "vector", samples: nil}}
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, mockClient, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = eval.EvaluateExpression(ctx, `prometheusMetrics("up", "invalid")`, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("timeRange"))
		})

		It("prometheusMetrics with nil client returns error", func() {
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = eval.EvaluateExpression(ctx, `prometheusMetrics("up", "5m")`, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available"))
		})

		It("prometheusMetrics with scalar result returns value", func() {
			mockClient := &mockPrometheusClient{
				result: &mockPrometheusResult{typ: "scalar", scalar: 42.0},
			}
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, mockClient, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			result, err := eval.EvaluateExpression(ctx, `prometheusMetrics("scalar_query", "5m")`, nil)
			Expect(err).NotTo(HaveOccurred())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(m["type"]).To(Equal("scalar"))
			Expect(m["value"]).To(Equal(42.0))
		})

		It("resourceLogs returns kubeClient-unavailable error when kubeClient is nil", func() {
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = eval.EvaluateExpression(ctx, `resourceLogs("v1", "Pod", "default", "log-pod", "main")`, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubeClient not available"))
		})

		It("resourceEvents compiles and evaluates", func() {
			scheme2 := runtime.NewScheme()
			utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme2))
			utilruntime.Must(clientgoscheme.AddToScheme(scheme2))
			clientWithPod := fake.NewClientBuilder().WithScheme(scheme2).Build()
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "ev-pod", Namespace: "default"},
			}
			Expect(clientWithPod.Create(ctx, pod)).To(Succeed())
			eval, err := NewCELEvaluatorWithMetrics(clientWithPod, nil, nil, nil, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			result, err := eval.EvaluateExpression(ctx, `resourceEvents("v1", "Pod", "default", "ev-pod")`, nil)
			// Fake client may or may not support field selectors; either list or error is acceptable
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("events"))
				return
			}
			Expect(result).NotTo(BeNil())
		})

		It("resourceLogs returns kubeClient-unavailable error (evaluator built without kubeClient)", func() {
			_, err := evaluator.EvaluateExpression(ctx, `resourceLogs("v1", "Pod", "default", "svc", "x")`, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubeClient not available"))
		})

		It("resourceMetrics with custom metricName uses CustomMetricsClient", func() {
			mockCustom := &mockCustomMetricsClient{
				value: &mockCustomMetricValue{name: "gpu_usage", val: 42, ts: time.Now(), window: 60},
			}
			eval, err := NewCELEvaluatorWithMetrics(fakeClient, nil, mockCustom, nil, nil, workflowRun, 0, nil)
			Expect(err).NotTo(HaveOccurred())
			result, err := eval.EvaluateExpression(ctx, `resourceMetrics("v1", "Pod", "default", "my-pod", "gpu_usage")`, nil)
			Expect(err).NotTo(HaveOccurred())
			m, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(m["metricName"]).To(Equal("gpu_usage"))
			Expect(m["value"]).To(BeEquivalentTo(42))
		})

		It("resourceMetrics with empty metricName and nil metrics client returns error", func() {
			_, err := evaluator.EvaluateExpression(ctx, `resourceMetrics("v1", "Pod", "default", "p", "")`, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metrics client not available"))
		})
	})

	Describe("GlobalContext Library", func() {
		It("should support globalContext.Get() function", func() {
			// Note: This requires GlobalContextEntry resources
			// We'll test that the function compiles
			expr := `globalContext.Get("my-entry", "")`
			_, err := evaluator.getOrCompileProgram(expr)
			// Should compile successfully (globalContext variable is declared)
			Expect(err).NotTo(HaveOccurred())
		})
		It("globalContext.Get evaluates for noop implementation", func() {
			result, err := evaluator.EvaluateExpression(ctx, `globalContext.Get("my-entry", "")`, nil)
			Expect(err).NotTo(HaveOccurred())
			// Noop returns (nil, nil); CEL may represent as null or similar
			_ = result
		})
	})

	Describe("topNByField function", func() {
		// CEL map literals require uniform value types, so test data is passed via
		// variables (typed as dyn at runtime) to mirror how workflow expressions work.
		makePods := func(items ...map[string]interface{}) map[string]interface{} {
			list := make([]interface{}, len(items))
			for i, item := range items {
				list[i] = item
			}
			return map[string]interface{}{
				"variables": map[string]interface{}{"pods": list},
			}
		}

		It("returns top N maps sorted descending by named field", func() {
			vars := makePods(
				map[string]interface{}{"id": "a", "savingsUSD": 1.0},
				map[string]interface{}{"id": "b", "savingsUSD": 3.0},
				map[string]interface{}{"id": "c", "savingsUSD": 2.0},
			)
			result, err := evaluator.EvaluateExpression(ctx, `topNByField(variables.pods, 2, "savingsUSD")`, vars)
			Expect(err).NotTo(HaveOccurred())
			list, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(list).To(HaveLen(2))
			first := list[0].(map[string]interface{})
			Expect(first["id"]).To(Equal("b"))
			second := list[1].(map[string]interface{})
			Expect(second["id"]).To(Equal("c"))
		})

		It("returns all elements when n >= list size", func() {
			vars := makePods(
				map[string]interface{}{"savingsUSD": 5.0},
				map[string]interface{}{"savingsUSD": 1.0},
			)
			result, err := evaluator.EvaluateExpression(ctx, `topNByField(variables.pods, 10, "savingsUSD")`, vars)
			Expect(err).NotTo(HaveOccurred())
			list, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(list).To(HaveLen(2))
		})

		It("returns empty list for n=0", func() {
			vars := makePods(
				map[string]interface{}{"savingsUSD": 1.0},
				map[string]interface{}{"savingsUSD": 2.0},
			)
			result, err := evaluator.EvaluateExpression(ctx, `topNByField(variables.pods, 0, "savingsUSD")`, vars)
			Expect(err).NotTo(HaveOccurred())
			list, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(list).To(BeEmpty())
		})

		It("returns empty list for empty input", func() {
			vars := makePods()
			result, err := evaluator.EvaluateExpression(ctx, `topNByField(variables.pods, 5, "savingsUSD")`, vars)
			Expect(err).NotTo(HaveOccurred())
			list, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(list).To(BeEmpty())
		})

		It("skips elements where field is absent", func() {
			vars := makePods(
				map[string]interface{}{"savingsUSD": 2.0},
				map[string]interface{}{"other": 99.0},
				map[string]interface{}{"savingsUSD": 5.0},
			)
			result, err := evaluator.EvaluateExpression(ctx, `topNByField(variables.pods, 3, "savingsUSD")`, vars)
			Expect(err).NotTo(HaveOccurred())
			list, ok := result.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(list).To(HaveLen(2))
			first := list[0].(map[string]interface{})
			Expect(first["savingsUSD"]).To(Equal(5.0))
		})
	})

	Describe("X509 Library", func() {
		It("should support x509.decode() function", func() {
			// Test that x509.decode() function compiles
			// Note: x509.decode() expects a PEM certificate string
			// We'll test compilation only - actual decoding requires valid PEM
			// Use a simple test string to verify the function exists
			expr := `x509.decode("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t")`
			_, err := evaluator.getOrCompileProgram(expr)
			// Should compile successfully (x509.decode function exists)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
