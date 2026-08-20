/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package metrics

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	ottoflowv1alpha1 "github.com/nirmata/ottoflow/api/v1alpha1"
)

const (
	customMetricPrefix = "ottoflow_workflow_"
)

// LabelEvaluator evaluates CEL expressions for metric labels
type LabelEvaluator interface {
	EvaluateExpression(ctx context.Context, expr string, vars map[string]interface{}) (interface{}, error)
}

// EmitOutputMetric emits a custom metric for an output.
// vars must include inputs, variables, steps, and outputs (for outputs that reference earlier ones).
func EmitOutputMetric(
	ctx context.Context,
	value interface{},
	outputDef ottoflowv1alpha1.Output,
	workflowName, namespace string,
	vars map[string]interface{},
	evaluator LabelEvaluator,
) {
	if outputDef.Metric == nil {
		return
	}
	m := outputDef.Metric

	// Build labels from CEL expressions
	labels := map[string]string{
		"workflow":  workflowName,
		"namespace": namespace,
	}
	for _, l := range m.Labels {
		result, err := evaluator.EvaluateExpression(ctx, l.Value, vars)
		if err != nil {
			klog.V(4).InfoS("Failed to evaluate metric label, skipping metric",
				"metric", m.Name, "label", l.Name, "error", err)
			return
		}
		labels[l.Name] = fmt.Sprintf("%v", result)
	}

	metricName := sanitizeMetricName(customMetricPrefix + m.Name)
	help := m.Help
	if help == "" {
		help = "Custom workflow metric"
	}

	switch m.Type {
	case "counter":
		f, err := toFloat64(value)
		if err != nil {
			klog.V(4).InfoS("Failed to convert metric value to float, skipping",
				"metric", metricName, "error", err)
			return
		}
		counter, err := getOrCreateCounter(metricName, help, labels)
		if err != nil {
			klog.V(4).InfoS("Failed to get/create counter, skipping", "metric", metricName, "error", err)
			return
		}
		counter.Add(f)
	case "gauge":
		f, err := toFloat64(value)
		if err != nil {
			klog.V(4).InfoS("Failed to convert metric value to float, skipping",
				"metric", metricName, "error", err)
			return
		}
		gauge, err := getOrCreateGauge(metricName, help, labels)
		if err != nil {
			klog.V(4).InfoS("Failed to get/create gauge, skipping", "metric", metricName, "error", err)
			return
		}
		gauge.Set(f)
	case "histogram":
		observations, err := toFloat64Slice(value)
		if err != nil {
			klog.V(4).InfoS("Failed to convert metric value for histogram, skipping",
				"metric", metricName, "error", err)
			return
		}
		histogram, err := getOrCreateHistogram(metricName, help, labels, m.Buckets)
		if err != nil {
			klog.V(4).InfoS("Failed to get/create histogram, skipping", "metric", metricName, "error", err)
			return
		}
		for _, obs := range observations {
			histogram.Observe(obs)
		}
	default:
		klog.V(4).InfoS("Unknown metric type, skipping", "metric", metricName, "type", m.Type)
	}
}

var (
	customCounters   = make(map[string]*prometheus.CounterVec)
	customGauges     = make(map[string]*prometheus.GaugeVec)
	customHistograms = make(map[string]*prometheus.HistogramVec)
)

func getOrCreateCounter(name, help string, labels map[string]string) (prometheus.Counter, error) {
	key := name
	if _, ok := customCounters[key]; !ok {
		labelNames := make([]string, 0, len(labels))
		for k := range labels {
			labelNames = append(labelNames, k)
		}
		vec := prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: name, Help: help},
			labelNames,
		)
		if err := prometheus.Register(vec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				customCounters[key] = are.ExistingCollector.(*prometheus.CounterVec)
			} else {
				return nil, err
			}
		} else {
			customCounters[key] = vec
		}
	}
	return customCounters[key].GetMetricWith(labels)
}

func getOrCreateGauge(name, help string, labels map[string]string) (prometheus.Gauge, error) {
	key := name
	if _, ok := customGauges[key]; !ok {
		labelNames := make([]string, 0, len(labels))
		for k := range labels {
			labelNames = append(labelNames, k)
		}
		vec := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: name, Help: help},
			labelNames,
		)
		if err := prometheus.Register(vec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				customGauges[key] = are.ExistingCollector.(*prometheus.GaugeVec)
			} else {
				return nil, err
			}
		} else {
			customGauges[key] = vec
		}
	}
	return customGauges[key].GetMetricWith(labels)
}

func getOrCreateHistogram(name, help string, labels map[string]string, buckets []float64) (prometheus.Observer, error) {
	key := name
	if _, ok := customHistograms[key]; !ok {
		labelNames := make([]string, 0, len(labels))
		for k := range labels {
			labelNames = append(labelNames, k)
		}
		opts := prometheus.HistogramOpts{Name: name, Help: help}
		if len(buckets) > 0 {
			opts.Buckets = buckets
		} else {
			opts.Buckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}
		}
		vec := prometheus.NewHistogramVec(opts, labelNames)
		if err := prometheus.Register(vec); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				customHistograms[key] = are.ExistingCollector.(*prometheus.HistogramVec)
			} else {
				return nil, err
			}
		} else {
			customHistograms[key] = vec
		}
	}
	return customHistograms[key].GetMetricWith(labels)
}

func sanitizeMetricName(name string) string {
	// Prometheus metric names: [a-zA-Z_:][a-zA-Z0-9_:]*
	re := regexp.MustCompile(`[^a-zA-Z0-9_:]`)
	return re.ReplaceAllString(name, "_")
}

func toFloat64(v interface{}) (float64, error) {
	switch x := v.(type) {
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case uint:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string to float: %w", err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toFloat64Slice(v interface{}) ([]float64, error) {
	switch x := v.(type) {
	case []interface{}:
		out := make([]float64, 0, len(x))
		for _, item := range x {
			f, err := toFloat64(item)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		}
		return out, nil
	case []float64:
		return x, nil
	case []int:
		out := make([]float64, len(x))
		for i, n := range x {
			out[i] = float64(n)
		}
		return out, nil
	default:
		// Single value - treat as one observation
		f, err := toFloat64(v)
		if err != nil {
			return nil, err
		}
		return []float64{f}, nil
	}
}
