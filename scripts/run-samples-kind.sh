#!/usr/bin/env bash
# Run sample workflows against a local kind cluster.
# Creates kind cluster if missing, installs CRDs, builds CLI, then runs workflows.
# For prometheusQuery steps to succeed, set PROMETHEUS_URL or pass --prometheus-url.

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
CLI_BIN="$ROOT_DIR/bin/ottoflow"
WORKFLOW_DIR="$ROOT_DIR/samples/workflows"

echo "=== OttoFlow samples on Kind ==="
echo "Root: $ROOT_DIR"
echo "Kind cluster: $KIND_CLUSTER"
echo ""

# Ensure kind cluster exists
if ! kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
  echo "Creating kind cluster: $KIND_CLUSTER"
  kind create cluster --name "$KIND_CLUSTER"
else
  echo "Kind cluster '$KIND_CLUSTER' already exists"
fi

# Use kind context
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
kubectl config use-context "kind-${KIND_CLUSTER}" || true

# Install CRDs
echo ""
echo "Installing CRDs..."
make install

# Build CLI
echo ""
echo "Building CLI..."
make build-cli

if [ ! -f "$CLI_BIN" ]; then
  echo "CLI binary not found at $CLI_BIN"
  exit 1
fi

echo ""
echo "Running workflows (workflow-dir=$WORKFLOW_DIR)"
echo ""

# 1. Basic workflow (no Prometheus)
echo "--- 1. basic-test ---"
"$CLI_BIN" run basic-test --workflow-dir "$WORKFLOW_DIR" --namespace default --input message="Kind" --input number="1" -o table || true
echo ""

# 2. Resource-query (may fail if default/test-pod does not exist)
echo "--- 2. resource-query-example ---"
"$CLI_BIN" run resource-query-example --workflow-dir "$WORKFLOW_DIR" --namespace default -o table || true
echo ""

# 3. Prometheus-query-example: step 1 (listPods) succeeds; step 2 (queryCpuUsage) needs Prometheus
echo "--- 3. prometheus-query-example ---"
echo "  (Step 'listPods' runs; step 'queryCpuUsage' requires --prometheus-url if no Prometheus)"
if [ -n "$PROMETHEUS_URL" ]; then
  "$CLI_BIN" run prometheus-query-example --workflow-dir "$WORKFLOW_DIR" --namespace default --input namespace=default --prometheus-url "$PROMETHEUS_URL" -o table || true
else
  "$CLI_BIN" run prometheus-query-example --workflow-dir "$WORKFLOW_DIR" --namespace default --input namespace=default -o table || true
fi
echo ""

# 4. Prometheus-query-simple (single prometheusQuery step; fails without Prometheus)
echo "--- 4. prometheus-query-simple ---"
if [ -n "$PROMETHEUS_URL" ]; then
  "$CLI_BIN" run prometheus-query-simple --workflow-dir "$WORKFLOW_DIR" --namespace default --prometheus-url "$PROMETHEUS_URL" -o table || true
else
  "$CLI_BIN" run prometheus-query-simple --workflow-dir "$WORKFLOW_DIR" --namespace default -o table || true
fi

echo ""
echo "Done. Set PROMETHEUS_URL for prometheusQuery steps to succeed."
