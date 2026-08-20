/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/workflow/token"
)

func newCallbackTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(ottoflowv1alpha1.AddToScheme(s))
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	return s
}

func validTestToken() string {
	return "cb_" + strings.Repeat("a1b2c3d4", 8) // 64 hex chars = 32 bytes / 256 bits
}

func newWRWithPendingCallback(name, tok string, expiresIn time.Duration) *ottoflowv1alpha1.WorkflowRun {
	return &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: ottoflowv1alpha1.WorkflowRunSpec{
			WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "test-wf"},
		},
		Status: ottoflowv1alpha1.WorkflowRunStatus{
			Phase: ottoflowv1alpha1.WorkflowRunPhaseRunning,
			PendingCallback: &ottoflowv1alpha1.CallbackState{
				TokenHash: token.HashToken(tok),
				StepName:  "approve",
				ExpiresAt: time.Now().Add(expiresIn).Unix(),
				CreatedAt: time.Now().Unix(),
			},
		},
	}
}

func TestCallbackServer_HappyPath(t *testing.T) {
	scheme := newCallbackTestScheme()
	tok := validTestToken()
	wr := newWRWithPendingCallback("run-1", tok, 1*time.Hour)

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(wr).
		WithObjects(wr).
		Build()

	cs := NewCallbackServer(k8s, nil, ":0")

	payload := `{"approved":true,"comment":"LGTM"}`
	path := fmt.Sprintf("/api/v1/workflow-runs/default/run-1/callback/%s", tok)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "accepted" {
		t.Errorf("status = %v, want accepted", resp["status"])
	}
}

func TestCallbackServer_InvalidToken(t *testing.T) {
	scheme := newCallbackTestScheme()
	tok := validTestToken()
	wr := newWRWithPendingCallback("run-2", tok, 1*time.Hour)

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(wr).
		WithObjects(wr).
		Build()

	cs := NewCallbackServer(k8s, nil, ":0")

	// Wrong token (valid format, wrong value)
	wrongToken := "cb_" + strings.Repeat("b2c3d4e5", 8)
	path := fmt.Sprintf("/api/v1/workflow-runs/default/run-2/callback/%s", wrongToken)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"approved":true}`))
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %d", w.Code)
	}
}

func TestCallbackServer_ExpiredToken(t *testing.T) {
	scheme := newCallbackTestScheme()
	tok := validTestToken()
	wr := newWRWithPendingCallback("run-3", tok, -1*time.Hour) // already expired

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(wr).
		WithObjects(wr).
		Build()

	cs := NewCallbackServer(k8s, nil, ":0")

	path := fmt.Sprintf("/api/v1/workflow-runs/default/run-3/callback/%s", tok)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"approved":true}`))
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", w.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "expired") {
		t.Errorf("error should mention 'expired', got: %s", resp["error"])
	}
}

func TestCallbackServer_NotFound(t *testing.T) {
	scheme := newCallbackTestScheme()
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()

	cs := NewCallbackServer(k8s, nil, ":0")

	tok := validTestToken()
	path := fmt.Sprintf("/api/v1/workflow-runs/default/nonexistent/callback/%s", tok)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent run, got %d", w.Code)
	}
}

func TestCallbackServer_InvalidTokenFormat(t *testing.T) {
	scheme := newCallbackTestScheme()
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	cs := NewCallbackServer(k8s, nil, ":0")

	path := "/api/v1/workflow-runs/default/run/callback/notavalidtoken"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid token format, got %d", w.Code)
	}
}

func TestCallbackServer_MethodNotAllowed(t *testing.T) {
	scheme := newCallbackTestScheme()
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	cs := NewCallbackServer(k8s, nil, ":0")

	tok := validTestToken()
	path := fmt.Sprintf("/api/v1/workflow-runs/default/run/callback/%s", tok)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", w.Code)
	}
}

func TestCallbackServer_Healthz(t *testing.T) {
	scheme := newCallbackTestScheme()
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	cs := NewCallbackServer(k8s, nil, ":0")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Error("healthz should return 'ok'")
	}
}

func TestCallbackServer_InvalidJSON(t *testing.T) {
	scheme := newCallbackTestScheme()
	tok := validTestToken()
	wr := newWRWithPendingCallback("run-json", tok, 1*time.Hour)

	k8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(wr).
		WithObjects(wr).
		Build()

	cs := NewCallbackServer(k8s, nil, ":0")

	path := fmt.Sprintf("/api/v1/workflow-runs/default/run-json/callback/%s", tok)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	cs.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestValidatePayloadAgainstSchema_Required(t *testing.T) {
	schemaJSON := `{"type":"object","required":["approved"],"properties":{"approved":{"type":"boolean"}}}`
	schema := &apiextensionsv1.JSON{Raw: []byte(schemaJSON)}

	t.Run("valid payload", func(t *testing.T) {
		err := validatePayloadAgainstSchema(schema, map[string]interface{}{"approved": true})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("missing required field", func(t *testing.T) {
		err := validatePayloadAgainstSchema(schema, map[string]interface{}{})
		if err == nil {
			t.Error("expected error for missing required field")
		}
		if !strings.Contains(err.Error(), "approved") {
			t.Errorf("error should mention 'approved', got: %v", err)
		}
	})
	t.Run("wrong type", func(t *testing.T) {
		err := validatePayloadAgainstSchema(schema, map[string]interface{}{"approved": "yes"})
		if err == nil {
			t.Error("expected error for wrong type (string for boolean)")
		}
	})
	t.Run("nil schema passes all", func(t *testing.T) {
		err := validatePayloadAgainstSchema(nil, map[string]interface{}{})
		if err != nil {
			t.Errorf("nil schema should pass: %v", err)
		}
	})
}
