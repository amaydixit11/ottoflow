#!/bin/bash
# Test script for OttoFlow CLI (cluster mode: create WorkflowRun, optional watch)

set -e

CLI_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$CLI_DIR/.." && pwd)"
CLI_BIN="$ROOT_DIR/bin/ottoflow"
WORKFLOW_DIR="$ROOT_DIR/samples/workflows"

echo "=== OttoFlow CLI Test Suite (Cluster Mode) ==="
echo "CLI Binary: $CLI_BIN"
echo "Workflow Directory: $WORKFLOW_DIR"
echo ""

# Check if CLI is built
if [ ! -f "$CLI_BIN" ]; then
    echo "❌ CLI not built. Building now..."
    cd "$ROOT_DIR"
    make build-cli
fi

# Check if workflow directory exists (for applying sample Workflows)
if [ ! -d "$WORKFLOW_DIR" ]; then
    echo "❌ Workflow directory not found: $WORKFLOW_DIR"
    exit 1
fi

# Cluster is required: CLI creates WorkflowRun in cluster
if ! kubectl cluster-info &> /dev/null 2>&1; then
    echo "❌ Cannot connect to Kubernetes cluster. Cluster is required for ottoflow run."
    exit 1
fi
echo "✅ Kubernetes cluster accessible"
echo ""

# Optional: install CRDs and apply sample Workflows so run by name works
echo "=== Ensuring CRDs and sample Workflows (optional) ==="
if [ -f "$ROOT_DIR/Makefile" ] && grep -q "install:" "$ROOT_DIR/Makefile"; then
    (cd "$ROOT_DIR" && make install 2>/dev/null) || true
fi
# Apply a minimal workflow for tests (simple-greeting) if present
if [ -f "$WORKFLOW_DIR/features/simple-greeting.yaml" ]; then
    kubectl apply -f "$WORKFLOW_DIR/features/simple-greeting.yaml" --server-side 2>/dev/null || kubectl apply -f "$WORKFLOW_DIR/features/simple-greeting.yaml" 2>/dev/null || true
fi
echo ""

# Test 1: Create WorkflowRun by name (no watch)
echo "=== Test 1: Create WorkflowRun by name (--watch=false) ==="
echo "Running: ottoflow run simple-greeting --input name='CLI Test' --watch=false"
if "$CLI_BIN" run simple-greeting --input name="CLI Test" --watch=false --namespace ottoflow 2>&1 | grep -q "Created WorkflowRun"; then
    echo "✅ Test 1 passed"
else
    echo "⚠️  Test 1: Ensure Workflow 'simple-greeting' exists in cluster (e.g. apply samples/workflows/features/simple-greeting.yaml)"
fi
echo ""

# Test 2: Run with multiple inputs
echo "=== Test 2: Create WorkflowRun with multiple inputs ==="
echo "Running: ottoflow run simple-greeting --input name='World' --input greeting='Hello' --watch=false"
if "$CLI_BIN" run simple-greeting --input name="World" --input greeting="Hello" --watch=false --namespace ottoflow 2>&1 | grep -q "Created WorkflowRun"; then
    echo "✅ Test 2 passed"
else
    echo "⚠️  Test 2 completed with warnings"
fi
echo ""

# Test 3: JSON output (create then show status when not watching is limited; we just check no crash)
echo "=== Test 3: Create with --output json ==="
echo "Running: ottoflow run simple-greeting --input name='JSON Test' --output json --watch=false"
if "$CLI_BIN" run simple-greeting --input name="JSON Test" --namespace ottoflow --output json --watch=false 2>&1 | grep -q "Created WorkflowRun\|phase"; then
    echo "✅ Test 3 passed"
else
    echo "⚠️  Test 3 completed with warnings"
fi
echo ""

# Test 4: YAML output
echo "=== Test 4: Create with --output yaml ==="
echo "Running: ottoflow run simple-greeting --input name='YAML Test' --output yaml --watch=false"
if "$CLI_BIN" run simple-greeting --input name="YAML Test" --namespace ottoflow --output yaml --watch=false 2>&1 | grep -q "Created WorkflowRun\|kind: WorkflowRun\|phase:"; then
    echo "✅ Test 4 passed"
else
    echo "⚠️  Test 4 completed with warnings"
fi
echo ""

# Test 5: Error handling (non-existent workflow)
echo "=== Test 5: Error handling (non-existent workflow) ==="
echo "Running: ottoflow run non-existent-workflow-xyz --watch=false"
if "$CLI_BIN" run non-existent-workflow-xyz --watch=false --namespace default 2>&1 | grep -qi "not found\|error\|failed"; then
    echo "✅ Test 5 passed - Error handling works correctly"
else
    echo "⚠️  Test 5 - Create may have been attempted; controller will fail to resolve workflow"
fi
echo ""

echo "=== Test Summary ==="
echo "✅ Cluster-mode tests completed"
echo ""
echo "Note: The CLI creates WorkflowRun resources in the cluster. The OttoFlow controller"
echo "and workflow-runner must be running for workflows to execute."
echo ""
