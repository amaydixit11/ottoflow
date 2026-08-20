/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/agent"
)

var _ = Describe("Agent Step Execution with Mock", func() {
	var (
		ctx              context.Context
		k8sClient        client.Client
		workflowRun      *ottoflowv1alpha1.WorkflowRun
		mockExecutor     *agent.MockAgentExecutor
		workflowExecutor *WorkflowExecutor
		scheme           *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		workflowRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-run",
				Namespace: "default",
			},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{
					Name: "test-workflow",
				},
			},
		}

		mockExecutor = agent.NewMockAgentExecutor()
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		var err error
		workflowExecutor, err = NewWorkflowExecutorWithAgentExecutor(
			k8sClient,
			nil, // metricsClient
			nil, // customMetricsClient
			nil, // prometheusClient
			workflowRun,
			mockExecutor,
			false, // localExecutionMode
			0,     // celCacheSize (use default)
			5,     // maxWorkers
			nil,   // eventRecorder
		)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("executeAgentStep with MockAgentExecutor", func() {
		It("should execute agent step successfully with mock response", func() {
			// Create Agent CRD
			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "You are a helpful assistant",
					ModelProvider: "openai",
					ModelName:     "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			// Create Workflow with agent step
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name: "agent-step",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name: "test-agent",
							},
							Outputs: []ottoflowv1alpha1.Output{
								{
									Name:       "result",
									Expression: `steps["agent-step"].response`,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Configure mock response
			mockExecutor.SetResponse("default/test-agent", "You are a helpful assistant", "Mock agent response")

			// Execute workflow
			err := workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred())

			// Verify agent was called
			Expect(mockExecutor.GetCallCount()).To(Equal(1))
			Expect(mockExecutor.WasCalledWith("default/test-agent", "You are a helpful assistant")).To(BeTrue())

			// Verify step completed
			stepStatus, exists := workflowRun.Status.StepStatuses["agent-step"]
			Expect(exists).To(BeTrue())
			Expect(stepStatus.Phase).To(Equal(ottoflowv1alpha1.StepPhaseSucceeded))

			// Verify output is in context
			contextData, err := workflowExecutor.GetContextManager().ReadContext(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(contextData["steps"]).NotTo(BeNil())
			steps := contextData["steps"].(map[string]interface{})
			Expect(steps["agent-step"]).NotTo(BeNil())
			agentStep := steps["agent-step"].(map[string]interface{})
			Expect(agentStep["response"]).To(Equal("Mock agent response"))
		})

		It("should set workflow and step execution times (reproduces 0ns duration bug)", func() {
			// WorkflowRun with Phase Pending (like CLI/local executor creates)
			workflowRunWithPending := &ottoflowv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-timing",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowRunSpec{
					WorkflowRef: ottoflowv1alpha1.WorkflowRef{
						Name: "test-workflow",
					},
				},
				Status: ottoflowv1alpha1.WorkflowRunStatus{
					Phase: ottoflowv1alpha1.WorkflowRunPhasePending,
				},
			}

			execWithPending, err := NewWorkflowExecutorWithAgentExecutor(
				k8sClient, nil, nil, nil, workflowRunWithPending, mockExecutor, false, 0, 5, nil)
			Expect(err).NotTo(HaveOccurred())

			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
				Spec:       ottoflowv1alpha1.AgentSpec{Prompt: "Test", ModelProvider: "openai", ModelName: "gpt-4"},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default"},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{{
						Name:     "agent-step",
						AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "test-agent"},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			mockExecutor.SetResponse("default/test-agent", "Test", "Response")

			err = execWithPending.ExecuteWorkflow(ctx, workflow, workflowRunWithPending)
			Expect(err).NotTo(HaveOccurred())

			// Workflow-level times: StartTime must be set when Phase was Pending
			Expect(workflowRunWithPending.Status.StartTime).NotTo(BeNil(), "workflow StartTime should be set")
			Expect(workflowRunWithPending.Status.CompletionTime).NotTo(BeNil(), "workflow CompletionTime should be set")
			duration := workflowRunWithPending.Status.CompletionTime.Sub(workflowRunWithPending.Status.StartTime.Time)
			Expect(duration).To(BeNumerically(">=", 0), "workflow duration should be non-negative")

			// Step-level times: must not reuse same pointer (causes 0ns duration bug)
			stepStatus, exists := workflowRunWithPending.Status.StepStatuses["agent-step"]
			Expect(exists).To(BeTrue())
			Expect(stepStatus.StartTime).NotTo(BeNil(), "step StartTime should be set")
			Expect(stepStatus.CompletionTime).NotTo(BeNil(), "step CompletionTime should be set")
			stepDuration := stepStatus.CompletionTime.Sub(stepStatus.StartTime.Time)
			Expect(stepDuration).To(BeNumerically(">=", 0), "step duration should be non-negative")
			// With the pointer-reuse bug, StartTime and CompletionTime pointed to same variable,
			// so display showed 0ns. After fix, they are distinct (duration may be 0 for very fast steps).
			Expect(stepStatus.StartTime).NotTo(BeIdenticalTo(stepStatus.CompletionTime), "StartTime and CompletionTime must not be the same pointer")
		})

		It("should handle agent execution error", func() {
			// Create Agent CRD
			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "You are a helpful assistant",
					ModelProvider: "openai",
					ModelName:     "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			// Create Workflow with agent step
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name:          "agent-step",
							FailurePolicy: ottoflowv1alpha1.FailurePolicyContinue,
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name: "test-agent",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Configure mock error
			testError := errors.New("agent execution failed")
			mockExecutor.SetError("default/test-agent", "You are a helpful assistant", testError)

			// Execute workflow
			err := workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred()) // Workflow executor handles errors gracefully

			// Verify agent was called
			Expect(mockExecutor.GetCallCount()).To(Equal(1))

			// Verify step failed
			stepStatus, exists := workflowRun.Status.StepStatuses["agent-step"]
			Expect(exists).To(BeTrue())
			Expect(stepStatus.Phase).To(Equal(ottoflowv1alpha1.StepPhaseFailed))
			Expect(stepStatus.Message).To(ContainSubstring("agent execution failed"))
		})

		It("should handle agent with additional prompts", func() {
			// Create Agent CRD
			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "You are a helpful assistant",
					ModelProvider: "openai",
					ModelName:     "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			// Create Workflow with agent step and additional prompts
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name: "agent-step",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name: "test-agent",
								AdditionalPrompts: []string{
									`"Analyze this data: " + inputs.data`,
								},
							},
						},
					},
					Inputs: []ottoflowv1alpha1.Input{
						{Name: "data", Required: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Set input values
			workflowRun.Spec.InputValues = map[string]string{
				"data": "test-data",
			}

			// Configure mock response for combined prompt
			expectedPrompt := "You are a helpful assistant\n\nAnalyze this data: test-data"
			mockExecutor.SetResponse("default/test-agent", expectedPrompt, "Analysis result")

			// Execute workflow
			err := workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred())

			// Verify agent was called with combined prompt
			Expect(mockExecutor.WasCalledWith("default/test-agent", expectedPrompt)).To(BeTrue())
		})

		It("should apply provider/model overrides via SetAgentOverrides", func() {
			// Create Agent with openai provider
			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "You are a helpful assistant",
					ModelProvider: "openai",
					ModelName:     "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name:     "agent-step",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "test-agent"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Create a local-mode executor with overrides (overrides are local-mode only)
			localMock := agent.NewMockAgentExecutor()
			localMock.SetDefaultResponse("Overridden response")
			localExec, err := NewWorkflowExecutorWithAgentExecutor(
				k8sClient, nil, nil, nil, workflowRun, localMock, true, 0, 5, nil)
			Expect(err).NotTo(HaveOccurred())
			localExec.SetAgentOverrides("anthropic", "claude-3-opus")

			err = localExec.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred())

			// Verify the agent CRD passed to mock had overridden values
			history := localMock.GetCallHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].Agent).NotTo(BeNil())
			Expect(history[0].Agent.Spec.ModelProvider).To(Equal("anthropic"))
			Expect(history[0].Agent.Spec.ModelName).To(Equal("claude-3-opus"))
		})

		It("should override only model when provider override is empty", func() {
			agentCRD := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "You are a helpful assistant",
					ModelProvider: "openai",
					ModelName:     "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD)).To(Succeed())

			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name:     "agent-step",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "test-agent"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Override only model, keep original provider
			localMock := agent.NewMockAgentExecutor()
			localMock.SetDefaultResponse("Model-only override")
			localExec, err := NewWorkflowExecutorWithAgentExecutor(
				k8sClient, nil, nil, nil, workflowRun, localMock, true, 0, 5, nil)
			Expect(err).NotTo(HaveOccurred())
			localExec.SetAgentOverrides("", "gpt-4-turbo")

			err = localExec.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred())

			history := localMock.GetCallHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].Agent).NotTo(BeNil())
			Expect(history[0].Agent.Spec.ModelProvider).To(Equal("openai"))  // Original provider kept
			Expect(history[0].Agent.Spec.ModelName).To(Equal("gpt-4-turbo")) // Model overridden
		})

		It("should handle multiple agent steps", func() {
			// Create Agent CRDs
			agentCRD1 := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-1",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "Agent 1",
					ModelProvider: "openai",
				},
			}
			agentCRD2 := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-2",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.AgentSpec{
					Prompt:        "Agent 2",
					ModelProvider: "openai",
				},
			}
			Expect(k8sClient.Create(ctx, agentCRD1)).To(Succeed())
			Expect(k8sClient.Create(ctx, agentCRD2)).To(Succeed())

			// Create Workflow with multiple agent steps
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Steps: []ottoflowv1alpha1.Step{
						{
							Name: "step-1",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name: "agent-1",
							},
						},
						{
							Name: "step-2",
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name: "agent-2",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			// Configure mock responses
			mockExecutor.SetResponse("default/agent-1", "Agent 1", "Response 1")
			mockExecutor.SetResponse("default/agent-2", "Agent 2", "Response 2")

			// Execute workflow
			err := workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
			Expect(err).NotTo(HaveOccurred())

			// Verify both agents were called
			Expect(mockExecutor.GetCallCount()).To(Equal(2))
			Expect(mockExecutor.WasCalledWith("default/agent-1", "Agent 1")).To(BeTrue())
			Expect(mockExecutor.WasCalledWith("default/agent-2", "Agent 2")).To(BeTrue())
		})

		// These two tests exercise contextBudget end-to-end through executeAgentStep (not just the
		// applyContextBudget helper in isolation): they run agent steps whose prompts surface the
		// filtered `steps` map, then assert the prompt the final agent actually received excludes
		// pruned step outputs while retaining non-step context (inputs). A regression that removed
		// or bypassed the budget wiring would fail here even though the unit tests still pass.
		It("should prune non-recent steps from the agent prompt under contextBudgetMode=lastN", func() {
			// a -> b -> c chained so completion order is deterministic, then a final step with lastN=2.
			for _, a := range []struct{ name, prompt string }{
				{"agent-a", "Agent A base"},
				{"agent-b", "Agent B base"},
				{"agent-c", "Agent C base"},
				{"agent-final", "Final base"},
			} {
				Expect(k8sClient.Create(ctx, &ottoflowv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Name: a.name, Namespace: "default"},
					Spec:       ottoflowv1alpha1.AgentSpec{Prompt: a.prompt, ModelProvider: "openai", ModelName: "gpt-4"},
				})).To(Succeed())
			}

			lastN := int32(2)
			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default"},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Inputs: []ottoflowv1alpha1.Input{{Name: "tag", Required: true}},
					Steps: []ottoflowv1alpha1.Step{
						{Name: "step-a", AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "agent-a"}},
						{Name: "step-b", DependsOn: []string{"step-a"}, AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "agent-b"}},
						{Name: "step-c", DependsOn: []string{"step-b"}, AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "agent-c"}},
						{
							Name:      "step-final",
							DependsOn: []string{"step-c"},
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name:               "agent-final",
								ContextBudgetMode:  "lastN",
								ContextBudgetLastN: &lastN,
								AdditionalPrompts: []string{
									`steps`,               // whole (filtered) steps map -> JSON in the prompt
									`"tag=" + inputs.tag`, // non-step context must survive pruning
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			mockExecutor.SetResponse("default/agent-a", "Agent A base", "RESP_A_UNIQUE")
			mockExecutor.SetResponse("default/agent-b", "Agent B base", "RESP_B_UNIQUE")
			mockExecutor.SetResponse("default/agent-c", "Agent C base", "RESP_C_UNIQUE")
			mockExecutor.SetDefaultResponse("final done") // agent-final's prompt is dynamic
			workflowRun.Spec.InputValues = map[string]string{"tag": "keep-me"}

			Expect(workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)).To(Succeed())

			finalCalls := mockExecutor.GetCallsForAgent("default/agent-final")
			Expect(finalCalls).To(HaveLen(1))
			finalPrompt := finalCalls[0].Prompt

			// lastN=2 over completion order [a,b,c] retains b and c, prunes a.
			Expect(finalPrompt).To(ContainSubstring("RESP_C_UNIQUE"), "most recent step must be retained")
			Expect(finalPrompt).To(ContainSubstring("RESP_B_UNIQUE"), "second-most-recent step must be retained")
			Expect(finalPrompt).NotTo(ContainSubstring("RESP_A_UNIQUE"), "pruned step output must not reach the prompt")
			Expect(finalPrompt).To(ContainSubstring("tag=keep-me"), "non-step context (inputs) must be retained")
		})

		It("should drop all step outputs but keep non-step context under contextBudgetMode=omit", func() {
			Expect(k8sClient.Create(ctx, &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-a", Namespace: "default"},
				Spec:       ottoflowv1alpha1.AgentSpec{Prompt: "Agent A base", ModelProvider: "openai", ModelName: "gpt-4"},
			})).To(Succeed())
			Expect(k8sClient.Create(ctx, &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-final", Namespace: "default"},
				Spec:       ottoflowv1alpha1.AgentSpec{Prompt: "Final base", ModelProvider: "openai", ModelName: "gpt-4"},
			})).To(Succeed())

			workflow := &ottoflowv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default"},
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Inputs: []ottoflowv1alpha1.Input{{Name: "tag", Required: true}},
					Steps: []ottoflowv1alpha1.Step{
						{Name: "step-a", AgentRef: &ottoflowv1alpha1.StepAgentRef{Name: "agent-a"}},
						{
							Name:      "step-final",
							DependsOn: []string{"step-a"},
							AgentRef: &ottoflowv1alpha1.StepAgentRef{
								Name:              "agent-final",
								ContextBudgetMode: "omit",
								AdditionalPrompts: []string{
									`steps`,
									`"tag=" + inputs.tag`,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			mockExecutor.SetResponse("default/agent-a", "Agent A base", "RESP_A_UNIQUE")
			mockExecutor.SetDefaultResponse("final done")
			workflowRun.Spec.InputValues = map[string]string{"tag": "keep-me"}

			Expect(workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)).To(Succeed())

			finalCalls := mockExecutor.GetCallsForAgent("default/agent-final")
			Expect(finalCalls).To(HaveLen(1))
			finalPrompt := finalCalls[0].Prompt

			Expect(finalPrompt).NotTo(ContainSubstring("RESP_A_UNIQUE"), "omit must drop all step outputs")
			Expect(finalPrompt).To(ContainSubstring("tag=keep-me"), "non-step context (inputs) must be retained")
		})
	})
})

