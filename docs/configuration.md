# Configuration

This page lists the flags, environment variables, and Helm values that
configure each component. Everything here is taken from the flag/env
definitions in source and from the Helm chart's `values.yaml`.

## CLI (`ottoflow`)

### Global flags

Defined in `cli/cmd/root.go`:

| Flag | Default | Description |
|---|---|---|
| `--namespace, -n` | current kubeconfig context namespace, then `ottoflow` | Kubernetes namespace |
| `--kubeconfig` | `$HOME/.kube/config` (via default loading rules, which honor `$KUBECONFIG`) | Path to kubeconfig |

When `--kubeconfig` is unset the CLI uses the standard client-go loading rules,
then falls back to in-cluster config (`cli/cmd/run.go`, `getKubeConfig`).

### `ottoflow run`

Defined in `cli/cmd/run.go`:

| Flag | Default | Description |
|---|---|---|
| `--workflow, -w` | — | Name of the workflow to execute |
| `--workflow-dir` | — | Load workflows from a directory and run **locally** (in-process). If set, the cluster path is not used. |
| `--input, -i` | — | `key=value` input pairs (repeatable) |
| `--timeout` | `10m` | Max time to wait for completion (cluster watch) |
| `--watch, -W` | `true` | Watch execution progress (cluster mode only) |
| `--output, -o` | `table` | Output format: `table`, `json`, `yaml` |
| `--include-inputs` | `false` | Include `spec.inputValues` in json/yaml output (may contain secrets) |
| `--max-workers` | `5` | Max concurrent workers for `forEach` steps (local mode only) |
| `--prometheus-url` | — | Prometheus URL for CEL/prometheus steps (local mode only) |
| `--output-dir` | — | Save run output (JSON + Markdown) to a directory |
| `--provider` | — | Override LLM provider for all agent steps (local mode only) |
| `--model` | — | Override LLM model for all agent steps (local mode only) |

`--provider`/`--model` only apply to local mode; in cluster mode the CLI warns
and ignores them (`cli/cmd/run.go`).

### `ottoflow status`

Defined in `cli/cmd/status.go`: `--output, -o` (`table`/`json`/`yaml`, default
`table`) and `--include-inputs` (default `false`).

### `ottoflow validate`

Defined in `cli/cmd/validate.go`:

| Flag | Default | Description |
|---|---|---|
| `--file, -f` | — | Load a workflow from a YAML file |
| `--workflow-dir` | — | Load workflows from a directory (local, no cluster) |
| `--generate-rbac` | `false` | After validation passes, generate RBAC manifests |
| `--output` | stdout | Write generated RBAC to a file (only with `--generate-rbac`) |
| `--agent-executor-namespace` | `ottoflow` | Namespace of the agent-executor Service (for agentRef RBAC rules) |

## Controller

