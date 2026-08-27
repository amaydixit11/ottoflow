/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
	"github.com/nirmata/ottoflow/internal/metrics"
)

func run(name string, phase ottoflowv1alpha1.WorkflowRunPhase) *ottoflowv1alpha1.WorkflowRun {
	start := metav1.NewTime(time.Unix(1000, 0))
	end := metav1.NewTime(time.Unix(1012, 0))
	return &ottoflowv1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: ottoflowv1alpha1.WorkflowRunSpec{
			WorkflowRef: ottoflowv1alpha1.WorkflowRef{Name: "wf", Namespace: "default"},
		},
		Status: ottoflowv1alpha1.WorkflowRunStatus{
			Phase:          phase,
			StartTime:      &start,
			CompletionTime: &end,
		},
	}
}

func counterValue(t *testing.T, vec *prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	var m dto.Metric
	c, err := vec.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("counter %v: %v", labels, err)
	}
	if err := c.Write(&m); err != nil {
		t.Fatalf("writing counter %v: %v", labels, err)
	}
	return m.GetCounter().GetValue()
}

func histogramCount(t *testing.T, vec *prometheus.HistogramVec, labels ...string) uint64 {
	t.Helper()
	var m dto.Metric
	o, err := vec.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("histogram %v: %v", labels, err)
	}
	if err := o.(prometheus.Metric).Write(&m); err != nil {
		t.Fatalf("writing histogram %v: %v", labels, err)
	}
	return m.GetHistogram().GetSampleCount()
}

func gaugeValue(t *testing.T, workflow, namespace string) float64 {
	t.Helper()
	var m dto.Metric
	g, err := metrics.WorkflowRunsActive.GetMetricWithLabelValues(workflow, namespace)
	if err != nil {
		t.Fatalf("gauge: %v", err)
	}
	if err := g.Write(&m); err != nil {
		t.Fatalf("writing gauge: %v", err)
	}
	return m.GetGauge().GetValue()
}

func TestRecordRunTransition(t *testing.T) {
	succeeded := run("r1", ottoflowv1alpha1.WorkflowRunPhaseSucceeded)
	before := counterValue(t, metrics.WorkflowRunsTotal, "wf", "default", "succeeded")
	durations := histogramCount(t, metrics.WorkflowRunDurationSeconds, "wf", "default")

	recordRunTransition(run("r1", ottoflowv1alpha1.WorkflowRunPhaseRunning), succeeded)

	if got := counterValue(t, metrics.WorkflowRunsTotal, "wf", "default", "succeeded"); got != before+1 {
		t.Errorf("runs_total = %v, want %v", got, before+1)
	}
	if got := histogramCount(t, metrics.WorkflowRunDurationSeconds, "wf", "default"); got != durations+1 {
		t.Errorf("run duration observations = %d, want %d", got, durations+1)
	}
}

// Reaching a terminal phase is what counts, not being in one. A re-sent update
// for an already-finished run must not count it twice.
func TestRecordRunTransitionIgnoresRepeats(t *testing.T) {
	terminal := run("r2", ottoflowv1alpha1.WorkflowRunPhaseFailed)
	before := counterValue(t, metrics.WorkflowRunsTotal, "wf", "default", "failed")

	recordRunTransition(terminal, terminal)
	recordRunTransition(run("r2", ottoflowv1alpha1.WorkflowRunPhaseRunning), run("r2", ottoflowv1alpha1.WorkflowRunPhaseRunning))

	if got := counterValue(t, metrics.WorkflowRunsTotal, "wf", "default", "failed"); got != before {
		t.Errorf("runs_total = %v, want it unchanged at %v", got, before)
	}
}

// Step metrics come out of status, which is how they cross the runner Job
// boundary at all.
func TestRecordRunTransitionRecordsSteps(t *testing.T) {
	start := metav1.NewTime(time.Unix(2000, 0))
	end := metav1.NewTime(time.Unix(2003, 0))
	finished := run("r3", ottoflowv1alpha1.WorkflowRunPhaseSucceeded)
	finished.Status.StepStatuses = map[string]ottoflowv1alpha1.StepStatus{
		"collect": {Phase: ottoflowv1alpha1.StepPhaseSucceeded, StartTime: &start, CompletionTime: &end},
		"notify":  {Phase: ottoflowv1alpha1.StepPhaseSkipped},
		"publish": {Phase: ottoflowv1alpha1.StepPhaseFailed, StartTime: &start, CompletionTime: &end},
	}

	base := map[string]float64{}
	for step, result := range map[string]string{"collect": "succeeded", "notify": "skipped", "publish": "failed"} {
		base[step] = counterValue(t, metrics.WorkflowStepsTotal, "wf", "default", step, result)
	}

	recordRunTransition(run("r3", ottoflowv1alpha1.WorkflowRunPhaseRunning), finished)

	for step, result := range map[string]string{"collect": "succeeded", "notify": "skipped", "publish": "failed"} {
		if got := counterValue(t, metrics.WorkflowStepsTotal, "wf", "default", step, result); got != base[step]+1 {
			t.Errorf("steps_total[%s=%s] = %v, want %v", step, result, got, base[step]+1)
		}
	}

	// A skipped step never ran, so it has no duration to observe.
	if got := histogramCount(t, metrics.WorkflowStepDurationSeconds, "wf", "default", "notify"); got != 0 {
		t.Errorf("skipped step recorded %d duration observations, want 0", got)
	}
}

func TestSyncActiveCountsRunningRuns(t *testing.T) {
	k8s := fake.NewClientBuilder().WithScheme(newMCPTestScheme(t)).WithObjects(
		run("a", ottoflowv1alpha1.WorkflowRunPhaseRunning),
		run("b", ottoflowv1alpha1.WorkflowRunPhaseRunning),
		run("c", ottoflowv1alpha1.WorkflowRunPhaseSucceeded),
	).Build()
	m := NewRunMetrics(nil, k8s)

	if err := m.syncActive(context.Background()); err != nil {
		t.Fatalf("syncActive: %v", err)
	}
	if got := gaugeValue(t, "wf", "default"); got != 2 {
		t.Errorf("runs_active = %v, want 2", got)
	}
}

// The gauge is recomputed rather than decremented, so a workflow whose runs
// have all finished reads zero instead of holding its last value.
func TestSyncActiveClearsFinishedWorkflows(t *testing.T) {
	k8s := fake.NewClientBuilder().WithScheme(newMCPTestScheme(t)).WithObjects(
		run("d", ottoflowv1alpha1.WorkflowRunPhaseSucceeded),
	).Build()
	m := NewRunMetrics(nil, k8s)

	metrics.WorkflowRunsActive.WithLabelValues("wf", "default").Set(5)
	if err := m.syncActive(context.Background()); err != nil {
		t.Fatalf("syncActive: %v", err)
	}
	if got := gaugeValue(t, "wf", "default"); got != 0 {
		t.Errorf("runs_active = %v, want 0", got)
	}
}
