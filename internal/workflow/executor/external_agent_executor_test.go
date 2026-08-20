/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("executeExternalAgentStep", func() {
	var (
		workflowRun      *ottoflowv1alpha1.WorkflowRun
		workflowExecutor *WorkflowExecutor
	)

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))

		workflowRun = &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowRunSpec{
				WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "test-workflow"},
				InputValues: map[string]string{"topic": "kubernetes"},
			},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		var err error
		workflowExecutor, err = NewWorkflowExecutorWithAgentExecutor(
			k8sClient, nil, nil, nil,
			workflowRun, nil, false, 0, 5, nil,
		)
		Expect(err).NotTo(HaveOccurred())

		// Initialize context so prompt CEL expressions can reference inputs
		err = workflowExecutor.contextManager.InitializeContext(
			context.Background(),
			&ottoflowv1alpha1.Workflow{
				Spec: ottoflowv1alpha1.WorkflowSpec{
					Inputs: []ottoflowv1alpha1.Input{{Name: "topic"}},
				},
			},
			workflowRun.Spec.InputValues,
		)
		Expect(err).NotTo(HaveOccurred())
	})

	It("executes an externalAgentRef step and exposes a2aResult in CEL outputs", func() {
		// Serve a minimal A2A agent: card + completed task
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/.well-known/agent-card.json":
				_, _ = w.Write([]byte(`{"name":"mock-agent","url":"` + "https://" + r.Host + `"}`))
			default:
				_ = json.NewEncoder(w).Encode(a2aJSONRPCResponse{
					JSONRPC: "2.0",
					Result: json.RawMessage(`{
						"kind":"task",
						"id":"task-1",
						"status":{"state":"completed"},
						"artifacts":[{"name":"result","parts":[{"kind":"text","text":"k8s analysis done"}]}]
					}`),
					ID: 1,
				})
			}
		}))
		defer srv.Close()

		// Inject TLS test client (uses srv's own cert pool)
		testA2AHTTPClient = srv.Client()
		defer func() { testA2AHTTPClient = nil }()

		step := ottoflowv1alpha1.Step{
			Name: "callAgent",
			ExternalAgentRef: &ottoflowv1alpha1.StepExternalAgentRef{
				URL:    srv.URL,
				Prompt: `"Analyze: " + inputs.topic`,
			},
			Outputs: []ottoflowv1alpha1.Output{
				{
					Name:       "analysis",
					Expression: `a2aResult.artifacts[0].parts[0].text`,
				},
			},
		}

		outputs, err := workflowExecutor.executeExternalAgentStep(context.Background(), workflowRun, step)
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs).To(HaveKey("analysis"))
		Expect(outputs["analysis"]).To(Equal("k8s analysis done"))
	})

	It("exposes a2aResult directly when no outputs defined", func() {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/.well-known/agent-card.json" {
				_, _ = w.Write([]byte(`{"name":"mock-agent","url":"` + "https://" + r.Host + `"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(a2aJSONRPCResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`{"kind":"task","id":"task-2","status":{"state":"completed"},"artifacts":[]}`),
				ID:      1,
			})
		}))
		defer srv.Close()

		testA2AHTTPClient = srv.Client()
		defer func() { testA2AHTTPClient = nil }()

		step := ottoflowv1alpha1.Step{
			Name: "callAgentNoOutputs",
			ExternalAgentRef: &ottoflowv1alpha1.StepExternalAgentRef{
				URL:    srv.URL,
				Prompt: `"hello"`,
			},
		}

		outputs, err := workflowExecutor.executeExternalAgentStep(context.Background(), workflowRun, step)
		Expect(err).NotTo(HaveOccurred())
		Expect(outputs).To(HaveKey("a2aResult"))
	})

	It("returns error when prompt CEL expression is invalid", func() {
		testA2AHTTPClient = &http.Client{} // won't be reached
		defer func() { testA2AHTTPClient = nil }()

		step := ottoflowv1alpha1.Step{
			Name: "badPrompt",
			ExternalAgentRef: &ottoflowv1alpha1.StepExternalAgentRef{
				URL:    "https://agent.example.com",
				Prompt: `[[[invalid cel`,
			},
		}

		_, err := workflowExecutor.executeExternalAgentStep(context.Background(), workflowRun, step)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("prompt"))
	})
})
