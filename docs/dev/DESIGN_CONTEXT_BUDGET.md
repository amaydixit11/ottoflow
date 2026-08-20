# Design: Context Window Budget Management for Agent Steps

**Status**: Design  
**Issue**: Tracked internally  
**Proposal**: [PROPOSAL_CONTEXT_BUDGET.md](PROPOSAL_CONTEXT_BUDGET.md)

---

## Overview

Agent steps in long workflows silently inflate their LLM prompt because
`ContextManager.inMemoryContext["steps"]` accumulates every prior step's full response string
without bound. When an `additionalPrompts` CEL expression dereferences a prior step's output
(e.g., `steps.generatePolicy.response`), the entire accumulated string is materialized into the
assembled prompt — with no warning, no WorkflowRun status signal, and no size guard before
materialization.

This design adds a `ContextBudgetMode` strategy field to `StepAgentRef` that controls which
portion of the accumulated step context is visible to CEL evaluation for a given agent step.
The budget is applied between `ReadContext()` and `BuildVariableMap()` — the only point where
filtering prevents materialization cost entirely. Two companion fixes are included: correcting
the `MaxAdditionalPromptTokens` rune/token estimation ratio and emitting a log line when that
field truncates.

---

## Root Cause

### How context accumulates

`ContextManager.inMemoryContext` is initialized once and never trimmed:

```go
// context.go:76-81
cm.inMemoryContext = map[string]interface{}{
    "inputs":      params,
    "expressions": make(map[string]interface{}),
    "variables":   make(map[string]interface{}),
    "steps":       make(map[string]interface{}),
}
```

When an agent step completes, `writeStepResponseToContext` (agent_executor.go:190-214) writes
the full LLM response string into `inMemoryContext["steps"][stepName]["response"]`. There is no
eviction, TTL, or size limit. A 10-step workflow where each agent step produces 50 KB of output
accumulates ~500 KB in the `steps` map alone before the final step runs.

### How it reaches the LLM

The exact path from stored context to LLM input:

```
executeAgentStep (agent_executor.go:70)
  contextData := contextManager.ReadContext(ctx)     // returns shallow copy of inMemoryContext
  vars := celEvaluator.BuildVariableMap(contextData) // converts map to CEL-accessible variables
  for _, tpl := range agentRef.AdditionalPrompts {
      evaluatedPrompt := celEvaluator.EvaluateExpression(ctx, tpl, vars)
      // tpl = `"Analyze:\n" + steps.step1.response`
      // → entire 50 KB response string is now in evaluatedPrompt
  }
  promptStr = base + "\n\n" + join(evaluated prompts)  // assembled string sent to LLM
  initializedConv.Stream(ctx, []interface{}{promptStr}) // executor.go:162 — LLM receives it
```

`contextData` itself is not sent to the LLM; the evaluated `promptStr` is. But if any
`additionalPrompts` template references `steps.*`, the entire referenced response string is
embedded in `promptStr` before the LLM call.

### Why existing guards are insufficient

`MaxAdditionalPromptTokens` (agent_executor.go:95-107) truncates the assembled
`additionalText` string, but it operates on the fully materialized string — **after** CEL has
already expanded all `steps.*` references into it. The peak memory allocation and CEL evaluation
cost are unchanged; truncation only prevents the oversized string from being forwarded to the
LLM. It also uses a `* 4` rune/token multiplier that underestimates real token counts for
YAML and code content by 20–33%.

### Why the main loop makes this safe to fix

The step dispatch loop in `executor.go:484-531` is strictly sequential: `readySteps` is iterated
with a plain `for` loop, and each `executeStep` call is synchronous — no goroutines. This means:

1. No concurrent writes to `inMemoryContext["steps"]` from the main loop.
2. Step completion order is deterministic and can be recorded with a simple append — no mutex
   needed.
3. Any `completionOrder []string` appended in the loop is safe to read during the next step's
   `executeAgentStep` call.

---

## Design

### 1. New fields on StepAgentRef

