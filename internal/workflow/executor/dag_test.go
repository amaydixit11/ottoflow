/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("DAG Builder", func() {
	Describe("BuildDAG", func() {
		It("should build a simple DAG with no dependencies", func() {
			steps := []ottoflowv1alpha1.Step{
				{Name: "step1"},
				{Name: "step2"},
				{Name: "step3"},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			Expect(dag).NotTo(BeNil())
			Expect(dag.GetNode("step1")).NotTo(BeNil())
			Expect(dag.GetNode("step2")).NotTo(BeNil())
			Expect(dag.GetNode("step3")).NotTo(BeNil())
		})

		It("should build a DAG with explicit dependencies", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output1", Expression: `"value1"`},
					},
				},
				{
					Name:      "step2",
					DependsOn: []string{"step1"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "result", Expression: `variables.output1`},
					},
				},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			Expect(dag).NotTo(BeNil())
			step1Node := dag.GetNode("step1")
			step2Node := dag.GetNode("step2")
			Expect(step1Node).NotTo(BeNil())
			Expect(step2Node).NotTo(BeNil())
			// Verify dependency relationship
			Expect(step2Node.Dependencies).To(ContainElement("step1"))
			Expect(step1Node.Dependents).To(ContainElement("step2"))
		})

		It("should detect circular dependencies", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output1", Expression: `"value1"`},
					},
				},
				{
					Name:      "step2",
					DependsOn: []string{"step1"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "temp", Expression: `variables.output1`},
					},
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output2", Expression: `expressions.temp`},
					},
				},
				{
					Name:      "step3",
					DependsOn: []string{"step2"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "temp", Expression: `variables.output2`},
					},
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output3", Expression: `expressions.temp`},
					},
				},
			}
			// Create a cycle: step1 -> step2 -> step3 -> step1
			steps[0].DependsOn = []string{"step3"}
			steps[0].Outputs[0].Expression = `variables.output3`

			dag, err := BuildDAG(steps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular"))
			Expect(dag).To(BeNil())
		})

		It("should build a complex DAG with multiple dependencies", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output1", Expression: `"value1"`},
					},
				},
				{
					Name: "step2",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output2", Expression: `"value2"`},
					},
				},
				{
					Name:      "step3",
					DependsOn: []string{"step1", "step2"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "result", Expression: `variables.output1 + variables.output2`},
					},
				},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			Expect(dag).NotTo(BeNil())
			step3Node := dag.GetNode("step3")
			Expect(step3Node).NotTo(BeNil())
			// Verify step3 depends on both step1 and step2
			Expect(step3Node.Dependencies).To(ContainElements("step1", "step2"))
		})

		It("should handle steps with no outputs", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "temp", Expression: `"temp"`},
					},
				},
				{
					Name:      "step2",
					DependsOn: []string{"step1"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "result", Expression: `variables.temp`},
					},
				},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			Expect(dag).NotTo(BeNil())
			Expect(dag.GetNode("step1")).NotTo(BeNil())
			Expect(dag.GetNode("step2")).NotTo(BeNil())
		})
	})

	Describe("GetReadySteps", func() {
		It("should return all steps when no dependencies", func() {
			steps := []ottoflowv1alpha1.Step{
				{Name: "step1"},
				{Name: "step2"},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())

			completed := make(map[string]bool)
			ready := dag.GetReadySteps(completed)
			Expect(ready).To(HaveLen(2))
			Expect(ready).To(ContainElements("step1", "step2"))
		})

		It("should return only independent steps when dependencies exist", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output1", Expression: `"value1"`},
					},
				},
				{
					Name:      "step2",
					DependsOn: []string{"step1"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "result", Expression: `variables.output1`},
					},
				},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())

			completed := make(map[string]bool)
			ready := dag.GetReadySteps(completed)
			Expect(ready).To(HaveLen(1))
			Expect(ready).To(ContainElement("step1"))
		})

		It("should return dependent step after dependency completes", func() {
			steps := []ottoflowv1alpha1.Step{
				{
					Name: "step1",
					Outputs: []ottoflowv1alpha1.Output{
						{Name: "output1", Expression: `"value1"`},
					},
				},
				{
					Name:      "step2",
					DependsOn: []string{"step1"},
					Expressions: []ottoflowv1alpha1.Expression{
						{Name: "result", Expression: `variables.output1`},
					},
				},
			}

			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())

			completed := make(map[string]bool)
			ready := dag.GetReadySteps(completed)
			Expect(ready).To(ContainElement("step1"))

			completed["step1"] = true
			ready = dag.GetReadySteps(completed)
			Expect(ready).To(ContainElement("step2"))
		})
	})

	Describe("GetAllNodes", func() {
		It("should return all nodes in the DAG", func() {
			steps := []ottoflowv1alpha1.Step{
				{Name: "a"},
				{Name: "b", DependsOn: []string{"a"}},
			}
			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			all := dag.GetAllNodes()
			Expect(all).To(HaveLen(2))
			names := make([]string, len(all))
			for i, n := range all {
				names[i] = n.Name
			}
			Expect(names).To(ContainElements("a", "b"))
		})
	})

	Describe("AddEdge", func() {
		It("should not duplicate when same edge is added twice", func() {
			steps := []ottoflowv1alpha1.Step{{Name: "a"}, {Name: "b"}}
			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			dag.AddEdge("a", "b")
			dag.AddEdge("a", "b")
			nodeA := dag.GetNode("a")
			Expect(nodeA.Dependents).To(HaveLen(1))
			Expect(nodeA.Dependents).To(ContainElement("b"))
			nodeB := dag.GetNode("b")
			Expect(nodeB.Dependencies).To(HaveLen(1))
			Expect(nodeB.Dependencies).To(ContainElement("a"))
		})

		It("should no-op when from or to node does not exist", func() {
			steps := []ottoflowv1alpha1.Step{{Name: "a"}}
			dag, err := BuildDAG(steps)
			Expect(err).NotTo(HaveOccurred())
			dag.AddEdge("missing", "a")
			dag.AddEdge("a", "missing")
			Expect(dag.GetNode("a").Dependencies).To(BeEmpty())
			Expect(dag.GetNode("a").Dependents).To(BeEmpty())
		})
	})

	Describe("findStepByName", func() {
		It("returns step when name exists", func() {
			steps := []ottoflowv1alpha1.Step{
				{Name: "first"},
				{Name: "second"},
			}
			s := findStepByName(steps, "second")
			Expect(s).NotTo(BeNil())
			Expect(s.Name).To(Equal("second"))
		})
		It("returns nil when name does not exist", func() {
			steps := []ottoflowv1alpha1.Step{{Name: "only"}}
			Expect(findStepByName(steps, "missing")).To(BeNil())
		})
	})
})
