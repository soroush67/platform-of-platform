// Package tracing sets up real OpenTelemetry distributed tracing -
// cross-cutting infrastructure, like httpserver/auth/outbox, not owned
// by any one bounded context. Real spans exported over real OTLP/HTTP
// to a real Jaeger instance (docker-compose.yml's own jaeger service),
// not a stand-in that only logs structured lines pretending to be
// traces - the whole point of tracing is seeing a request's actual path
// across process boundaries (HTTP -> gRPC dispatch -> Worker), which a
// per-process log can't show on its own.
package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Setup configures the global TracerProvider and text-map propagator
// (W3C tracecontext - the standard that lets otelgrpc's client/server
// interceptors carry a trace across the Control Plane <-> Worker gRPC
// boundary, the actually valuable "distributed" half of this). Reads
// OTEL_EXPORTER_OTLP_ENDPOINT (docker-compose.yml sets this to
// "http://jaeger:4318" for both control-plane and worker) - if unset,
// tracing is a real, silent no-op (an OTel SDK "you get a working
// no-op tracer" behavior by design, not a crash), matching this
// codebase's own "degrade honestly, don't fail startup over an
// optional subsystem" posture elsewhere (e.g. idempotency).
func Setup(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		// Jaeger's OTLP/HTTP receiver in this compose topology has no TLS
		// in front of it (internal-network-only, never published to the
		// host) - matching CockroachDB's own --insecure posture in this
		// same dev deployment, not a production TLS decision.
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: creating OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
