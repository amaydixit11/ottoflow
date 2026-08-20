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

// mockMCPClientForBuildSessionTools implements MCPClient for buildSessionToolsFromMCP tests.
type mockMCPClientForBuildSessionTools struct {
	tools []MCPToolMeta
	err   error
}

func (m *mockMCPClientForBuildSessionTools) ListTools(ctx context.Context) ([]MCPToolMeta, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tools, nil
}

func (m *mockMCPClientForBuildSessionTools) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (interface{}, error) {
	return nil, nil
}

func (m *mockMCPClientForBuildSessionTools) Close() error { return nil }

// mockMCPProviderForBuildSessionTools returns a fixed client.
type mockMCPProviderForBuildSessionTools struct {
	client MCPClient
	err    error
}

func (m *mockMCPProviderForBuildSessionTools) GetClient(ctx context.Context, serverName string, namespace string) (MCPClient, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.client, nil
}

var _ = Describe("buildSessionToolsFromMCP", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns nil when provider is nil", func() {
		tools, err := buildSessionToolsFromMCP(ctx, []string{"s1:tool1"}, "default", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(BeNil())
	})

	It("returns nil when mcpTools is empty", func() {
		provider := &mockMCPProviderForBuildSessionTools{client: &mockMCPClientForBuildSessionTools{}}
		tools, err := buildSessionToolsFromMCP(ctx, nil, "default", provider)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(BeNil())
	})

	It("returns nil when mcpTools only has empty/whitespace entries", func() {
		provider := &mockMCPProviderForBuildSessionTools{client: &mockMCPClientForBuildSessionTools{}}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"", "  ", "\t"}, "default", provider)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(BeNil())
	})

	It("builds session tools with unique keys (server:tool)", func() {
		mockClient := &mockMCPClientForBuildSessionTools{
			tools: []MCPToolMeta{
				{Name: "tool1", Description: "First tool", InputSchema: nil},
				{Name: "tool2", Description: "Second tool", InputSchema: nil},
			},
		}
		provider := &mockMCPProviderForBuildSessionTools{client: mockClient}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"myserver:tool1", "myserver:tool2"}, "default", provider)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(2))
		Expect(tools).To(HaveKey("myserver:tool1"))
		Expect(tools).To(HaveKey("myserver:tool2"))
		Expect(tools["myserver:tool1"].Name()).To(Equal("myserver:tool1"))
		Expect(tools["myserver:tool2"].Description()).To(Equal("Second tool"))
	})

	It("filters to only requested tools", func() {
		mockClient := &mockMCPClientForBuildSessionTools{
			tools: []MCPToolMeta{
				{Name: "tool1", Description: "First", InputSchema: nil},
				{Name: "tool2", Description: "Second", InputSchema: nil},
				{Name: "tool3", Description: "Third", InputSchema: nil},
			},
		}
		provider := &mockMCPProviderForBuildSessionTools{client: mockClient}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"s:tool2"}, "ns", provider)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveLen(1))
		Expect(tools).To(HaveKey("s:tool2"))
	})

	It("returns error when GetClient fails", func() {
		provider := &mockMCPProviderForBuildSessionTools{err: errors.New("no client")}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"s:tool1"}, "default", provider)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get MCP client"))
		Expect(tools).To(BeNil())
	})

	It("returns error when ListTools fails", func() {
		mockClient := &mockMCPClientForBuildSessionTools{err: errors.New("list failed")}
		provider := &mockMCPProviderForBuildSessionTools{client: mockClient}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"s:tool1"}, "default", provider)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("list tools"))
		Expect(tools).To(BeNil())
	})

	It("returned tool implements Tool and Run invokes client", func() {
		mockClient := &mockMCPClientForBuildSessionTools{
			tools: []MCPToolMeta{
				{Name: "echo", Description: "Echo tool", InputSchema: nil},
			},
		}
		provider := &mockMCPProviderForBuildSessionTools{client: mockClient}
		tools, err := buildSessionToolsFromMCP(ctx, []string{"svc:echo"}, "default", provider)
		Expect(err).NotTo(HaveOccurred())
		Expect(tools).To(HaveKey("svc:echo"))
		tool := tools["svc:echo"]
		Expect(tool.Name()).To(Equal("svc:echo"))
		Expect(tool.Description()).To(Equal("Echo tool"))
		Expect(tool.FunctionDefinition()).NotTo(BeNil())
		Expect(tool.FunctionDefinition().Name).To(Equal("svc:echo"))
		interactive, err := tool.IsInteractive(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(interactive).To(BeFalse())
		Expect(tool.CheckModifiesResource(nil)).To(Equal("unknown"))
		result, err := tool.Run(ctx, map[string]any{"msg": "hi"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil()) // our mock CallTool returns nil
	})

	It("Run returns error when tool has nil client", func() {
		tool := &ottoflowMCPTool{serverName: "s", toolName: "t", client: nil}
		_, err := tool.Run(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("MCP client not set"))
	})
})

var _ = Describe("DefaultAgentExecutor ExecuteAgent", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns error when createLLMClient fails (unknown provider)", func() {
		exec := NewDefaultAgentExecutor(nil)
		agentCRD := &ottoflowv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Spec:       ottoflowv1alpha1.AgentSpec{ModelProvider: "unknown-provider-that-does-not-exist"},
		}
		_, _, err := exec.ExecuteAgent(ctx, agentCRD, "prompt", nil, "default")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown provider"))
	})
})
