# OttoFlow CLI

A command-line tool for running OttoFlow workflows — either locally (in-process) or in a Kubernetes cluster.

Use `--workflow-dir` to load and run workflows from a local directory without a cluster. Without `--workflow-dir`, the CLI creates `WorkflowRun` resources in the cluster; the OttoFlow controller and workflow-runner execute workflows in-cluster. Use `--watch` to wait for completion or `--watch=false` to create and exit.

## Installation

Build from the project root:

```bash
# Build CLI binary
make build-cli

# Build for all platforms
make build-cli-all

# Install to $GOPATH/bin
make install-cli

# Install to /usr/local/bin (requires sudo)
make install-cli-local
```

Or build manually:
```bash
go build -o bin/ottoflow ./cli/main.go
```

## Usage

### Run Local Workflows

Run a workflow locally without a cluster — load workflows from a directory and execute in-process:

```bash
# Run a named workflow from a directory
ottoflow run simple-greeting --workflow-dir samples/workflows

# Run with input values
ottoflow run simple-greeting --workflow-dir samples/workflows --input name="World"
```

Use `--workflow-dir` to point at any directory containing Workflow YAML files. Pass the workflow name as a positional argument to run a specific workflow, or omit it to run all workflows in the directory.

### Create and Run a Workflow in the Cluster

Create a WorkflowRun in the cluster (controller and workflow-runner must be running to execute):

```bash
# Create WorkflowRun by workflow name (Workflow must exist in cluster)
ottoflow run simple-greeting --input name="World"

# Create without watching
ottoflow run my-workflow --watch=false --input name=value

# Output format when watching
ottoflow run my-workflow --output json --input key=value
```

**Key Features:**
- **Local execution**: Use `--workflow-dir` to load workflows from a directory and run in-process (no cluster required)
- **Cluster execution**: Creates `WorkflowRun`; controller runs the workflow in a Job
- **Optional watch**: Use `--watch=false` to create and exit, or default watch until completion

### Get Workflow Status

Get the status of a workflow run:

```bash
ottoflow status my-workflow-run-1234567890
```

Get status in JSON format:

```bash
ottoflow status my-workflow-run-1234567890 --output json
```

Get status in YAML format:

```bash
ottoflow status my-workflow-run-1234567890 --output yaml
```

## Global Flags

- `--namespace, -n`: Kubernetes namespace (default: "default")
- `--kubeconfig`: Path to kubeconfig file (defaults to $HOME/.kube/config or $KUBECONFIG)

## Commands

### `ottoflow run`

Execute a workflow.

**Flags:**
- `--workflow, -w`: Name of the workflow to execute
- `--workflow-dir`: Load workflows from a directory and run locally (in-process); if set, cluster path is not used
- `--input, -i`: Input values as key=value pairs (can be specified multiple times)
- `--timeout`: Maximum time to wait for workflow completion (default: "10m")
- `--watch, -W`: Watch workflow execution progress (default: true, cluster mode only)
- `--output, -o`: Output format: table, json, yaml (default: "table")
- `--max-workers`: Max concurrent workers for forEach steps (local mode only, default: 5)
- `--prometheus-url`: Prometheus server URL for CEL/prometheus steps (local mode only)

**Examples:**
```bash
# Run a local workflow from a directory (no cluster required)
ottoflow run simple-greeting --workflow-dir samples/workflows --input name="World"

# Run with inline inputs (cluster mode)
ottoflow run my-workflow --input pod-name=nginx --input namespace=default

# Run without watching (cluster mode)
ottoflow run my-workflow --watch=false
```

### `ottoflow status`

Get the status of a workflow run.

**Flags:**
- `--output, -o`: Output format: table, json, yaml (default: "table")

**Examples:**
```bash
# Get status
ottoflow status my-workflow-run-1234567890

# Get status in JSON
ottoflow status my-workflow-run-1234567890 -o json
```

## Output Formats

### Table Format (default)

Displays a formatted table with:
- WorkflowRun metadata (name, namespace, phase)
- Step statuses with phases and messages
- Workflow outputs

### JSON Format

Outputs the workflow status as JSON, suitable for scripting and automation.

### YAML Format

Outputs the workflow status as YAML, matching the Kubernetes resource format.

## Environment Variables

- `OTTOFLO_NAMESPACE`: Default namespace to use (overridden by --namespace flag)
- `KUBECONFIG`: Path to kubeconfig file (overridden by --kubeconfig flag)

## Examples

### Basic Workflow Execution

```bash
# Create a WorkflowRun (Workflow must exist in cluster, e.g. kubectl apply -R -f samples/)
ottoflow run simple-greeting --input name=World

# Check status of a run
ottoflow status <workflowrun-name>
```

### Workflow with Multiple Inputs

```bash
ottoflow run pod-diagnostics-workflow \
  --input pod-name=nginx \
  --input namespace=default \
  --timeout 5m
```

### Watch Workflow Progress

```bash
ottoflow run my-workflow --watch
```

The CLI will continuously poll and display the workflow status until completion or timeout.
