/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package metrics

import (
	"context"
	"testing"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name    string
		v       interface{}
		want    float64
		wantErr bool
	}{
		{"bool true", true, 1, false},
		{"bool false", false, 0, false},
		{"float64", float64(3.14), 3.14, false},
		{"float32", float32(2.5), 2.5, false},
		{"int", int(42), 42, false},
		{"int64", int64(100), 100, false},
		{"int32", int32(10), 10, false},
		{"uint", uint(7), 7, false},
		{"uint64", uint64(1), 1, false},
		{"string number", "1.5", 1.5, false},
		{"string invalid", "abc", 0, true},
		{"unsupported", struct{}{}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat64(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("toFloat64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("toFloat64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToFloat64Slice(t *testing.T) {
	tests := []struct {
		name    string
		v       interface{}
		wantLen int
		wantErr bool
	}{
		{"[]interface{}", []interface{}{1.0, 2.0, 3.0}, 3, false},
		{"[]float64", []float64{1, 2}, 2, false},
		{"[]int", []int{10, 20}, 2, false},
		{"single value", 42.0, 1, false},
		{"single int", 7, 1, false},
		{"invalid element", []interface{}{"not", "numbers"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toFloat64Slice(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("toFloat64Slice() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("toFloat64Slice() len = %v, wantLen %v", len(got), tt.wantLen)
			}
		})
	}
}

func TestSanitizeMetricName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"alphanumeric", "ottoflow_workflow_foo", "ottoflow_workflow_foo"},
		{"replace dots", "a.b.c", "a_b_c"},
		{"replace hyphen", "my-metric", "my_metric"},
		{"mixed", "my.metric-name", "my_metric_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMetricName(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeMetricName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

type mockLabelEvaluator struct {
	result interface{}
	err    error
}

func (m *mockLabelEvaluator) EvaluateExpression(ctx context.Context, expr string, vars map[string]interface{}) (interface{}, error) {
	return m.result, m.err
}

func TestEmitOutputMetric_NoMetricDef(t *testing.T) {
	// No panic when output has no metric definition
	EmitOutputMetric(context.Background(), 1.0, ottoflowv1alpha1.Output{
		Name: "out",
		// Metric is nil
	}, "wf", "default", nil, nil)
}

func TestEmitOutputMetric_Counter(t *testing.T) {
	eval := &mockLabelEvaluator{result: "labelval"}
	EmitOutputMetric(context.Background(), 1.0, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:   "my_counter",
			Type:   "counter",
			Help:   "Test counter",
			Labels: []ottoflowv1alpha1.MetricLabel{{Name: "l1", Value: "1"}},
		},
	}, "wf", "default", map[string]interface{}{}, eval)
	// Second call with same name hits AlreadyRegistered path
	EmitOutputMetric(context.Background(), 2.0, ottoflowv1alpha1.Output{
		Name: "out2",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:   "my_counter",
			Type:   "counter",
			Help:   "Test counter",
			Labels: []ottoflowv1alpha1.MetricLabel{{Name: "l1", Value: "1"}},
		},
	}, "wf", "default", map[string]interface{}{}, eval)
}

func TestEmitOutputMetric_Gauge(t *testing.T) {
	eval := &mockLabelEvaluator{result: "v"}
	EmitOutputMetric(context.Background(), 10.0, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:   "my_gauge",
			Type:   "gauge",
			Help:   "Test gauge",
			Labels: []ottoflowv1alpha1.MetricLabel{{Name: "x", Value: "y"}},
		},
	}, "wf", "ns", nil, eval)
}

func TestEmitOutputMetric_Histogram(t *testing.T) {
	eval := &mockLabelEvaluator{result: "v"}
	EmitOutputMetric(context.Background(), []float64{0.1, 0.5, 1.0}, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:    "my_hist",
			Type:    "histogram",
			Help:    "Test histogram",
			Buckets: []float64{0.1, 0.5, 1, 2},
			Labels:  []ottoflowv1alpha1.MetricLabel{{Name: "h", Value: "1"}},
		},
	}, "wf", "ns", nil, eval)
	// Single value as observation
	EmitOutputMetric(context.Background(), 0.25, ottoflowv1alpha1.Output{
		Name: "out2",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:   "my_hist2",
			Type:   "histogram",
			Help:   "Test histogram 2",
			Labels: []ottoflowv1alpha1.MetricLabel{{Name: "h", Value: "2"}},
		},
	}, "wf", "ns", nil, eval)
}

func TestEmitOutputMetric_UnknownType(t *testing.T) {
	// Should not panic; logs and skips
	EmitOutputMetric(context.Background(), 1.0, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name: "unknown_metric",
			Type: "unknown",
			Help: "Help",
		},
	}, "wf", "ns", nil, &mockLabelEvaluator{})
}

func TestEmitOutputMetric_LabelEvalError(t *testing.T) {
	eval := &mockLabelEvaluator{err: context.DeadlineExceeded}
	// When label evaluation fails, metric is skipped (no panic)
	EmitOutputMetric(context.Background(), 1.0, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name:   "skip_counter",
			Type:   "counter",
			Help:   "Skip",
			Labels: []ottoflowv1alpha1.MetricLabel{{Name: "l", Value: "expr"}},
		},
	}, "wf", "ns", nil, eval)
}

func TestEmitOutputMetric_InvalidValueForCounter(t *testing.T) {
	// Value that cannot convert to float64 - should skip without panic
	EmitOutputMetric(context.Background(), struct{}{}, ottoflowv1alpha1.Output{
		Name: "out",
		Metric: &ottoflowv1alpha1.OutputMetric{
			Name: "bad_counter",
			Type: "counter",
			Help: "Bad",
		},
	}, "wf", "ns", nil, &mockLabelEvaluator{})
}
