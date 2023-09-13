// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/tomb.v2"

	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/version"
)

type tracer struct {
	tomb tomb.Tomb

	namespace          string
	client             otlptrace.Client
	clientProvider     *sdktrace.TracerProvider
	clientTracer       trace.Tracer
	stackTracesEnabled bool
	logger             Logger
}

// NewTracerWorker returns a new tracer worker.
func NewTracerWorker(
	ctx context.Context,
	namespace string,
	endpoint string,
	insecureSkipVerify bool,
	stackTracesEnabled bool,
	logger Logger,
) (TrackedTracer, error) {
	client, clientProvider, clientTracer, err := newClient(ctx, namespace, endpoint, insecureSkipVerify)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &tracer{
		namespace:          namespace,
		client:             client,
		clientProvider:     clientProvider,
		clientTracer:       clientTracer,
		stackTracesEnabled: stackTracesEnabled,
		logger:             logger,
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
func (t *tracer) Start(ctx context.Context, name string, opts ...coretrace.Option) (context.Context, coretrace.Span) {
	o := coretrace.NewTracerOptions()
	for _, opt := range opts {
		opt(o)
	}

	// Tie the lifetime of the span to the lifetime of the worker. If the
	// worker dies then the span will be ended. The consumer of the span
	// should use the context returned from this method to ensure that the
	// they also die at the same time as the worker.
	var (
		cancel context.CancelFunc
		span   trace.Span
	)
	ctx, cancel = t.scopedContext(ctx)

	// Grab any attributes from the options and add them to the span.
	attrs := attributes(o.Attributes())
	ctx, span = t.clientTracer.Start(ctx, name, trace.WithAttributes(attrs...))

	if t.logger.IsTraceEnabled() {
		spanContext := span.SpanContext()
		t.logger.Tracef("SpanContext: span-id %s, trace-id %s", spanContext.SpanID(), spanContext.TraceID())
	}

	return ctx, &managedSpan{
		span:               span,
		cancel:             cancel,
		stackTracesEnabled: t.requiresStackTrace(o.StackTrace()),
	}
}

// requiresStackTrace returns true if the stack trace should be enabled on the
// span or if the stack trace is enabled on the tracer (globally).
func (t *tracer) requiresStackTrace(spanStackTrace bool) bool {
	if spanStackTrace {
		return true
	}
	return t.stackTracesEnabled
}

func (t *tracer) loop() error {
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		if err := t.client.Stop(ctx); err != nil {
			t.logger.Infof("failed to stop client: %v", err)
		}

		if err := t.clientProvider.Shutdown(ctx); err != nil {
			t.logger.Infof("failed to shutdown provider: %v", err)
		}
	}()

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

func newClient(ctx context.Context, namespace, endpoint string, insecureSkipVerify bool) (otlptrace.Client, *sdktrace.TracerProvider, trace.Tracer, error) {
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
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(namespace)),
	)

	return client, tp, tp.Tracer(namespace), nil
}

func newResource(namespace string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(fmt.Sprintf("juju-%s", namespace)),
		semconv.ServiceVersion(version.Current.String()),
	)
}

type managedSpan struct {
	span               trace.Span
	cancel             context.CancelFunc
	stackTracesEnabled bool
}

// AddEvent will record an event for this span. This is a manual mechanism
// for recording an event, it is useful to log information about what
// happened during the lifetime of a span.
func (s *managedSpan) AddEvent(message string, attrs ...coretrace.Attribute) {
	// According to the docs, events can only be recorded if the span
	// is being recorded.
	if s.span.IsRecording() {
		return
	}

	s.span.AddEvent(message, trace.WithAttributes(attributes(attrs)...))
}

// RecordError will record err as an exception span event for this span. This
// also sets the span status to Error if the error is not nil.
func (s *managedSpan) RecordError(err error, attrs ...coretrace.Attribute) {
	if err == nil {
		return
	}

	s.span.RecordError(err, trace.WithAttributes(attributes(attrs)...))
	s.span.SetStatus(codes.Error, err.Error())
}

func (s *managedSpan) End(attrs ...coretrace.Attribute) {
	defer s.cancel()

	s.span.SetAttributes(attributes(attrs)...)
	s.span.End(trace.WithStackTrace(s.stackTracesEnabled))
}

func attributes(attrs []coretrace.Attribute) []attribute.KeyValue {
	kv := make([]attribute.KeyValue, len(attrs))
	for _, attr := range attrs {
		kv = append(kv, attribute.String(attr.Key(), attr.Value()))
	}
	return kv
}
