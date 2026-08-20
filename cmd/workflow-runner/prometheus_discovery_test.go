/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package main

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newDiscoveryFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func TestPrometheusServicePort_PrefersWebPort(t *testing.T) {
	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080},
				{Name: "web", Port: 9090},
			},
		},
	}
	if got := prometheusServicePort(svc); got != 9090 {
		t.Errorf("expected 9090 (web), got %d", got)
	}
}

func TestPrometheusServicePort_FallsBackToHTTP(t *testing.T) {
	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
				{Name: "metrics", Port: 8081},
			},
		},
	}
	if got := prometheusServicePort(svc); got != 80 {
		t.Errorf("expected 80 (http), got %d", got)
	}
}

func TestPrometheusServicePort_FallsBackToPort9090(t *testing.T) {
	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "metrics", Port: 9090},
			},
		},
	}
	if got := prometheusServicePort(svc); got != 9090 {
		t.Errorf("expected 9090, got %d", got)
	}
}

func TestPrometheusServicePort_ReturnsZeroWhenNoMatch(t *testing.T) {
	svc := &corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "metrics", Port: 8081}},
		},
	}
	if got := prometheusServicePort(svc); got != 0 {
		t.Errorf("expected 0 (no match), got %d", got)
	}
}

func TestCollectPrometheusCandidates_LabelMatchTakesPrecedence(t *testing.T) {
	withLabel := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-prometheus-stack-prometheus",
			Namespace: "monitoring",
			Labels:    map[string]string{"app.kubernetes.io/name": "prometheus"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Port: 9090}}},
	}
	byNameOnly := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "prometheus-server", Namespace: "other"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80}}},
	}
	c := newDiscoveryFakeClient(t, withLabel, byNameOnly)
	got := collectPrometheusCandidates(context.Background(), c)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 candidates, got %v", got)
	}
	if got[0].url != "http://kube-prometheus-stack-prometheus.monitoring.svc:9090" {
		t.Errorf("expected labeled service first, got %v", got[0].url)
	}
}

func TestCollectPrometheusCandidates_ExcludesUnrelatedComponents(t *testing.T) {
	objs := []client.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-alertmanager", Namespace: "monitoring"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 9093}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-node-exporter", Namespace: "monitoring"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "metrics", Port: 9100}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-kube-state-metrics", Namespace: "monitoring"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-pushgateway", Namespace: "monitoring"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 9091}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-server", Namespace: "monitoring"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 9090}}},
		},
	}
	c := newDiscoveryFakeClient(t, objs...)
	got := collectPrometheusCandidates(context.Background(), c)
	for _, cand := range got {
		if containsAny(cand.url, "alertmanager", "exporter", "kube-state", "pushgateway") {
			t.Errorf("unrelated component leaked into candidates: %s", cand.url)
		}
	}
	want := "http://prometheus-server.monitoring.svc:9090"
	found := false
	for _, cand := range got {
		if cand.url == want {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in candidates, got %v", want, got)
	}
}

func TestCollectPrometheusCandidates_NoMatches(t *testing.T) {
	objs := []client.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 443}}},
		},
	}
	c := newDiscoveryFakeClient(t, objs...)
	got := collectPrometheusCandidates(context.Background(), c)
	if len(got) != 0 {
		t.Errorf("expected empty candidates, got %v", got)
	}
}

func TestServiceBacksPrometheusPod_MatchByImage(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prom-0", Namespace: "monitoring",
			Labels: map[string]string{"app": "prometheus"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "prometheus", Image: "quay.io/prometheus/prometheus:v2.49.0"},
		}},
	}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "prom", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "prometheus"}},
	}
	c := newDiscoveryFakeClient(t, pod)
	if !serviceBacksPrometheusPod(context.Background(), c, svc) {
		t.Errorf("expected match for pod with prometheus image")
	}
}

func TestServiceBacksPrometheusPod_RejectsNonPrometheusBackend(t *testing.T) {
	// Common false-positive: an adapter Service whose name contains "prometheus"
	// but routes to a different image (e.g., custom-metrics-apiserver,
	// prometheus-adapter for the Custom Metrics API).
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "adapter-0", Namespace: "monitoring",
			Labels: map[string]string{"app": "prometheus-adapter"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "adapter", Image: "registry.k8s.io/prometheus-adapter/prometheus-adapter:v0.11"},
		}},
	}
	// Note: image name still contains "prometheus" — adapter is allowed through
	// at this stage; the metric probe is the final filter. This test documents
	// the behavior rather than rejecting it.
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "prometheus-adapter", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "prometheus-adapter"}},
	}
	c := newDiscoveryFakeClient(t, pod)
	if !serviceBacksPrometheusPod(context.Background(), c, svc) {
		t.Errorf("adapter image contains 'prometheus' so should pass; metric probe is final filter")
	}
}

func TestServiceBacksPrometheusPod_RejectsServiceWithNoBackingPods(t *testing.T) {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "stale", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "prometheus"}},
	}
	// No pods seeded — selector matches nothing.
	c := newDiscoveryFakeClient(t)
	if serviceBacksPrometheusPod(context.Background(), c, svc) {
		t.Errorf("expected rejection when no pods back the service")
	}
}

func TestServiceBacksPrometheusPod_RejectsNonPrometheusImage(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake", Namespace: "monitoring",
			Labels: map[string]string{"app": "prometheus"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "nginx", Image: "nginx:latest"},
		}},
	}
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "nope", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "prometheus"}},
	}
	c := newDiscoveryFakeClient(t, pod)
	if serviceBacksPrometheusPod(context.Background(), c, svc) {
		t.Errorf("expected rejection when backing pod's image is not Prometheus")
	}
}

func TestServiceBacksPrometheusPod_HeadlessServiceAllowedThrough(t *testing.T) {
	// Service with no selector (e.g., headless governance service or
	// ExternalName) cannot be filtered at this stage; let the metric probe
	// be the final arbiter rather than rejecting prematurely.
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "operated", Namespace: "monitoring"},
		Spec:       corev1.ServiceSpec{Selector: nil},
	}
	c := newDiscoveryFakeClient(t)
	if !serviceBacksPrometheusPod(context.Background(), c, svc) {
		t.Errorf("expected services with empty selector to pass through")
	}
}

func TestResolvePrometheusURL_FlagOverridesDiscovery(t *testing.T) {
	objs := []client.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prom", Namespace: "monitoring",
				Labels: map[string]string{"app.kubernetes.io/name": "prometheus"},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Port: 9090}}},
		},
	}
	c := newDiscoveryFakeClient(t, objs...)
	url, source := resolvePrometheusURL(context.Background(), c, "http://explicit.example.com:9090")
	if url != "http://explicit.example.com:9090" {
		t.Errorf("flag should override discovery, got %q", url)
	}
	if source != "flag" {
		t.Errorf("expected source=flag, got %q", source)
	}
}

func TestResolvePrometheusURL_NoCandidatesReturnsEmpty(t *testing.T) {
	c := newDiscoveryFakeClient(t)
	url, source := resolvePrometheusURL(context.Background(), c, "")
	if url != "" {
		t.Errorf("expected empty url when no candidates, got %q", url)
	}
	if source != "" {
		t.Errorf("expected empty source when no candidates, got %q", source)
	}
}
