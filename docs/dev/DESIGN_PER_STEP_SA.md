# Design: Per-step Service Accounts — Least-Privilege Identity for Each Step

**Status**: Design  
**Issue**: Tracked internally  
**Proposal**: [PROPOSAL_PER_STEP_SA.md](PROPOSAL_PER_STEP_SA.md)

---

## Overview

Every step in a WorkflowRun currently shares one Kubernetes identity: the runner Job's
ServiceAccount. Steps that only need to read cluster state carry the same RBAC rights as steps
that mutate resources, and all Kubernetes API calls in the audit log are attributed to the same
SA.

This design adds a `serviceAccountRef` field to `Step` and mints a short-lived Kubernetes
TokenRequest token for the specified SA before the step executes. The token builds a scoped
`client.Client` used for all Kubernetes API calls within that step — including `resource.get`
and `resource.list` CEL macro calls. After the step completes the client is discarded and the
token expires within 5 minutes.

---

## Root Cause

### One shared client across all steps

`WorkflowExecutor` holds a single `client client.Client` (the target-cluster client) that is
set once at construction and reused for every step:

```go
// internal/workflow/executor/executor.go — type WorkflowExecutor
type WorkflowExecutor struct {
    client         client.Client   // target cluster — ResourceQuery, Mutate
    controlClient  client.Client   // control plane — Agent CRDs, StepTemplate, MCPServer secrets
    celEvaluator   *CELEvaluator   // captures client in closures at construction
    // ...
}
```

`executeResourceQuery` calls `e.client.Get` / `e.client.List`. `executeMutate` calls
`e.client.Get` and `e.client.Patch`. The `CELEvaluator` captures `client` in CEL macro
closures at construction time — see the macro option builders `GetResourceMacroOptions` and
`GetResourceMacroOptionsWithMetrics` in `resource_macros.go` — so `resource.get(...)` and
`resource.list(...)` expressions also use `e.client`.

There is no mechanism to scope either the direct calls or the macro closures to a per-step
identity.

### Why swapping e.client is not safe for forEach

ForEach processes items concurrently in goroutines (`foreach_executor.go`), all calling
`e.executeStep()` on the same executor. Mutating `e.client` before a step would race across
concurrent goroutines. The existing `macroEvalMu` mutex serialises only the CEL context holder
(not the client), so it does not protect a client swap.

### controlClient is out of scope

`controlClient` is used to fetch OttoFlow's own CRDs (Agent, StepTemplate, MCPServer) and
resolve MCP server secrets — these are OttoFlow's internal configuration objects, not user
workload state. Scoping `controlClient` to a per-step SA would require the SA to have read
access to OttoFlow's namespace, adding complexity with no security benefit. Per-step identity
applies only to `e.client` (the target cluster).

---

## Design

### 1. CRD changes

#### `Step.ServiceAccountRef`

```go
// api/v1alpha1/workflow_types.go — added to Step struct

// ServiceAccountRef names a pre-existing ServiceAccount in the WorkflowRun's namespace.
// When set, the runner mints a short-lived TokenRequest token for this SA before the step
// executes. All Kubernetes API calls within the step — ResourceQuery, Mutate, and CEL
// resource macros — use this identity instead of the runner pod's SA.
//
// The ServiceAccount must already exist with appropriate RBAC bindings; OttoFlow does not
// create or manage per-step ServiceAccounts.
//
// Steps without this field (and no workflow-level defaultServiceAccount) continue to use the
// runner pod's mounted SA token — backward compatible.
//
// +optional
ServiceAccountRef string `json:"serviceAccountRef,omitempty"`
```

#### `WorkflowRunExecutionSpec.DefaultServiceAccount`

```go
// api/v1alpha1/workflowrun_types.go — added to WorkflowRunExecutionSpec

// DefaultServiceAccount names a ServiceAccount to use for steps that do not declare their
// own serviceAccountRef. When set, all steps without an explicit serviceAccountRef mint a
// token for this SA instead of using the runner pod SA.
//
// +optional
DefaultServiceAccount string `json:"defaultServiceAccount,omitempty"`
```

`WorkflowRunExecutionSpec` is the same type used by both `Workflow.Spec.Execution` and
`WorkflowRun.Spec.Execution`. Setting it on the Workflow provides a per-workflow default;
setting it on the WorkflowRun overrides that default at run time.

