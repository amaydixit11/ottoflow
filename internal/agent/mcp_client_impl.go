/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// realMCPClient wraps kubectl-ai MCP client to implement our MCPClient interface
type realMCPClient struct {
	serverName        string
	client            *mcp.Client
	connectMu         sync.Mutex
	connected         bool
	connectionTimeout time.Duration // used for initial Connect() so stdio has time to start (e.g. uvx)
}

// ListTools returns metadata for all tools from the MCP server (for LLM tool registration).
func (c *realMCPClient) ListTools(ctx context.Context) ([]MCPToolMeta, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	tools, err := c.client.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]MCPToolMeta, 0, len(tools))
	for _, t := range tools {
		out = append(out, MCPToolMeta{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out, nil
}

// CallTool calls an MCP tool and returns the result as interface{} (parses JSON when possible)
func (c *realMCPClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (interface{}, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	result, err := c.client.CallTool(ctx, toolName, arguments)
	if err != nil {
		return nil, err
	}

	// Parse as JSON when possible for structured CEL access (e.g. toolResult.count)
	return parseToolResult(result)
}

// Close closes the MCP client connection
func (c *realMCPClient) Close() error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if c.client == nil {
		return nil
	}
	klog.V(4).InfoS("Closing MCP client", "server", c.serverName)
	err := c.client.Close()
	c.client = nil
	c.connected = false
	return err
}

func (c *realMCPClient) ensureConnected(ctx context.Context) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	if c.connected && c.client != nil {
		return nil
	}
	if c.client == nil {
		return fmt.Errorf("MCP client not initialized for server %s", c.serverName)
	}
	// Use a dedicated timeout for Connect so stdio servers (e.g. uvx) have time to start
	// even when the parent context (e.g. reconcile) has a short deadline.
	connectCtx := ctx
	if c.connectionTimeout > 0 {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), c.connectionTimeout)
		defer cancel()
	}
	if err := c.client.Connect(connectCtx); err != nil {
		return fmt.Errorf("connecting to MCP server %s: %w", c.serverName, err)
	}
	c.connected = true
	return nil
}

// parseToolResult attempts to parse the tool result as JSON; returns raw string if not valid JSON
func parseToolResult(result string) (interface{}, error) {
	if result == "" {
		return "", nil
	}
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result, nil
	}

	// Try JSON object or array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var parsed interface{}
		if err := json.Unmarshal([]byte(result), &parsed); err == nil {
			return parsed, nil
		}
	}

	// Try JSON number
	if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return num, nil
	}
	if b, err := strconv.ParseBool(trimmed); err == nil {
		return b, nil
	}

	return result, nil
}

// buildMCPClientConfig converts MCPServer CRD to kubectl-ai mcp.ClientConfig
func buildMCPClientConfig(ctx context.Context, k8sClient client.Client, mcpServer *ottoflowv1alpha1.MCPServer) (mcp.ClientConfig, error) {
	cfg := mcp.ClientConfig{
		Name: mcpServer.Name,
	}

	transport := mcpServer.Spec.Transport
	switch transport.Type {
	case "stdio":
		if len(transport.Command) == 0 {
			return cfg, fmt.Errorf("stdio transport requires command")
		}
		cfg.Command = transport.Command[0]
		if len(transport.Command) > 1 {
			cfg.Args = transport.Command[1:]
		}
		if mcpServer.Spec.Timeout != "" {
			if d, err := time.ParseDuration(mcpServer.Spec.Timeout); err == nil && d > 0 {
				cfg.Timeout = int(d.Seconds())
			}
		}
		if cfg.Timeout <= 0 {
			cfg.Timeout = 90
		}
		// Resolve env from MCPServer spec
		for _, ev := range mcpServer.Spec.Env {
			cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", ev.Name, resolveEnvValue(ctx, k8sClient, mcpServer.Namespace, &ev)))
		}
	case "http", "sse":
		if transport.Address == "" {
			return cfg, fmt.Errorf("http/sse transport requires address")
		}
		cfg.URL = transport.Address
		cfg.UseStreaming = (transport.Type == "sse")
		if mcpServer.Spec.Timeout != "" {
			if d, err := time.ParseDuration(mcpServer.Spec.Timeout); err == nil {
				cfg.Timeout = int(d.Seconds())
			}
		}
		cfg.Headers = transport.Headers
		// Resolve auth
		if mcpServer.Spec.Auth != nil {
			authCfg, oauthCfg, err := resolveAuthConfigs(ctx, k8sClient, mcpServer)
			if err != nil {
				return cfg, err
			}
			cfg.Auth = authCfg
			cfg.OAuthConfig = oauthCfg
		}
	default:
		return cfg, fmt.Errorf("unsupported transport type: %s", transport.Type)
	}

	return cfg, nil
}

