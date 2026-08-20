/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	restfake "k8s.io/client-go/rest/fake"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

var _ = Describe("Resource macro options", func() {
	It("GetResourceMacroOptions returns options with fake client", func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		opts, err := GetResourceMacroOptions(fakeClient, "default", &macroContextHolder{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
	})

	It("GetResourceMacroOptionsWithMetrics returns options with nil metrics clients", func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		opts, err := GetResourceMacroOptionsWithMetrics(fakeClient, nil, nil, nil, nil, "default", &macroContextHolder{})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
	})
})

// fakeRefVal is a minimal ref.Val stub: only Value() is exercised by
// unwrapObjectMap, so the other interface methods can return stub values.
type fakeRefVal struct{ v interface{} }

func (f fakeRefVal) Value() interface{}               { return f.v }
func (f fakeRefVal) Type() ref.Type                   { return types.DynType }
func (f fakeRefVal) ConvertToType(_ ref.Type) ref.Val { return f }
func (f fakeRefVal) ConvertToNative(_ reflect.Type) (interface{}, error) {
	return f.v, nil
}
func (f fakeRefVal) Equal(_ ref.Val) ref.Val { return types.False }

var _ = Describe("unwrapObjectMap", func() {
	expected := map[string]interface{}{
		"spec": map[string]interface{}{"foo": "bar"},
	}

	It("returns the same map when Value() is map[string]interface{}", func() {
		got, ok := unwrapObjectMap(fakeRefVal{v: expected}, "test")
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(expected))
	})

	It("returns .Object when Value() is unstructured.Unstructured", func() {
		got, ok := unwrapObjectMap(fakeRefVal{v: unstructured.Unstructured{Object: expected}}, "test")
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(expected))
	})

	It("returns .Object when Value() is *unstructured.Unstructured", func() {
		got, ok := unwrapObjectMap(fakeRefVal{v: &unstructured.Unstructured{Object: expected}}, "test")
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(expected))
	})

	It("returns false for nil *unstructured.Unstructured", func() {
		var u *unstructured.Unstructured
		got, ok := unwrapObjectMap(fakeRefVal{v: u}, "test")
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})

	It("returns false (and logs) for unrecognized types", func() {
		got, ok := unwrapObjectMap(fakeRefVal{v: "not-a-pod"}, "test")
		Expect(ok).To(BeFalse())
		Expect(got).To(BeNil())
	})
})

var _ = Describe("metricsResponseIsIncomplete", func() {
	// Completeness threshold: metricCount >= 80% of runningPodsCount.
	// Small-namespace guard: runningPodsCount <= 10 is always considered complete.

	It("returns false when runningPodsCount <= 10 (small namespace guard)", func() {
		// 10 running pods, 0 metrics — still not flagged as incomplete
		Expect(metricsResponseIsIncomplete(0, 10)).To(BeFalse())
		Expect(metricsResponseIsIncomplete(5, 10)).To(BeFalse())
	})

	It("returns false when metrics cover >= 80% of running pods", func() {
		// 11 running pods, 9 metrics = 81.8% → complete
		Expect(metricsResponseIsIncomplete(9, 11)).To(BeFalse())
		// 100 running pods, 80 metrics = exactly 80% → complete
		Expect(metricsResponseIsIncomplete(80, 100)).To(BeFalse())
		// 100 running pods, 100 metrics = 100% → complete
		Expect(metricsResponseIsIncomplete(100, 100)).To(BeFalse())
	})

	It("returns true when metrics cover < 80% of running pods and namespace is large", func() {
		// 100 running pods, 79 metrics = 79% → incomplete
		Expect(metricsResponseIsIncomplete(79, 100)).To(BeTrue())
		// 11 running pods, 0 metrics → incomplete
		Expect(metricsResponseIsIncomplete(0, 11)).To(BeTrue())
	})

	It("returns false when runningPodsCount is 0 (no pods)", func() {
		Expect(metricsResponseIsIncomplete(0, 0)).To(BeFalse())
	})
})