**Backward compatibility**: Both fields are `omitempty` with no `+kubebuilder:default`. All
existing workflows that omit these fields continue to use the runner pod SA unchanged.

### 2. targetRESTConfig in WorkflowExecutor

A minted bearer token must be placed in a `rest.Config` to build a scoped `client.Client`.
`client.Client` does not expose its underlying `rest.Config`, so the executor stores it
directly:

```go
// executor.go — new field on WorkflowExecutor
type WorkflowExecutor struct {
    client             client.Client
    controlClient      client.Client
    targetRESTConfig   *rest.Config   // NEW: shallow-cloned per step to insert minted token
    celEvaluator       *CELEvaluator
    // ...
}
```

`targetRESTConfig` is passed from `cmd/workflow-runner/main.go`, where it already exists as
`targetRestConfig` (returned by `cluster.RestConfigForClusterRef`). It is added as a parameter
to `NewWorkflowExecutorWithClientsAndAgentExecutor`.

### 3. Token minting (new file: step_identity.go)

```
internal/workflow/executor/step_identity.go
```

Three functions:

**`resolveStepSA`**: returns the effective SA name for a step — `step.ServiceAccountRef` if
set, otherwise `workflowRun.Spec.Execution.DefaultServiceAccount` (merging Workflow-level and
WorkflowRun-level defaults), otherwise empty string (no per-step identity).

**`mintStepToken`**: calls `targetClient.SubResource("token").Create()`. For local clusters
(`workflowRun.Spec.ClusterRef == nil` or `Local: true`), a `BoundObjectRef` ties the token to
the runner pod (env var `POD_NAME`), so the token is invalidated once that pod is deleted.
Note the binding is to the pod, not the Job: a completed Job does not necessarily delete its
pod immediately, so pod deletion — not Job completion — is what revokes the token. For remote
clusters the `BoundObjectRef` is omitted — the runner pod does not exist on the remote API
server. `ExpirationSeconds` is 300 (5 minutes) in both cases, which bounds token lifetime even
where no `BoundObjectRef` applies.

```go
func mintStepToken(
    ctx context.Context,
    targetClient client.Client,
    saName, namespace string,
    isLocalCluster bool,
    podName string,
) (string, error) {
    sa := &corev1.ServiceAccount{}
    sa.Name = saName
    sa.Namespace = namespace

    tokenReq := &authenticationv1.TokenRequest{
        Spec: authenticationv1.TokenRequestSpec{
            Audiences:         []string{"ottoflow-step"},
            ExpirationSeconds: ptr.To(int64(300)),
        },
    }
    if isLocalCluster && podName != "" {
        tokenReq.Spec.BoundObjectRef = &authenticationv1.BoundObjectReference{
            Kind: "Pod",
            Name: podName,
        }
    }

    if err := targetClient.SubResource("token").Create(ctx, sa, tokenReq); err != nil {
        return "", fmt.Errorf("TokenRequest for SA %s/%s: %w", namespace, saName, err)
    }
    return tokenReq.Status.Token, nil
}
```

**`buildScopedClient`**: shallow-clones `targetRESTConfig`, replaces all auth fields with the
minted bearer token, and creates a new `client.Client`:

```go
func buildScopedClient(baseConfig *rest.Config, token string, scheme *runtime.Scheme) (client.Client, error) {
    cfg := rest.CopyConfig(baseConfig)
    cfg.BearerToken = token
    cfg.BearerTokenFile = ""
    cfg.TLSClientConfig.CertFile = ""
    cfg.TLSClientConfig.CertData = nil
    cfg.KeyFile = ""
    cfg.KeyData = nil
    cfg.Username = ""
    cfg.Password = ""
    return client.New(cfg, client.Options{Scheme: scheme})
}
```

Clearing existing auth fields is required: `rest.CopyConfig` copies all fields including the
current SA's bearer token or cert, which would take precedence over the new `BearerToken` if
not cleared.

### 4. Per-step executor creation

Before each step executes, the main dispatch loop (or `executeStep`) resolves the SA name and
creates a scoped executor if one is needed:

