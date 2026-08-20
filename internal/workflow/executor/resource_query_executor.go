/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const (
	defaultListPageSize = 500
)

// executeResourceQuery executes a resource query step using client-go directly
func (e *WorkflowExecutor) executeResourceQuery(ctx context.Context, workflowRun *ottoflowv1alpha1.WorkflowRun, step ottoflowv1alpha1.Step) (map[string]interface{}, error) {
	if e.client == nil {
		return nil, fmt.Errorf("kubernetes client not available (no kubeconfig); this workflow requires a cluster")
	}

	resourceQuery := step.ResourceQuery

	contextData, err := e.contextManager.ReadContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read context: %w", err)
	}

	vars := e.celEvaluator.BuildVariableMap(contextData)

	namespace, err := e.resolveNamespace(ctx, resourceQuery, workflowRun, vars)
	if err != nil {
		return nil, err
	}

	gvk, err := parseGVK(resourceQuery)
	if err != nil {
		return nil, err
	}

	isListQuery := resourceQuery.Name == ""
	var resourceData, itemsVar interface{}

	if isListQuery {
		items, err := e.executeListQuery(ctx, resourceQuery, gvk, namespace, vars)
		if err != nil {
			return nil, err
		}
		resourceData = items
		itemsVar = items
	} else {
		obj, err := e.executeSingleQuery(ctx, resourceQuery, gvk, namespace, vars)
		if err != nil {
			return nil, err
		}
		resourceData = obj
	}

	outputContext := make(map[string]interface{}, len(contextData)+2)
	for k, v := range contextData {
		outputContext[k] = v
	}
	if isListQuery {
		outputContext["items"] = itemsVar
		outputContext["resource"] = resourceData
	} else {
		outputContext["resource"] = resourceData
	}

	outputVars := e.celEvaluator.BuildVariableMap(outputContext)
	outputVars["object"] = resourceData
	outputVars["items"] = itemsVar

	outputs, err := e.evaluateOutputs(ctx, resourceQuery, outputVars)
	if err != nil {
		return nil, err
	}

	// Step-level outputs are evaluated in addition to the resourceQuery's own outputs.
	// Every other step type does this (agent, externalAgent, forEach, mcpToolCall,
	// openReport); resourceQuery previously returned early, so a step declaring
	// `outputs:` alongside `resourceQuery.outputs` had them silently dropped while the
	// step still reported Succeeded.
	if len(step.Outputs) > 0 {
		// Step-level outputs run after the query's own, so they can reference them by name
		// (variables.<queryOutput>). Copy the variables map rather than mutating the one
		// held in the shared context.
		stepVars := make(map[string]interface{}, len(outputVars))
		for k, v := range outputVars {
			stepVars[k] = v
		}
		mergedVariables := make(map[string]interface{})
		if existing, ok := outputVars["variables"].(map[string]interface{}); ok {
			for k, v := range existing {
				mergedVariables[k] = v
			}
		}
		for k, v := range outputs {
			mergedVariables[k] = v
		}
		stepVars["variables"] = mergedVariables

		stepOutputs, err := e.celEvaluator.EvaluateStepOutputs(ctx, step, stepVars)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate step outputs: %w", err)
		}
		for k, v := range stepOutputs {
			// On a name collision the step-level output wins, since it is evaluated later and
			// may deliberately refine the query output of the same name. Log it: a silently
			// shadowed output is the same failure shape this executor exists to fix.
			if _, collides := outputs[k]; collides {
				klog.V(1).InfoS("step-level output shadows a resourceQuery output of the same name",
					"step", step.Name, "output", k)
			}
			outputs[k] = v
		}
	}

	if err := e.contextManager.WriteStepOutputs(ctx, step.Name, outputs); err != nil {
		return nil, fmt.Errorf("failed to write step outputs: %w", err)
	}

	return outputs, nil
}

