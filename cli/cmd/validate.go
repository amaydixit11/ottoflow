/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	celapi "github.com/google/cel-go/cel"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	cliexec "github.com/nirmata/ottoflow/cli/internal/executor"
	clibac "github.com/nirmata/ottoflow/cli/internal/rbac"
	"github.com/nirmata/ottoflow/internal/webhook"
	"github.com/nirmata/ottoflow/internal/workflow/executor"
)

var (
	validateFile                   string
	validateWorkflowDir            string
	validateGenerateRBAC           bool
	validateOutput                 string
	validateAgentExecutorNamespace string
)

var validateCmd = &cobra.Command{
	Use:          "validate [workflow-name]",
	Short:        "Validate a Workflow definition without executing it",
	SilenceUsage: true,
	Long: `Validate a Workflow definition by running static checks without executing any steps.

Checks performed:
  - DAG cycle detection and invalid dependsOn references
  - Step expression-to-dependsOn alignment (MISSING_DEPENDS_ON)
  - Undefined inputs.* references (UNDEFINED_INPUT)
  - CEL expression syntax (compile-time, no evaluation)
  - WorkflowRef and AgentRef existence (when connected to a cluster)

Examples:
  ottoflow validate --workflow-dir samples        # validate all workflows in directory
  ottoflow validate my-workflow --workflow-dir samples
  ottoflow validate -f workflow.yaml
  ottoflow validate my-workflow

  # Generate RBAC manifests after validation
  ottoflow validate -f workflow.yaml --generate-rbac
  ottoflow validate --workflow-dir samples --generate-rbac --output rbac.yaml`,
	Args: cobra.MaximumNArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringVarP(&validateFile, "file", "f", "", "Load workflow from a YAML file")
	validateCmd.Flags().StringVar(&validateWorkflowDir, "workflow-dir", "",
		"Load workflows from directory (local mode, no cluster required)")
	validateCmd.Flags().BoolVar(&validateGenerateRBAC, "generate-rbac", false,
		"After validation passes, generate RBAC manifests for the workflow")
	validateCmd.Flags().StringVar(&validateOutput, "output", "",
		"Write generated RBAC to a file (default: stdout); only used with --generate-rbac")
	validateCmd.Flags().StringVar(&validateAgentExecutorNamespace, "agent-executor-namespace", "ottoflow",
		"Namespace where the agent-executor service runs (used for agentRef RBAC rules)")
}

type validationError struct {
	code    string
	step    string
	message string
}

func (e validationError) String() string {
	if e.step != "" {
		return fmt.Sprintf("ERROR [%-20s] step %q: %s", e.code, e.step, e.message)
	}
	return fmt.Sprintf("ERROR [%-20s] %s", e.code, e.message)
}

func runValidate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate all workflows in the directory when no name is given.
	if validateWorkflowDir != "" && len(args) == 0 {
		return runValidateDir(ctx, validateWorkflowDir)
	}

	wf, k8sClient, err := loadWorkflowForValidation(ctx, args)
	if err != nil {
		// A file holding only sibling OttoFlow resources (Agent, MCPServer, StepTemplate,
		// WorkflowRun) has nothing for this command to check. Report and succeed: failing
		// would make `validate` unusable across a directory of mixed manifests.
		var noWorkflow *noWorkflowInFileError
		if errors.As(err, &noWorkflow) {
			// If any document in the file failed to parse, surface that as an error rather
			// than skipping the file.
			if noWorkflow.parseErr != nil {
				return err
			}
			if len(noWorkflow.otherKinds) > 0 {
				fmt.Printf("SKIP %s: no Workflow to validate (contains %s)\n",
					noWorkflow.path, strings.Join(noWorkflow.otherKinds, ", "))
				return nil
			}
		}
		return err
	}

	if validateGenerateRBAC && namespace == "" {
		return fmt.Errorf("--namespace is required with --generate-rbac\n" +
			"       use the namespace where your WorkflowRuns will be submitted (e.g. --namespace my-namespace)")
	}

	celEnv, celEnvErr := executor.NewValidationCELEnv()
	if celEnvErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not build CEL environment, skipping CEL checks: %v\n", celEnvErr)
	}

	errs := checkWorkflow(ctx, wf, k8sClient, celEnv, celEnvErr)
	if len(errs) == 0 {
		fmt.Printf("OK   workflow %q passed all checks\n", wf.Name)
		if validateGenerateRBAC {
			gen, err := clibac.New(clibac.Options{
				Namespace:              getNamespace(),
				AgentExecutorNamespace: validateAgentExecutorNamespace,
			})
			if err != nil {
				return fmt.Errorf("initialize RBAC generator: %w", err)
			}
			out, err := generateRBACBytes(gen, wf)
			if err != nil {
				return err
			}
			return writeOutput(validateOutput, out)
		}
		return nil
	}
	for _, e := range errs {
		fmt.Println(e.String())
	}
	os.Exit(1)
	return nil
}

