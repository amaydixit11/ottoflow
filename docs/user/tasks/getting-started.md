# Getting Started

This guide walks you through creating your first OttoFlow workflow.

## Prerequisites

- Kubernetes cluster (1.29+)
- `kubectl` configured to access your cluster
- OttoFlow CRDs installed (see [Installation](installation.md))

## Your First Workflow

Let's create a simple workflow that greets a user:

```yaml
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: hello-world
  namespace: default
spec:
  inputs:
    - name: name
      description: "Name to greet"
      required: true
  steps:
    - name: greet
      message: "Generate greeting message"
      expressions:
        - name: greeting
          expression: '"Hello, " + inputs.name + "!"'
      outputs:
        - name: message
          expression: 'expressions.greeting'
  outputs:
    - name: result
      expression: 'variables.message'
```

## Running the Workflow

### Option 1: Using kubectl

1. **Create the workflow:**
   ```bash
   kubectl apply -f hello-world.yaml
   ```

2. **Create a WorkflowRun:**
   ```bash
   kubectl apply -f - <<EOF
   apiVersion: ottoflow.nirmata.io/v1alpha1
   kind: WorkflowRun
   metadata:
     name: hello-world-run
     namespace: default
   spec:
     workflowRef:
       name: hello-world
     inputValues:
       name: "OttoFlow"
   EOF
   ```

3. **Check the status:**
   ```bash
   kubectl get workflowrun hello-world-run
   ```

4. **View the result:**
   ```bash
   kubectl get workflowrun hello-world-run -o jsonpath='{.status.outputs.result}'
   ```

### Option 2: Using the CLI

```bash
# Build CLI
make build-cli

# Create a WorkflowRun (Workflow must exist in cluster)
./bin/ottoflow run hello-world --input name="OttoFlow"
```

### Option 3: Local execution with `-f`, no cluster needed

`-f`/`--file` runs the manifest directly, in-process, without ever creating a Workflow or
WorkflowRun in a cluster:

```bash
# From the saved file
./bin/ottoflow run -f hello-world.yaml --input name="OttoFlow"

# Or piped in on stdin
cat hello-world.yaml | ./bin/ottoflow run -f - --input name="OttoFlow"
```

The namespace used is whatever `metadata.namespace` the Workflow declares (`default` in
the manifest above). `--namespace` only matters when the loaded manifest contains more than
one workflow (e.g. the same name in different namespaces) -- it then disambiguates which one
to run.

## Understanding the Workflow

### Inputs

The workflow defines one input parameter:
- `name`: Required string input

### Steps

The workflow has one step:
- **Name**: `greet`
- **Expressions**: Evaluates a CEL expression to create a greeting
- **Outputs**: Stores the greeting in `variables.message`

### Outputs

The workflow exposes one output:
- `result`: References `variables.message` from the step

## Next Steps

- Learn about the different [step types](../reference/api/workflow.md#step-types-one-per-step), including [resource queries](../reference/api/workflow.md#resourcequery)
- Set up [triggers](triggers.md) to run workflows automatically
- See [sample workflows](../../../samples/workflows/) for more examples