// resolveNamespace evaluates the namespace CEL expression or falls back to the workflowRun namespace.
func (e *WorkflowExecutor) resolveNamespace(ctx context.Context, resourceQuery *ottoflowv1alpha1.StepResourceQuery, workflowRun *ottoflowv1alpha1.WorkflowRun, vars map[string]interface{}) (string, error) {
	if resourceQuery.Namespace == "" {
		return workflowRun.Namespace, nil
	}
	result, err := e.celEvaluator.EvaluateExpression(ctx, resourceQuery.Namespace, vars)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate namespace: %w", err)
	}
	ns, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("namespace must evaluate to a string, got %T", result)
	}
	return ns, nil
}

// parseGVK parses and validates the GroupVersionKind from the resource query.
func parseGVK(resourceQuery *ottoflowv1alpha1.StepResourceQuery) (schema.GroupVersionKind, error) {
	gv, err := schema.ParseGroupVersion(resourceQuery.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid apiVersion '%s': %w", resourceQuery.APIVersion, err)
	}
	gvk := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: resourceQuery.Resource}
	if gvk.Group == "" && gvk.Version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid apiVersion '%s': must specify at least version (e.g., 'v1' or 'apps/v1')", resourceQuery.APIVersion)
	}
	if gvk.Kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("invalid resource kind: cannot be empty")
	}
	return gvk, nil
}

// buildListOptions resolves label and field selectors from CEL expressions and returns client.ListOptions.
func (e *WorkflowExecutor) buildListOptions(ctx context.Context, resourceQuery *ottoflowv1alpha1.StepResourceQuery, namespace string, vars map[string]interface{}) ([]client.ListOption, error) {
	opts := []client.ListOption{client.InNamespace(namespace)}

	if len(resourceQuery.LabelSelector) > 0 {
		labelMap := make(map[string]string, len(resourceQuery.LabelSelector))
		for k, v := range resourceQuery.LabelSelector {
			result, err := e.celEvaluator.EvaluateExpression(ctx, v, vars)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate labelSelector value for key '%s': %w", k, err)
			}
			if str, ok := result.(string); ok {
				labelMap[k] = str
			} else {
				labelMap[k] = fmt.Sprintf("%v", result)
			}
		}
		opts = append(opts, client.MatchingLabels(labelMap))
	}

	if resourceQuery.FieldSelector != "" {
		result, err := e.celEvaluator.EvaluateExpression(ctx, resourceQuery.FieldSelector, vars)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate fieldSelector: %w", err)
		}
		str, ok := result.(string)
		if !ok {
			return nil, fmt.Errorf("fieldSelector must evaluate to a string, got %T", result)
		}
		selector, err := fields.ParseSelector(str)
		if err != nil {
			return nil, fmt.Errorf("invalid fieldSelector '%s': %w", str, err)
		}
		opts = append(opts, client.MatchingFieldsSelector{Selector: selector})
	}

	return opts, nil
}