```go
// api/v1alpha1/workflow_types.go — appended after MaxAdditionalPromptTokens (line 409)

// ContextBudgetMode controls how much of the accumulated step context is visible to
// CEL evaluation when evaluating additionalPrompts. Applied before BuildVariableMap,
// so no materialization cost is paid for entries that are filtered out.
//
// full: all step outputs are visible (default when omitted — preserves current behavior)
// lastN: only the N most recently completed step outputs are visible
// omit: no step outputs are visible
//
// Only the step outputs are filtered. Inputs, variables, and expressions remain fully
// visible in every mode.
//
// +kubebuilder:validation:Enum=full;lastN;omit
// +optional
ContextBudgetMode string `json:"contextBudgetMode,omitempty"`

// ContextBudgetLastN is the number of most-recently-completed steps to include when
// ContextBudgetMode=lastN. Steps beyond the last N are omitted from CEL context.
// Ignored for other modes. Defaults to 5 when omitted or zero.
// +optional
ContextBudgetLastN *int32 `json:"contextBudgetLastN,omitempty"`
```

**Backward compatibility**: `ContextBudgetMode` is `omitempty` with no `+kubebuilder:default`.
Empty string is treated as `"full"` in the executor — all existing workflows that omit this
field behave exactly as before. No CRD migration required.

**Pattern alignment**: This follows the established flat-enum pattern used by
`StepForEach.ItemFailurePolicy` (line 773) and `BackoffConfig.Strategy` (line 886) — a
`string` field with a `+kubebuilder:validation:Enum` marker and an optional sibling `*int32`
for the numeric parameter.

### 2. Completion order tracking in ContextManager

Add to the `ContextManager` struct (`context.go`):

```go
type ContextManager struct {
    workflowRun        *ottoflowv1alpha1.WorkflowRun
    inMemoryContext    map[string]interface{}
    contextInitialized bool
    completionOrder    []string  // step names in execution-completion order
}
```

Add two methods:

```go
// RecordStepCompletion appends stepName to the completion order.
// Must be called by the executor after each successful executeStep.
func (cm *ContextManager) RecordStepCompletion(stepName string) {
    cm.completionOrder = append(cm.completionOrder, stepName)
}

// CompletionOrder returns the recorded step completion sequence.
func (cm *ContextManager) CompletionOrder() []string {
    return cm.completionOrder
}
```

**Call site** (`executor.go`, just after the successful `executeStep` return around line 531):

```go
_, err = e.executeStep(ctx, workflowRun, *step)
if err != nil {
    // ... error handling unchanged
} else {
    e.contextManager.RecordStepCompletion(stepName)
}
```

`completionOrder` accumulates skipped steps too only if the caller records them — but skipped
steps write nothing to the `steps` map, so they do not need to be tracked. Only successful
(non-skipped) step completions should be recorded. The call site must be placed after the
`Succeeded` phase assignment and before the next loop iteration.

### 3. Budget application (new file: context_budget.go)

```go
package executor

import (
    ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// applyContextBudget filters contextData according to the ContextBudgetMode in agentRef.
// Returns contextData unchanged for "full" or empty mode (zero allocation, no copy).
// Returns a filtered copy for "lastN" and "omit" modes.
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
    default: // "full" or ""
        return contextData
    }
}

// applyLastNBudget returns a copy of contextData where the "steps" map only contains entries
// for the last n *steps that produced an entry in the steps map*. completionOrder is filtered
// to names present in the steps map BEFORE taking the last n, so a step that completed but
// wrote no output (no steps entry) does not consume a slot in the window.
// inputs, variables, and expressions are always preserved unchanged.
func applyLastNBudget(contextData map[string]interface{}, n int, completionOrder []string) map[string]interface{} {
    stepsMap, ok := contextData["steps"].(map[string]interface{})
    if !ok || len(stepsMap) == 0 {
        // Nothing to prune; still return a copy with the (empty/absent) steps map.
        return copyContextWithSteps(contextData, stepsMap)
    }

    // Filter completionOrder down to names that actually have a steps entry, THEN take last n.
    present := make([]string, 0, len(completionOrder))
    for _, name := range completionOrder {
        if _, exists := stepsMap[name]; exists {
            present = append(present, name)
        }
    }
    start := len(present) - n
    if start < 0 {
        start = 0
    }
    keepSet := make(map[string]bool, len(present)-start)
    for _, name := range present[start:] {
        keepSet[name] = true
    }

    filteredSteps := make(map[string]interface{}, len(keepSet))
    for name := range keepSet {
        filteredSteps[name] = stepsMap[name]
    }
    return copyContextWithSteps(contextData, filteredSteps)
}

// applyOmitBudget returns a copy of contextData where the "steps" map is replaced with
// an empty map. inputs, variables, and expressions are always preserved unchanged.
func applyOmitBudget(contextData map[string]interface{}) map[string]interface{} {
    filtered := make(map[string]interface{}, len(contextData))
    for k, v := range contextData {
        filtered[k] = v
    }
    filtered["steps"] = map[string]interface{}{}
    return filtered
}
```

