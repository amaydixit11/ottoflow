/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

package executor

import (
	"context"
	"fmt"
	"time"
)

// PrometheusClient interface for Prometheus queries
type PrometheusClient interface {
	Query(ctx context.Context, query string, ts time.Time) (PrometheusResult, error)
}

// PrometheusResult represents a Prometheus query result
type PrometheusResult interface {
	Type() string
	GetVector() []PrometheusSample
	GetScalar() float64
}

// PrometheusSample represents a single Prometheus sample
type PrometheusSample interface {
	Metric() map[string]string
	Value() float64
	Timestamp() time.Time
}

// CustomMetricsClient interface for Custom Metrics API queries
type CustomMetricsClient interface {
	GetMetric(ctx context.Context, apiVersion, kind, namespace, name, metricName string) (CustomMetricValue, error)
}

// CustomMetricValue represents a custom metric value
type CustomMetricValue interface {
	MetricName() string
	Value() int64
	Timestamp() time.Time
	WindowSeconds() int64
}

// NoOpPrometheusClient is a no-op implementation when Prometheus is not configured
type NoOpPrometheusClient struct{}

func (n *NoOpPrometheusClient) Query(ctx context.Context, query string, ts time.Time) (PrometheusResult, error) {
	return nil, fmt.Errorf("prometheus client not configured")
}

// NoOpCustomMetricsClient is a no-op implementation when Custom Metrics API is not available
type NoOpCustomMetricsClient struct{}

func (n *NoOpCustomMetricsClient) GetMetric(ctx context.Context, apiVersion, kind, namespace, name, metricName string) (CustomMetricValue, error) {
	return nil, fmt.Errorf("custom metrics client not configured")
}