```go
// Conceptual flow in executeStep (executor.go)

saName := resolveStepSA(step, workflowRun)
if saName == "" {
    // No per-step SA — execute on e directly (existing behavior)
    return e.dispatchStep(ctx, workflowRun, step)
}

token, err := mintStepToken(ctx, e.client, saName, workflowRun.Namespace,
    isLocalCluster(workflowRun), os.Getenv("POD_NAME"))
if err != nil {
    return nil, fmt.Errorf("step %s: %w", step.Name, err)
}

scopedClient, err := buildScopedClient(e.targetRESTConfig, token, scheme)
if err != nil {
    return nil, fmt.Errorf("step %s: build scoped client: %w", step.Name, err)
}

stepExec := e.newStepExecutor(scopedClient)
return stepExec.dispatchStep(ctx, workflowRun, step)
```

**`newStepExecutor`**: extends `newChildExecutor` to accept an override client. It creates a
new `CELEvaluator` from the scoped client (so macro closures use the per-step identity) and a
new `ContextManager`. All other fields — `controlClient`, `agentExecutor`, `mcpManager`,
`prometheusClient`, `maxWorkers`, `eventRecorder` — are shared with the parent executor.

The per-step executor is allocated on the stack / GC heap for the step's duration and is not
stored anywhere after `dispatchStep` returns.

### 5. ForEach handling

ForEach runs items concurrently in goroutines. The solution is to mint the token **once** for
the forEach step's SA before `processItemsConcurrently`, then give each goroutine its own child
executor that shares the per-step `client.Client`.

```
executeForEach:
  saName := resolveStepSA(forEachStep, workflowRun)
  if saName != "" {
      token  → mintStepToken(...)            // once
      client → buildScopedClient(token)      // once, shared
  }
  processItemsConcurrently:
    per goroutine:
      exec := e.newStepExecutor(client)      // new executor, shared client
      exec.executeStep(ctx, childStep)
```

`client.Client` is safe to share across goroutines — its HTTP transport pools connections and
serialises TLS internally. Each goroutine's child executor has its own `CELEvaluator` (with the
shared per-step client in its closures) and its own `macroContextHolder` + `macroEvalMu`, so
concurrent CEL macro calls do not race on context.

ForEach items inherit the forEach step's `serviceAccountRef`. There is no per-item SA field —
`StepForEachStep` does not gain a `serviceAccountRef`.

### 6. RBAC

Add to `charts/ottoflow/templates/clusterrole.yaml`, alongside the existing `serviceaccounts`
rule:

```yaml
# ServiceAccount token minting — required for per-step least-privilege identity
- apiGroups:
    - ""
  resources:
    - serviceaccounts/token
  verbs:
    - create
```

This grants the runner SA (and the controller SA, which shares the ClusterRole) the ability to
call TokenRequest for any ServiceAccount in any namespace. In practice, the TokenRequest is only
called for SAs explicitly named in `serviceAccountRef` fields authored by the workflow operator.

### 7. Multi-cluster behavior

When `WorkflowRun.Spec.ClusterRef` targets a remote cluster via `KubeConfigSecretRef`:

- `targetClient` points at the remote API server
- `mintStepToken` calls `targetClient.SubResource("token").Create(...)` — the request goes to
  the remote API server using the remote kubeconfig's credential
- The SA named in `serviceAccountRef` must exist on the **remote** cluster
- The remote kubeconfig credential must have `create serviceaccounts/token` on that SA in the
  remote cluster — this is the user's responsibility and is documented as a prerequisite
- `BoundObjectRef` is omitted for remote clusters; the runner pod lives on the control cluster
  and has no representation on the remote API server

For local clusters (`ClusterRef` absent or `Local: true`), all of the above happens against
the in-cluster API server and `BoundObjectRef` applies normally.

---

## Edge Cases

### SA does not exist

`mintStepToken` returns an error from the API server (404 Not Found). The step fails with a
descriptive error: `"TokenRequest for SA default/ottoflow-reader: serviceaccounts "ottoflow-reader"
not found"`. The WorkflowRun status records this as a step failure. OttoFlow does not create or
auto-provision per-step ServiceAccounts.

### Missing RBAC on runner SA

If the runner SA lacks `create serviceaccounts/token`, `mintStepToken` returns a 403 Forbidden
error. The step fails immediately. This is a misconfiguration that surfaces clearly at step
execution time rather than silently degrading.

### Token expires during a long step

ResourceQuery and Mutate steps are expected to complete within seconds to minutes. A 300-second
TTL is sufficient. Steps that exceed 300 seconds (e.g., a Mutate step iterating over thousands
of resources) will receive 401 Unauthorized errors from the API server after expiry. If this
becomes a real concern, `ExpirationSeconds` can be made configurable as a follow-on; 300s is
the right default.