// TestTruncateAdditionalPrompt_ExactBoundaryNotTruncated: RuneCount == tokenBudget (maxTokens*3)
// must NOT truncate. This is the exact boundary the int32-overflow fix depends on getting right.
func TestTruncateAdditionalPrompt_ExactBoundaryNotTruncated(t *testing.T) {
	const maxTokens = int32(10)
	text := strings.Repeat("a", int(maxTokens)*3) // exactly at budget (30 runes)

	result, truncated := truncateAdditionalPrompt(text, maxTokens)

	if truncated {
		t.Errorf("expected no truncation when RuneCount == tokenBudget, got truncated=true, result=%q", result)
	}
	if result != text {
		t.Errorf("expected text unchanged at exact boundary, got %q", result)
	}
}

// TestTruncateAdditionalPrompt_OneOverBoundaryTruncates is the complementary case: one rune past
// the budget must truncate, and the returned string (content + marker) must still fit within
// tokenBudget runes. The marker is reserved inside the budget, not appended on top of a full slice.
func TestTruncateAdditionalPrompt_OneOverBoundaryTruncates(t *testing.T) {
	const maxTokens = int32(10)
	tokenBudget := int(maxTokens) * 3          // 30 runes
	text := strings.Repeat("a", tokenBudget+1) // one rune past budget

	result, truncated := truncateAdditionalPrompt(text, maxTokens)

	if !truncated {
		t.Error("expected truncation when RuneCount > tokenBudget")
	}
	// The whole point of the cap: marker included, the result never exceeds the budget.
	if got := utf8.RuneCountInString(result); got > tokenBudget {
		t.Errorf("truncated result exceeds budget: got %d runes, want <= %d (result=%q)", got, tokenBudget, result)
	}
	want := strings.Repeat("a", tokenBudget-len("...")) + "..."
	if result != want {
		t.Errorf("unexpected truncated result: got %q, want %q", result, want)
	}
}

// TestTruncateAdditionalPrompt_LargeBudgetNoInt32Overflow guards the int64 math fix: a budget
// whose *3 would overflow int32 (max ~715,827,882) must not panic and must not wrongly truncate.
func TestTruncateAdditionalPrompt_LargeBudgetNoInt32Overflow(t *testing.T) {
	const maxTokens = int32(800_000_000) // maxTokens*3 overflows int32
	text := "short text"

	result, truncated := truncateAdditionalPrompt(text, maxTokens)

	if truncated {
		t.Errorf("short text under a huge budget must not be truncated, got %q", result)
	}
	if result != text {
		t.Errorf("expected text unchanged, got %q", result)
	}
}
