### Installation

#### Prerequisites

- Kubernetes 1.20+
- Helm 3.0+ (for Helm installation)

#### Installing OttoFlow

**Option A: Via Helm (recommended)**

Use `helm upgrade --install` so installs and upgrades both work:

From OCI registry (recommended, works for public and private repos):
```bash
helm upgrade --install ottoflow oci://ghcr.io/nirmata/ottoflow --version <version> --namespace ottoflow --create-namespace
```

From local chart:
```bash
helm upgrade --install ottoflow ./charts/ottoflow --namespace ottoflow --create-namespace
```

Use `--namespace <name>` to install into a different namespace.

If you see "already exists" errors from a previous partial install, uninstall first:
```bash
helm uninstall ottoflow -n ottoflow
# Then run the install command again
```

**Option B: Via generated manifests**

```bash
make deploy
```

Or manually:
```bash
make generate-manifests
kubectl apply -f config/generated/install.yaml
```

See the [Helm Chart README](../../../charts/ottoflow/README.md) for chart options and the [Configuration Reference](../reference/configuration.md) for all flags and environment variables.

#### Nirmata LLM credentials (optional)

> **Note (provider required):** `Agent.spec.modelProvider` is a required field with no default — every Agent must set it explicitly. Valid values are `nirmata`, `openai`, `anthropic`, `azure-openai`, `google`, `gemini`, and `local`. `nirmata` is served by the **OttoFlow enterprise plugin** and is **not available in the open-source build**; selecting it there returns a clear error asking you to pick a supported provider. The open-source build supports `openai`, `anthropic`, `azure-openai`, `google`/`gemini`, and `local`. The Nirmata LLM credentials below apply only when running with the enterprise plugin.

For **in-cluster** workflow runs that use Nirmata LLM (agent steps), configure credentials securely using a Kubernetes Secret. Create the secret in the same namespace as your WorkflowRuns (or a namespace the runner can reference):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: nirmata-llm-credentials
  namespace: ottoflow   # or the namespace where you create WorkflowRuns
type: Opaque
stringData:
  NIRMATA_LLM_TOKEN: "<your-token>"   # preferred (single key)
  # Or legacy keys:
  # NIRMATA_LLM_SERVICEACCOUNT_TOKEN: "<your-token>"
  # NIRMATA_LLM_APIKEY: "<your-api-key>"
  # NIRMATA_URL: "https://nirmata.io"   # optional
```

Reference the secret so the workflow-runner pod receives it as environment variables (never put secrets in plain `value`; use `valueFrom.secretKeyRef`).

**Option A – Workflow (recommended for cron-triggered runs)**  
Add `spec.execution` to the **Workflow**. The scheduler copies it into every WorkflowRun it creates (e.g. from cron), so all runs use the secret:

```yaml
# In your Workflow YAML (e.g. cloud-cost-daily)
spec:
  execution:
    job:
      env:
        - name: NIRMATA_LLM_TOKEN
          valueFrom:
            secretKeyRef:
              name: nirmata-llm-credentials
              key: NIRMATA_LLM_TOKEN
        # Or legacy keys: NIRMATA_LLM_SERVICEACCOUNT_TOKEN / NIRMATA_LLM_APIKEY (same secret, different key)
  triggers:
    - cron: ...
  steps: ...
```

Ensure the Secret exists in the **same namespace** as the Workflow (and thus the WorkflowRun).

**Option B – WorkflowRun (for one-off or manually created runs)**  
Add `spec.execution` to the **WorkflowRun** when creating it:

```yaml
spec:
  execution:
    job:
      env:
        - name: NIRMATA_LLM_TOKEN
          valueFrom:
            secretKeyRef:
              name: nirmata-llm-credentials
              key: NIRMATA_LLM_TOKEN
```

For the **CLI** (`ottoflow run`), set the same variable names as environment variables in your shell (e.g. `export NIRMATA_LLM_TOKEN="<your-token>"`). Do not commit tokens to source control.

**Updating the secret to use the new key (`NIRMATA_LLM_TOKEN`)** — use one of these approaches.

1. **Patch the existing secret** (add or replace key in place; replace `NAMESPACE` and token value):

```bash
kubectl patch secret nirmata-llm-credentials -n NAMESPACE -p '{"stringData":{"NIRMATA_LLM_TOKEN":"<your-token>"}}'
```

2. **Replace the secret** (e.g. from a file or literal; base64-encode if using `data`):

```bash
# From literal (stringData avoids base64)
kubectl create secret generic nirmata-llm-credentials \
  --from-literal=NIRMATA_LLM_TOKEN="<your-token>" \
  -n NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
```

3. **Update Workflows/WorkflowRuns** so `spec.execution.job.env` references the new key (see Option A/B above: use `name: NIRMATA_LLM_TOKEN` and `secretKeyRef.key: NIRMATA_LLM_TOKEN`). New runs will then use the new key; no controller or executor restart is required (the executor reads env at runtime in the runner pod).
