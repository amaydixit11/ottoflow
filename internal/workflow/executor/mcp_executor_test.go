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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/agent"
)

// mockMCPClient implements agent.MCPClient for tests
type mockMCPClient struct {
	callToolResult interface{}
	callToolErr    error
}

func (m *mockMCPClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (interface{}, error) {
	if m.callToolErr != nil {
		return nil, m.callToolErr
	}
	return m.callToolResult, nil
}

func (m *mockMCPClient) ListTools(ctx context.Context) ([]agent.MCPToolMeta, error) {
	return nil, nil
}

func (m *mockMCPClient) Close() error { return nil }

// mockMCPManager returns a fixed MCP client
type mockMCPManager struct {
	client agent.MCPClient
}

func (m *mockMCPManager) GetClient(ctx context.Context, serverName string, namespace string) (agent.MCPClient, error) {
	return m.client, nil
}

func (m *mockMCPManager) Close() error { return nil }

var _ = Describe("MCPManager", func() {
	It("NewMCPManager and GetClient can be called", func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		manager, err := NewMCPManager(k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(manager).NotTo(BeNil())
		defer manager.Close() //nolint:errcheck
		ctx := context.Background()
		_, err = manager.GetClient(ctx, "test-server", "default")
		// May succeed or fail depending on MCPServer CRD; we only care that the path is exercised
		_ = err
	})
})

var _ = Describe("MCP Tool Call Step Execution", func() {
	var (
		ctx              context.Context
		k8sClient        client.Client
		scheme           *runtime.Scheme
		workflowRun      *ottoflowv1alpha1.WorkflowRun
		workflowExecutor *WorkflowExecutor
		mockMCP          *mockMCPManager
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		workflowRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default"},
			Spec:       ottoflowv1alpha1.WorkflowRunSpec{WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "test-wf"}},
		}
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("should execute MCP tool call and write result to step outputs", func() {
		mockMCP = &mockMCPManager{
			client: &mockMCPClient{
				callToolResult: map[string]interface{}{"output": "hello from tool", "count": float64(42)},
			},
		}
		var err error
		workflowExecutor, err = NewWorkflowExecutorWithAgentExecutorAndMCPManager(
			k8sClient, nil, nil, nil, workflowRun, agent.NewMockAgentExecutor(), mockMCP, false, 0, 5, nil)
		Expect(err).NotTo(HaveOccurred())

		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "mcpStep",
						MCPToolCall: &ottoflowv1alpha1.StepMCPToolCall{
							Server: "my-server",
							Tool:   "echo",
							Arguments: map[string]string{
								"message": `"hello"`,
							},
						},
						Outputs: []ottoflowv1alpha1.Output{
							{Name: "out", Expression: "toolResult.output"},
							{Name: "n", Expression: "int(toolResult.count)"},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		err = workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())
		Expect(workflowRun.Status.Phase).To(Equal(ottoflowv1alpha1.WorkflowRunPhaseSucceeded))
		Expect(workflowRun.Status.StepStatuses["mcpStep"].Phase).To(Equal(ottoflowv1alpha1.StepPhaseSucceeded))

		ctxData, err := workflowExecutor.GetContextManager().ReadContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		variables := ctxData["variables"].(map[string]interface{})
		Expect(variables["out"]).To(Equal("hello from tool"))
		Expect(variables["n"]).To(Equal(int64(42)))
	})

	It("should store raw tool result when step has no output definitions", func() {
		mockMCP = &mockMCPManager{
			client: &mockMCPClient{
				callToolResult: "raw string result",
			},
		}
		var err error
		workflowExecutor, err = NewWorkflowExecutorWithAgentExecutorAndMCPManager(
			k8sClient, nil, nil, nil, workflowRun, agent.NewMockAgentExecutor(), mockMCP, false, 0, 5, nil)
		Expect(err).NotTo(HaveOccurred())

		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "mcpStep",
						MCPToolCall: &ottoflowv1alpha1.StepMCPToolCall{
							Server: "s", Tool: "t",
							Arguments: map[string]string{},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		err = workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).NotTo(HaveOccurred())
		ctxData, _ := workflowExecutor.GetContextManager().ReadContext(ctx)
		variables := ctxData["variables"].(map[string]interface{})
		Expect(variables).To(HaveKey("result"))
		Expect(variables["result"]).To(Equal("raw string result"))
	})

	It("should fail when MCP client returns error", func() {
		mockMCP = &mockMCPManager{
			client: &mockMCPClient{callToolErr: context.DeadlineExceeded},
		}
		var err error
		workflowExecutor, err = NewWorkflowExecutorWithAgentExecutorAndMCPManager(
			k8sClient, nil, nil, nil, workflowRun, agent.NewMockAgentExecutor(), mockMCP, false, 0, 5, nil)
		Expect(err).NotTo(HaveOccurred())

		workflow := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "test-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{
					{
						Name: "mcpStep",
						MCPToolCall: &ottoflowv1alpha1.StepMCPToolCall{
							Server: "s", Tool: "t",
							Arguments: map[string]string{},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

		err = workflowExecutor.ExecuteWorkflow(ctx, workflow, workflowRun)
		Expect(err).To(HaveOccurred())
		Expect(workflowRun.Status.StepStatuses["mcpStep"].Phase).To(Equal(ottoflowv1alpha1.StepPhaseFailed))
	})
})