**Copy semantics**: Both helpers make a shallow copy of the top-level `contextData` map and a
shallow copy of the `steps` map. Step entry values (the per-step `map[string]interface{}`
containing `response` and `outputs`) are shared references — not duplicated. This means
individual step data maps are shared with the original, but since Go strings are immutable and
the main dispatch loop is sequential, these shared references are safe and allocation cost is
proportional only to the number of map entries, not the size of the strings.

### 4. Interception in agent_executor.go

```go
// Read current context for prompt evaluation
contextData, err := e.contextManager.ReadContext(ctx)
if err != nil {
    return nil, fmt.Errorf("failed to read context: %w", err)
}

// Apply context budget before CEL evaluation (prevents materialization of pruned entries)
if agentRef.ContextBudgetMode != "" && agentRef.ContextBudgetMode != "full" {
    contextData = applyContextBudget(contextData, agentRef, e.contextManager.CompletionOrder())
}

// Build variable map for CEL evaluation
vars := e.celEvaluator.BuildVariableMap(contextData)
```

The guard `agentRef.ContextBudgetMode != "" && agentRef.ContextBudgetMode != "full"` ensures
that when the mode is unset or explicitly `"full"`, the function returns the input unchanged
(zero allocation) — so the hot path for existing workflows has no overhead.

### 5. Token estimation and truncation signal

**Estimation fix** (`agent_executor.go:99`):

```go
// was: tokenBudget := *agentRef.MaxAdditionalPromptTokens * 4
// Kyverno YAML and Go source code tokenize at ~3 runes/token with Claude's tokenizer.
// The previous * 4 multiplier let 20-33% more tokens through than the stated budget.
tokenBudget := *agentRef.MaxAdditionalPromptTokens * 3
```

**Truncation log** (immediately after the truncation check):

```go
if int32(utf8.RuneCountInString(additionalText)) > tokenBudget {
    runes := []rune(additionalText)
    additionalText = string(runes[:tokenBudget]) + "..."
    klog.V(2).InfoS("additionalPrompts truncated by MaxAdditionalPromptTokens",
        "step", step.Name,
        "budgetTokens", *agentRef.MaxAdditionalPromptTokens,
        "actualRunes", utf8.RuneCountInString(additionalText))
}
```

---

## Edge Cases

### N ≥ total completed steps

When `ContextBudgetLastN >= len(completionOrder)`, `start` clamps to 0, so `keepSet` contains
all recorded steps. The resulting `filteredSteps` is identical to the original. No entries are
dropped. This is the correct behavior: "keep the last 10 steps" when only 3 have completed
means "keep all 3."

### First agent step in a workflow

`completionOrder` is empty when the first agent step runs. For `lastN` mode: `keepSet` is
empty, so `filteredSteps` is empty — identical to `omit` behavior. For `omit` mode: `steps` is
already empty at this point (no prior steps have run), so the result is the same. Either mode
on the first step is safe and produces no change to available context.

### forEach child agent steps

ForEach creates a scoped context by deep-copying the parent context at
`foreach_executor.go:292-305`, then sets `scopedContextKey` on the Go context. When a forEach
child step is an agent step, `ReadContext(ctx)` returns the scoped (isolated) copy, not the
parent `inMemoryContext`. The `applyContextBudget` call operates on this scoped copy — so the
budget is applied correctly within the scope. Pruned entries from the scoped copy are not
visible to CEL in the child step.

