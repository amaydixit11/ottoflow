/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("MockAgentExecutor", func() {
	var (
		mockExecutor *MockAgentExecutor
		ctx          context.Context
		agentCRD     *ottoflowv1alpha1.Agent
	)

	BeforeEach(func() {
		mockExecutor = NewMockAgentExecutor()
		ctx = context.Background()
		agentCRD = &ottoflowv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-agent",
				Namespace: "default",
			},
			Spec: ottoflowv1alpha1.AgentSpec{
				Prompt:        "You are a test agent",
				ModelProvider: "openai",
				ModelName:     "gpt-4",
			},
		}
	})

	Describe("ExecuteAgent", func() {
		It("should return default response when no specific response is configured", func() {
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(ContainSubstring("Mock response"))
			Expect(response).To(ContainSubstring("test-agent"))
			Expect(response).To(ContainSubstring("test prompt"))
		})

		It("should return configured response for specific agent and prompt", func() {
			mockExecutor.SetResponse("default/test-agent", "test prompt", "Custom response")
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal("Custom response"))
		})

		It("should return configured error for specific agent and prompt", func() {
			testError := errors.New("test error")
			mockExecutor.SetError("default/test-agent", "test prompt", testError)
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(err).To(Equal(testError))
			Expect(response).To(BeEmpty())
		})

		It("should return default response when SetDefaultResponse is used", func() {
			mockExecutor.SetDefaultResponse("Default response")
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "any prompt", nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal("Default response"))
		})

		It("should return default error when SetDefaultError is used", func() {
			testError := errors.New("default error")
			mockExecutor.SetDefaultError(testError)
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "any prompt", nil, "")
			Expect(err).To(Equal(testError))
			Expect(response).To(BeEmpty())
		})

		It("should handle context data", func() {
			contextData := map[string]interface{}{
				"key": "value",
			}
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", contextData, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).NotTo(BeEmpty())
		})

		It("should handle agent without namespace", func() {
			agentCRD.Namespace = ""
			mockExecutor.SetResponse("test-agent", "test prompt", "Response for agent without namespace")
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal("Response for agent without namespace"))
		})
	})

	Describe("Call History", func() {
		It("should record all calls", func() {
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 1", nil, "")
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 2", nil, "")

			history := mockExecutor.GetCallHistory()
			Expect(history).To(HaveLen(2))
			Expect(history[0].Prompt).To(Equal("prompt 1"))
			Expect(history[1].Prompt).To(Equal("prompt 2"))
		})

		It("should record call details including context", func() {
			contextData := map[string]interface{}{
				"input": "test",
			}
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", contextData, "")

			history := mockExecutor.GetCallHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].AgentName).To(Equal("default/test-agent"))
			Expect(history[0].Prompt).To(Equal("test prompt"))
			Expect(history[0].Context).To(Equal(contextData))
		})

		It("should record errors in call history", func() {
			testError := errors.New("test error")
			mockExecutor.SetError("default/test-agent", "test prompt", testError)
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")

			history := mockExecutor.GetCallHistory()
			Expect(history).To(HaveLen(1))
			Expect(history[0].Error).To(Equal(testError))
		})

		It("should return correct call count", func() {
			Expect(mockExecutor.GetCallCount()).To(Equal(0))
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 1", nil, "")
			Expect(mockExecutor.GetCallCount()).To(Equal(1))
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 2", nil, "")
			Expect(mockExecutor.GetCallCount()).To(Equal(2))
		})

		It("should check if executor was called with specific parameters", func() {
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(mockExecutor.WasCalledWith("default/test-agent", "test prompt")).To(BeTrue())
			Expect(mockExecutor.WasCalledWith("default/test-agent", "other prompt")).To(BeFalse())
		})

		It("should return calls for specific agent", func() {
			agentCRD2 := &ottoflowv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-agent",
					Namespace: "default",
				},
			}

			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 1", nil, "")
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "prompt 2", nil, "")
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD2, "prompt 3", nil, "")

			calls := mockExecutor.GetCallsForAgent("default/test-agent")
			Expect(calls).To(HaveLen(2))
			Expect(calls[0].Prompt).To(Equal("prompt 1"))
			Expect(calls[1].Prompt).To(Equal("prompt 2"))
		})
	})

	Describe("Reset", func() {
		It("should clear all responses, errors, and call history", func() {
			mockExecutor.SetResponse("default/test-agent", "test prompt", "response")
			mockExecutor.SetError("default/test-agent", "error prompt", errors.New("error"))
			mockExecutor.SetDefaultResponse("default")
			mockExecutor.SetDefaultError(errors.New("default error"))
			_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")

			mockExecutor.Reset()

			Expect(mockExecutor.GetCallHistory()).To(BeEmpty())
			Expect(mockExecutor.GetCallCount()).To(Equal(0))
			// Should return generated response since everything is reset
			response, _, err := mockExecutor.ExecuteAgent(ctx, agentCRD, "test prompt", nil, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(ContainSubstring("Mock response"))
		})
	})

	Describe("Concurrent Access", func() {
		It("should be thread-safe", func() {
			done := make(chan bool)
			for i := 0; i < 10; i++ {
				go func(idx int) {
					defer func() { done <- true }()
					prompt := fmt.Sprintf("prompt-%d", idx)
					mockExecutor.SetResponse("default/test-agent", prompt, fmt.Sprintf("response-%d", idx))
					_, _, _ = mockExecutor.ExecuteAgent(ctx, agentCRD, prompt, nil, "")
				}(i)
			}

			for i := 0; i < 10; i++ {
				<-done
			}

			Expect(mockExecutor.GetCallCount()).To(Equal(10))
		})
	})
})
