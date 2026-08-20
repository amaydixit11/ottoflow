# Defining Custom Metrics

Workflows can publish custom Prometheus metrics by adding a `metric` field to workflow-level outputs. The output's evaluated value becomes the metric value, and metric metadata (name, type, labels) is declared inline.

## Overview

- **Where**: Add `metric` to any output in `spec.outputs`
- **When**: Metrics are emitted at workflow completion, after outputs are evaluated
- **Value**: The output's expression result is the metric value
- **Prefix**: All custom metrics are prefixed with `ottoflow_workflow_`

## Output Metric Configuration

```yaml
outputs:
  - name: myValue
    expression: "variables.count"
    metric:
      name: my_metric_total      # Final name: ottoflow_workflow_my_metric_total
      type: counter              # counter | gauge | histogram
      help: "Description for Prometheus"
      labels:
        - name: workflow
          value: '"my-workflow"'           # CEL expression (string)
        - name: namespace
          value: "string(inputs.namespace)" # CEL expression
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Metric name (invalid chars replaced with `_`). Prefixed with `ottoflow_workflow_`. |
| `type` | Yes | `counter`, `gauge`, or `histogram` |
| `help` | No | Metric description (default: "Custom workflow metric") |
| `labels` | No | Label key-value pairs. `value` is a CEL expression (must evaluate to string). |
| `buckets` | No | Histogram buckets (only for `type: histogram`). Default: `[0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300]` |

### Label Expressions

Label `value` fields are CEL expressions. You can reference:

- `inputs.<name>` – Workflow inputs
- `variables.<name>` – Variables and step outputs
- `outputs.<name>` – Earlier outputs in the same list

Labels `workflow` and `namespace` are always added automatically.

## Metric Types

### Counter

Use for values that only increase (e.g., total count of events).

```yaml
outputs:
  - name: errorCount
    expression: "variables.errors"
    metric:
      name: errors_total
      type: counter
      help: "Total errors detected"
```

### Gauge

Use for values that can go up or down (e.g., current count, ratio).

```yaml
outputs:
  - name: podCount
    expression: "size(variables.pods)"
    metric:
      name: pods_count
      type: gauge
      help: "Number of pods in namespace"
      labels:
        - name: namespace
          value: "string(inputs.namespace)"
```

### Histogram

Use for distributions (e.g., latencies, sizes). Value can be a single number or a list of observations.

```yaml
outputs:
  - name: durations
    expression: "variables.stepDurations"
    metric:
      name: step_durations_seconds
      type: histogram
      help: "Step execution durations"
      buckets: [0.1, 0.5, 1, 2, 5]
```

## Complete Example

```yaml
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: pod-health-metrics
spec:
  inputs:
    - name: namespace
      default: "default"
  steps:
    - name: countPods
      resourceQuery:
        apiVersion: v1
        resource: Pod
        namespace: "inputs.namespace"
        outputs:
          totalCount: "size(items)"
  outputs:
    - name: totalPods
      expression: "variables.totalCount"
      metric:
        name: total_pods
        type: gauge
        help: "Total number of pods in the namespace"
        labels:
          - name: workflow
            value: '"pod-health-metrics"'
          - name: namespace
            value: "string(inputs.namespace)"
```

## Derived Metrics

To publish a metric from multiple step outputs, define an output that computes the value:

```yaml
outputs:
  - name: unhealthyCount
    expression: "variables.unhealthyPods"
  - name: totalCount
    expression: "variables.allPods"
  - name: unhealthyRatio
    expression: "double(outputs.unhealthyCount) / double(outputs.totalCount)"
    metric:
      name: unhealthy_ratio
      type: gauge
      help: "Ratio of unhealthy to total pods"
```

## Best Practices

1. **Cardinality**: Avoid high-cardinality labels (e.g., pod names, UUIDs). Prefer bounded values like namespace, workflow name, or environment.
2. **Errors**: If metric emission fails (e.g., invalid value type), the workflow still completes. Check controller logs at `-v=4` for metric errors.
3. **Naming**: Use snake_case for metric names. Invalid characters are replaced with `_`.
