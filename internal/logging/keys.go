/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

// Package logging defines standard structured-logging key names used by the
// controller and workflow runner for filtering and correlation (e.g. in
// log aggregators). Use these constants with logr.WithValues, klog.InfoS, etc.
package logging

// Standard keys for structured logs. Use these so log backends can filter and
// correlate by workflow, run, step, and phase.
const (
	KeyWorkflow    = "workflow"    // Workflow name (workflowRef.name)
	KeyWorkflowRun = "workflowRun" // WorkflowRun name
	KeyNamespace   = "namespace"   // Namespace of the WorkflowRun
	KeyStep        = "step"        // Step name
	KeyPhase       = "phase"       // WorkflowRun or step phase (e.g. succeeded, failed, running)
)

// KeysForRun returns key-value pairs for a workflow run, for use with
// klog.InfoS/klog.ErrorS or similar. Example:
//
//	klog.InfoS("WorkflowRun completed", append(logging.KeysForRun(wfName, ns, runName), logging.KeyPhase, "succeeded")...)
func KeysForRun(workflowName, namespace, workflowRunName string) []interface{} {
	return []interface{}{
		KeyWorkflow, workflowName,
		KeyNamespace, namespace,
		KeyWorkflowRun, workflowRunName,
	}
}

// KeysForStep returns key-value pairs for a step within a run. Use with
// KeysForRun when logging step-level events:
//
//	klog.InfoS("Step started", append(logging.KeysForRun(wf, ns, run), logging.KeysForStep(stepName)...)...)
func KeysForStep(stepName string) []interface{} {
	return []interface{}{KeyStep, stepName}
}
