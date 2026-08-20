# OttoFlow API Reference

This section contains the Custom Resource Definition (CRD) reference for OttoFlow, generated from the OpenAPI schemas. For an overview of the API design, see [Design - API](../../../dev/DESIGN.md#api-design).

## Custom Resources

| Resource | Description |
|----------|-------------|
| [Workflow](workflow.md) | Immutable workflow template defining steps, inputs, and optional triggers. |
| [WorkflowRun](workflowrun.md) | Execution instance of a Workflow; holds input values and execution status. |
| [Agent](agent.md) | Reusable AI agent configuration for workflow steps (prompt, model, MCP tools). |
| [MCPServer](mcpserver.md) | MCP (Model Context Protocol) server connection configuration. |
| [StepTemplate](steptemplate.md) | Reusable step definition instantiated with parameters. |

## API Group and Version

- **Group:** `ottoflow.nirmata.io`
- **Version:** `v1alpha1`

All resources are **namespaced** unless otherwise noted.

## Quick Reference

```yaml
# Workflow (template)
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: my-workflow
  namespace: default
spec:
  inputs: []
  steps: []
  outputs: []

# WorkflowRun (instance)
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: WorkflowRun
metadata:
  name: my-run
  namespace: default
spec:
  workflowRef:
    name: my-workflow
  inputValues: {}

# Agent
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Agent
metadata:
  name: my-agent
  namespace: default
spec:
  prompt: ""
  modelProvider: openai
  modelName: ""

# MCPServer
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: MCPServer
metadata:
  name: my-mcp
  namespace: default
spec:
  transport:
    type: stdio
    command: []

# StepTemplate
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: StepTemplate
metadata:
  name: my-template
  namespace: default
spec:
  parameters: []
  step: {}
```

## Generated API reference (Markdown)

You can generate a single-file Markdown API reference from the Go API types using [elastic/crd-ref-docs](https://github.com/elastic/crd-ref-docs) (same approach as [openreports/reports-api](https://github.com/openreports/reports-api)):

```bash
make codegen-api-docs
```

This writes [api-docs.md](api-docs.md). The tool is installed automatically when you run the target.

## Schema Source

The authoritative schemas are the CRD manifests under `config/crd/bases/`. The markdown reference is derived from those schemas for readability.