func resolveEnvValue(ctx context.Context, k8sClient client.Client, namespace string, ev *corev1.EnvVar) string {
	if ev.Value != "" {
		return ev.Value
	}
	if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
		secret := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: ev.ValueFrom.SecretKeyRef.Name, Namespace: namespace}, secret); err != nil {
			klog.V(2).InfoS("Failed to resolve secret for env", "name", ev.Name, "error", err)
			return ""
		}
		if v, ok := secret.Data[ev.ValueFrom.SecretKeyRef.Key]; ok {
			return string(v)
		}
	}
	return ""
}

// resolveAuthConfigs returns AuthConfig and OAuthConfig for kubectl-ai MCP client
func resolveAuthConfigs(ctx context.Context, k8sClient client.Client, mcpServer *ottoflowv1alpha1.MCPServer) (*mcp.AuthConfig, *mcp.OAuthConfig, error) {
	auth := mcpServer.Spec.Auth
	if auth == nil {
		return nil, nil, nil
	}

	// Map CRD auth types to kubectl-ai (apiKey -> api-key)
	authType := auth.Type
	if authType == "apiKey" {
		authType = "api-key"
	}
	ac := &mcp.AuthConfig{Type: authType}
	var oauthCfg *mcp.OAuthConfig

	switch auth.Type {
	case "bearer", "apiKey":
		if auth.SecretRef == nil {
			return nil, nil, fmt.Errorf("secretRef is required for %s auth", auth.Type)
		}
		secret := &corev1.Secret{}
		ns := auth.SecretRef.Namespace
		if ns == "" {
			ns = mcpServer.Namespace
		}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: auth.SecretRef.Name, Namespace: ns}, secret); err != nil {
			return nil, nil, fmt.Errorf("failed to get auth secret: %w", err)
		}
		token, ok := secret.Data[auth.SecretRef.Key]
		if !ok {
			return nil, nil, fmt.Errorf("key %s not found in secret", auth.SecretRef.Key)
		}
		ac.Token = string(token)
		if auth.Type == "apiKey" {
			ac.ApiKey = ac.Token
		}
	case "basic":
		if auth.SecretRef == nil {
			return nil, nil, fmt.Errorf("secretRef is required for basic auth")
		}
		secret := &corev1.Secret{}
		ns := auth.SecretRef.Namespace
		if ns == "" {
			ns = mcpServer.Namespace
		}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: auth.SecretRef.Name, Namespace: ns}, secret); err != nil {
			return nil, nil, fmt.Errorf("failed to get auth secret: %w", err)
		}
		if username, ok := secret.Data["username"]; ok {
			ac.Username = string(username)
		}
		if password, ok := secret.Data["password"]; ok {
			ac.Password = string(password)
		}
		if ac.Username == "" || ac.Password == "" {
			return nil, nil, fmt.Errorf("secret for basic auth must contain username and password keys")
		}
	case "oauth2":
		if auth.OAuth2 == nil {
			return nil, nil, fmt.Errorf("oauth2 auth type requires oauth2 config")
		}
		oauth2 := auth.OAuth2
		oauthCfg = &mcp.OAuthConfig{
			TokenURL: oauth2.TokenURL,
			Scopes:   oauth2.Scopes,
		}
		if oauth2.ClientCredentialsRef != nil {
			secret := &corev1.Secret{}
			ns := oauth2.ClientCredentialsRef.Namespace
			if ns == "" {
				ns = mcpServer.Namespace
			}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: oauth2.ClientCredentialsRef.Name, Namespace: ns}, secret); err != nil {
				return nil, nil, fmt.Errorf("failed to get OAuth2 credentials secret: %w", err)
			}
			if id, ok := secret.Data["client_id"]; ok {
				oauthCfg.ClientID = string(id)
			}
			if secret, ok := secret.Data["client_secret"]; ok {
				oauthCfg.ClientSecret = string(secret)
			}
		} else if oauth2.ClientID != "" && oauth2.ClientSecretRef != nil {
			oauthCfg.ClientID = oauth2.ClientID
			secret := &corev1.Secret{}
			ns := oauth2.ClientSecretRef.Namespace
			if ns == "" {
				ns = mcpServer.Namespace
			}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: oauth2.ClientSecretRef.Name, Namespace: ns}, secret); err != nil {
				return nil, nil, fmt.Errorf("failed to get OAuth2 client secret: %w", err)
			}
			if v, ok := secret.Data[oauth2.ClientSecretRef.Key]; ok {
				oauthCfg.ClientSecret = string(v)
			}
		}
		if oauthCfg.ClientID == "" || oauthCfg.ClientSecret == "" {
			return nil, nil, fmt.Errorf("oauth2 requires client_id and client_secret")
		}
	}

	return ac, oauthCfg, nil
}
