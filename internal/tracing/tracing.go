/*
Copyright 2026 Nirmata, Inc.

Use of this source code is governed by the Business Source License 1.1
that can be found in the LICENSE.md file.
*/

// Package tracing provides OTel TracerProvider initialization for OttoFlow binaries.
// Both the controller and the workflow-runner import this package to ensure a consistent
// setup: OTLP gRPC exporter with batch processor when OTEL_EXPORTER_OTLP_ENDPOINT is set,
// and a zero-overhead no-op provider when it is unset.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// InitTracerProvider initializes an OTel TracerProvider and registers it globally.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset the returned provider is a no-op —
// zero allocation, zero CPU overhead for users without a collector.
//
// When set, an OTLP gRPC exporter is initialized with a batch span processor.
// Standard OTel env vars control behaviour:
//   - OTEL_EXPORTER_OTLP_ENDPOINT  — collector address (e.g. "localhost:4317")
//   - OTEL_SERVICE_NAME            — overrides the serviceName argument
//   - OTEL_RESOURCE_ATTRIBUTES     — extra resource attributes
//
// The W3C TraceContext propagator is always registered so that Extract/Inject
// work correctly even on the no-op path.
//
// Returns the provider, a flush function (call before process exit to drain
// in-flight spans), and any init error.
func InitTracerProvider(ctx context.Context, serviceName string) (trace.TracerProvider, func(context.Context) error, error) {
	// Always register the W3C propagator. Both the controller (Inject) and the
	// runner (Extract) rely on it being set before any span context is read or written.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		tp := noop.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, nil, err
	}

	res := resource.NewSchemaless(attribute.String("service.name", serviceName))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	// ForceFlush (not Shutdown) so the runner can drain spans before pod exit
	// without cancelling the SDK's own cleanup routines.
	return tp, tp.ForceFlush, nil
}
