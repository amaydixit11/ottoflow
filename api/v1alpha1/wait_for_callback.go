/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package v1alpha1

import apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

// WaitForCallbackStep defines a step that pauses execution and waits for an external callback.
// This enables human-in-the-loop and AI-to-human-to-AI workflows.
type WaitForCallbackStep struct {
	// Timeout is the maximum duration to wait for the callback (e.g., "24h", "30m").
	// If the callback is not received within this duration, the step fails.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+(ns|us|ms|s|m|h)$`
	Timeout string `json:"timeout"`

	// CallbackRef is a reference identifier for the callback (used for logging and documentation).
	// This is a semantic label, not a cryptographic identifier.
	// +optional
	CallbackRef string `json:"callbackRef,omitempty"`

	// OutputSchema defines the expected structure of the callback payload.
	// The callback payload must conform to this schema before the step resumes.
	// Schema is a JSON schema object (properties, type, etc.).
	// +optional
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`

	// Message is a human-readable message displayed to users awaiting callback.
	// Can include instructions for calling the callback endpoint.
	// +optional
	Message string `json:"message,omitempty"`

	// FailurePolicy determines workflow behavior when the callback timeout is exceeded.
	// Continue: proceed to the next step; the gate resumes with empty outputs (not Failed).
	// Fail: Workflow fails (default)
	// +kubebuilder:validation:Enum=Continue;Fail
	// +kubebuilder:default=Fail
	// +optional
	FailurePolicy string `json:"failurePolicy,omitempty"`
}

// CallbackState represents the pending state of a waitForCallback step.
// Stored in WorkflowRun.status.pendingCallback.
type CallbackState struct {
	// TokenHash is the SHA256 hex digest of the callback token (64 lowercase hex chars).
	// The plaintext token is never stored in status; it is only available in the step's
	// in-memory output context (key: "callbackToken") and in controller logs.
	// Storing the hash prevents token theft by any principal with get/list workflowruns RBAC.
	TokenHash string `json:"tokenHash"`

	// StepName is the name of the step waiting for the callback.
	StepName string `json:"stepName"`

	// ExpiresAt is the deadline for the callback (step timeout).
	// If the callback is not received by this time, the step fails.
	ExpiresAt int64 `json:"expiresAt"` // Unix timestamp in seconds

	// Outputs contains the callback payload once received.
	// Initially empty; populated when the callback arrives.
	// +optional
	Outputs apiextensionsv1.JSON `json:"outputs,omitempty"`

	// CreatedAt is the timestamp when the callback token was generated.
	// +optional
	CreatedAt int64 `json:"createdAt,omitempty"` // Unix timestamp in seconds
}
