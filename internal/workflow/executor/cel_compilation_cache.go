/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	celapi "github.com/google/cel-go/cel"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

// rawExpression pairs an identifier with the CEL expression text.
type rawExpression struct {
	Name string // e.g. "var.threshold", "step.ping.expr.ts"
	Text string // the actual CEL source
}

// compiledEntry stores a compiled program alongside its source text.
type compiledEntry struct {
	Name    string
	Text    string
	Program celapi.Program
}

// CELCompilationCache compiles and caches CEL programs at Workflow load time.
// Programs are keyed by workflow name + expression name so they can be
// invalidated and recompiled when a Workflow is updated.  The cache is
// shared across WorkflowRun reconciliations so each run skips compilation.
type CELCompilationCache struct {
	mu       sync.RWMutex
	env      *celapi.Env
	progOpts []celapi.ProgramOption
	logger   logr.Logger
	// workflowKey ("namespace/name") -> exprName -> compiledEntry
	programs map[string]map[string]compiledEntry
}

// NewCELCompilationCache creates a shared, thread-safe cache backed by a
// single CEL environment.  Create one instance at controller startup and
// pass it to both the Workflow and WorkflowRun reconcilers.
func NewCELCompilationCache(
	k8sClient client.Client,
	metricsClient metricsclientset.Interface,
	customMetricsClient CustomMetricsClient,
	prometheusClient PrometheusClient,
	logger logr.Logger,
) (*CELCompilationCache, error) {
	// The compilation cache uses its own macroContextHolder. Pre-compiled programs
	// from this cache will use context.Background() in macro calls (a known limitation).
	cacheHolder := &macroContextHolder{}
	env, progOpts, err := createCELEnvironment(k8sClient, nil, metricsClient, customMetricsClient, prometheusClient, "", cacheHolder)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment for compilation cache: %w", err)
	}

	return &CELCompilationCache{
		env:      env,
		progOpts: progOpts,
		logger:   logger.WithName("cel-cache"),
		programs: make(map[string]map[string]compiledEntry),
	}, nil
}

// WorkflowKey returns the canonical cache key for a workflow.
func WorkflowKey(namespace, name string) string {
	return namespace + "/" + name
}

// CompileWorkflow extracts every CEL expression from a Workflow, compiles it,
// and stores the resulting program.  Any previously cached programs for the
// same workflow are replaced (handles reload / update).  Returns a list of
// compilation errors; successfully compiled expressions are cached even when
// others fail. Uses the workflow's CELCostLimit (or default) for program options.
func (c *CELCompilationCache) CompileWorkflow(workflow *ottoflowv1alpha1.Workflow) []error {
	workflowKey := WorkflowKey(workflow.Namespace, workflow.Name)
	costLimit := ResolveCELCostLimit(&workflow.Spec)
	exprs := extractCELExpressions(workflow)

	compiled := make(map[string]compiledEntry, len(exprs))
	var errs []error
	for _, raw := range exprs {
		prog, err := c.compileProgram(raw.Text, costLimit)
		if err != nil {
			errs = append(errs, fmt.Errorf("expression %q (%s): %w", raw.Name, raw.Text, err))
			continue
		}
		compiled[raw.Name] = compiledEntry{
			Name:    raw.Name,
			Text:    raw.Text,
			Program: prog,
		}
	}

	c.mu.Lock()
	c.programs[workflowKey] = compiled
	c.mu.Unlock()

	c.logger.Info("Compiled workflow CEL expressions",
		"workflow", workflowKey,
		"total", len(exprs),
		"compiled", len(compiled),
		"errors", len(errs))
	return errs
}

// InvalidateWorkflow removes all cached programs for the given workflow.
func (c *CELCompilationCache) InvalidateWorkflow(workflowKey string) {
	c.mu.Lock()
	delete(c.programs, workflowKey)
	c.mu.Unlock()
}

// GetPrograms returns pre-compiled programs for a workflow, keyed by expression
// text (the key the per-evaluator LRU uses).  Returns nil if nothing is cached.
func (c *CELCompilationCache) GetPrograms(workflowKey string) map[string]celapi.Program {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, ok := c.programs[workflowKey]
	if !ok {
		return nil
	}

	result := make(map[string]celapi.Program, len(entries))
	for _, entry := range entries {
		result[entry.Text] = entry.Program
	}
	return result
}

// compileProgram compiles a single CEL expression into an executable program.
func (c *CELCompilationCache) compileProgram(expr string, costLimit uint64) (celapi.Program, error) {
	ast, issues := c.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compilation failed: %w", issues.Err())
	}
	if costLimit == 0 {
		costLimit = DefaultCELCostLimit
	}
	programOpts := []celapi.ProgramOption{
		celapi.EvalOptions(celapi.OptOptimize),
		celapi.CostLimit(costLimit),
	}
	if len(c.progOpts) > 0 {
		programOpts = append(programOpts, c.progOpts...)
	}

	prg, err := c.env.Program(ast, programOpts...)
	if err != nil {
		return nil, fmt.Errorf("program creation failed: %w", err)
	}
	return prg, nil
}

