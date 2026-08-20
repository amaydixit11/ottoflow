/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package webhook

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// WorkflowRunValidator validates WorkflowRun resources at admission time.
type WorkflowRunValidator struct{}

// ValidateCreate implements admission.Validator.
func (v *WorkflowRunValidator) ValidateCreate(ctx context.Context, run *ottoflowv1alpha1.WorkflowRun) (admission.Warnings, error) {
	return v.validate(run)
}

// ValidateUpdate implements admission.Validator.
func (v *WorkflowRunValidator) ValidateUpdate(ctx context.Context, oldRun, run *ottoflowv1alpha1.WorkflowRun) (admission.Warnings, error) {
	return v.validate(run)
}

// ValidateDelete implements admission.Validator.
func (v *WorkflowRunValidator) ValidateDelete(ctx context.Context, run *ottoflowv1alpha1.WorkflowRun) (admission.Warnings, error) {
	return nil, nil
}

func (v *WorkflowRunValidator) validate(run *ottoflowv1alpha1.WorkflowRun) (admission.Warnings, error) {
	if run == nil {
		return nil, nil
	}
	if run.Spec.WorkflowRef.Name == "" {
		return nil, fmt.Errorf("WorkflowRun %q spec.workflowRef.name is required", run.Name)
	}
	if run.Spec.Execution != nil && run.Spec.Execution.LLMCredentialsSecret != nil {
		ref := run.Spec.Execution.LLMCredentialsSecret
		if ref.Name == "" {
			return nil, fmt.Errorf("WorkflowRun %q spec.execution.llmCredentialsSecret.name is required when llmCredentialsSecret is set", run.Name)
		}
		if ref.Namespace != "" && ref.Namespace != run.Namespace {
			return nil, fmt.Errorf("WorkflowRun %q spec.execution.llmCredentialsSecret.namespace must be empty or match the WorkflowRun namespace %q; cross-namespace Secret references are not supported because SecretKeyRef is namespace-scoped to the runner pod", run.Name, run.Namespace)
		}
	}
	return nil, nil
}
