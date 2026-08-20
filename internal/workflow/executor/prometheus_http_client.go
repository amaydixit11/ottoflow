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

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// prometheusQueryAPI is the subset of prometheus v1 API used for instant queries.
// Inject a mock in tests to avoid a real Prometheus server.
// Signature matches prometheusv1.API so real API can be assigned.
type prometheusQueryAPI interface {
	Query(ctx context.Context, query string, ts time.Time, opts ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error)
}

// HTTPPrometheusClient queries a Prometheus server over its v1 HTTP API.
type HTTPPrometheusClient struct {
	api prometheusQueryAPI
}

// NewHTTPPrometheusClient creates a client that connects to the given Prometheus URL
// (e.g. "http://localhost:9090").
func NewHTTPPrometheusClient(url string) (*HTTPPrometheusClient, error) {
	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: url,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Prometheus API client: %w", err)
	}
	return &HTTPPrometheusClient{
		api: prometheusv1.NewAPI(client),
	}, nil
}

// NewHTTPPrometheusClientWithAPI creates a client that uses the given API (for tests).
func NewHTTPPrometheusClientWithAPI(api prometheusQueryAPI) *HTTPPrometheusClient {
	return &HTTPPrometheusClient{api: api}
}

// Query executes a PromQL instant query at the given timestamp.
func (c *HTTPPrometheusClient) Query(ctx context.Context, query string, ts time.Time) (PrometheusResult, error) {
	result, warnings, err := c.api.Query(ctx, query, ts)
	if err != nil {
		return nil, fmt.Errorf("prometheus query %q: %w", query, err)
	}
	_ = warnings

	switch v := result.(type) {
	case model.Vector:
		return &vectorResult{vector: v}, nil
	case *model.Scalar:
		return &scalarResult{scalar: v}, nil
	default:
		return &vectorResult{}, nil
	}
}

// vectorResult wraps a Prometheus instant-vector response.
type vectorResult struct {
	vector model.Vector
}

func (r *vectorResult) Type() string       { return "vector" }
func (r *vectorResult) GetScalar() float64 { return 0 }
func (r *vectorResult) GetVector() []PrometheusSample {
	samples := make([]PrometheusSample, len(r.vector))
	for i, s := range r.vector {
		samples[i] = &promSample{sample: s}
	}
	return samples
}

// scalarResult wraps a Prometheus scalar response.
type scalarResult struct {
	scalar *model.Scalar
}

func (r *scalarResult) Type() string                  { return "scalar" }
func (r *scalarResult) GetVector() []PrometheusSample { return nil }
func (r *scalarResult) GetScalar() float64 {
	if r.scalar == nil {
		return 0
	}
	return float64(r.scalar.Value)
}

// promSample adapts a model.Sample to the PrometheusSample interface.
type promSample struct {
	sample *model.Sample
}

func (s *promSample) Value() float64 { return float64(s.sample.Value) }
func (s *promSample) Timestamp() time.Time {
	return s.sample.Timestamp.Time()
}
func (s *promSample) Metric() map[string]string {
	m := make(map[string]string, len(s.sample.Metric))
	for k, v := range s.sample.Metric {
		m[string(k)] = string(v)
	}
	return m
}
