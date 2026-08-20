# Architecture

OttoFlow codifies a **Collect → Analyze → Publish** loop into a declarative
Kubernetes engine (`README.md`). Workflows are custom resources; a controller
reconciles them and runs each workflow in an isolated Job.

## Three binaries, three images

OttoFlow ships three separate binaries, each built into its own container
image. The split is confirmed by the three `main` packages under `cmd/` and the
`ko` import paths in the `Makefile`.

| Binary (`cmd/`) | Image | Kubernetes shape | Role |
|---|---|---|---|
| `cmd/controller` | `ghcr.io/nirmata/ottoflow/controller` | Deployment (persistent) | Reconciles CRDs, builds the DAG, spawns runner Jobs, runs triggers/cron/callbacks, serves admission webhooks, manages TLS certs (`cmd/controller/main.go`) |
| `cmd/agent-executor` | `ghcr.io/nirmata/ottoflow/agent-executor` | Deployment (persistent) | HTTPS service that executes LLM/agent steps at `POST /api/exec/{ns}/{agent}` (`cmd/agent-executor/main.go`) |
| `cmd/workflow-runner` | `ghcr.io/nirmata/ottoflow/workflow-runner` | Job pod (ephemeral, one per WorkflowRun) | Loads the Workflow + WorkflowRun, evaluates CEL, executes all step types in-process, calls agent-executor for agent steps (`cmd/workflow-runner/main.go`) |

The `Makefile` maps each to a `ko` build target
(`MANAGER_IMPORT_PATH`, `AGENT_EXECUTOR_IMPORT_PATH`,
`WORKFLOW_RUNNER_IMPORT_PATH`). `make build` produces the controller as
`bin/manager` from `cmd/controller/main.go`.

## Execution flow

Reconstructed from `cmd/controller/main.go`, `cmd/workflow-runner/main.go`,
`cmd/agent-executor/main.go`, and `cli/cmd/run.go`:

```
kubectl apply / ottoflow run   →   WorkflowRun created in cluster
        │
        ▼
Controller (Deployment)
  · validates the CRD via admission webhooks
  · reconciles WorkflowRun, resolves the DAG
  · creates a Kubernetes Job → runner pod uses --workflow-runner-image
        │
        ▼
Workflow Runner (Job pod, one per WorkflowRun)
  · loads Workflow + WorkflowRun from the API server
  · resolves the target cluster and clients (cluster.RestConfigForClusterRef)
  · executes steps in-process (CEL, resource queries, mutate, MCP, etc.)
  · for agentRef steps: POST https://<agent-executor>:8443/api/exec/{ns}/{agent}
  · persists status back to the WorkflowRun
        │
        ▼
Agent Executor (Deployment, HTTPS :8443)
  · authenticates the caller (TokenReview + SubjectAccessReview)
  · looks up the Agent CRD, builds the prompt, calls the LLM, extracts outputs
```