// runValidateDir validates every Workflow found in dir and reports per-workflow results.
func runValidateDir(ctx context.Context, dir string) error {
	if validateGenerateRBAC && namespace == "" {
		return fmt.Errorf("--namespace is required with --generate-rbac\n" +
			"       use the namespace where your WorkflowRuns will be submitted (e.g. --namespace my-namespace)")
	}

	exec := cliexec.NewLocalWorkflowExecutor(nil, "", 0, "", "")
	if err := exec.LoadFromDirectory(dir); err != nil {
		return fmt.Errorf("load workflow dir: %w", err)
	}
	workflows, err := exec.ListWorkflows(ctx)
	if err != nil {
		return err
	}
	if len(workflows) == 0 {
		fmt.Println("no workflows found in directory")
		return nil
	}

	celEnv, celEnvErr := executor.NewValidationCELEnv()
	if celEnvErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not build CEL environment, skipping CEL checks: %v\n", celEnvErr)
	}

	anyFailed := false
	for _, wf := range workflows {
		errs := checkWorkflow(ctx, wf, nil, celEnv, celEnvErr)
		if len(errs) == 0 {
			fmt.Printf("OK   workflow %q passed all checks\n", wf.Name)
		} else {
			anyFailed = true
			fmt.Printf("FAIL workflow %q\n", wf.Name)
			for _, e := range errs {
				fmt.Printf("     %s\n", e.String())
			}
		}
	}
	if anyFailed {
		os.Exit(1)
	}
	if validateGenerateRBAC {
		gen, err := clibac.New(clibac.Options{
			Namespace:              getNamespace(),
			AgentExecutorNamespace: validateAgentExecutorNamespace,
		})
		if err != nil {
			return fmt.Errorf("initialize RBAC generator: %w", err)
		}
		var combined []byte
		for i, wf := range workflows {
			b, err := generateRBACBytes(gen, wf)
			if err != nil {
				return err
			}
			if i > 0 {
				combined = append(combined, []byte("---\n")...)
			}
			combined = append(combined, b...)
		}
		return writeOutput(validateOutput, combined)
	}
	return nil
}