// extractCELExpressions walks a Workflow and returns every CEL expression
// paired with a human-readable identifier.
func extractCELExpressions(workflow *ottoflowv1alpha1.Workflow) []rawExpression {
	var exprs []rawExpression

	// Workflow-level variables
	for _, v := range workflow.Spec.Variables {
		exprs = append(exprs, rawExpression{Name: "var." + v.Name, Text: v.Expression})
	}

	// Workflow-level outputs
	for _, o := range workflow.Spec.Outputs {
		if o.Expression != "" {
			exprs = append(exprs, rawExpression{Name: "out." + o.Name, Text: o.Expression})
		}
		if o.Metric != nil {
			for _, l := range o.Metric.Labels {
				exprs = append(exprs, rawExpression{Name: "out." + o.Name + ".metric.label." + l.Name, Text: l.Value})
			}
		}
	}

	// Steps
	for _, step := range workflow.Spec.Steps {
		prefix := "step." + step.Name
		extractStepExpressions(&exprs, prefix, &step)
	}

	return exprs
}

// extractStepExpressions appends all CEL expressions from a single Step.
func extractStepExpressions(exprs *[]rawExpression, prefix string, step *ottoflowv1alpha1.Step) {
	for _, e := range step.Expressions {
		*exprs = append(*exprs, rawExpression{Name: prefix + ".expr." + e.Name, Text: e.Expression})
	}

	for _, o := range step.Outputs {
		if o.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".out." + o.Name, Text: o.Expression})
		}
		if o.Metric != nil {
			for _, l := range o.Metric.Labels {
				*exprs = append(*exprs, rawExpression{Name: prefix + ".out." + o.Name + ".metric.label." + l.Name, Text: l.Value})
			}
		}
	}

	for _, mc := range step.MatchConditions {
		*exprs = append(*exprs, rawExpression{Name: prefix + ".match." + mc.Name, Text: mc.Expression})
	}

	if step.WorkflowRef != nil {
		for k, v := range step.WorkflowRef.Inputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".wfRef.input." + k, Text: v})
		}
	}

	if step.MCPToolCall != nil {
		for k, v := range step.MCPToolCall.Arguments {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mcp.arg." + k, Text: v})
		}
	}

	if step.ResourceQuery != nil {
		for k, v := range step.ResourceQuery.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".rq.out." + k, Text: v})
		}
	}

	if step.PrometheusQuery != nil {
		for k, v := range step.PrometheusQuery.Variables {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".pq.var." + k, Text: v})
		}
		for k, v := range step.PrometheusQuery.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".pq.out." + k, Text: v})
		}
	}

	if step.Mutate != nil {
		if step.Mutate.ApplyConfiguration != nil && step.Mutate.ApplyConfiguration.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.apply", Text: step.Mutate.ApplyConfiguration.Expression})
		}
		if step.Mutate.JSONPatch != nil && step.Mutate.JSONPatch.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.jsonPatch", Text: step.Mutate.JSONPatch.Expression})
		}
		for k, v := range step.Mutate.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.out." + k, Text: v})
		}
	}

	if step.StepTemplateRef != nil {
		for k, v := range step.StepTemplateRef.Arguments {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".tpl.arg." + k, Text: v})
		}
	}

	if step.ForEach != nil {
		*exprs = append(*exprs, rawExpression{Name: prefix + ".forEach.items", Text: step.ForEach.Items})
		if step.ForEach.Step != nil {
			extractForEachStepExpressions(exprs, prefix+".forEach", step.ForEach.Step)
		}
		if step.ForEach.StepTemplateRef != nil {
			for k, v := range step.ForEach.StepTemplateRef.Arguments {
				*exprs = append(*exprs, rawExpression{Name: prefix + ".forEach.tpl.arg." + k, Text: v})
			}
		}
	}
}

// extractForEachStepExpressions appends CEL expressions from a forEach inline step.
func extractForEachStepExpressions(exprs *[]rawExpression, prefix string, step *ottoflowv1alpha1.StepForEachStep) {
	for _, e := range step.Expressions {
		*exprs = append(*exprs, rawExpression{Name: prefix + ".expr." + e.Name, Text: e.Expression})
	}

	for _, o := range step.Outputs {
		if o.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".out." + o.Name, Text: o.Expression})
		}
	}

	for _, mc := range step.MatchConditions {
		*exprs = append(*exprs, rawExpression{Name: prefix + ".match." + mc.Name, Text: mc.Expression})
	}

	if step.WorkflowRef != nil {
		for k, v := range step.WorkflowRef.Inputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".wfRef.input." + k, Text: v})
		}
	}

	if step.MCPToolCall != nil {
		for k, v := range step.MCPToolCall.Arguments {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mcp.arg." + k, Text: v})
		}
	}

	if step.ResourceQuery != nil {
		for k, v := range step.ResourceQuery.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".rq.out." + k, Text: v})
		}
	}

	if step.PrometheusQuery != nil {
		for k, v := range step.PrometheusQuery.Variables {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".pq.var." + k, Text: v})
		}
		for k, v := range step.PrometheusQuery.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".pq.out." + k, Text: v})
		}
	}

	if step.Mutate != nil {
		if step.Mutate.ApplyConfiguration != nil && step.Mutate.ApplyConfiguration.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.apply", Text: step.Mutate.ApplyConfiguration.Expression})
		}
		if step.Mutate.JSONPatch != nil && step.Mutate.JSONPatch.Expression != "" {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.jsonPatch", Text: step.Mutate.JSONPatch.Expression})
		}
		for k, v := range step.Mutate.Outputs {
			*exprs = append(*exprs, rawExpression{Name: prefix + ".mutate.out." + k, Text: v})
		}
	}

}
