/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package display

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const (
	ellipsis  = "..."
	maxMsgLen = 35
)

// StreamingRenderer renders workflow progress to the console as steps complete.
// It streams step completions incrementally for better UX during long-running workflows.
type StreamingRenderer struct {
	mu            sync.Mutex
	out           io.Writer
	stepOrder     []string
	printedSteps  map[string]bool // steps we've already printed (completed/skipped/failed)
	runningStep   string
	headerPrinted bool // true after initial header has been printed
}

// NewStreamingRenderer creates a new streaming renderer.
// stepOrder is the order of steps (from workflow.Spec.Steps) for consistent display.
func NewStreamingRenderer(out io.Writer, stepOrder []string) *StreamingRenderer {
	return &StreamingRenderer{
		out:          out,
		stepOrder:    stepOrder,
		printedSteps: make(map[string]bool),
	}
}

// ensureHeader prints the initial header on first Update. Called automatically.
func (r *StreamingRenderer) ensureHeader(workflowRun *ottoflowv1alpha1.WorkflowRun) {
	if r.headerPrinted {
		return
	}
	_, _ = fmt.Fprintf(r.out, "WorkflowRun: %s/%s\n", workflowRun.Namespace, workflowRun.Name)
	_, _ = fmt.Fprintf(r.out, "Phase: %s %s\n", getPhaseIcon(string(workflowRun.Status.Phase)), orDefault(string(workflowRun.Status.Phase), "Pending"))
	if workflowRun.Status.StartTime != nil {
		_, _ = fmt.Fprintf(r.out, "Started: %s\n", formatTime(workflowRun.Status.StartTime.Time))
	}
	_, _ = fmt.Fprintln(r.out)
	_, _ = fmt.Fprintln(r.out, "Steps:")
	r.headerPrinted = true
}

// Update is called when workflow state changes (e.g., step completes).
// It updates the display in place for the current running step and appends completed steps.
func (r *StreamingRenderer) Update(workflowRun *ottoflowv1alpha1.WorkflowRun, workflow *ottoflowv1alpha1.Workflow) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureHeader(workflowRun)

	// Print newly completed steps in order (skip already printed)
	for _, stepName := range r.stepOrder {
		if r.printedSteps[stepName] {
			continue
		}
		status, ok := workflowRun.Status.StepStatuses[stepName]
		if !ok {
			continue
		}

		phase := string(status.Phase)
		icon := getPhaseIcon(phase)

		switch status.Phase {
		case ottoflowv1alpha1.StepPhaseSucceeded, ottoflowv1alpha1.StepPhaseSkipped:
			// If this step was the one showing "running", overwrite that line
			if r.runningStep == stepName {
				_, _ = fmt.Fprint(r.out, "\r\033[K")
				r.runningStep = ""
			}
			timeStr := ""
			if status.StartTime != nil && status.CompletionTime != nil {
				d := status.CompletionTime.Sub(status.StartTime.Time)
				timeStr = formatDuration(d)
			} else if status.Phase == ottoflowv1alpha1.StepPhaseSkipped {
				timeStr = "skipped"
			}
			_, _ = fmt.Fprintf(r.out, "  %s %s", icon, stepName)
			if timeStr != "" {
				_, _ = fmt.Fprintf(r.out, " (%s)", timeStr)
			}
			_, _ = fmt.Fprintln(r.out)
			r.printedSteps[stepName] = true
		case ottoflowv1alpha1.StepPhaseFailed:
			if r.runningStep == stepName {
				_, _ = fmt.Fprint(r.out, "\r\033[K")
				r.runningStep = ""
			}
			msg := status.Error
			if msg == "" {
				msg = status.Message
			}
			if len(msg) > maxMsgLen {
				msg = msg[:maxMsgLen-3] + ellipsis
			}
			_, _ = fmt.Fprintf(r.out, "  %s %s (failed: %s)\n", icon, stepName, msg)
			r.printedSteps[stepName] = true
		case ottoflowv1alpha1.StepPhaseRunning:
			// Overwrite the same line when only progress (e.g. forEach N/M items) changes
			if r.runningStep == stepName {
				_, _ = fmt.Fprint(r.out, "\r\033[K") // start of line, clear to end
			}
			r.runningStep = stepName
			runningMsg := ellipsis
			if status.Message != "" {
				runningMsg = " " + status.Message
			}
			_, _ = fmt.Fprintf(r.out, "  %s %s (running%s)\n", icon, stepName, runningMsg)
			return
		}
	}

	// Check if any step is now running
	for _, stepName := range r.stepOrder {
		if status, ok := workflowRun.Status.StepStatuses[stepName]; ok && status.Phase == ottoflowv1alpha1.StepPhaseRunning {
			if r.runningStep == stepName {
				_, _ = fmt.Fprint(r.out, "\r\033[K")
			}
			r.runningStep = stepName
			runningMsg := ellipsis
			if status.Message != "" {
				runningMsg = " " + status.Message
			}
			_, _ = fmt.Fprintf(r.out, "  %s %s (running%s)\n", getPhaseIcon("Running"), stepName, runningMsg)
			return
		}
	}
}