// checkWorkflow runs all static checks against a single workflow and returns any errors found.
func checkWorkflow(
	ctx context.Context,
	wf *ottoflowv1alpha1.Workflow,
	k8sClient client.Client,
	celEnv *celapi.Env,
	celEnvErr error,
) []validationError {
	var errs []validationError

	// Check 1+2: DAG cycle detection and invalid dependsOn references.
	if _, dagErr := executor.BuildDAG(wf.Spec.Steps); dagErr != nil {
		code := "MISSING_DEPENDS_ON"
		if strings.Contains(dagErr.Error(), "circular") {
			code = "CYCLE_DETECTED"
		}
		errs = append(errs, validationError{code: code, message: dagErr.Error()})
	}

	// Check 3: Expression-to-dependsOn alignment.
	if depErr := webhook.ValidateStepDependencies(&wf.Spec); depErr != nil {
		errs = append(errs, validationError{code: "MISSING_DEPENDS_ON", message: depErr.Error()})
	}

	// Check 4: Undefined inputs.* references.
	if inputErr := webhook.ValidateInputRefs(&wf.Spec); inputErr != nil {
		errs = append(errs, validationError{code: "UNDEFINED_INPUT", message: inputErr.Error()})
	}

	// Check 5: CEL syntax per expression.
	if celEnvErr == nil && celEnv != nil {
		for i := range wf.Spec.Steps {
			step := &wf.Spec.Steps[i]
			for _, expr := range collectCELExpressions(step) {
				if _, iss := celEnv.Compile(expr); iss != nil && iss.Err() != nil {
					if isCELTypeOnlyError(iss.Err().Error()) {
						continue
					}
					errs = append(errs, validationError{
						code:    "CEL_SYNTAX_ERROR",
						step:    step.Name,
						message: fmt.Sprintf("expr %q: %s", expr, formatCELError(iss.Err().Error())),
					})
				}
			}
		}
	}

	// Check 6: WorkflowRef and AgentRef existence (cluster mode only).
	if k8sClient != nil {
		ns := getNamespace()
		seen := make(map[string]struct{})
		for _, step := range wf.Spec.Steps {
			if step.WorkflowRef != nil && step.WorkflowRef.Name != "" {
				key := "wf/" + step.WorkflowRef.Name
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					var ref ottoflowv1alpha1.Workflow
					refNS := step.WorkflowRef.Namespace
					if refNS == "" {
						refNS = ns
					}
					nn := types.NamespacedName{Namespace: refNS, Name: step.WorkflowRef.Name}
					if err := k8sClient.Get(ctx, nn, &ref); err != nil {
						if apierrors.IsNotFound(err) {
							errs = append(errs, validationError{
								code:    "REF_NOT_FOUND",
								step:    step.Name,
								message: fmt.Sprintf("workflowRef %q not found in namespace %q", step.WorkflowRef.Name, refNS),
							})
						}
					}
				}
			}
			if step.AgentRef != nil && step.AgentRef.Name != "" {
				key := "agent/" + step.AgentRef.Name
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					var ref ottoflowv1alpha1.Agent
					refNS := step.AgentRef.Namespace
					if refNS == "" {
						refNS = ns
					}
					if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: refNS, Name: step.AgentRef.Name}, &ref); err != nil {
						if apierrors.IsNotFound(err) {
							errs = append(errs, validationError{
								code:    "REF_NOT_FOUND",
								step:    step.Name,
								message: fmt.Sprintf("agentRef %q not found in namespace %q", step.AgentRef.Name, refNS),
							})
						}
					}
				}
			}
		}
	}

	return errs
}

// loadWorkflowForValidation loads a Workflow from file, directory, or cluster.
// Returns nil k8sClient when loading locally (no cluster checks will be run).
func loadWorkflowForValidation(ctx context.Context, args []string) (*ottoflowv1alpha1.Workflow, client.Client, error) {
	if validateFile != "" {
		wf, err := loadWorkflowFromFile(validateFile)
		return wf, nil, err
	}

	if validateWorkflowDir != "" {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		exec := cliexec.NewLocalWorkflowExecutor(nil, "", 0, "", "")
		if err := exec.LoadFromDirectory(validateWorkflowDir); err != nil {
			return nil, nil, fmt.Errorf("load workflow dir: %w", err)
		}
		wf, err := exec.GetWorkflow(ctx, name, getNamespace())
		return wf, nil, err
	}

	// Cluster mode.
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("workflow name required (or use --file / --workflow-dir for local validation)")
	}
	config, err := getKubeConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	k8sClient, err := createK8sClient(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	var wf ottoflowv1alpha1.Workflow
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: getNamespace(), Name: args[0]}, &wf); err != nil {
		return nil, nil, fmt.Errorf("get workflow %q: %w", args[0], err)
	}
	return &wf, k8sClient, nil
}