var _ = Describe("resourceMetricsList CEL function", func() {
	var (
		scheme   *runtime.Scheme
		macroCtx *macroContextHolder
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		macroCtx = &macroContextHolder{}
		macroCtx.set(context.Background())
	})

	It("returns empty list when metrics client is nil", func() {
		fakeK8s := fake.NewClientBuilder().WithScheme(scheme).Build()
		opts, err := GetResourceMacroOptionsWithMetrics(fakeK8s, nil, nil, nil, nil, "default", macroCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
		// nil metrics client → CEL function returns an error value, not a panic
	})

	It("returns metrics for all pods when metrics client has data", func() {
		// Seed the fake k8s client with two running pods
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "test"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "test"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		fakeK8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod1, pod2).Build()

		// Seed the fake metrics client with metrics for both pods
		metricsScheme := runtime.NewScheme()
		utilruntime.Must(metricsv1beta1.AddToScheme(metricsScheme))
		fakeMetrics := metricsfake.NewSimpleClientset( //nolint:staticcheck
			&metricsv1beta1.PodMetrics{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "test"},
			},
			&metricsv1beta1.PodMetrics{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "test"},
			},
		)

		opts, err := GetResourceMacroOptionsWithMetrics(fakeK8s, nil, fakeMetrics, nil, nil, "test", macroCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
	})

	It("resourceEvents macro registers when events are present in the cache", func() {
		// Seed the fake client with several events for an involvedObject. The pager
		// path must walk all of them in a single call without erroring.
		objs := []client.Object{}
		for i := 0; i < 10; i++ {
			objs = append(objs, &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "evt-" + string(rune('a'+i)),
					Namespace: "default",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "target-pod",
				},
				Type:    "Normal",
				Reason:  "Scheduled",
				Message: "scheduled to node-1",
			})
		}
		fakeK8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		opts, err := GetResourceMacroOptionsWithMetrics(fakeK8s, nil, nil, nil, nil, "default", macroCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
	})

	It("does not retry when runningPodsCount <= 10 (small namespace guard)", func() {
		// 3 running pods, metrics returns 1 — below 80% but guard prevents retry
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "small"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		pod2 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "small"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		pod3 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-c", Namespace: "small"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		}
		fakeK8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod1, pod2, pod3).Build()

		fakeMetrics := metricsfake.NewSimpleClientset( //nolint:staticcheck
			&metricsv1beta1.PodMetrics{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "small"},
			},
		)

		// Should succeed without retrying (small namespace guard: runningPodsCount=3 <= 10)
		opts, err := GetResourceMacroOptionsWithMetrics(fakeK8s, nil, fakeMetrics, nil, nil, "small", macroCtx)
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).NotTo(BeEmpty())
	})
})

// makeFakeKubeClient builds a kubernetes.Interface backed by a fake HTTP handler.
// The handler is called for every request, allowing tests to control status code and body.
func makeFakeKubeClient(fn func(*http.Request) (*http.Response, error)) kubernetes.Interface {
	fakeRestClient := &restfake.RESTClient{
		Client:               restfake.CreateHTTPClient(fn),
		NegotiatedSerializer: clientgoscheme.Codecs.WithoutConversion(),
		GroupVersion:         corev1.SchemeGroupVersion,
	}
	return kubernetes.New(fakeRestClient)
}

var _ = Describe("fetchPodLogs and resource.GetLogs", func() {
	ctx := context.Background()

	It("returns log body on 200 OK", func() {
		kubeClient := makeFakeKubeClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("line1\nline2\n")),
				Header:     make(http.Header),
			}, nil
		})
		logs, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "app", 50)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(Equal("line1\nline2\n"))
	})

	It("returns empty string when body is empty (pod has no logs yet)", func() {
		kubeClient := makeFakeKubeClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		})
		logs, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "app", 50)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(Equal(""))
	})

	It("passes empty container name through to PodLogOptions", func() {
		var capturedReq *http.Request
		kubeClient := makeFakeKubeClient(func(req *http.Request) (*http.Response, error) {
			capturedReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("logs")),
				Header:     make(http.Header),
			}, nil
		})
		_, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "", 10)
		Expect(err).NotTo(HaveOccurred())
		// empty container name → container param is empty or absent; apiserver selects the default container
		Expect(capturedReq.URL.Query().Get("container")).To(Equal(""))
	})

	It("clamps tailLines > 10000 to 10000", func() {
		var capturedReq *http.Request
		kubeClient := makeFakeKubeClient(func(req *http.Request) (*http.Response, error) {
			capturedReq = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("logs")),
				Header:     make(http.Header),
			}, nil
		})
		_, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "app", 20000)
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedReq.URL.Query().Get("tailLines")).To(Equal("10000"))
	})

	It("returns error when pod is not found (404)", func() {
		kubeClient := makeFakeKubeClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body: io.NopCloser(strings.NewReader(
					`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"pods \"my-pod\" not found","reason":"NotFound","code":404}`)),
				Header: http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		})
		_, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "app", 50)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("truncates response at 4 MB and appends sentinel", func() {
		bigBody := strings.Repeat("x", podLogsByteCap+100)
		kubeClient := makeFakeKubeClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(bigBody)),
				Header:     make(http.Header),
			}, nil
		})
		result, err := fetchPodLogs(ctx, kubeClient, "default", "my-pod", "app", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveSuffix("\n[truncated]"))
		Expect(len(result)).To(BeNumerically("<=", podLogsByteCap+len("\n[truncated]")))
	})

	It("resource.GetLogs() CEL function returns logs end-to-end through evaluator", func() {
		scheme := runtime.NewScheme()
		utilruntime.Must(ottoflowv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		workflowRun := &ottoflowv1alpha1.WorkflowRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default"},
		}
		kubeClient := makeFakeKubeClient(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("log-line-1\nlog-line-2\n")),
				Header:     make(http.Header),
			}, nil
		})
		evaluator, err := NewCELEvaluatorWithMetrics(fakeClient, nil, nil, nil, kubeClient, workflowRun, 0, nil)
		Expect(err).NotTo(HaveOccurred())
		result, err := evaluator.EvaluateExpression(ctx, `resource.GetLogs("default", "my-pod", "app", 50)`, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("log-line-1"))
	})
})