The `completionOrder` reflects the parent workflow's completion sequence (step names recorded
by the sequential main loop). ForEach child steps use synthetic name `"_item_"` and do not
record completions in `completionOrder`. This means `lastN` on a forEach child agent step uses
the parent's completion order — which is correct: the child step should see recent parent-level
context, not per-item transient context.

### omit mode and steps.*.outputs

`omit` mode replaces `steps` with an empty map. This means `steps.stepName.outputs.someKey`
is also unavailable in CEL. Steps that need extracted output values from prior steps must not
use `omit` mode — they should use `lastN` with sufficient N, or `full`, or use `variables.*`
(which WriteStepOutputs writes flat output keys to and which is never pruned).

This is intentional: `omit` is for agent steps that construct their entire prompt from `inputs`
and `expressions`, with no reference to step history. If a step needs any prior step's outputs,
it should use `lastN` instead.

### Skipped steps

Skipped steps do not write to `inMemoryContext["steps"]` and do not call
`RecordStepCompletion`. A skipped step's name never appears in `completionOrder`. If a
downstream step uses `lastN=2`, the 2 most recent *executed* (non-skipped) steps are included.
This is the correct behavior — skipped steps have no data to contribute.

---

## What This Design Does Not Address

- **Automatic workflow-level budget**: Pruning steps automatically without per-step annotation
  (Approach D in the proposal) is deferred. Silent data loss when a step expects prior context
  but receives an empty map is a worse failure mode than oversized context. Future work.
- **LLM-based summarization**: Replacing response strings with LLM-generated summaries
  (Approach E) is deferred. Secondary LLM call per step adds latency.
- **inMemoryContext memory growth**: This design reduces what CEL sees, but `inMemoryContext`
  itself still grows without bound. The actual string data is not evicted from the map. The
  in-process memory footprint of the runner Job is unchanged — only what gets materialized
  into `promptStr` is reduced.
- **workflowContext dead parameter**: `ExecuteAgent`'s `workflowContext map[string]interface{}`
  parameter is never used and remains dead. Removing it would require a breaking interface
  change. Deferred.

---

## Testing

### Unit tests (context_budget_test.go)

Test `applyLastNBudget` and `applyOmitBudget` directly with a mock 5-step context:

| Scenario | Input | Expected |
|---|---|---|
| `lastN=2`, 5 steps in order | steps A,B,C,D,E | only D,E in result `steps` map |
| `lastN=2`, 1 step in order | step A | only A in result (N > total — no-op) |
| `lastN=5`, 5 steps | steps A..E | all 5 returned (N = total — no-op) |
| `lastN=0` (edge, default to 5) | steps A..E | only A..E (using default 5) |
| `omit`, 5 steps | any | result `steps` is `{}` |
| `omit`, 0 steps | empty | result `steps` is `{}` |
| `full` (default) | any | original map returned unchanged (pointer equality) |
| inputs/variables/expressions | present in any mode | always preserved unchanged |

### Integration (existing executor tests)

Add a test workflow with 3 agent steps using `lastN=1` and verify:
- Step 3's agent only receives step 2's data in `steps` (step 1 is pruned)
- Step 3 can still access `variables.*` from step 1 (WriteStepOutputs writes there too)

### Verification commands

```bash
go test ./internal/workflow/executor/...   # unit tests
make manifests && make generate            # regenerate CRDs after types change
go build ./...                             # compile check
```

---

## Files Changed

| File | Change |
|---|---|
| `api/v1alpha1/workflow_types.go` | Add `ContextBudgetMode` + `ContextBudgetLastN` to `StepAgentRef` |
| `internal/workflow/executor/context.go` | Add `completionOrder []string`, `RecordStepCompletion`, `CompletionOrder` |
| `internal/workflow/executor/executor.go` | Call `RecordStepCompletion` after each successful `executeStep` |
| `internal/workflow/executor/context_budget.go` | New — `applyContextBudget`, `applyLastNBudget`, `applyOmitBudget` |
| `internal/workflow/executor/agent_executor.go` | Wire interception; fix `*4`→`*3`; add truncation log |
| `config/crd/bases/` + `charts/ottoflow/crds/` | Auto-updated via `make manifests` |
