/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package v1alpha1

// StepOpenReport defines an OpenReports.io report generation step.
//
// The executor checks whether the openreports.io/v1alpha1 Report CRD is installed in the
// cluster. If it is, a Report CRD object is created with the evaluated results. If it is
// not, the report data is stored in context as reportResult.data and a Warning Kubernetes
// Event is emitted on the WorkflowRun — the step still succeeds so downstream steps can
// continue.
//
// After execution, reportResult is available in CEL output expressions:
//
//	reportResult.mode      — "crd" if a Report CRD was created, "data" if OpenReports is absent
//	reportResult.name      — CRD name (mode=crd) or empty string (mode=data)
//	reportResult.namespace — CRD namespace (mode=crd) or empty string (mode=data)
//	reportResult.summary   — map with pass/fail/warn/error/skip integer counts
//	reportResult.data      — the raw results list (always present in both modes)
type StepOpenReport struct {
	// ReportName is the name of the Report CRD to create.
	// +kubebuilder:validation:MinLength=1
	ReportName string `json:"reportName"`

	// Namespace where the Report CRD is created.
	// Defaults to the WorkflowRun's namespace if omitted.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Source identifies the producer of this report (e.g. "ottoflow", "kyverno").
	// Defaults to "ottoflow" if omitted.
	// +optional
	Source string `json:"source,omitempty"`

	// ScopeExpression is an optional CEL expression evaluating to an object reference map
	// with keys: apiVersion, kind, name, namespace. Used to associate the report with a
	// specific Kubernetes resource (e.g. a Deployment or Namespace).
	// +optional
	ScopeExpression string `json:"scopeExpression,omitempty"`

	// ResultsExpression is a CEL expression evaluating to a list of policy check results.
	// Each item must match the OpenReports ReportResult schema:
	//   {policy, result, scored, timestamp?, source?, rule?, severity?, message?, ...}
	// The result field must be one of: pass, fail, warn, error, skip.
	// +kubebuilder:validation:MinLength=1
	ResultsExpression string `json:"resultsExpression"`

	// SummaryExpression is an optional CEL expression evaluating to a summary map with
	// integer keys: pass, fail, warn, error, skip.
	// If omitted, the executor auto-computes the summary by counting result values in
	// the ResultsExpression output.
	// +optional
	SummaryExpression string `json:"summaryExpression,omitempty"`
}