// generateRBACBytes generates RBAC manifests for wf using gen and returns the YAML bytes.
// Dynamic namespace warnings are printed to stderr.
func generateRBACBytes(gen *clibac.Generator, wf *ottoflowv1alpha1.Workflow) ([]byte, error) {
	out, warnings, err := gen.GenerateForWorkflow(wf)
	if err != nil {
		return nil, fmt.Errorf("generate RBAC for workflow %q: %w", wf.Name, err)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}
	return out, nil
}

// writeOutput writes data to path if set, or to stdout otherwise.
func writeOutput(path string, data []byte) error {
	if path != "" {
		return os.WriteFile(path, data, 0o644)
	}
	_, err := os.Stdout.Write(data)
	return err
}

// noWorkflowInFileError reports a file that contained no Workflow. It records the kinds it
// did find so callers can tell "this is a valid manifest with nothing for validate to
// check" apart from "this file could not be parsed at all". Without that distinction,
// running validate over a directory fails on every manifest that is not a Workflow --
// including the companion Agent, MCPServer and ConfigMap resources that workflows need.
//
// Kinds outside the OttoFlow API group count too: samples ship plain Kubernetes objects
// (a state ConfigMap, for instance) alongside the workflows that use them. The kinds are
// named in the message, so a mistyped `kind: Workflw` still surfaces rather than passing
// silently as an unsupported kind.
type noWorkflowInFileError struct {
	path       string
	otherKinds []string
	// parseErr is the first YAML error encountered, kept so that a malformed file is not
	// reported as merely "no Workflow found" - that message sent a genuinely broken sample
	// looking like an unsupported-kind case.
	parseErr error
}

func (e *noWorkflowInFileError) Error() string {
	switch {
	case len(e.otherKinds) > 0:
		return fmt.Sprintf("no Workflow found in file %q (found %s)", e.path, strings.Join(e.otherKinds, ", "))
	case e.parseErr != nil:
		return fmt.Sprintf("no Workflow found in file %q; the file could not be parsed: %v", e.path, e.parseErr)
	default:
		return fmt.Sprintf("no Workflow found in file %q", e.path)
	}
}

func loadWorkflowFromFile(filePath string) (*ottoflowv1alpha1.Workflow, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	// YAML document separators must appear at the start of a line.
	// Splitting on bare "---" incorrectly splits on "---" that appears
	// inside string values (e.g. markdown table separators like |---|---|).
	// Prepend a newline so we can uniformly split on "\n---" regardless of
	// whether the file starts with a document marker.
	var otherKinds []string
	var firstParseErr error
	seen := map[string]bool{}
	for _, doc := range strings.Split("\n"+string(data), "\n---") {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		// Read the kind first. TypeMeta accepts almost any document, so unmarshalling it
		// before the Workflow gives us the document's declared identity independently of
		// whether its body is well-formed.
		var meta metav1.TypeMeta
		if err := yaml.Unmarshal([]byte(doc), &meta); err != nil {
			if firstParseErr == nil {
				firstParseErr = err
			}
			continue
		}
		if meta.Kind == "Workflow" {
			// A document that declares itself a Workflow but does not parse as one is a
			// broken Workflow, not an unsupported kind. Returning the unmarshal error keeps
			// it from being reclassified as a skippable sibling manifest: the permissive
			// TypeMeta parse above succeeds for any body, so falling through to otherKinds
			// here made `validate` print SKIP and exit 0 on e.g. `steps` given as a string.
			wf := &ottoflowv1alpha1.Workflow{}
			if err := yaml.Unmarshal([]byte(doc), wf); err != nil {
				return nil, fmt.Errorf("parse Workflow in %q: %w", filePath, err)
			}
			return wf, nil
		}
		// Not a Workflow. Record the kind so the caller can distinguish a sibling manifest
		// from an unrelated or malformed file.
		if meta.Kind != "" && !seen[meta.Kind] {
			seen[meta.Kind] = true
			otherKinds = append(otherKinds, meta.Kind)
		}
	}
	sort.Strings(otherKinds)
	return nil, &noWorkflowInFileError{path: filePath, otherKinds: otherKinds, parseErr: firstParseErr}
}

