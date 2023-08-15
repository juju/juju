// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/errors"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/tomb.v2"
)

// TracerOptions are options that can be passed to the Tracer.Start() method.
type TracerOption func(*tracerOption)

type tracerOption struct{}

func newTracerOptions() *tracerOption {
	return &tracerOption{}
}

// Tracer is the interface that all tracers must implement.
type Tracer interface {
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
	Start(context.Context, string, ...TracerOption) (context.Context, Span)
}

// Span is the individual component of a trace. It represents a single named
// and timed operation of a workflow that is traced. A Tracer is used to
// create a Span and it is then up to the operation the Span represents to
// properly end the Span when the operation itself ends.
type Span interface {
	// End completes the Span. The Span is considered complete and ready to be
	// delivered through the rest of the telemetry pipeline after this method
	// is called. Therefore, updates to the Span are not allowed after this
	// method has been called.
	End()
}

type tracer struct {
	tomb tomb.Tomb

	namespace    string
	client       otlptrace.Client
	clientTracer trace.Tracer
}

// NewTracerWorker returns a new tracer worker.
func NewTracerWorker(ctx context.Context, namespace string) (TrackedTracer, error) {
	client, clientTracer, err := newClient(ctx, namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &tracer{
		namespace:    namespace,
		client:       client,
		clientTracer: clientTracer,
	}

	t.tomb.Go(t.loop)
	return t, nil
}

// Kill implements the worker.Worker interface.
func (t *tracer) Kill() {
	t.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (t *tracer) Wait() error {
	return t.tomb.Wait()
}

// Start creates a span and a context.Context containing the newly-created span.
func (t *tracer) Start(ctx context.Context, name string, opts ...TracerOption) (context.Context, Span) {
	o := newTracerOptions()
	for _, opt := range opts {
		opt(o)
	}

	var (
		cancel context.CancelFunc
		span   trace.Span
	)
	ctx, cancel = t.scopedContext(ctx)
	ctx, span = t.clientTracer.Start(ctx, name)

	return ctx, &managedSpan{
		span:   span,
		cancel: cancel,
	}
}

func (t *tracer) loop() error {
	defer t.client.Stop(context.Background())

	<-t.tomb.Dying()
	return tomb.ErrDying
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *tracer) scopedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	return w.tomb.Context(ctx), cancel
}

func newClient(ctx context.Context, namespace string) (otlptrace.Client, trace.Tracer, error) {
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("192.168.0.60:4317"),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(namespace)),
	)

	return client, tp.Tracer(namespace), nil
}

func newResource(namespace string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(namespace),
		semconv.ServiceVersion("0.0.1"),
	)
}

type managedSpan struct {
	span   trace.Span
	cancel context.CancelFunc
}

func (s *managedSpan) End() {
	defer s.cancel()
	s.span.End()
}
