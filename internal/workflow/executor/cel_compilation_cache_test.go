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
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("CELCompilationCache", func() {
	var (
		k8sClient client.Client
		cache     *CELCompilationCache
	)

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		var err error
		cache, err = NewCELCompilationCache(k8sClient, nil, nil, nil, textlogger.NewLogger(textlogger.NewConfig()))
		Expect(err).NotTo(HaveOccurred())
		Expect(cache).NotTo(BeNil())
	})

	It("should compile valid CEL expressions", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Variables: []ottoflowv1alpha1.Variable{
					{Name: "threshold", Expression: "42"},
				},
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "compute",
						Expressions: []ottoflowv1alpha1.Expression{
							{Name: "sum", Expression: "1 + 2"},
							{Name: "msg", Expression: `"hello " + "world"`},
						},
						Outputs: []ottoflowv1alpha1.Output{
							{Name: "result", Expression: "expressions.sum * 10"},
						},
					},
				},
				Outputs: []ottoflowv1alpha1.Output{
					{Name: "final", Expression: "variables.threshold"},
				},
			},
		}

		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())

		programs := cache.GetPrograms(WorkflowKey("default", "test-wf"))
		Expect(programs).NotTo(BeNil())
		Expect(programs).To(HaveLen(5))
		Expect(programs).To(HaveKey("42"))
		Expect(programs).To(HaveKey("1 + 2"))
		Expect(programs).To(HaveKey(`"hello " + "world"`))
		Expect(programs).To(HaveKey("expressions.sum * 10"))
		Expect(programs).To(HaveKey("variables.threshold"))
	})

	It("should compile workflow with ForEach step expressions", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "foreach-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "fe",
						ForEach: &ottoflowv1alpha1.StepForEach{
							Items: `["a", "b"]`,
							Step: &ottoflowv1alpha1.StepForEachStep{
								Expressions: []ottoflowv1alpha1.Expression{{Name: "e", Expression: `variables.item + "-suffix"`}},
								Outputs:     []ottoflowv1alpha1.Output{{Name: "out", Expression: `expressions.e`}},
							},
						},
					},
				},
			},
		}
		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())
		programs := cache.GetPrograms(WorkflowKey("default", "foreach-wf"))
		Expect(programs).NotTo(BeNil())
		Expect(programs).To(HaveKey(`["a", "b"]`))
		Expect(programs).To(HaveKey(`variables.item + "-suffix"`))
		Expect(programs).To(HaveKey("expressions.e"))
	})

	It("should report compilation errors for invalid expressions", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "broken",
						Expressions: []ottoflowv1alpha1.Expression{
							{Name: "ok", Expression: "1 + 1"},
							{Name: "bad", Expression: "???invalid!!!"},
						},
					},
				},
			},
		}

		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Error()).To(ContainSubstring("bad"))

		programs := cache.GetPrograms(WorkflowKey("default", "bad-wf"))
		Expect(programs).To(HaveLen(1))
		Expect(programs).To(HaveKey("1 + 1"))
	})

	It("should invalidate cached programs on workflow deletion", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "del-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "s", Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: "1"}}},
				},
			},
		}

		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())
		Expect(cache.GetPrograms(WorkflowKey("default", "del-wf"))).To(HaveLen(1))

		cache.InvalidateWorkflow(WorkflowKey("default", "del-wf"))
		Expect(cache.GetPrograms(WorkflowKey("default", "del-wf"))).To(BeNil())
	})

	It("should replace all programs when workflow is reloaded", func() {
		wfKey := WorkflowKey("default", "reload-wf")

		v1 := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "reload-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "a", Expressions: []ottoflowv1alpha1.Expression{{Name: "x", Expression: "1"}}},
				},
			},
		}
		Expect(cache.CompileWorkflow(v1)).To(BeEmpty())
		Expect(cache.GetPrograms(wfKey)).To(HaveLen(1))
		Expect(cache.GetPrograms(wfKey)).To(HaveKey("1"))

		v2 := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "reload-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "b", Expressions: []ottoflowv1alpha1.Expression{
						{Name: "y", Expression: "2"},
						{Name: "z", Expression: "3"},
					}},
				},
			},
		}
		Expect(cache.CompileWorkflow(v2)).To(BeEmpty())
		progs := cache.GetPrograms(wfKey)
		Expect(progs).To(HaveLen(2))
		Expect(progs).To(HaveKey("2"))
		Expect(progs).To(HaveKey("3"))
		Expect(progs).NotTo(HaveKey("1"))
	})

	It("should compile step with WorkflowRef, MCPToolCall, ResourceQuery, PrometheusQuery, Mutate, Output Metric (extractStepExpressions branches)", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "full-step-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "full",
						WorkflowRef: &ottoflowv1alpha1.StepWorkflowRef{
							Name:   "sub",
							Inputs: map[string]string{"p": "variables.x"},
						},
						MCPToolCall: &ottoflowv1alpha1.StepMCPToolCall{
							Server:    "srv",
							Tool:      "t",
							Arguments: map[string]string{"arg": "1"},
						},
						ResourceQuery: &ottoflowv1alpha1.StepResourceQuery{
							APIVersion: "v1",
							Resource:   "pods",
							Outputs:    map[string]string{"name": "string(\"x\")"},
						},
						PrometheusQuery: &ottoflowv1alpha1.StepPrometheusQuery{
							Query:     "up",
							Variables: map[string]string{"v": "\"5m\""},
							Outputs:   map[string]string{"val": "1"},
						},
						Mutate: &ottoflowv1alpha1.StepMutate{
							Target:             ottoflowv1alpha1.StepMutateTarget{APIVersion: "v1", Resource: "Pod", Name: "x"},
							PatchType:          "ApplyConfiguration",
							ApplyConfiguration: &ottoflowv1alpha1.MutateApplyConfiguration{Expression: "object"},
							JSONPatch:          &ottoflowv1alpha1.MutateJSONPatch{Expression: "[]"},
							Outputs:            map[string]string{"o": "object.metadata.name"},
						},
						Outputs: []ottoflowv1alpha1.Output{
							{Name: "out", Expression: "1", Metric: &ottoflowv1alpha1.OutputMetric{Name: "m", Type: "gauge", Labels: []ottoflowv1alpha1.MetricLabel{{Name: "l", Value: "\"v\""}}}},
						},
					},
				},
			},
		}
		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())
		programs := cache.GetPrograms(WorkflowKey("default", "full-step-wf"))
		Expect(programs).NotTo(BeNil())
		Expect(programs).To(HaveKey("variables.x"))
		Expect(programs).To(HaveKey("1"))
		Expect(programs).To(HaveKey("string(\"x\")"))
		Expect(programs).To(HaveKey("\"5m\""))
		Expect(programs).To(HaveKey("1"))
		Expect(programs).To(HaveKey("object"))
		Expect(programs).To(HaveKey("[]"))
		Expect(programs).To(HaveKey("object.metadata.name"))
		Expect(programs).To(HaveKey("\"v\""))
	})

	It("should compile forEach step with WorkflowRef, MCP, ResourceQuery, PrometheusQuery, Mutate (extractForEachStepExpressions)", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "foreach-full-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "fe",
						ForEach: &ottoflowv1alpha1.StepForEach{
							Items: `[1]`,
							Step: &ottoflowv1alpha1.StepForEachStep{
								WorkflowRef:     &ottoflowv1alpha1.StepWorkflowRef{Name: "w", Inputs: map[string]string{"i": "item"}},
								MCPToolCall:     &ottoflowv1alpha1.StepMCPToolCall{Server: "s", Tool: "t", Arguments: map[string]string{"a": "item"}},
								ResourceQuery:   &ottoflowv1alpha1.StepResourceQuery{APIVersion: "v1", Resource: "pods", Outputs: map[string]string{"n": "string(\"n\")"}},
								PrometheusQuery: &ottoflowv1alpha1.StepPrometheusQuery{Query: "up", Variables: map[string]string{"t": "\"5m\""}, Outputs: map[string]string{"v": "1"}},
								Mutate: &ottoflowv1alpha1.StepMutate{
									Target:             ottoflowv1alpha1.StepMutateTarget{APIVersion: "v1", Resource: "Pod", Name: "p"},
									PatchType:          "ApplyConfiguration",
									ApplyConfiguration: &ottoflowv1alpha1.MutateApplyConfiguration{Expression: "object"},
									Outputs:            map[string]string{"x": "object"},
								},
								MatchConditions: []ottoflowv1alpha1.MatchCondition{{Name: "m", Expression: "true"}},
							},
						},
					},
				},
			},
		}
		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())
		programs := cache.GetPrograms(WorkflowKey("default", "foreach-full-wf"))
		Expect(programs).NotTo(BeNil())
		Expect(programs).To(HaveKey("item"))
		Expect(programs).To(HaveKey("object"))
		Expect(programs).To(HaveKey("true"))
	})

	It("should compile matchConditions and forEach expressions", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "complex-wf", Namespace: "ns"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "cond",
						MatchConditions: []ottoflowv1alpha1.MatchCondition{
							{Name: "check", Expression: "true"},
						},
						Expressions: []ottoflowv1alpha1.Expression{
							{Name: "val", Expression: `"ok"`},
						},
					},
					{
						Name: "loop",
						ForEach: &ottoflowv1alpha1.StepForEach{
							Items: `[1, 2, 3]`,
							Step: &ottoflowv1alpha1.StepForEachStep{
								Expressions: []ottoflowv1alpha1.Expression{
									{Name: "doubled", Expression: "item * 2"},
								},
								Outputs: []ottoflowv1alpha1.Output{
									{Name: "res", Expression: "expressions.doubled"},
								},
							},
						},
					},
				},
			},
		}

		errs := cache.CompileWorkflow(workflow)
		Expect(errs).To(BeEmpty())

		programs := cache.GetPrograms(WorkflowKey("ns", "complex-wf"))
		Expect(programs).NotTo(BeNil())
		Expect(programs).To(HaveKey("true"))
		Expect(programs).To(HaveKey(`"ok"`))
		Expect(programs).To(HaveKey("[1, 2, 3]"))
		Expect(programs).To(HaveKey("item * 2"))
		Expect(programs).To(HaveKey("expressions.doubled"))
	})

	It("should preload evaluator from cache", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "preload-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "s", Expressions: []ottoflowv1alpha1.Expression{
						{Name: "x", Expression: "1 + 2"},
					}},
				},
			},
		}
		Expect(cache.CompileWorkflow(workflow)).To(BeEmpty())

		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "run-1", Namespace: "default"},
		}
		evaluator, err := NewCELEvaluator(k8sClient, workflowRun)
		Expect(err).NotTo(HaveOccurred())

		evaluator.PreloadFromCache(cache, WorkflowKey("default", "preload-wf"))

		cached, ok := evaluator.programCache.Get("1 + 2")
		Expect(ok).To(BeTrue())
		Expect(cached).NotTo(BeNil())

		result, err := evaluator.EvaluateExpression(context.Background(), "1 + 2", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeEquivalentTo(int64(3)))
	})

	It("should keep separate caches for different workflows", func() {
		wf1 := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf1", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "s", Expressions: []ottoflowv1alpha1.Expression{{Name: "a", Expression: `"alpha"`}}},
				},
			},
		}
		wf2 := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf2", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "s", Expressions: []ottoflowv1alpha1.Expression{{Name: "b", Expression: `"beta"`}}},
				},
			},
		}

		Expect(cache.CompileWorkflow(wf1)).To(BeEmpty())
		Expect(cache.CompileWorkflow(wf2)).To(BeEmpty())

		p1 := cache.GetPrograms(WorkflowKey("default", "wf1"))
		Expect(p1).To(HaveLen(1))
		Expect(p1).To(HaveKey(`"alpha"`))

		p2 := cache.GetPrograms(WorkflowKey("default", "wf2"))
		Expect(p2).To(HaveLen(1))
		Expect(p2).To(HaveKey(`"beta"`))

		cache.InvalidateWorkflow(WorkflowKey("default", "wf1"))
		Expect(cache.GetPrograms(WorkflowKey("default", "wf1"))).To(BeNil())
		Expect(cache.GetPrograms(WorkflowKey("default", "wf2"))).To(HaveLen(1))
	})
})

