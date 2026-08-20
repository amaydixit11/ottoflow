# MCPServer

**MCPServer** defines an MCP (Model Context Protocol) server configuration: transport, authentication, timeout, and environment.

- **API Group:** `ottoflow.nirmata.io`
- **Version:** `v1alpha1`
- **Kind:** `MCPServer`
- **Scope:** Namespaced
- **Short names:** `mcpserver`, `mcpservers`

---

## Spec (MCPServerSpec)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `transport` | [TransportConfig](#transportconfig) | Yes | How to connect to the MCP server (stdio, http, sse). |
| `timeout` | string | No | Connection timeout (e.g. `30s`, `5m`). |
| `env` | []core/v1.EnvVar | No | Environment variables for the MCP server process (e.g. for stdio). |
| `auth` | [AuthConfig](#authconfig) | No | Authentication (none, bearer, apiKey, basic, oauth2). |

### TransportConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | One of: `stdio`, `http`, `sse`. |
| `command` | []string | No | Command to execute for stdio transport. |
| `address` | string | No | Server URL for http/sse. |
| `headers` | map[string]string | No | HTTP headers for http/sse. |

### AuthConfig

Credentials must be provided via **SecretRef** (or OAuth2 secret refs). Inline credentials are not supported.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | No | One of: `none`, `bearer`, `apiKey`, `basic`, `oauth2`. Default: `none`. |
| `secretRef` | [SecretReference](#secretreference) | For bearer/apiKey/basic | Reference to a Secret. For bearer/apiKey use a single key (token). For basic the Secret must contain keys `username` and `password`. |
| `oauth2` | [OAuth2Config](#oauth2config) | No | OAuth 2.0 client credentials flow (when type is oauth2). |

### SecretReference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the Secret. |
| `namespace` | string | No | Namespace of the Secret (defaults to MCPServer namespace). |
| `key` | string | Yes | Key in the Secret. |

### OAuth2Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tokenURL` | string | Yes | OAuth2 token endpoint (e.g. `https://auth.example.com/oauth/token`). |
| `clientId` | string | No | OAuth2 client ID (alternative to clientCredentialsRef). |
| `clientSecretRef` | SecretReference | No | Secret key for client_secret (use with clientId). |
| `clientCredentialsRef` | [NamespacedSecretRef](#namespacedsecretref) | No | Secret with keys `client_id` and `client_secret`. |
| `scopes` | []string | No | Optional OAuth2 scopes. |

### NamespacedSecretRef

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the Secret. |
| `namespace` | string | No | Namespace (defaults to MCPServer namespace). |

---

## Status (MCPServerStatus)

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | One of: `Ready`, `NotReady`. |
| `message` | string | Additional information about the server status. |
| `lastConnected` | string (date-time) | When the server was last successfully connected. |
| `availableTools` | []string | List of tools available from this MCP server. |
