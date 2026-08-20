/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// signRequest computes the X-OttoFlow-Signature for the given request components.
func signRequest(secret []byte, ts, path string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("v1:"))
	mac.Write([]byte(ts))
	mac.Write([]byte(":"))
	mac.Write([]byte(path))
	mac.Write([]byte(":"))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// makeWebhookRequest builds a signed POST to /webhooks/{ns}/{name} and returns the response.
func makeWebhookRequest(ts *httptest.Server, secret []byte, ns, name string, body []byte) *http.Response { //nolint:unparam
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	path := fmt.Sprintf("/webhooks/%s/%s", ns, name)
	sig := signRequest(secret, timestamp, path, body)

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OttoFlow-Timestamp", timestamp)
	req.Header.Set("X-OttoFlow-Signature", sig)

	resp, err := ts.Client().Do(req)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { _ = resp.Body.Close() })
	return resp
}

// hmacSecret is 32 bytes of test secret material.
var hmacSecret = bytes.Repeat([]byte("s"), 32)

var _ = Describe("Webhook Trigger", func() {
	var (
		ctx            context.Context
		cancelCtx      context.CancelFunc
		triggerManager *TriggerManager
		scheduler      *Scheduler
		webhookSrv     *WebhookServer
		httpSrv        *httptest.Server
		wf             *ottoflowv1alpha1.Workflow
		hmacSecretObj  *corev1.Secret
	)

	BeforeEach(func() {
		ctx, cancelCtx = context.WithCancel(context.Background())
		scheduler = NewScheduler(k8sClient, ctrl.Log)
		go func() {
			defer GinkgoRecover()
			_ = scheduler.Start(ctx)
		}()

		var err error
		triggerManager, err = NewTriggerManagerWithConfig(k8sClient, k8sClient.Scheme(), cfg, scheduler)
		Expect(err).NotTo(HaveOccurred())

		// Create the HMAC secret.
		hmacSecretObj = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "wh-secret", Namespace: "default"},
			Data:       map[string][]byte{"hmac-key": hmacSecret},
		}
		Expect(k8sClient.Create(ctx, hmacSecretObj)).To(Succeed())

		// Create a Workflow with a webhook trigger.
		wf = &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wh-workflow", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef: ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, wf)).To(Succeed())

		// Spin up WebhookServer backed by httptest (tests the handler directly, no port binding).
		webhookSrv = NewWebhookServer(":0", ctrl.Log.WithName("wh-test"), k8sClient, triggerManager)
		httpSrv = httptest.NewServer(http.HandlerFunc(webhookSrv.handleWebhook))
	})

	AfterEach(func() {
		httpSrv.Close()
		// Use a fresh context for cleanup so canceling the test context does not block deletes.
		cleanCtx := context.Background()
		var list ottoflowv1alpha1.WorkflowRunList
		_ = k8sClient.List(cleanCtx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "wh-workflow"})
		for i := range list.Items {
			_ = k8sClient.Delete(cleanCtx, &list.Items[i])
		}
		_ = k8sClient.Delete(cleanCtx, wf)
		_ = k8sClient.Delete(cleanCtx, hmacSecretObj)
		cancelCtx()
	})

	It("should create WorkflowRun on valid webhook request", func() {
		body := []byte(`{"action":"push"}`)
		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "wh-workflow", body)
		Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

		var result WebhookResponse
		Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
		Expect(result.RunName).NotTo(BeEmpty())
		Expect(result.Status).To(Equal("Pending"))

		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{
				"ottoflow.nirmata.io/workflow": "wh-workflow",
				"ottoflow.nirmata.io/trigger":  "webhook",
			})).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].Status.Trigger).NotTo(BeNil())
		Expect(list.Items[0].Status.Trigger.Type).To(Equal("Webhook"))
	})

	It("should return 401 on invalid HMAC signature", func() {
		wrongSecret := bytes.Repeat([]byte("x"), 32)
		resp := makeWebhookRequest(httpSrv, wrongSecret, "default", "wh-workflow", []byte(`{}`))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	It("should return 401 on stale timestamp", func() {
		body := []byte(`{}`)
		staleTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		path := "/webhooks/default/wh-workflow"
		sig := signRequest(hmacSecret, staleTS, path, body)

		req, err := http.NewRequest(http.MethodPost, httpSrv.URL+path, bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-OttoFlow-Timestamp", staleTS)
		req.Header.Set("X-OttoFlow-Signature", sig)

		resp, err := httpSrv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = resp.Body.Close() })
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	It("should return 401 for unknown workflow — not 404", func() {
		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "nonexistent-workflow", []byte(`{}`))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	It("should return 401 for workflow with no webhook trigger — not 404", func() {
		noTriggerWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "no-wh-trigger", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, noTriggerWF)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, noTriggerWF) }()

		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "no-wh-trigger", []byte(`{}`))
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	It("should return 200 (no run) when celFilter is false", func() {
		celWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "cel-filter-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef: ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						CELFilter: `object.severity == "critical"`,
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, celWF)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, celWF) }()

		// severity=low → filter returns false → 200, no run
		body := []byte(`{"severity":"low"}`)
		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "cel-filter-wf", body)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		var result map[string]string
		Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
		Expect(result["status"]).To(Equal("filtered"))

		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "cel-filter-wf"})).To(Succeed())
		Expect(list.Items).To(BeEmpty())
	})

	It("should return 400 when celFilter expression is invalid CEL", func() {
		badCELWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-cel-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef: ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						CELFilter: `!!!invalid CEL`,
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, badCELWF)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, badCELWF) }()

		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "bad-cel-wf", []byte(`{}`))
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("should map inputMapping CEL expressions to WorkflowRun inputs", func() {
		mappingWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "input-map-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef:    ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						InputMapping: map[string]string{"ns": "object.namespace", "env": "object.environment"},
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, mappingWF)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, mappingWF)
			var list ottoflowv1alpha1.WorkflowRunList
			_ = k8sClient.List(ctx, &list, client.InNamespace("default"),
				client.MatchingLabels{"ottoflow.nirmata.io/workflow": "input-map-wf"})
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		}()

		body := []byte(`{"namespace":"production","environment":"prod"}`)
		resp := makeWebhookRequest(httpSrv, hmacSecret, "default", "input-map-wf", body)
		Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "input-map-wf"})).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].Spec.InputValues).To(HaveKeyWithValue("ns", "production"))
		Expect(list.Items[0].Spec.InputValues).To(HaveKeyWithValue("env", "prod"))
	})

	It("should deduplicate requests with the same dedupKey within dedupWindow", func() {
		dedupWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "dedup-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef:   ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						DedupKey:    "object.run_id",
						DedupWindow: &metav1.Duration{Duration: 10 * time.Minute},
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, dedupWF)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, dedupWF)
			var list ottoflowv1alpha1.WorkflowRunList
			_ = k8sClient.List(ctx, &list, client.InNamespace("default"),
				client.MatchingLabels{"ottoflow.nirmata.io/workflow": "dedup-wf"})
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		}()

		body := []byte(`{"run_id":"abc-123"}`)
		// First request — should create a run.
		resp1 := makeWebhookRequest(httpSrv, hmacSecret, "default", "dedup-wf", body)
		Expect(resp1.StatusCode).To(Equal(http.StatusAccepted))

		// Second request with same dedupKey within window — should be deduped.
		resp2 := makeWebhookRequest(httpSrv, hmacSecret, "default", "dedup-wf", body)
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		var result map[string]string
		Expect(json.NewDecoder(resp2.Body).Decode(&result)).To(Succeed())
		Expect(result["status"]).To(Equal("deduped"))

		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "dedup-wf"})).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
	})

	It("should NOT deduplicate requests with different dedupKey values", func() {
		dedupWF2 := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "dedup-diff-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef:   ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						DedupKey:    "object.run_id",
						DedupWindow: &metav1.Duration{Duration: 10 * time.Minute},
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, dedupWF2)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, dedupWF2)
			var list ottoflowv1alpha1.WorkflowRunList
			_ = k8sClient.List(ctx, &list, client.InNamespace("default"),
				client.MatchingLabels{"ottoflow.nirmata.io/workflow": "dedup-diff-wf"})
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		}()

		// Two different run_ids — both should create a WorkflowRun.
		resp1 := makeWebhookRequest(httpSrv, hmacSecret, "default", "dedup-diff-wf", []byte(`{"run_id":"abc"}`))
		Expect(resp1.StatusCode).To(Equal(http.StatusAccepted))

		resp2 := makeWebhookRequest(httpSrv, hmacSecret, "default", "dedup-diff-wf", []byte(`{"run_id":"xyz"}`))
		Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))

		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "dedup-diff-wf"})).To(Succeed())
		Expect(list.Items).To(HaveLen(2))
	})

	It("should suppress all requests within dedupWindow when dedupKey is not set", func() {
		windowWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "window-dedup-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef:   ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
						DedupWindow: &metav1.Duration{Duration: 10 * time.Minute},
						// No DedupKey — any payload within the window is suppressed.
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, windowWF)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, windowWF)
			var list ottoflowv1alpha1.WorkflowRunList
			_ = k8sClient.List(ctx, &list, client.InNamespace("default"),
				client.MatchingLabels{"ottoflow.nirmata.io/workflow": "window-dedup-wf"})
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		}()

		resp1 := makeWebhookRequest(httpSrv, hmacSecret, "default", "window-dedup-wf", []byte(`{"a":1}`))
		Expect(resp1.StatusCode).To(Equal(http.StatusAccepted))

		// Different payload, same workflow — should be deduped (window-only dedup).
		resp2 := makeWebhookRequest(httpSrv, hmacSecret, "default", "window-dedup-wf", []byte(`{"a":2}`))
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		var result map[string]string
		Expect(json.NewDecoder(resp2.Body).Decode(&result)).To(Succeed())
		Expect(result["status"]).To(Equal("deduped"))
	})

	It("should return 429 when MaxConcurrentRuns is reached", func() {
		maxRunsWF := &ottoflowv1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "max-runs-wf", Namespace: "default"},
			Spec: ottoflowv1alpha1.WorkflowSpec{
				Steps: []ottoflowv1alpha1.Step{{
					Name:        "echo",
					Expressions: []ottoflowv1alpha1.Expression{{Name: "r", Expression: `"ok"`}},
				}},
				Run: &ottoflowv1alpha1.RunPolicy{MaxConcurrentRuns: func() *int32 { v := int32(1); return &v }()},
				Triggers: []ottoflowv1alpha1.Trigger{{
					Webhook: &ottoflowv1alpha1.WebhookTrigger{
						SecretRef: ottoflowv1alpha1.WebhookSecretRef{Name: "wh-secret"},
					},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, maxRunsWF)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(ctx, maxRunsWF)
			var list ottoflowv1alpha1.WorkflowRunList
			_ = k8sClient.List(ctx, &list, client.InNamespace("default"),
				client.MatchingLabels{"ottoflow.nirmata.io/workflow": "max-runs-wf"})
			for i := range list.Items {
				_ = k8sClient.Delete(ctx, &list.Items[i])
			}
		}()

		// First request creates a run.
		resp1 := makeWebhookRequest(httpSrv, hmacSecret, "default", "max-runs-wf", []byte(`{}`))
		Expect(resp1.StatusCode).To(Equal(http.StatusAccepted))

		// Mark it as Running so it counts toward MaxConcurrentRuns.
		var list ottoflowv1alpha1.WorkflowRunList
		Expect(k8sClient.List(ctx, &list, client.InNamespace("default"),
			client.MatchingLabels{"ottoflow.nirmata.io/workflow": "max-runs-wf"})).To(Succeed())
		Expect(list.Items).NotTo(BeEmpty())
		wr := &list.Items[0]
		wr.Status.Phase = ottoflowv1alpha1.WorkflowRunPhaseRunning
		Expect(k8sClient.Status().Update(ctx, wr)).To(Succeed())

		// Second request should be rate-limited.
		resp2 := makeWebhookRequest(httpSrv, hmacSecret, "default", "max-runs-wf", []byte(`{}`))
		Expect(resp2.StatusCode).To(Equal(http.StatusTooManyRequests))
	})
})