var _ = Describe("extractCELExpressions", func() {
	It("should extract all CEL expression types from a workflow", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Variables: []ottoflowv1alpha1.Variable{
					{Name: "v1", Expression: "expr_v1"},
				},
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "step1",
						Expressions: []ottoflowv1alpha1.Expression{
							{Name: "e1", Expression: "expr_e1"},
						},
						Outputs: []ottoflowv1alpha1.Output{
							{Name: "o1", Expression: "expr_o1"},
						},
						MatchConditions: []ottoflowv1alpha1.MatchCondition{
							{Name: "mc1", Expression: "expr_mc1"},
						},
						MCPToolCall: &ottoflowv1alpha1.StepMCPToolCall{
							Server:    "srv",
							Tool:      "tool",
							Arguments: map[string]string{"arg1": "expr_arg1"},
						},
						ResourceQuery: &ottoflowv1alpha1.StepResourceQuery{
							APIVersion: "v1",
							Resource:   "Pod",
							Outputs:    map[string]string{"rqo": "expr_rqo"},
						},
					},
				},
				Outputs: []ottoflowv1alpha1.Output{
					{Name: "wo1", Expression: "expr_wo1"},
				},
			},
		}

		exprs := extractCELExpressions(workflow)

		byName := make(map[string]string, len(exprs))
		for _, e := range exprs {
			byName[e.Name] = e.Text
		}

		Expect(byName).To(HaveKeyWithValue("var.v1", "expr_v1"))
		Expect(byName).To(HaveKeyWithValue("step.step1.expr.e1", "expr_e1"))
		Expect(byName).To(HaveKeyWithValue("step.step1.out.o1", "expr_o1"))
		Expect(byName).To(HaveKeyWithValue("step.step1.match.mc1", "expr_mc1"))
		Expect(byName).To(HaveKeyWithValue("step.step1.mcp.arg.arg1", "expr_arg1"))
		Expect(byName).To(HaveKeyWithValue("step.step1.rq.out.rqo", "expr_rqo"))
		Expect(byName).To(HaveKeyWithValue("out.wo1", "expr_wo1"))
	})

	It("should extract forEach step expressions", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "loop",
						ForEach: &ottoflowv1alpha1.StepForEach{
							Items: "items_expr",
							Step: &ottoflowv1alpha1.StepForEachStep{
								Expressions: []ottoflowv1alpha1.Expression{
									{Name: "fe1", Expression: "fe_expr1"},
								},
								Outputs: []ottoflowv1alpha1.Output{
									{Name: "feo", Expression: "fe_out_expr"},
								},
								MatchConditions: []ottoflowv1alpha1.MatchCondition{
									{Name: "femc", Expression: "fe_match_expr"},
								},
							},
						},
					},
				},
			},
		}

		exprs := extractCELExpressions(workflow)

		byName := make(map[string]string, len(exprs))
		for _, e := range exprs {
			byName[e.Name] = e.Text
		}

		Expect(byName).To(HaveKeyWithValue("step.loop.forEach.items", "items_expr"))
		Expect(byName).To(HaveKeyWithValue("step.loop.forEach.expr.fe1", "fe_expr1"))
		Expect(byName).To(HaveKeyWithValue("step.loop.forEach.out.feo", "fe_out_expr"))
		Expect(byName).To(HaveKeyWithValue("step.loop.forEach.match.femc", "fe_match_expr"))
	})

	It("should handle empty workflow", func() {
		workflow := &ottoflowv1alpha1.Workflow{
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{Name: "noop"},
				},
			},
		}
		exprs := extractCELExpressions(workflow)
		Expect(exprs).To(BeEmpty())
	})
})