Flags from `cmd/controller/main.go`. Where a flag reads an environment variable
for its default, the env var is shown; the "Default" column is the effective
default (from the flag definition or its help text).

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--metrics-bind-address` | — | `:8080` | Metrics endpoint address |
| `--health-probe-bind-address` | — | `:8081` | Health/readiness probe address |
| `--leader-elect` | — | `true` | Enable leader election (set `false` for local dev only) |
| `--namespace` | — | `ottoflow` | Namespace for the leader-election lease |
| `--metrics-secure` | — | `false` | Serve metrics over HTTPS |
| `--enable-http2` | — | `false` | Enable HTTP/2 on metrics/webhook servers |
| `--cel-cache-size` | — | `1000` | Max compiled CEL expressions cached |
| `--prometheus-url` | — | — | Prometheus URL for CEL `prometheusMetrics()` |
| `--agent-executor-service-name` | `AGENT_EXECUTOR_SERVICE_NAME` | — | Agent-executor Service name for internal TLS cert controller |
| `--agent-executor-namespace` | `AGENT_EXECUTOR_NAMESPACE` | `ottoflow` | Namespace for agent-executor TLS cert controller |
| `--workflow-runner-image` | `WORKFLOW_RUNNER_IMAGE` | `ghcr.io/nirmata/ottoflow/workflow-runner:latest` | Image for runner Jobs |
| `--workflow-runner-service-account` | `WORKFLOW_RUNNER_SERVICE_ACCOUNT` | empty → derived `{workflow}-runner` | Runner Job service account |
| `--workflow-runner-cluster-role` | `WORKFLOW_RUNNER_CLUSTER_ROLE` | required (controller refuses to start if unset); the Helm chart sets `<fullname>-runner-role` (narrowed, runner-only role) | ClusterRole for runner Job RBAC |
| `--agent-executor-caller-cluster-role` | `AGENT_EXECUTOR_CALLER_CLUSTER_ROLE` | — (empty disables) | ClusterRole for agent-executor caller RBAC |
| `--workflow-runner-agent-executor-ca-secret` | — | — (empty disables) | Secret with agent-executor CA for runner TLS |
| `--secret-source-namespace` | — | workflow namespace | Namespace to copy runner Secret volumes from when missing |
| `--workflow-runner-image-pull-secrets` | `WORKFLOW_RUNNER_IMAGE_PULL_SECRETS` | — | Comma-separated imagePullSecret names for runner pods |
| `--workflow-runner-image-pull-policy` | `WORKFLOW_RUNNER_IMAGE_PULL_POLICY` | `IfNotPresent` | Runner container imagePullPolicy |
| `--workflow-runner-pod-labels-part-of` | `WORKFLOW_RUNNER_POD_LABELS_PART_OF` | `ottoflow` | Value for runner pod `app.kubernetes.io/part-of` label |
| `--workflow-runner-ttl-seconds-after-finished` | — | `0` (= 3600) | Seconds before finished runner Jobs are deleted |
| `--workflow-runner-llm-credentials-secret` | `WORKFLOW_RUNNER_LLM_CREDENTIALS_SECRET` | — (empty disables) | Secret for LLM credential injection into runner Jobs |
| `--webhook-trigger-addr` | — | — (empty disables) | Address for the webhook-trigger HTTP server |

Additional controller env vars (read directly, not exposed as flags,
`cmd/controller/main.go`): `WEBHOOK_SERVICE_NAME` (default `ottoflow-webhook`)
and `WEBHOOK_CONFIG_NAME` (default `ottoflow-validating`).

The runner image can be changed without redeploying the controller by editing
the `--workflow-runner-image` arg — subsequent WorkflowRuns pick it up
(`DEVELOPER.md`).

## Agent executor

Flags from `cmd/agent-executor/main.go`:

| Flag | Default | Description |
|---|---|---|
| `--tls-port` | `8443` | HTTPS server port |
| `--agent-executor-caller-namespace` | `ottoflow` | Namespace checked by the SubjectAccessReview auth (`get configmaps/agent-executor-caller`) |
| `--profile` | `false` | Enable pprof endpoint — never enable in production |
| `--profiler-port` | `6060` | pprof HTTP port (only with `--profile`) |

TLS material is read from `/etc/tls/tls.crt` and `/etc/tls/tls.key`
(`cmd/agent-executor/main.go`).

## Workflow runner

Flags/env from `cmd/workflow-runner/main.go`. The runner is normally launched by
the controller, which sets these on the Job pod:

| Flag | Env var | Description |
|---|---|---|
| `--workflow-run-name` | `WORKFLOW_RUN_NAME` | WorkflowRun name (required) |
| `--workflow-run-namespace` | `WORKFLOW_RUN_NAMESPACE` | WorkflowRun namespace (required) |
| `--prometheus-url` | — | Prometheus URL for CEL `prometheusMetrics()` |
| `--job-name` | `JOB_NAME` | Runner pod identity (required) |
| `--pod-name` | `POD_NAME` | Runner pod identity (required) |

When no Prometheus URL is provided, the runner attempts in-cluster
auto-discovery of a Prometheus Service and probes it for cAdvisor metrics before
using it (`cmd/workflow-runner/main.go`).

## LLM provider credentials

For the **Nirmata** LLM provider (`spec.modelProvider: nirmata`; requires the
Nirmata enterprise plugin), credentials come from environment variables
(`README.md`). `spec.modelProvider` is required and has no default
(`api/v1alpha1/agent_types.go`). In-cluster, inject them via a Secret referenced in the
WorkflowRun's `spec.execution` (or the controller's
`--workflow-runner-llm-credentials-secret`); for the local CLI, export them in
your shell.

| Variable | Description |
|---|---|
| `NIRMATA_LLM_TOKEN` | Nirmata token for LLM access (preferred) |
| `NIRMATA_LLM_SERVICEACCOUNT_TOKEN` | Service account token (legacy) |
| `NIRMATA_LLM_APIKEY` | API key (legacy) |
| `NIRMATA_URL` | Nirmata endpoint (default `https://nirmata.io`) |

