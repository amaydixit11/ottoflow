# Design: OTel GenAI Semantic Conventions — Workflow Observability

**Date**: June 2026  
**Status**: Implemented (v1.0.0)

---

## Problem

OttoFlow produces no OpenTelemetry signal. There is no trace hierarchy, no per-step timing, and no token usage metrics across LLM agent calls, MCP tool invocations, Kubernetes resource queries, or CEL expression evaluation.

The architectural constraint is that execution is split across a process boundary. The controller (long-lived `manager` process) reconciles WorkflowRuns and creates Jobs; the actual workflow logic runs inside a short-lived `workflow-runner` pod. A trace started in the controller is in a different OS process than the execution it spawns. W3C TraceContext solves cross-process trace propagation via the `traceparent` and `tracestate` values — but in a Kubernetes Job, the only channel from controller to runner at creation time is the Job spec (env vars, volumes, annotations). Env vars are the correct channel: the runner already reads all its configuration from env vars, and reading its own pod annotations would require a self-describing Kubernetes GET with additional RBAC.

---

## Design

### Trace Bridge: Controller → Job

When `buildWorkflowRunnerJob` assembles the runner pod spec, it serializes the current span context into two env vars using `propagation.MapCarrier`:

```
TRACEPARENT=00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
TRACESTATE=
```

The runner extracts these at `main()` startup, recreates the remote span context, and starts the root `invoke_workflow` span as a child. Every subsequent span inside the runner is a descendant of this span, which is itself a child of the controller's reconcile span. The result is a single continuous trace from trigger through all step execution to completion.

```
Controller (manager process)
  └─ [reconcile span]
       │  injects TRACEPARENT env var
       ▼
  Job Pod (workflow-runner process)
       └─ invoke_workflow  [INTERNAL, WorkflowRun lifetime]
            ├─ step: fetchPolicies       [expression/resourceQuery — internal]
            ├─ invoke_agent: analyze     [INTERNAL, AgentRef]
            │    └─ chat                 [CLIENT, each LLM turn]
            ├─ execute_tool: apply       [CLIENT, MCPToolCall]
            └─ invoke_agent: notify      [CLIENT, ExternalAgentRef / A2A]
```

The runner initializes a TracerProvider with an OTLP gRPC exporter before any workflow execution and defers `ForceFlush` + `Shutdown` to ensure all spans are exported before the Job pod exits. When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, the TracerProvider is a no-op — zero overhead for users without a collector.

### Span Hierarchy: OTel GenAI SemConv

The [OTel GenAI Agent Spans spec](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/) defines span types that map directly onto OttoFlow's step taxonomy:

| OTel span operation | OttoFlow step type | Span kind |
|---|---|---|
| `invoke_workflow` | WorkflowRun execution (root) | INTERNAL |
| `invoke_agent` | `AgentRef` (in-process LLM agent) | INTERNAL |
| `invoke_agent` | `ExternalAgentRef` (remote A2A call) | CLIENT |
| `execute_tool` | `MCPToolCall` (direct MCP invocation) | CLIENT |
| `chat` | Each LLM turn within an agent | CLIENT |

**As-built span hierarchy** (all steps get a `step.*` INTERNAL wrapper; GenAI steps add a child span):

```
Controller (manager process)
  └─ workflow_run.reconcile    [INTERNAL]
       │  injects TRACEPARENT/TRACESTATE env vars into Job spec
       ▼
Runner Job (workflow-runner process)
  └─ invoke_workflow            [INTERNAL, WorkflowRun lifetime]
       ├─ step.fetchPolicies    [INTERNAL, non-GenAI steps]
       ├─ step.analyze          [INTERNAL wrapper]
       │    └─ invoke_agent     [INTERNAL, AgentRef]
       ├─ step.apply            [INTERNAL wrapper]
       │    └─ execute_tool     [CLIENT, MCPToolCall]
       └─ step.notify           [INTERNAL wrapper]
            └─ invoke_agent     [CLIENT, ExternalAgentRef]
```

Every step type (including GenAI) gets a `step.<stepName>` INTERNAL wrapper from `executor.go`. This gives a consistent timing layer in waterfall views. GenAI executors start their specific child span (`invoke_agent`, `execute_tool`) inside the wrapper. Non-GenAI steps (`expression`, `resourceQuery`, `mutate`, `forEach`, `prometheusQuery`, `workflowRef`) are leaf spans — the wrapper IS their span.

**Key attributes on `invoke_workflow`**:

| Attribute | Value |
|---|---|
| `gen_ai.system` | `"ottoflow"` |
| `gen_ai.workflow.name` | `workflowRun.Spec.WorkflowRef.Name` |
| `workflow.run.name` | `workflowRun.Name` |
| `workflow.run.namespace` | `workflowRun.Namespace` |

**Key attributes on `invoke_agent` / `chat`**:

| Attribute | Value |
|---|---|
| `gen_ai.system` | LLM provider (e.g. `"aws.bedrock"`, `"anthropic"`) |
| `gen_ai.request.model` | Model ID |
| `gen_ai.conversation.id` | Conversation UUID |
| `gen_ai.usage.input_tokens` | Input token count for this turn |
| `gen_ai.usage.output_tokens` | Output token count for this turn |
| `gen_ai.usage.cache_read.input_tokens` | Cache-read token count (if provider reports it) |

**Key attributes on `execute_tool`**:

| Attribute | Value |
|---|---|
| `gen_ai.tool.name` | MCP tool name |
| `mcp.server.name` | MCPServer resource name |
| `mcp.transport.type` | `stdio` / `http` / `sse` |

