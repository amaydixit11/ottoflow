# Proposal: Context Window Budget Management for Agent Steps

**Status**: Draft  
**Issue**: [nirmata/ottoflow-enterprise#90](https://github.com/nirmata/ottoflow-enterprise/issues/90)

## Problem

`ContextManager` accumulates every agent step's full LLM response string in
`inMemoryContext["steps"]` for the lifetime of the workflow. When a later agent step evaluates
`additionalPrompts` CEL expressions that reference prior step outputs (e.g.,
`steps.step1.response`), those strings are materialized into the assembled prompt string and
sent to the LLM. In long workflows this silently inflates the prompt past the LLM's context
window, causing degraded or truncated output — while WorkflowRun status still reports `Succeeded`.

**Concrete worst case**: A 10-step workflow where each agent step produces 50 KB of policy YAML.
By step 10, `inMemoryContext["steps"]` holds ~500 KB of response strings. If additionalPrompts
references all prior steps, the assembled prompt sent to the LLM exceeds 500 KB before the LLM
even receives its base instruction.

**Existing partial mitigation**: `StepAgentRef.MaxAdditionalPromptTokens` truncates the
assembled additional prompts, but (a) truncation happens *after* CEL materializes the full
context into memory — peak allocation is not reduced; (b) it uses a 4 runes/token heuristic
that underestimates real token counts for YAML/code content by 20–33%; (c) it emits no user
signal when truncation occurs, so the workflow silently degrades.

**Correct interception point**: Between `ReadContext()` and `BuildVariableMap()` at
`agent_executor.go:70-76` — before CEL strings are materialized. Any approach acting later
still pays the full materialization cost.

## Options Considered

### Option A: Fix existing MaxAdditionalPromptTokens (no new API)

Tighten the existing mechanism without adding new fields:

1. Fix the token estimation multiplier: `* 4` → `* 3` (`agent_executor.go:99`) — improves
   accuracy for YAML/code content from ±33% over-budget to ±10%.
2. Emit `klog.V(2).InfoS("additionalPrompts truncated", ...)` when truncation occurs.

**Pros**: Zero API surface change; addresses the silent failure.  
**Cons**: Does not prevent the context from being assembled in the first place. CEL still
materializes the full context before truncation. Requires per-step opt-in to
`MaxAdditionalPromptTokens` on every agent step.  
**Decision**: Necessary hygiene but insufficient on its own. These fixes are included in Option C.

---

### Option B: Response-omit mode (strip raw response, keep outputs)

Add `OmitStepResponses: bool` to `StepAgentRef`. When set, `response` strings are removed from
all prior steps before CEL evaluation; only `outputs` (extracted key-value pairs) remain.

**Why targeted**: The large data per step is `response` (full LLM output, 10–100 KB). The small
data is `outputs` (extracted key-value pairs, typically <1 KB). Downstream steps that need a
specific value from a prior step should configure `outputExtraction` rather than referencing
`steps.stepName.response` directly.

**Pros**: Eliminates peak materialization cost for the dominant production pattern. Simple bool.  
**Cons**: Cannot help when a step legitimately needs `steps.prev.response`.  
**Decision**: Valid but subsumed by Option C's `omit` mode. Not added as a separate field.

---

### Option C: ContextBudgetMode enum + completion ordering (implemented)

Add a `ContextBudgetMode` field to `StepAgentRef` with three strategies, applied before
`BuildVariableMap` at the correct interception point:

- `full` (default, empty): pass all of `contextData` unchanged — current behavior, zero risk
- `lastN`: include only the N most recently completed steps in `steps`; drop older entries
- `omit`: strip the entire `steps` map; only `inputs` and `expressions` visible to CEL

Requires tracking step completion order via `completionOrder []string` on `ContextManager`,
appended in the main dispatch loop after each successful `executeStep`. This is safe because
the main loop is strictly sequential (`executor.go:484-531`) — no goroutines, no mutex needed.

**Pros**: Complete solution covering all use cases. Backward compatible (empty = `full`). Clean
per-step control. Integrates naturally with existing `additionalPrompts` pattern.  
**Cons**: Requires user awareness to opt in; `full` mode still has the problem by default.  
**Decision**: Implemented (see below). Combined with Option A fixes.

---

### Option D: Workflow-level automatic budget (deferred)

Add a `contextBudget` field at the `Workflow` spec level that auto-prunes oldest steps when the
estimated token count exceeds a threshold — no per-step annotation required.

**Pros**: Zero annotation burden; good for 50-step CVE remediation pipelines.  
**Cons**: Dynamic pruning is hard to reason about. A step receiving empty data where it expected
prior outputs is a worse failure mode than receiving oversized context. Token estimation
inaccuracies (20–33% for YAML) make the threshold unreliable. Requires WorkflowRun conditions
to surface every pruning event to avoid silent data loss.  
**Decision**: Deferred. Implement after Option C is proven in production and token estimation
is improved.

---

### Option E: LLM-based summary compression (deferred)

On step completion, run a secondary LLM call to summarize the response; store only the summary.
Accessing `steps.stepName.response` returns the summary.

**Pros**: Preserves semantic richness while reducing token usage.  
**Cons**: Secondary LLM call per step adds latency and cost proportional to workflow length.  
**Decision**: Deferred to a future iteration.

---

## Implementation (Option C + A)

### New fields on StepAgentRef

```go
// ContextBudgetMode controls how much of the accumulated step context is visible to
// CEL evaluation when evaluating additionalPrompts. Applied before BuildVariableMap,
// so no materialization cost is paid for pruned entries.
//
// full: all step outputs visible (default when omitted — preserves current behavior)
// lastN: only the N most recently completed steps are visible; inputs and expressions unaffected
// omit: step outputs are hidden; only inputs and expressions are visible
//
// +kubebuilder:validation:Enum=full;lastN;omit
// +optional
ContextBudgetMode string `json:"contextBudgetMode,omitempty"`

// ContextBudgetLastN is the number of most-recently-completed steps to include when
// ContextBudgetMode=lastN. Ignored for other modes. Defaults to 5 when omitted.
// +optional
ContextBudgetLastN *int32 `json:"contextBudgetLastN,omitempty"`
```

These follow the existing flat-enum pattern established by `RetryPolicy.Backoff.Strategy` and
`StepForEach.ItemFailurePolicy`.

### Completion ordering

Add to `ContextManager` (`context.go`):

```go
completionOrder []string  // step names in execution-completion order
```

```go
// RecordStepCompletion appends stepName to the completion order for use by context budget strategies.
func (cm *ContextManager) RecordStepCompletion(stepName string) {
    cm.completionOrder = append(cm.completionOrder, stepName)
}

func (cm *ContextManager) CompletionOrder() []string {
    return cm.completionOrder
}
```

Call `RecordStepCompletion` in `executor.go` immediately after a successful `executeStep` return
(main dispatch loop, around line 531). No mutex needed — the loop is sequential.

### Budget application (new file: `context_budget.go`)

```go
package executor

import ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"

func applyContextBudget(
    contextData map[string]interface{},
    agentRef *ottoflowv1alpha1.StepAgentRef,
    completionOrder []string,
) map[string]interface{} {
    switch agentRef.ContextBudgetMode {
    case "lastN":
        n := 5
        if agentRef.ContextBudgetLastN != nil && *agentRef.ContextBudgetLastN > 0 {
            n = int(*agentRef.ContextBudgetLastN)
        }
        return applyLastNBudget(contextData, n, completionOrder)
    case "omit":
        return applyOmitBudget(contextData)
    default:
        return contextData
    }
}
```

`applyLastNBudget`: shallow-copy top-level map + `steps` map; delete keys for all steps not in
`completionOrder[max(0, len(completionOrder)-n):]`.

`applyOmitBudget`: shallow-copy top-level map; replace `steps` with an empty map.

Both helpers copy only the maps they modify — shared sub-map references (e.g., individual step
data maps) are not duplicated, keeping allocation minimal.

### Interception in agent_executor.go

```go
// Read current context for prompt evaluation
contextData, err := e.contextManager.ReadContext(ctx)
if err != nil {
    return nil, fmt.Errorf("failed to read context: %w", err)
}

// Apply context budget before CEL sees the data (prevents materialization of pruned entries)
if agentRef.ContextBudgetMode != "" && agentRef.ContextBudgetMode != "full" {
    contextData = applyContextBudget(contextData, agentRef, e.contextManager.CompletionOrder())
}

// Build variable map for CEL evaluation
vars := e.celEvaluator.BuildVariableMap(contextData)
```

### Token estimation fix

```go
// agent_executor.go:99 — was * 4, underestimates real token count for YAML/code by 20-33%
tokenBudget := *agentRef.MaxAdditionalPromptTokens * 3
```

### Truncation signal

```go
if int32(utf8.RuneCountInString(additionalText)) > tokenBudget {
    runes := []rune(additionalText)
    additionalText = string(runes[:tokenBudget]) + "..."
    klog.V(2).InfoS("additionalPrompts truncated by MaxAdditionalPromptTokens",
        "step", step.Name, "budgetTokens", *agentRef.MaxAdditionalPromptTokens)
}
```

### Example usage

```yaml
# Step that only needs context from the 3 most recent prior steps:
- name: summarize-violations
  agentRef:
    name: policy-analyzer
    contextBudgetMode: lastN
    contextBudgetLastN: 3
    additionalPrompts:
      - '"Analyze violations from recent steps:\n" + steps.scanViolations.response'

# Step that builds its own prompt and doesn't need any step history:
- name: generate-report
  agentRef:
    name: report-writer
    contextBudgetMode: omit
    additionalPrompts:
      - '"Generate a report for namespace: " + inputs.namespace'
```

## Files Changed

| File | Change |
|---|---|
| `internal/workflow/executor/context.go` | Add `completionOrder []string`, `RecordStepCompletion`, `CompletionOrder` |
| `internal/workflow/executor/executor.go` | Call `RecordStepCompletion` after each successful `executeStep` |
| `api/v1alpha1/workflow_types.go` | Add `ContextBudgetMode` + `ContextBudgetLastN` to `StepAgentRef` |
| `internal/workflow/executor/context_budget.go` | New — `applyContextBudget`, `applyLastNBudget`, `applyOmitBudget` |
| `internal/workflow/executor/agent_executor.go` | Wire interception; fix `*4`→`*3`; add truncation log |
| `config/crd/bases/` + `charts/ottoflow/crds/` | Auto-updated via `make manifests` |
