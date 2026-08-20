/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package webhook

import (
	"context"
	"fmt"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// agentConfigKeysReadByDefaultExecutor lists the spec.config keys the built-in
// DefaultAgentExecutor actually consumes (see internal/agent/default_executor.go).
//
// Unrecognised keys are reported as warnings, never errors: spec.config is a free-form
// map and an alternative AgentExecutor implementation may legitimately read keys this
// build knows nothing about. The warning exists to catch typos and keys the CRD
// doc-comment advertises but no executor in this build reads.
var agentConfigKeysReadByDefaultExecutor = map[string]struct{}{
	"endpoint":      {},
	"skipVerifySSL": {},
}

// AgentValidator validates Agent resources.
type AgentValidator struct{}

func (v *AgentValidator) ValidateCreate(ctx context.Context, a *ottoflowv1alpha1.Agent) (admission.Warnings, error) {
	return validateAgentConfig(a), nil
}
func (v *AgentValidator) ValidateUpdate(ctx context.Context, oldA, a *ottoflowv1alpha1.Agent) (admission.Warnings, error) {
	return validateAgentConfig(a), nil
}

// validateAgentConfig warns about spec.config keys no executor in this build reads, and
// about malformed values for keys that are read.
func validateAgentConfig(a *ottoflowv1alpha1.Agent) admission.Warnings {
	if a == nil || len(a.Spec.Config) == 0 {
		return nil
	}

	var unknown []string
	var warnings admission.Warnings
	for k, val := range a.Spec.Config {
		if _, ok := agentConfigKeysReadByDefaultExecutor[k]; !ok {
			unknown = append(unknown, k)
			continue
		}
		if k == "skipVerifySSL" && val != "true" && val != "false" {
			warnings = append(warnings, fmt.Sprintf(
				"spec.config.skipVerifySSL must be \"true\" or \"false\"; got %q, which is treated as false", val))
		}
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		warnings = append(warnings, fmt.Sprintf(
			"spec.config keys %v are not read by the built-in agent executor (it reads only endpoint, skipVerifySSL); "+
				"they will be ignored unless a custom executor consumes them", unknown))
	}
	return warnings
}
func (v *AgentValidator) ValidateDelete(ctx context.Context, a *ottoflowv1alpha1.Agent) (admission.Warnings, error) {
	return nil, nil
}

// MCPServerValidator validates MCPServer resources (placeholder for future rules).
type MCPServerValidator struct{}

func (v *MCPServerValidator) ValidateCreate(ctx context.Context, m *ottoflowv1alpha1.MCPServer) (admission.Warnings, error) {
	return nil, nil
}
func (v *MCPServerValidator) ValidateUpdate(ctx context.Context, oldM, m *ottoflowv1alpha1.MCPServer) (admission.Warnings, error) {
	return nil, nil
}
func (v *MCPServerValidator) ValidateDelete(ctx context.Context, m *ottoflowv1alpha1.MCPServer) (admission.Warnings, error) {
	return nil, nil
}