### OOMKill Resilience

The `invoke_workflow` span buffer lives in-memory in the runner pod. If the pod is OOMKilled, the OTLP export never fires and the trace is orphaned. The design compensates with a structured log event at each step boundary:

```
klog.InfoS("step completed",
    "step", stepName,
    "type", stepType,
    "durationMs", elapsed.Milliseconds(),
    "inputTokens", usage.InputTokens,
    "outputTokens", usage.OutputTokens,
    "status", "succeeded")
```

Pod logs are captured by the node's log driver independently of the process exiting cleanly. Even when the trace is orphaned, the structured log events provide a partial audit trail with step-level timing and token counts, and they are queryable via `kubectl logs` or any log aggregation pipeline.

### TracerProvider Initialization

The runner initializes the TracerProvider after extracting the trace context and before constructing the executor:

```go
// Init is a no-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset.
tp, shutdown, err := tracing.InitTracerProvider(ctx, "workflow-runner")
if err != nil {
    klog.ErrorS(err, "failed to init tracer provider, continuing without traces")
    tp = trace.NewNoopTracerProvider()
    shutdown = func(context.Context) error { return nil }
}
defer shutdown(ctx)
```

`tracing.InitTracerProvider` (a new package `internal/tracing`) reads `OTEL_EXPORTER_OTLP_ENDPOINT` from the environment. When unset, it returns a no-op provider immediately. When set, it initializes an OTLP gRPC exporter with the standard OTel SDK batch processor and sets the service resource attributes.

The controller also initializes a TracerProvider on startup (same package) so the reconcile span it creates before injecting `TRACEPARENT` has a real trace ID to propagate.

---

## API Changes

None. No CRD schema changes are required. All configuration uses standard OTel environment variables:

| Variable | Purpose | Default |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector gRPC endpoint | unset (no-op) |
| `OTEL_SERVICE_NAME` | Service name override | `workflow-runner` / `ottoflow-controller` |
| `OTEL_RESOURCE_ATTRIBUTES` | Additional resource attributes | unset |

The `OTEL_EXPORTER_OTLP_ENDPOINT` variable can be injected into runner pods via the existing `spec.execution.job.env` mechanism or via the well-known LLM credentials secret pattern, giving operators full control without any new CRD fields.

### Endpoint URL Format

The OTLP gRPC exporter requires a **full URL** with scheme, not just `host:port`. Without a scheme the Go URL parser treats the hostname as the scheme and produces an empty address:

```
# Correct — http:// signals insecure HTTP/2 (no TLS)
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.monitoring.svc.cluster.local:4317

# Wrong — produces "invalid target address: missing address"
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector.monitoring.svc.cluster.local:4317
```

Use `https://` for collectors with TLS; `http://` for in-cluster collectors without TLS (the standard case).

### NetworkPolicy

The Helm chart's controller `NetworkPolicy` restricts egress to a known-good port allowlist. Port 4317 is included in `charts/ottoflow/values.yaml` under `networkPolicy.egress`. If you disable or override the NetworkPolicy, ensure egress to TCP 4317 is permitted for the controller pod.

Runner Job pods are not governed by the controller's NetworkPolicy and can reach the collector without additional policy changes.

---

## Implementation

### New Package: `internal/tracing`

A single file exposing `InitTracerProvider(ctx, serviceName) (trace.TracerProvider, func(context.Context) error, error)`. Handles OTLP gRPC exporter init, batch span processor, and resource attribute setup. Returns a no-op provider when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset. Both the controller and runner import this package.

### Files Changed

| File | Change |
|---|---|
| `internal/tracing/tracing.go` | New package: `InitTracerProvider`, no-op fast-path when endpoint unset; always registers W3C propagator |
| `internal/tracing/semconv.go` | New file: GenAI + OttoFlow attribute key constants (`gen_ai.system`, `gen_ai.tool.name`, `workflow.step.type`, etc.) |
| `cmd/controller/main.go` | Init TracerProvider on startup; set global `otel.SetTracerProvider`; defer flush |
| `internal/workflow/controller/workflowrun_controller.go` | `reconcileJobExecution`: start `workflow_run.reconcile` span; `buildWorkflowRunnerJob`: serialize span context into `TRACEPARENT`/`TRACESTATE` env vars via `propagation.MapCarrier` |
| `cmd/workflow-runner/main.go` | Extract trace context from env; init TracerProvider; start root `invoke_workflow` span (adds `gen_ai.workflow.name` after CRD load); defer flush |
| `internal/workflow/executor/executor.go` | Wrap every step dispatch in `step.<name>` INTERNAL span with `workflow.step.type` attribute; `stepType()` helper |
| `internal/workflow/executor/agent_executor.go` | Emit `invoke_agent` INTERNAL span; attrs: provider, model (token usage deferred — gollm doesn't expose it yet) |
| `internal/workflow/executor/external_agent_executor.go` | Emit `invoke_agent` CLIENT span for A2A calls; attrs: `gen_ai.system=a2a`, URL |
| `internal/workflow/executor/mcp_executor.go` | Emit `execute_tool` CLIENT span with tool name and server attributes |
| `internal/agent/executor.go` | Emit `chat` span per LLM turn with explicit ERROR/OK status |
| `charts/ottoflow/values.yaml` | Add port 4317 OTLP egress rule to `networkPolicy.egress` |
| `go.mod` | Promote `go.opentelemetry.io/otel`, `otlptracegrpc`, `otel/trace`, `otel/sdk` from indirect to direct |
