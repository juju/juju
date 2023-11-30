// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"

	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/version"
)

// This file solely exists so that we do not tie ourselves to OTEL directly
// into Juju. This allows us to swap out the tracing implementation in the
// future if we need to. I've retooled a codebase from Jaeger to opentracing
// and this lesson was learned the hard way.

// Client manages connections to the collector, handles the
// transformation of data into wire format, and the transmission of that
// data to the collector.
type Client interface {
	// Start should establish connection(s) to endpoint(s). It is
	// called just once by the exporter, so the implementation
	// does not need to worry about idempotence and locking.
	Start(ctx context.Context) error

	// Stop should close the connections. The function is called
	// only once by the exporter, so the implementation does not
	// need to worry about idempotence, but it may be called
	// concurrently with UploadTraces, so proper
	// locking is required. The function serves as a
	// synchronization point - after the function returns, the
	// process of closing connections is assumed to be finished.
	Stop(ctx context.Context) error
}

// ClientSpan is directly equivalent to the opentelemetry Span interface, minus
// the embedded interface.
type ClientSpan interface {
	// End completes the Span. The Span is considered complete and ready to be
	// delivered through the rest of the telemetry pipeline after this method
	// is called. Therefore, updates to the Span are not allowed after this
	// method has been called.
	End(options ...trace.SpanEndOption)

	// AddEvent adds an event with the provided name and options.
	AddEvent(name string, options ...trace.EventOption)

	// IsRecording returns the recording state of the Span. It will return
	// true if the Span is active and events can be recorded.
	IsRecording() bool

	// RecordError will record err as an exception span event for this span. An
	// additional call to SetStatus is required if the Status of the Span should
	// be set to Error, as this method does not change the Span status. If this
	// span is not being recorded or err is nil then this method does nothing.
	RecordError(err error, options ...trace.EventOption)

	// SpanContext returns the SpanContext of the Span. The returned SpanContext
	// is usable even after the End method has been called for the Span.
	SpanContext() trace.SpanContext

	// SetStatus sets the status of the Span in the form of a code and a
	// description, provided the status hasn't already been set to a higher
	// value before (OK > Error > Unset). The description is only included in a
	// status when the code is for an error.
	SetStatus(code codes.Code, description string)

	// SetName sets the Span name.
	SetName(name string)

	// SetAttributes sets kv as attributes of the Span. If a key from kv
	// already exists for an attribute of the Span it will be overwritten with
	// the value contained in kv.
	SetAttributes(kv ...attribute.KeyValue)

	// TracerProvider returns a TracerProvider that can be used to generate
	// additional Spans on the same telemetry pipeline as the current Span.
	TracerProvider() trace.TracerProvider
}

// Tracer is the creator of Spans.
type ClientTracer interface {
	// Start creates a span and a context.Context containing the newly-created span.
	//
	// If the context.Context provided in `ctx` contains a Span then the newly-created
	// Span will be a child of that span, otherwise it will be a root span. This behavior
	// can be overridden by providing `WithNewRoot()` as a SpanOption, causing the
	// newly-created Span to be a root span even if `ctx` contains a Span.
	//
	// When creating a Span it is recommended to provide all known span attributes using
	// the `WithAttributes()` SpanOption as samplers will only have access to the
	// attributes provided when a Span is created.
	//
	// Any Span that is created MUST also be ended. This is the responsibility of the user.
	// Implementations of this API may leak memory or other resources if Spans are not ended.
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, ClientSpan)
}

// ClientTracerProvider is the interface for a tracer provider.
type ClientTracerProvider interface {
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// NewClient returns a new tracing client.
func NewClient(ctx context.Context, namespace coretrace.TaggedTracerNamespace, endpoint string, insecureSkipVerify bool, sampleRatio float64) (Client, ClientTracerProvider, ClientTracer, error) {
	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecureSkipVerify {
		options = append(options, otlptracegrpc.WithInsecure())
	}

	client := otlptracegrpc.NewClient(options...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRatio)),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(namespace.ServiceName())),
	)
	return client, tp, clientSpanShim{tp.Tracer(namespace.String())}, nil
}

// clientSpanShim exists to mask out the embedded interface within the
// trace.Span
type clientSpanShim struct {
	tracer trace.Tracer
}

// Start creates a span and a context.Context containing the newly-created span.
//
// If the context.Context provided in `ctx` contains a Span then the newly-created
// Span will be a child of that span, otherwise it will be a root span. This behavior
// can be overridden by providing `WithNewRoot()` as a SpanOption, causing the
// newly-created Span to be a root span even if `ctx` contains a Span.
//
// When creating a Span it is recommended to provide all known span attributes using
// the `WithAttributes()` SpanOption as samplers will only have access to the
// attributes provided when a Span is created.
//
// Any Span that is created MUST also be ended. This is the responsibility of the user.
// Implementations of this API may leak memory or other resources if Spans are not ended.
func (s clientSpanShim) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, ClientSpan) {
	return s.tracer.Start(ctx, spanName, opts...)
}

func newResource(serviceName string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(version.Current.String()),
	)
}