// executeListQuery fetches all matching resources using paginated client.List calls.
// Pages are fetched until either the continue token is exhausted or resourceQuery.Limit items
// have been accumulated (0 = no limit). Expired continue tokens (HTTP 410) are handled by
// restarting from scratch. Each page fetch is wrapped in a per-page timeout.
func (e *WorkflowExecutor) executeListQuery(ctx context.Context, resourceQuery *ottoflowv1alpha1.StepResourceQuery, gvk schema.GroupVersionKind, namespace string, vars map[string]interface{}) ([]interface{}, error) {
	baseOpts, err := e.buildListOptions(ctx, resourceQuery, namespace, vars)
	if err != nil {
		return nil, err
	}

	effectivePageSize := int64(defaultListPageSize)
	if resourceQuery.PageSize > 0 {
		effectivePageSize = resourceQuery.PageSize
	}

	gvkList := schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: fmt.Sprintf("%sList", gvk.Kind)}
	var allItems []interface{}
	continueToken := ""
	totalLimit := resourceQuery.Limit

	for {
		pageSize := effectivePageSize
		if totalLimit > 0 {
			remaining := totalLimit - int64(len(allItems))
			if remaining <= 0 {
				break
			}
			if remaining < pageSize {
				pageSize = remaining
			}
		}

		page := &unstructured.UnstructuredList{}
		page.SetGroupVersionKind(gvkList)

		pageOpts := append(baseOpts, client.Limit(pageSize)) //nolint:gocritic
		if continueToken != "" {
			pageOpts = append(pageOpts, client.Continue(continueToken))
		}

		pageCtx, cancel := context.WithTimeout(ctx, listPageTimeout)
		listErr := e.client.List(pageCtx, page, pageOpts...)
		cancel()

		if listErr != nil {
			if k8serrors.IsResourceExpired(listErr) && continueToken != "" {
				allItems = allItems[:0]
				continueToken = ""
				continue
			}
			if gvk.Group != "" && gvk.Group != "core" {
				return nil, fmt.Errorf("failed to list CRD resource '%s/%s/%s' in namespace '%s': %w (ensure CRD is installed and resource is namespace-scoped if namespace is specified)", gvk.Group, gvk.Version, gvk.Kind, namespace, listErr)
			}
			return nil, fmt.Errorf("failed to list resource '%s/%s' in namespace '%s': %w", resourceQuery.APIVersion, resourceQuery.Resource, namespace, listErr)
		}

		if page.GetRemainingItemCount() != nil {
			remaining := int(*page.GetRemainingItemCount())
			if avail := cap(allItems) - len(allItems); avail < remaining {
				grown := make([]interface{}, len(allItems), len(allItems)+remaining)
				copy(grown, allItems)
				allItems = grown
			}
		}

		for i := range page.Items {
			if totalLimit > 0 && int64(len(allItems)) >= totalLimit {
				break
			}
			allItems = append(allItems, page.Items[i].Object)
		}

		continueToken = page.GetContinue()
		if continueToken == "" || (totalLimit > 0 && int64(len(allItems)) >= totalLimit) {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("context cancelled during resource list pagination: %w", ctxErr)
		}
	}

	return allItems, nil
}

// executeSingleQuery fetches a single named resource using client.Get.
func (e *WorkflowExecutor) executeSingleQuery(ctx context.Context, resourceQuery *ottoflowv1alpha1.StepResourceQuery, gvk schema.GroupVersionKind, namespace string, vars map[string]interface{}) (map[string]interface{}, error) {
	nameResult, err := e.celEvaluator.EvaluateExpression(ctx, resourceQuery.Name, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate name: %w", err)
	}
	name, ok := nameResult.(string)
	if !ok {
		return nil, fmt.Errorf("name must evaluate to a string, got %T", nameResult)
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := e.client.Get(ctx, key, obj); err != nil {
		if gvk.Group != "" && gvk.Group != "core" {
			return nil, fmt.Errorf("failed to get CRD resource '%s/%s/%s' named '%s' in namespace '%s': %w (ensure CRD is installed and resource exists)", gvk.Group, gvk.Version, gvk.Kind, name, namespace, err)
		}
		return nil, fmt.Errorf("failed to get resource '%s/%s/%s/%s': %w", resourceQuery.APIVersion, resourceQuery.Resource, namespace, name, err)
	}

	return obj.Object, nil
}

// evaluateOutputs evaluates each CEL output expression against the resolved resource data.
func (e *WorkflowExecutor) evaluateOutputs(ctx context.Context, resourceQuery *ottoflowv1alpha1.StepResourceQuery, outputVars map[string]interface{}) (map[string]interface{}, error) {
	outputs := make(map[string]interface{}, len(resourceQuery.Outputs))
	for outputName, celExpr := range resourceQuery.Outputs {
		result, err := e.celEvaluator.EvaluateExpression(ctx, celExpr, outputVars)
		if err != nil {
			if strings.Contains(err.Error(), "no such key") || strings.Contains(err.Error(), "undefined field") {
				return nil, fmt.Errorf("failed to evaluate output '%s': CEL expression '%s' references a non-existent field - check resource schema or use 'has()' to check field existence", outputName, celExpr)
			}
			if strings.Contains(err.Error(), "index out of range") || strings.Contains(err.Error(), "index") {
				return nil, fmt.Errorf("failed to evaluate output '%s': array index in CEL expression '%s' is out of range - ensure array has elements before indexing", outputName, celExpr)
			}
			return nil, fmt.Errorf("failed to evaluate output '%s' (CEL expression: '%s'): %w", outputName, celExpr, err)
		}
		outputs[outputName] = result
	}
	return outputs, nil
}
