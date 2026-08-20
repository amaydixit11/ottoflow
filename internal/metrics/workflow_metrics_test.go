/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package metrics

import (
	"testing"
)

// Test workflow metrics can be used without panicking (covers init and metric vectors).
func TestWorkflowMetrics_RecordSample(t *testing.T) {
	WorkflowRunsTotal.WithLabelValues("wf", "default", "succeeded").Inc()
	WorkflowRunsTotal.WithLabelValues("wf", "default", "failed").Add(0)

	WorkflowRunDurationSeconds.WithLabelValues("wf", "default").Observe(1.5)
	WorkflowStepsTotal.WithLabelValues("wf", "default", "step1", "succeeded").Inc()
	WorkflowStepDurationSeconds.WithLabelValues("wf", "default", "step1").Observe(0.25)
	WorkflowRunsActive.WithLabelValues("wf", "default").Set(1)
	WorkflowRunsActive.WithLabelValues("wf", "default").Dec()
}
