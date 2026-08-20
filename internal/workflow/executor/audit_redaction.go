/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const sensitiveRedactedPlaceholder = "<redacted:sensitive>"

// sensitiveOutputNames returns the set of output names across the workflow (workflow-level
// Outputs, every step's Outputs, and forEach inline step Outputs) that are marked
// sensitive:true. Step-level outputs write directly into context["variables"] with no
// step-name prefix (see ContextManager.WriteStepOutputs), so this is a flat name set rather
// than one scoped per step.
func sensitiveOutputNames(workflow *ottoflowv1alpha1.Workflow) map[string]bool {
	names := make(map[string]bool)
	collect := func(outputs []ottoflowv1alpha1.Output) {
		for _, o := range outputs {
			if o.Sensitive {
				names[o.Name] = true
			}
		}
	}
	collect(workflow.Spec.Outputs)
	for _, step := range workflow.Spec.Steps {
		collect(step.Outputs)
		if step.ForEach != nil && step.ForEach.Step != nil {
			collect(step.ForEach.Step.Outputs)
		}
	}
	return names
}

// redactSensitiveContext returns a deep copy of ctxData with entries under any "outputs" or
// "variables" map redacted when their key is in sensitiveNames. This is what makes
// SaveAuditSnapshot's persisted copy honor the same Sensitive guarantee already made for
// WorkflowRun.Status (see evaluateWorkflowOutputs): GetContext() returns the live in-memory
// map by reference, and that map's "outputs"/"variables" hold the raw evaluated values, not
// the redacted ones evaluateWorkflowOutputs marshals separately for Status. Persisting that
// live map unredacted would leak sensitive values into a namespace-readable ConfigMap, so
// every write path that persists context (this one) must redact its own copy first.
func redactSensitiveContext(ctxData map[string]interface{}, sensitiveNames map[string]bool) map[string]interface{} {
	out, _ := redactValue(ctxData, sensitiveNames).(map[string]interface{})
	if out == nil {
		out = map[string]interface{}{}
	}
	return out
}

func redactValue(v interface{}, sensitiveNames map[string]bool) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, nv := range val {
			if k == "outputs" || k == "variables" {
				out[k] = redactNamedMap(nv, sensitiveNames)
				continue
			}
			out[k] = redactValue(nv, sensitiveNames)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, nv := range val {
			out[i] = redactValue(nv, sensitiveNames)
		}
		return out
	default:
		return v
	}
}

// redactNamedMap redacts direct keys of an "outputs"/"variables" map that match
// sensitiveNames, and otherwise recurses (so nested forEach per-item "outputs" maps, which
// sit a few levels below context["steps"][name]["results"][i]["outputs"], are also caught).
func redactNamedMap(v interface{}, sensitiveNames map[string]bool) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return redactValue(v, sensitiveNames)
	}
	out := make(map[string]interface{}, len(m))
	for k, nv := range m {
		if len(sensitiveNames) > 0 && sensitiveNames[k] {
			out[k] = sensitiveRedactedPlaceholder
			continue
		}
		out[k] = redactValue(nv, sensitiveNames)
	}
	return out
}
