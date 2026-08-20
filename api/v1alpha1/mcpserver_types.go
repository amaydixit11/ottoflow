/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=mcpserver;mcpservers
//+kubebuilder:subresource:status

// MCPServer is the Schema for the mcpservers API
// MCPServer defines an MCP (Model Context Protocol) server configuration
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

// MCPServerSpec defines the desired state of MCPServer
type MCPServerSpec struct {
	// Transport defines how to connect to the MCP server
	// +kubebuilder:validation:Required
	Transport TransportConfig `json:"transport"`

	// Timeout is the connection timeout (e.g., "30s", "5m")
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// Env is a list of environment variables to set for the MCP server process
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Auth defines authentication configuration
	// +optional
	Auth *AuthConfig `json:"auth,omitempty"`
}

// TransportConfig defines the transport mechanism for MCP server connection
type TransportConfig struct {
	// Type is the transport type
	// Options: stdio, http, sse
	// +kubebuilder:validation:Enum=stdio;http;sse
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Address is the server address (for http/sse)
	// +optional
	Address string `json:"address,omitempty"`

	// Command is the command to execute (for stdio)
	// +optional
	Command []string `json:"command,omitempty"`

	// Headers are HTTP headers (for http/sse)
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
}

// AuthConfig defines authentication configuration for MCP server.
// Credentials must be provided via SecretRef (or OAuth2 secret refs); inline credentials are not supported.
type AuthConfig struct {
	// Type is the authentication type
	// Options: none, bearer, apiKey, basic, oauth2
	// +kubebuilder:validation:Enum=none;bearer;apiKey;basic;oauth2
	// +kubebuilder:default=none
	// +optional
	Type string `json:"type,omitempty"`

	// SecretRef references a Kubernetes Secret containing credentials.
	// Required for bearer, apiKey, and basic auth. For bearer/apiKey the secret key holds the token.
	// For basic auth the secret must contain keys "username" and "password".
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// OAuth2 config for OAuth 2.0/2.1 client credentials flow (machine-to-machine)
	// Used when type is oauth2
	// +optional
	OAuth2 *OAuth2Config `json:"oauth2,omitempty"`
}

// OAuth2Config defines OAuth 2.0 client credentials flow configuration
// Used for machine-to-machine auth when connecting to MCP servers
type OAuth2Config struct {
	// TokenURL is the OAuth2 token endpoint (e.g., https://auth.example.com/oauth/token)
	// +kubebuilder:validation:Required
	TokenURL string `json:"tokenURL"`

	// ClientID is the OAuth2 client ID (alternative to ClientCredentialsRef)
	// +optional
	ClientID string `json:"clientId,omitempty"`

	// ClientSecretRef references a Secret for credentials
	// For client credentials flow: use key "client_secret" (with ClientID above), or
	// use ClientCredentialsRef for both client_id and client_secret
	// +optional
	ClientSecretRef *SecretReference `json:"clientSecretRef,omitempty"`

	// ClientCredentialsRef references a Secret with keys "client_id" and "client_secret"
	// Namespace defaults to MCPServer namespace
	// +optional
	ClientCredentialsRef *NamespacedSecretRef `json:"clientCredentialsRef,omitempty"`

	// Scopes are optional OAuth2 scopes (space-separated)
	// +optional
	Scopes []string `json:"scopes,omitempty"`
}

// NamespacedSecretRef references a Secret by name and namespace (no specific key)
type NamespacedSecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// SecretReference references a Kubernetes Secret
type SecretReference struct {
	// Name is the name of the Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Secret. Defaults to the namespace of the
	// resource that references this Secret (MCPServer, Workflow, etc.).
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key is the key in the Secret
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// MCPServerStatus defines the observed state of MCPServer
type MCPServerStatus struct {
	// Phase represents the current phase of the MCPServer
	// +kubebuilder:validation:Enum=Ready;NotReady
	// +optional
	Phase MCPServerPhase `json:"phase,omitempty"`

	// Message provides additional information about the server status
	// +optional
	Message string `json:"message,omitempty"`

	// LastConnected is when the server was last successfully connected
	// +optional
	LastConnected *metav1.Time `json:"lastConnected,omitempty"`

	// AvailableTools is a list of tools available from this MCP server
	// +optional
	AvailableTools []string `json:"availableTools,omitempty"`
}

// MCPServerPhase represents the phase of an MCPServer
// +kubebuilder:validation:Enum=Ready;NotReady
type MCPServerPhase string

const (
	// MCPServerPhaseReady indicates the server is ready to use
	MCPServerPhaseReady MCPServerPhase = "Ready"

	// MCPServerPhaseNotReady indicates the server is not ready
	MCPServerPhaseNotReady MCPServerPhase = "NotReady"
)

func init() {
	// MCPServer registration moved to agent_types.go to avoid duplicate registration
}
