/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=steptemplate;steptemplates
//+kubebuilder:subresource:status

// StepTemplate is the Schema for the steptemplates API
// StepTemplate defines a reusable step definition that can be instantiated with parameters
type StepTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StepTemplateSpec   `json:"spec,omitempty"`
	Status StepTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StepTemplateList contains a list of StepTemplate
type StepTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StepTemplate `json:"items"`
}

// StepTemplateSpec defines the desired state of StepTemplate
type StepTemplateSpec struct {
	// Step defines the step template that will be instantiated
	// The step.name field will be replaced with the actual step name when instantiated
	// Parameter placeholders in expressions/outputs use CEL variable syntax: {{.parameterName}}
	// +kubebuilder:validation:Required
	Step StepTemplateStep `json:"step"`

	// Parameters defines the parameters that can be provided when instantiating this template
	// +optional
	Parameters []StepTemplateParameter `json:"parameters,omitempty"`

	// Description provides a human-readable description of what this template does
	// +optional
	Description string `json:"description,omitempty"`
}

// StepTemplateStep defines a step that can be parameterized
// This is similar to Step but allows parameter placeholders
type StepTemplateStep struct {
	// Message is a human-readable description of what the step does
	// Can contain parameter placeholders: {{.parameterName}}
	// +optional
	Message string `json:"message,omitempty"`

	// Expressions are CEL expressions evaluated before step execution
	// Can contain parameter placeholders: {{.parameterName}}
	// +optional
	Expressions []Expression `json:"expressions,omitempty"`

	// Outputs defines key-value pairs written to shared context
	// Expression values can contain parameter placeholders: {{.parameterName}}
	// +optional
	Outputs []Output `json:"outputs,omitempty"`

	// DependsOn explicitly declares step dependencies
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// MatchConditions defines conditional execution rules for this step
	// Expression values can contain parameter placeholders: {{.parameterName}}
	// +optional
	MatchConditions []MatchCondition `json:"matchConditions,omitempty"`

	// Retry defines retry configuration for step execution
	// +optional
	Retry *RetryPolicy `json:"retry,omitempty"`

	// Timeout defines maximum duration for step execution
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// FailurePolicy determines workflow behavior on step failure
	// +kubebuilder:validation:Enum=Fail;Continue
	// +kubebuilder:default=Fail
	// +optional
	FailurePolicy string `json:"failurePolicy,omitempty"`

	// ResourceQuery defines a simplified resource query
	// Field values can contain parameter placeholders: {{.parameterName}}
	// +optional
	ResourceQuery *StepResourceQuery `json:"resourceQuery,omitempty"`

	// PrometheusQuery defines a Prometheus (PromQL) query step
	// Field values can contain parameter placeholders: {{.parameterName}}
	// +optional
	PrometheusQuery *StepPrometheusQuery `json:"prometheusQuery,omitempty"`

	// AgentRef references an Agent CRD for AI-powered step execution
	// AdditionalPrompts can contain parameter placeholders: {{.parameterName}}
	// +optional
	AgentRef *StepAgentRef `json:"agentRef,omitempty"`

	// MCPToolCall defines a direct MCP tool invocation
	// Argument values can contain parameter placeholders: {{.parameterName}}
	// +optional
	MCPToolCall *StepMCPToolCall `json:"mcpToolCall,omitempty"`

	// WorkflowRef references another workflow to execute as a sub-workflow
	// Input values can contain parameter placeholders: {{.parameterName}}
	// +optional
	WorkflowRef *StepWorkflowRef `json:"workflowRef,omitempty"`

	// ExternalAgentRef defines a call to an external A2A-compatible agent service
	// Prompt can contain parameter placeholders: {{.parameterName}}
	// +optional
	ExternalAgentRef *StepExternalAgentRef `json:"externalAgentRef,omitempty"`
}

// StepTemplateParameter defines a parameter for the template
type StepTemplateParameter struct {
	// Name is the parameter name (used in placeholders: {{.name}})
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Description provides a human-readable description of the parameter
	// +optional
	Description string `json:"description,omitempty"`

	// Default is the default value for the parameter (CEL expression)
	// Evaluated in workflow context if not provided in arguments
	// +optional
	Default string `json:"default,omitempty"`

	// Required indicates if the parameter must be provided
	// +optional
	Required bool `json:"required,omitempty"`
}

// StepTemplateStatus defines the observed state of StepTemplate
type StepTemplateStatus struct {
	// Conditions represent the latest available observations of the template's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// StepTemplateRef references a StepTemplate to instantiate
type StepTemplateRef struct {
	// Name is the name of the StepTemplate to use
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the StepTemplate
	// Defaults to the parent workflow namespace if not specified
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Arguments provides parameter values for the template
	// Keys match parameter names defined in the StepTemplate
	// Values are CEL expressions that are evaluated in the workflow context
	// +optional
	Arguments map[string]string `json:"arguments,omitempty"`
}
