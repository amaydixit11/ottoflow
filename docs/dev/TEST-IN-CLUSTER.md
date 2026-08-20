# Testing OttoFlow changes in cluster

Use this when you have **ottoflow** already running and want to update it with your latest build and verify (e.g. receive a Slack report).

## Images to build

The chart uses **three** container images. Build all of them so controller, workflow-runner Jobs, and agent-executor use your code:

| Image            | Make target              | Used by |
|------------------|--------------------------|--------|
| **controller**   | `ko-build-manager`       | Controller deployment (ottoflow ns) |
| **workflow-runner** | `ko-build-workflow-runner` | Jobs created by controller for each WorkflowRun |
| **agent-executor**  | `ko-build-agent-executor`  | Agent steps |

## 1. Set your registry and build + push

Use a registry your cluster can pull from (e.g. Docker Hub, GHCR, ECR).

**Image tags are semantic by default:** when you run `make ko-push`, the image tag is set automatically:

- **On a git tag** (e.g. you checked out `v0.1.0`): images are tagged with that tag, e.g. `v0.1.0`.
- **Not on a tag**: images are tagged `0.0.0-g<short-sha>` (e.g. `0.0.0-g5e797f3`).

Override by setting `IMAGE_TAG` when you run make. See `make image-version` to print the tag that will be used.

```bash
# From ottoflow repo root
export KO_DOCKER_REPO=docker.io/YOUR_USER/ottoflow   # or ghcr.io/YOUR_ORG/ottoflow, etc.

make image-version   # optional: show the tag that will be used
make ko-push
```

Images are named like `$(KO_DOCKER_REPO)/controller:$(IMAGE_TAG)` (tag is automatic; see above). Use those full URLs in the helm upgrade step below.

To see what was built (e.g. after `ko-build` without push):

```bash
docker images | grep -E "controller|workflow-runner|agent-executor"
```

If you use **Kind** and don't push to a registry, build then load:

```bash
export KO_DOCKER_REPO=ko.local
make ko-build-manager
make ko-build-workflow-runner
make ko-build-agent-executor
# Then load into kind (use the image names from the ko build output)
kind load docker-image ko.local/controller-xxx:yyy --name YOUR_CLUSTER
kind load docker-image ko.local/workflow-runner-xxx:yyy --name YOUR_CLUSTER
kind load docker-image ko.local/agent-executor-xxx:yyy --name YOUR_CLUSTER
```

## 2. Upgrade Helm release(s)

Point the chart at your new images using `fullOverride` so the controller, workflow-runner, and agent-executor use the images you just built.

**If you have one release in `ottoflow`**:

```bash
helm upgrade ottoflow ./charts/ottoflow -n ottoflow \
  --set controller.image.fullOverride=YOUR_CONTROLLER_IMAGE \
  --set workflowRunner.image.fullOverride=YOUR_WORKFLOW_RUNNER_IMAGE \
  --set agentExecutor.image.fullOverride=YOUR_AGENT_EXECUTOR_IMAGE
```

Example (use the tag from `make image-version`, e.g. `v0.1.0` or `0.0.0-dev-a1b2c3d`):

```bash
# Replace :TAG with the output of make image-version (e.g. v0.1.0 or 0.0.0-g<sha>)
helm upgrade ottoflow ./charts/ottoflow -n ottoflow \
  --set controller.image.fullOverride=ghcr.io/nirmata/ottoflow/controller:TAG \
  --set workflowRunner.image.fullOverride=ghcr.io/nirmata/ottoflow/workflow-runner:TAG \
  --set agentExecutor.image.fullOverride=ghcr.io/nirmata/ottoflow/agent-executor:TAG
# e.g. TAG = v0.1.0 or 0.0.0-g5e797f3
```

**If you use a values file**, pass it and the overrides:

```bash
helm upgrade ottoflow ./charts/ottoflow -n ottoflow \
  -f /path/to/your-values.yaml \
  --set controller.image.fullOverride=YOUR_CONTROLLER_IMAGE \
  --set workflowRunner.image.fullOverride=YOUR_WORKFLOW_RUNNER_IMAGE \
  --set agentExecutor.image.fullOverride=YOUR_AGENT_EXECUTOR_IMAGE
```

If you have a **second release** in another namespace, run a similar `helm upgrade` for that release with the same image overrides.

## 3. Restart controller and agent-executor (if needed)

After the upgrade, pods should roll with the new images. If the deployment doesn’t roll (e.g. same tag):

```bash
kubectl rollout restart deployment -n ottoflow -l app.kubernetes.io/name=ottoflow
kubectl rollout restart deployment -n ottoflow -l app.kubernetes.io/name=agent-executor
```

New WorkflowRuns will use the new **workflow-runner** image automatically (controller passes it to the Job).

## 4. Trigger a run and verify Slack

Create a WorkflowRun for a workflow that posts to Slack (or use an existing cron). Ensure the workflow has `slackWebhookUrl` set (input or Secret). Then confirm the run completes and a Slack report is received.

```bash
# Example: list runs
kubectl get workflowruns -n YOUR_NAMESPACE

# Watch a run
kubectl get workflowrun -n YOUR_NAMESPACE -w
```

If the run uses an **agent** step, the agent-executor pod will use your new agent-executor image. The **controller** and **workflow-runner** images are used for every run.
