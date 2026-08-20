# Logging

OttoFlow uses **structured logging** in the controller and workflow runner so you can filter and correlate logs by workflow, run, step, and phase. This document describes the log fields, log levels, and recommended scraping and aggregation.

---

## Structured log fields

All workflow-related log lines from the controller and runner include these keys when applicable:

| Key | Description | Example |
|-----|-------------|---------|
| `workflow` | Workflow template name (`workflowRef.name`) | `my-pipeline` |
| `workflowRun` | WorkflowRun resource name | `my-pipeline-1734567890` |
| `namespace` | Namespace of the WorkflowRun | `default` |
| `step` | Step name (for step-level logs) | `deploy` |
| `phase` | Run or step phase | `succeeded`, `failed`, `running` |

Use these fields in your log backend to:

- **Filter** by workflow or run: `workflow=my-pipeline`, `workflowRun=my-pipeline-1734567890`
- **Correlate** controller and runner logs for the same run: `workflowRun=...` and `namespace=...`
- **Debug** a specific step: `step=deploy` and `workflowRun=...`

---

## Log levels

The **controller**, **workflow runner**, and **agent-executor** all use [klog](https://github.com/kubernetes/klog). Verbosity is controlled by the `-v` flag (e.g. `-v=0`, `-v=2`).

| Level | Meaning |
|-------|--------|
| `0` (default) | Important events only (lifecycle, errors, key decisions) |
| `2` | Extra detail (e.g. reconciliation, trigger registration, agent step progress) |
| `4` | Very verbose (e.g. mutate step details, metric skips, internal state) |

In production, `-v=0` is usually sufficient. Use `-v=2` for troubleshooting; use `-v=4` only for deep debugging.

---

## Recommended scraping and aggregation

### JSON output

For production, emit logs in **JSON** so aggregators can index the structured keys. Both controller and runner use klog; configure klog for your environment (e.g. `-logtostderr=false` with a JSON sink, or rely on your log collector to parse the default format). The standard keys (`workflow`, `workflowRun`, `namespace`, `step`, `phase`) appear in log output for correlation. For runner pods, the controller sets `WORKFLOW_RUN_NAME` and `WORKFLOW_RUN_NAMESPACE` in the environment so a log pipeline can add or correlate `workflow` / `workflowRun` if needed.

### Log aggregation

1. **Collect** stdout/stderr from the controller Deployment and from the workflow runner Job pods (e.g. Fluent Bit, Fluentd, or cluster logging).
2. **Index** the structured keys (`workflow`, `workflowRun`, `namespace`, `step`, `phase`) as dimensions.
3. **Query** examples:
   - All logs for a run: `workflowRun=<run-name> AND namespace=<ns>`
   - All errors for a workflow: `workflow=<name> AND (level=error OR msg=*failed*)`
   - Step-level logs: `step=* AND workflowRun=<run-name>`

### Correlation with metrics

Use the same labels as in [Metrics](metrics.md): `workflow`, `namespace`, and (for steps) `step`. Logs and Prometheus metrics can be correlated by `workflow` + `namespace` + `workflowRun` (logs) vs `workflow` + `namespace` (metrics).

---

## See also

- [Metrics](metrics.md) – Prometheus metrics and scraping
- [Configuration](configuration.md) – Controller and runner flags