The workflow-runner is the **only** component that evaluates CEL and executes
steps — the controller and agent-executor do not (`DEVELOPER.md`; the executor
package lives at `internal/workflow/executor/`, imported by both
`cmd/workflow-runner/main.go` and the CLI's local executor).

Local CLI execution (`--workflow-dir`) runs the same executor package
in-process instead of creating a Job (`cli/internal/executor/local_executor.go`,
referenced from `cli/cmd/run.go`).

## Custom resources

The API group is `ottoflow.nirmata.io`, version `v1alpha1`, all namespaced
(`PROJECT`, `api/v1alpha1/`). Types are defined in `api/v1alpha1/`:

| Kind | Short name | Source | Purpose |
|---|---|---|---|
| `Workflow` | `flo` | `workflow_types.go` | Immutable template: inputs, variables, steps, outputs, triggers. Has no status — it is a reusable blueprint. |
| `WorkflowRun` | `florun` | `workflowrun_types.go` | One execution of a Workflow, with inputs and status. |
| `Agent` | `agent` | `agent_types.go` | Reusable LLM agent config: provider, model, MCP tools, output extraction. |
| `MCPServer` | `mcpserver` | `mcpserver_types.go` | An MCP server the agent/steps can call tools on. |
| `StepTemplate` | `steptemplate` | `steptemplate_types.go` | Parameterized, reusable step definition. |

CRD manifests are generated from these Go types into `config/crd/bases/`
(source of truth) and synced into `charts/ottoflow/crds/` by `make manifests`
(`Makefile`, `DEVELOPER.md`).

### Step types

A `Step` (`api/v1alpha1/workflow_types.go`) selects exactly one action in
addition to its `expressions`/`outputs`. The available actions are:

- `resourceQuery` — query Kubernetes resources (single Get or list) and extract
  outputs via CEL.
- `agentRef` — run an LLM agent step against an `Agent` CRD.
- `mcpToolCall` — call an MCP tool directly (no LLM).
- `workflowRef` — run another Workflow as a sub-workflow.
- `prometheusQuery` — run a PromQL query with variable substitution.
- `mutate` — patch a single resource (Kyverno-style `ApplyConfiguration` or
  `JSONPatch`).
- `stepTemplateRef` — instantiate a `StepTemplate`.
- `forEach` — run a child step in parallel over a list.
- `externalAgentRef` — call an external A2A-compatible agent service.
- `openReport` — emit results as an OpenReports.io `Report`.
- `waitForCallback` — pause the run and wait for an external callback
  (human-in-the-loop).

Steps support `dependsOn`, `matchConditions` (conditional execution), `retry`
with backoff, `timeout`, and `failurePolicy` (`Fail`/`Continue`).

### Triggers

A Workflow may declare triggers that auto-create WorkflowRuns
(`api/v1alpha1/workflow_types.go`, `Trigger`):

- `cron` — scheduled, with timezone and concurrency policy.
- `event` — react to Kubernetes resource events, with label/field selectors,
  CEL filter, and input mapping.
- `webhook` — HMAC-signed HTTP POST at `/webhooks/{namespace}/{workflowName}`,
  with optional CEL filter, input mapping, dedup, and rate limiting.

## Source layout

Verified against the working tree:

```
api/v1alpha1/               CRD Go types + generated deepcopy/OpenAPI
cmd/
  controller/               controller binary (main.go)
  agent-executor/           agent-executor binary (main.go)
  workflow-runner/          runner binary (main.go)
internal/
  workflow/
    controller/             reconcilers, cron scheduler, trigger manager,
                            callback server, webhook trigger server
    executor/               CEL evaluation + all step execution (61 .go files)
    cluster/                target-cluster client resolution
    token/                  token handling
  agent/                    Agent/LLM provider integration (+ toolloop/)
  mcp/                      (referenced by DEVELOPER.md; not present as a
                            top-level internal package in the current tree —
                            MCP client code lives under internal/agent)
  auth/                     TokenReview + SubjectAccessReview authenticator
  certmanager/              internal TLS cert bootstrap (no cert-manager)
  webhook/                  admission validators
  metrics/                  Prometheus metric registration
  tracing/                  OpenTelemetry tracer provider
  logging/                  structured logging helpers
cli/                        ottoflow CLI (cmd/, internal/executor, rbac, output, display)
config/crd/bases/           generated CRD manifests (source of truth)
charts/ottoflow/            Helm chart (templates + synced crds/)
samples/workflows/          example workflows
```

> Note: `DEVELOPER.md` describes an `internal/controller/` and `internal/mcp/`
> layout, but the current tree places controllers under
> `internal/workflow/controller/` and MCP code under `internal/agent/`. The
> paths above reflect the actual tree.

## Controller internals

From `cmd/controller/main.go`, the controller manager wires up:

- **`WorkflowReconciler`** and **`WorkflowRunReconciler`**
  (`internal/workflow/controller`) — reconcile the two CRDs; the run reconciler
  holds the `RunnerConfig` used to build runner Jobs.
- **Cron scheduler** — `NewScheduler`, added to the manager so it only fires on
  the elected leader.
- **Callback server** — HTTP server on `:8084` for `waitForCallback` steps
  (`/api/v1/workflow-runs/.../callback/...`), leader-elected.
- **Trigger manager** — dynamic-client watcher for event/webhook triggers.
- **Webhook trigger server** — optional HTTP server enabled via
  `--webhook-trigger-addr` (chart default `:8083`), leader-elected.
- **Validating admission webhooks** — for `Workflow`, `WorkflowRun`, `Agent`,
  and `MCPServer` (`internal/webhook`), served over TLS bootstrapped by the
  **internal cert manager** (`internal/certmanager`) — no external cert-manager
  required.
- **Metrics** (`:8080`) and **health/readiness probes** (`:8081`).
- Optional **OpenTelemetry tracing** (`internal/tracing`).

## Agent executor internals

From `cmd/agent-executor/main.go`: an HTTPS-only server (TLS ≥ 1.2) on port
`8443` (`--tls-port`), serving `/api/exec/` behind an authentication middleware.
The authenticator (`internal/auth`) uses Kubernetes `TokenReview` and
`SubjectAccessReview` — only callers allowed to `get` the
`agent-executor-caller` ConfigMap in the caller namespace may invoke it. It
constructs an MCP client manager and an `OttoFlowAgentExecutor`
(`internal/workflow/executor`) to run agent steps. Health probes are served on
`/healthz` and `/readyz`; pprof is available only when `--profile` is set.