### defaultServiceAccount and explicit serviceAccountRef

Explicit `serviceAccountRef` on a step always takes precedence over `defaultServiceAccount`.
Precedence order: step `serviceAccountRef` > WorkflowRun `defaultServiceAccount` > Workflow
`defaultServiceAccount` > empty (pod SA).

### controlClient is never scoped

The `controlClient` (used to fetch Agent CRDs, StepTemplate CRDs, MCP server secrets) is never
replaced by a per-step client. These reads are OttoFlow's own configuration lookups in the
workflow's namespace and do not represent access to user workload state.

### Remote cluster kubeconfig rotation

If the kubeconfig secret for a remote cluster is rotated mid-workflow, steps that run after
rotation will use the new credential when calling `mintStepToken` (since `targetClient` is
rebuilt each run from the current secret). Tokens already minted from the previous credential
remain valid until their 300-second TTL expires. This matches normal Kubernetes token behavior.

---

## What This Design Does Not Address

- **Per-item SA in forEach**: Each item in a forEach step shares the parent step's SA. There is
  no `serviceAccountRef` field on `StepForEachStep`. Per-item identity would require a separate
  design.
- **External API scope translation**: Steps calling external services (MCPToolCall,
  ExternalAgentRef) do not use Kubernetes identity at all. Their authorization model is handled
  by `authRef` fields already present on those step types.
- **Auto-provisioning of per-step SAs**: OttoFlow does not create, update, or delete the
  ServiceAccounts named in `serviceAccountRef`. Users are responsible for their lifecycle and
  RBAC bindings. A future Helm chart component could provide starter SA profiles
  (`ottoflow-reader`, `ottoflow-mutator`) as opt-in resources.
- **Token caching across steps**: If two sequential steps share the same `serviceAccountRef`,
  two tokens are minted independently. Token caching across steps is not implemented; minting
  is cheap and keeps each step's exposure window independent.

---

## Testing

### Unit tests (step_identity_test.go)

| Scenario | Expected |
|---|---|
| `resolveStepSA` — step has explicit ref | returns step ref |
| `resolveStepSA` — step empty, run-level default set | returns run-level default |
| `resolveStepSA` — step empty, workflow-level default set | returns workflow-level default |
| `resolveStepSA` — all empty | returns `""` (no per-step SA) |
| `buildScopedClient` — input config has BearerToken | output config has only new BearerToken |
| `buildScopedClient` — input config has CertFile + KeyFile | output config has only new BearerToken, cert fields cleared |
| `mintStepToken` — local cluster | BoundObjectRef set with pod name |
| `mintStepToken` — remote cluster | BoundObjectRef nil |

### Integration

Add a test workflow with two agent steps, one ResourceQuery step, and one Mutate step:
- ResourceQuery and Mutate steps declare `serviceAccountRef: ottoflow-reader`
- Verify that the Kubernetes audit log (or a mock API server in tests) records API calls under
  `ottoflow-reader` for those steps and under the runner SA for the agent steps.

### Verification commands

```bash
make manifests && make generate    # regenerate CRDs after type changes
go build ./...                     # compile check
go test ./internal/workflow/executor/...   # unit tests
go test ./api/...                  # type validation tests
```

---

## Files Changed

| File | Change |
|---|---|
| `api/v1alpha1/workflow_types.go` | Add `ServiceAccountRef` to `Step` |
| `api/v1alpha1/workflowrun_types.go` | Add `DefaultServiceAccount` to `WorkflowRunExecutionSpec` |
| `api/v1alpha1/zz_generated.deepcopy.go` | Auto-updated via `make generate` |
| `internal/workflow/executor/executor.go` | Add `targetRESTConfig` field; wire per-step executor |
| `internal/workflow/executor/step_identity.go` | New — `resolveStepSA`, `mintStepToken`, `buildScopedClient`, `newStepExecutor` |
| `internal/workflow/executor/foreach_executor.go` | Mint token once; pass per-step client to child executors |
| `cmd/workflow-runner/main.go` | Pass `targetRESTConfig` to executor constructor |
| `charts/ottoflow/templates/clusterrole.yaml` | Add `create serviceaccounts/token` rule |
| `config/crd/bases/` + `charts/ottoflow/crds/` | Auto-updated via `make manifests` |