// Finish clears the running line (if any) and prints the final outputs.
// Call when workflow execution completes.
func (r *StreamingRenderer) Finish(workflowRun *ottoflowv1alpha1.WorkflowRun) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureHeader(workflowRun)

	// Print any remaining steps that weren't shown yet
	for _, stepName := range r.stepOrder {
		if r.printedSteps[stepName] {
			continue
		}
		status, ok := workflowRun.Status.StepStatuses[stepName]
		if !ok {
			continue
		}
		icon := getPhaseIcon(string(status.Phase))
		timeStr := ""
		if status.StartTime != nil && status.CompletionTime != nil {
			timeStr = formatDuration(status.CompletionTime.Sub(status.StartTime.Time))
		} else if status.Phase == ottoflowv1alpha1.StepPhaseSkipped {
			timeStr = "skipped"
		}
		_, _ = fmt.Fprintf(r.out, "  %s %s", icon, stepName)
		if timeStr != "" {
			_, _ = fmt.Fprintf(r.out, " (%s)", timeStr)
		}
		_, _ = fmt.Fprintln(r.out)
	}

	_, _ = fmt.Fprintln(r.out)
	_, _ = fmt.Fprintln(r.out, strings.Repeat("─", 80))

	// Summary
	_, _ = fmt.Fprintf(r.out, "Phase: %s %s\n", getPhaseIcon(string(workflowRun.Status.Phase)), orDefault(string(workflowRun.Status.Phase), "Pending"))
	if workflowRun.Status.CompletionTime != nil {
		_, _ = fmt.Fprintf(r.out, "Completed: %s\n", formatTime(workflowRun.Status.CompletionTime.Time))
	}
	if workflowRun.Status.CompletionTime != nil && workflowRun.Status.StartTime != nil {
		duration := workflowRun.Status.CompletionTime.Sub(workflowRun.Status.StartTime.Time)
		_, _ = fmt.Fprintf(r.out, "Duration: %s\n", duration.Round(time.Second))
	}
	_, _ = fmt.Fprintln(r.out)

	// Outputs
	if len(workflowRun.Status.Outputs) > 0 {
		_, _ = fmt.Fprintln(r.out, "Outputs:")
		_, _ = fmt.Fprintln(r.out, strings.Repeat("─", 80))
		for name, value := range workflowRun.Status.Outputs {
			var outputValue interface{}
			if err := json.Unmarshal(value.Raw, &outputValue); err == nil {
				if formatted := FormatOutputValue(name, outputValue, r.out); formatted != "" {
					_, _ = fmt.Fprintf(r.out, "  %s:\n%s\n", name, formatted)
				} else {
					outputJSON, _ := json.MarshalIndent(outputValue, "  ", "  ")
					_, _ = fmt.Fprintf(r.out, "  %s:\n%s\n", name, string(outputJSON))
				}
			} else {
				_, _ = fmt.Fprintf(r.out, "  %s: %s\n", name, string(value.Raw))
			}
		}
		_, _ = fmt.Fprintln(r.out)
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
