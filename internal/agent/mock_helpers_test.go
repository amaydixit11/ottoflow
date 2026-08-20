/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("MockAgentHelper", func() {
	var (
		ctx      context.Context
		exec     *MockAgentExecutor
		helper   *MockAgentHelper
		agentCRD *ottoflowv1alpha1.Agent
	)

	BeforeEach(func() {
		ctx = context.Background()
		exec = NewMockAgentExecutor()
		helper = NewMockAgentHelper(exec)
		agentCRD = &ottoflowv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "my-agent", Namespace: "default"},
		}
	})

	It("SetSuccessResponse and GetCallHistory", func() {
		helper.SetSuccessResponse("default/my-agent", "prompt", "ok")
		_, _, _ = exec.ExecuteAgent(ctx, agentCRD, "prompt", nil, "")
		Expect(helper.GetCallHistory()).To(HaveLen(1))
		Expect(helper.GetCallCount()).To(Equal(1))
	})

	It("SetErrorResponse", func() {
		helper.SetErrorResponse("default/my-agent", "prompt", errors.New("fail"))
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "prompt", nil, "")
		Expect(err).To(HaveOccurred())
	})

	It("SetJSONResponse marshals and sets response", func() {
		err := helper.SetJSONResponse("default/my-agent", "p", map[string]string{"k": "v"})
		Expect(err).NotTo(HaveOccurred())
		resp, _, _ := exec.ExecuteAgent(ctx, agentCRD, "p", nil, "")
		Expect(resp).To(ContainSubstring("k"))
		Expect(resp).To(ContainSubstring("v"))
	})

	It("SetDefaultSuccessResponse and SetDefaultErrorResponse", func() {
		helper.SetDefaultSuccessResponse("default-ok")
		resp, _, _ := exec.ExecuteAgent(ctx, agentCRD, "any", nil, "")
		Expect(resp).To(Equal("default-ok"))
		helper.SetDefaultErrorResponse(errors.New("default-err"))
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "any2", nil, "")
		Expect(err).To(HaveOccurred())
	})

	It("Reset clears state", func() {
		helper.SetSuccessResponse("default/my-agent", "p", "x")
		_, _, _ = exec.ExecuteAgent(ctx, agentCRD, "p", nil, "")
		helper.Reset()
		Expect(helper.GetCallCount()).To(Equal(0))
		Expect(helper.GetCallHistory()).To(BeEmpty())
	})

	It("WasCalledWith and GetCallsForAgent", func() {
		helper.SetSuccessResponse("default/my-agent", "q", "a")
		_, _, _ = exec.ExecuteAgent(ctx, agentCRD, "q", nil, "")
		Expect(helper.WasCalledWith("default/my-agent", "q")).To(BeTrue())
		Expect(helper.WasCalledWith("default/my-agent", "other")).To(BeFalse())
		calls := helper.GetCallsForAgent("default/my-agent")
		Expect(calls).To(HaveLen(1))
		Expect(calls[0].Prompt).To(Equal("q"))
	})

	It("SetScenario_SuccessfulAnalysis", func() {
		helper.SetScenario_SuccessfulAnalysis("default/my-agent")
		resp, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(ContainSubstring("analysis"))
	})

	It("SetScenario_FailedExecution with nil error uses default message", func() {
		helper.SetScenario_FailedExecution("default/my-agent", nil)
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("agent execution failed"))
	})

	It("SetScenario_FailedExecution", func() {
		helper.SetScenario_FailedExecution("default/my-agent", errors.New("failed"))
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).To(HaveOccurred())
	})

	It("SetScenario_Timeout", func() {
		helper.SetScenario_Timeout("default/my-agent")
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("timeout"))
	})

	It("SetScenario_EmptyResponse", func() {
		helper.SetScenario_EmptyResponse("default/my-agent")
		resp, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeEmpty())
	})

	It("SetScenario_LargeResponse", func() {
		helper.SetScenario_LargeResponse("default/my-agent", 2) // 2KB
		resp, _, err := exec.ExecuteAgent(ctx, agentCRD, "", nil, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(len(resp)).To(Equal(2 * 1024))
	})
})