// collectCELExpressions returns only the fields of a step that are definitively CEL
// (not LLM prompts or raw JSON literals).
func collectCELExpressions(step *ottoflowv1alpha1.Step) []string {
	var out []string
	for _, e := range step.Expressions {
		if e.Expression != "" {
			out = append(out, e.Expression)
		}
	}
	for _, o := range step.Outputs {
		if o.Expression != "" {
			out = append(out, o.Expression)
		}
		// o.Value is a raw JSON literal — do not compile as CEL.
	}
	for _, m := range step.MatchConditions {
		if m.Expression != "" {
			out = append(out, m.Expression)
		}
	}
	if step.WorkflowRef != nil {
		for _, v := range step.WorkflowRef.Inputs {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	if step.MCPToolCall != nil {
		for _, v := range step.MCPToolCall.Arguments {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	if step.ResourceQuery != nil {
		for _, v := range []string{step.ResourceQuery.Namespace, step.ResourceQuery.Name, step.ResourceQuery.FieldSelector} {
			if v != "" {
				out = append(out, v)
			}
		}
		// ResourceQuery.Outputs are evaluated against the fetched resource, which is bound
		// to `object` (see resource_query_executor.go) — not to `resource`, which is the
		// macro namespace and cannot be field-selected. They are validated normally.
		for _, v := range step.ResourceQuery.Outputs {
			if v != "" {
				out = append(out, v)
			}
		}
		// LabelSelector values are CEL expressions too (resource_query_executor.go
		// evaluates every one), so a literal label value must be quoted inside the
		// expression. Compiling them here catches the unquoted form.
		for _, v := range step.ResourceQuery.LabelSelector {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	if step.PrometheusQuery != nil {
		for _, v := range step.PrometheusQuery.Variables {
			if v != "" {
				out = append(out, v)
			}
		}
		for _, v := range step.PrometheusQuery.Outputs {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	if step.ForEach != nil && step.ForEach.Items != "" {
		out = append(out, step.ForEach.Items)
	}
	if step.Mutate != nil {
		if step.Mutate.ApplyConfiguration != nil && step.Mutate.ApplyConfiguration.Expression != "" {
			out = append(out, step.Mutate.ApplyConfiguration.Expression)
		}
		if step.Mutate.JSONPatch != nil && step.Mutate.JSONPatch.Expression != "" {
			out = append(out, step.Mutate.JSONPatch.Expression)
		}
	}
	// AgentRef.AdditionalPrompts are LLM text, not CEL — intentionally excluded.
	return out
}

// formatCELError strips source-snippet lines from a CEL error and returns a concise
// location:message string — e.g. "6:15: unexpected token '!'" instead of the full
// multi-line output including source excerpts and caret pointers.
func formatCELError(celErr string) string {
	var msgs []string
	for _, line := range strings.Split(celErr, "\n") {
		if !strings.Contains(line, "ERROR:") {
			continue // source snippet or caret pointer line
		}
		// "ERROR: <input>:6:15: message" → "6:15: message"
		msg := strings.TrimPrefix(strings.TrimSpace(line), "ERROR: <input>:")
		msgs = append(msgs, msg)
	}
	switch len(msgs) {
	case 0:
		return celErr
	case 1:
		return msgs[0]
	default:
		return fmt.Sprintf("%s (+%d more)", msgs[0], len(msgs)-1)
	}
}

// isCELTypeOnlyError reports whether every error in a CEL Issues error message is a
// type-check error rather than a syntax error. The strict validation CEL environment
// can reject expressions that are valid at runtime — notably map literals where the
// first value is dyn but subsequent values are string literals (or vice versa). Syntax
// errors use different wording and are always meaningful.
func isCELTypeOnlyError(msg string) bool {
	for _, line := range strings.Split(msg, "\n") {
		if !strings.Contains(line, "ERROR:") {
			continue // source snippet or caret pointer line
		}
		if !strings.Contains(line, "expected type") {
			return false // a non-type-check error is present
		}
	}
	return true
}
