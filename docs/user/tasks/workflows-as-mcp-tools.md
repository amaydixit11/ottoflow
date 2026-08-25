# Serving Workflows as MCP Tools

OttoFlow can serve the Workflows in a cluster as **MCP tools**, so an agent framework
can call a workflow the way it calls any other tool. This is the inbound direction:
[MCP Servers and MCP Tool Calls](mcp-servers-and-tools.md) covers the outbound one,
where a workflow step calls someone else's MCP server.

The controller is the MCP **client** there and the MCP **server** here. Both can be on
at once; they share nothing but the protocol.

## What a caller sees

One tool per Workflow, named `<namespace>__<workflow>`:

- `tools/list` enumerates the Workflows the server can see, with each workflow's inputs
  as the tool's input schema.
- `tools/call` creates a WorkflowRun, waits for it, and returns its outputs.

The tool set follows the cluster. A Workflow created a second ago is callable; one that
was deleted stops being listed, rather than remaining as a tool whose call would fail.

## Enabling it

```bash
helm upgrade --install ottoflow ./charts/ottoflow \
  --namespace ottoflow --create-namespace \
  --set controller.mcp.enabled=true
```

That serves `/mcp` on `:8084` and creates a `ottoflow-mcp` Service. It is off by
default: an endpoint that runs workflows is not something to open without asking.

The server speaks plain HTTP, like the webhook trigger. Terminate TLS at an ingress or a
service mesh sidecar if callers are outside the cluster.

## Authorizing callers

A caller presents a ServiceAccount token as `Authorization: Bearer <token>`. The server
runs a **TokenReview** to establish who that is, then a **SubjectAccessReview** asking
whether that identity may `get` the `mcp-caller` ConfigMap. If it may, the call proceeds.

So "who may invoke workflows" is a RoleBinding:

```bash
kubectl create clusterrolebinding kagent-ottoflow-mcp-caller \
  --clusterrole=ottoflow-mcp-caller \
  --serviceaccount=kagent:kagent
```

There is no shared secret and nothing to rotate; revoking access is deleting the
binding. This is the same model the agent-executor uses, with a separate ClusterRole:
calling the agent executor and running any workflow in the cluster are different grants,
and one role for both would make the narrower one impossible to give out.

> The ConfigMap's contents are never read. The permission is the whole signal.

## Describing a workflow for a model

`WorkflowSpec` has no description field, so the tool description comes from an
annotation:

```yaml
apiVersion: ottoflow.nirmata.io/v1alpha1
kind: Workflow
metadata:
  name: namespace-report
  annotations:
    ottoflow.nirmata.io/description: >-
      Summarize the workloads running in a Kubernetes namespace: how many pods,
      which are unhealthy, and what images they run.
```

Without it the tool still lists, with a description generated from the workflow's name.
That is enough for a human reading `tools/list` and thin for a model choosing between
tools — the annotation is the difference between a workflow that gets called at the
right moment and one that does not.

## Inputs

Every tool input is a **string**, because `Workflow.spec.inputs` has no type: an input is
a name, a description, an optional default, and a required flag. `inputValues` on a
WorkflowRun is `map[string]string` on the way in, so a richer JSON-schema type here would
describe a contract nothing downstream enforces.

- An input marked `required` with no `default` must be supplied, or the call is rejected
  before a run is created.
- A `default` is published in the schema and repeated in the description, since the model
  reads the description.
- An argument the workflow does not declare is **rejected**, not ignored. A misspelled
  input would otherwise run the default and read as the workflow quietly disregarding
  what was asked.

## Results

A run that succeeds returns its outputs as JSON, keeping their structure:

```json
{
  "workflowRun": "namespace-report-a1b2-0f3c8d21",
  "namespace": "ottoflow",
  "outputs": { "podCount": 12 }
}
```

A run that fails returns a tool error carrying the run's status message. A run that is
still going when the call's five-minute deadline passes returns a tool error too — and
in every one of those cases the WorkflowRun's name is in the message, because the run is
still executing and the name is what lets you go look at it:

```text
workflow run namespace-report-a1b2-0f3c8d21 did not finish within 5m0s and is still
running; read its status with: kubectl -n ottoflow get workflowrun namespace-report-a1b2-0f3c8d21
```

Runs created this way are labelled `ottoflow.nirmata.io/trigger=mcp`, so they are
greppable apart from cron, event, and webhook runs:

```bash
kubectl get workflowruns -A -l ottoflow.nirmata.io/trigger=mcp
```

A workflow's `maxConcurrentRuns` applies here exactly as it does to every other trigger.

## Connecting kagent

kagent consumes any MCP URL through its `RemoteMCPServer` CRD. A complete bundle —
the ClusterRoleBinding, the `RemoteMCPServer`, a described Workflow, and an Agent wired
to the tool — is in
[`samples/kagent-workflows-as-tools/`](../../../samples/kagent-workflows-as-tools/remote-mcpserver.yaml).

```yaml
apiVersion: kagent.dev/v1alpha2
kind: RemoteMCPServer
metadata:
  name: ottoflow-workflows
  namespace: kagent
spec:
  protocol: STREAMABLE_HTTP
  url: http://ottoflow-mcp.ottoflow.svc.cluster.local:8084/mcp
```

## Limits

- **The call waits.** `tools/call` polls to completion under a deadline; there is no
  fire-and-forget mode that returns a run id immediately. Workflows that routinely run
  for longer than the deadline are a poor fit for a synchronous tool call today.
- **Authorization is coarse.** The check is "may this identity invoke workflows", not
  "may this identity invoke *this* workflow". A caller that is allowed in can call every
  workflow the server lists.
- **No TLS of its own.** Terminate it in front of the Service.