The executor checks `NIRMATA_LLM_TOKEN` first, then the legacy variables
(`README.md`). Non-Nirmata providers (`openai`, `anthropic`, `azure-openai`,
`google`, `gemini`, `local`) are selected per `Agent` CRD via
`spec.modelProvider`. API keys are never read from `spec.config` — they come
from the agent-executor pod's process environment, set via the Helm chart's
`agentExecutor.env` (secretKeyRef supported). `spec.config` only carries
`endpoint` (effective only for `azure-openai`) and `skipVerifySSL` (ignored by
`gemini`) (`api/v1alpha1/agent_types.go`).

## Tracing

Both the controller and runner initialize OpenTelemetry
(`internal/tracing`, `cmd/*/main.go`). The exporter endpoint is read from the
standard `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable (grep of
`os.Getenv` across the tree). The runner also honors `TRACEPARENT`/`TRACESTATE`
to chain spans to the controller (`cmd/workflow-runner/main.go`).

## Helm chart values

Key values from `charts/ottoflow/values.yaml` (see that file and
`charts/ottoflow/README.md` for the full set):

| Value | Default | Description |
|---|---|---|
| `global.imageRegistry` | `ghcr.io/nirmata/ottoflow` | Base image registry |
| `global.imagePullPolicy` | `IfNotPresent` | Default pull policy |
| `controller.replicaCount` | `1` | Controller replicas |
| `controller.args` | `[--leader-elect]` | Controller flags |
| `controller.namespace` | `""` (release namespace) | Leader-election namespace (`--namespace`) |
| `controller.webhookTrigger.enabled` | `true` | Pass `--webhook-trigger-addr` |
| `controller.webhookTrigger.addr` | `:8083` | Webhook-trigger listen address |
| `webhook.enabled` | `true` | Validating admission webhooks (internal CA) |
| `webhook.failurePolicy` | `Ignore` | Webhook failure policy |
| `webhook.prometheusURL` | `""` | Prometheus URL passed to runner Jobs |
| `agentExecutor.enabled` | `true` | Deploy the agent-executor |
| `agentExecutor.service.port` | `8443` | Agent-executor HTTPS port |
| `agentExecutor.goMemLimit` | `800MiB` | `GOMEMLIMIT` soft memory limit |
| `agentExecutor.tls.internal.enabled` | `true` | Provision self-signed TLS via internal controller |
| `workflowRunner.imagePullPolicy` | `IfNotPresent` | Runner Job pull policy |
| `workflowRunner.ttlSecondsAfterFinished` | `0` (= 3600) | Runner Job TTL |
| `workflowRunner.llmCredentialsSecret` | `""` | Secret for LLM credential injection |
| `rbac.create` | `true` | Create RBAC resources |
| `networkPolicy.create` | `true` | Create a NetworkPolicy (DNS, agent-executor, API, HTTPS egress, Prometheus, OTLP) |
| `crds.install` | `true` | Install CRDs with the chart |

## Build-time variables (Makefile)

From `Makefile`:

| Variable | Default | Purpose |
|---|---|---|
| `KO_DOCKER_REPO` | `ghcr.io/nirmata/ottoflow` | Target registry for `ko` image builds |
| `IMAGE_TAG` | git tag, else `0.0.0-g<short-sha>` | Image tag for `ko-build`/`ko-push` |
| `IMG` / `WORKFLOW_RUNNER_IMG` / `AGENT_EXECUTOR_IMG` | — | Image overrides for `make generate-manifests`/`deploy` |
| `HELM_CHART_PATH` | `./charts/ottoflow` | Chart location |
| `HELM_RELEASE_NAME` | `ottoflow` | Release name |
| `HELM_NAMESPACE` | `ottoflow` | Install namespace |
| `ENVTEST_K8S_VERSION` | `1.29.0` | Kubernetes test-binary version for `make test` |
