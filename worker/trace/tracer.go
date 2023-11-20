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
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
}

// ClientTracerProvider is the interface for a tracer provider.
type ClientTracerProvider interface {
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// NewClientFunc is the function signature for creating a new client.
type NewClientFunc func(context.Context, coretrace.TaggedTracerNamespace, string, bool) (Client, ClientTracerProvider, ClientTracer, error)

type tracer struct {
	tomb tomb.Tomb

	namespace          coretrace.TaggedTracerNamespace
	client             Client
	clientProvider     ClientTracerProvider
	clientTracer       ClientTracer
	stackTracesEnabled bool
	logger             Logger
}

// NewTracerWorker returns a new tracer worker.
func NewTracerWorker(
	ctx context.Context,
	namespace coretrace.TaggedTracerNamespace,
	endpoint string,
	insecureSkipVerify bool,
	stackTracesEnabled bool,
	logger Logger,
	newClient NewClientFunc,
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
	ctx = t.buildRequestContext(ctx)
	ctx, cancel = t.scopedContext(ctx)

	// Grab any attributes from the options and add them to the span.
	attrs := attributes(o.Attributes())
	attrs = append(attrs,
		attribute.String("namespace", t.namespace.Namespace),
		attribute.String("namespace.short", t.namespace.ShortNamespace()),
		attribute.String("namespace.tag", t.namespace.Tag.String()),
		attribute.String("namespace.kind", string(t.namespace.Kind)),
	)

	ctx, span = t.clientTracer.Start(ctx, name, trace.WithAttributes(attrs...))

	if t.logger.IsTraceEnabled() {
		spanContext := span.SpanContext()
		t.logger.Tracef("SpanContext: span-id %s, trace-id %s", spanContext.SpanID(), spanContext.TraceID())
	}

	managed := &managedSpan{
		span:               span,
		cancel:             cancel,
		scope:              managedScope{span: span},
		stackTracesEnabled: t.requiresStackTrace(o.StackTrace()),
	}
	return coretrace.WithSpan(ctx, &limitedSpan{
		Span:   managed,
		logger: t.logger,
	}), managed
}

// Enabled returns if the tracer is enabled.
func (t *tracer) Enabled() bool {
	return true
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

		if err := t.clientProvider.ForceFlush(ctx); err != nil {
			t.logger.Infof("failed to flush client: %v", err)
		}

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

// buildRequestContext returns a context that may contain a remote span context.
func (t *tracer) buildRequestContext(ctx context.Context) context.Context {
	traceID, spanID := coretrace.ScopeFromContext(ctx)
	if traceID == "" || spanID == "" {
		return ctx
	}
	traceHex, err := trace.TraceIDFromHex(traceID)
	if err != nil {
		// There is clearly something wrong with the trace ID, so we
		// should remove it from all future requests. That way we don't attempt
		// to parse it again.
		return coretrace.WithTraceScope(ctx, "", "")
	}
	spanHex, err := trace.SpanIDFromHex(spanID)
	if err != nil {
		// There is clearly something wrong with the span ID, so we
		// should remove it from all future requests. That way we don't attempt
		// to parse it again.
		return coretrace.WithTraceScope(ctx, "", "")
	}

	// It might be wise to encode more additional information into the context.
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceHex,
		SpanID:     spanHex,
		TraceFlags: trace.FlagsSampled,
	})

	// We have a remote span context, so we should use it. We should then remove
	// the traceID and spanID from the context so that we don't attempt to parse
	// them again.
	ctx = coretrace.WithTraceScope(ctx, "", "")
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

// NewClient returns a new tracing client.
func NewClient(ctx context.Context, namespace coretrace.TaggedTracerNamespace, endpoint string, insecureSkipVerify bool) (Client, ClientTracerProvider, ClientTracer, error) {
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

	serviceName := fmt.Sprintf("juju-%s", namespace.Kind)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(serviceName, namespace.Namespace)),
	)

	return client, tp, tp.Tracer(namespace.String()), nil
}

func newResource(serviceName, serviceID string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(version.Current.String()),
		semconv.ServiceInstanceID(serviceID),
	)
}

type managedSpan struct {
	span               trace.Span
	cancel             context.CancelFunc
	scope              coretrace.Scope
	stackTracesEnabled bool
}

// Scope returns the scope of the span.
func (s *managedSpan) Scope() coretrace.Scope {
	return s.scope
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

// End completes the Span. The Span is considered complete and ready to be
// delivered through the rest of the telemetry pipeline after this method
// is called. Therefore, updates to the Span are not allowed after this
// method has been called.
func (s *managedSpan) End(attrs ...coretrace.Attribute) {
	defer s.cancel()

	s.span.SetAttributes(attributes(attrs)...)
	s.span.End(trace.WithStackTrace(s.stackTracesEnabled))
}

type managedScope struct {
	span trace.Span
}

// TraceID returns the trace ID of the span.
func (s managedScope) TraceID() string {
	return s.span.SpanContext().TraceID().String()
}

// SpanID returns the span ID of the span.
func (s managedScope) SpanID() string {
	return s.span.SpanContext().SpanID().String()
}

// limitedSpan prevents you shooting yourself in the foot by ending a span that
// you don't own.
type limitedSpan struct {
	coretrace.Span
	logger Logger
}

func (s *limitedSpan) End(attrs ...coretrace.Attribute) {
	s.logger.Warningf("attempted to end a span that you don't own")
}

func attributes(attrs []coretrace.Attribute) []attribute.KeyValue {
	kv := make([]attribute.KeyValue, len(attrs))
	for _, attr := range attrs {
		kv = append(kv, attribute.String(attr.Key(), attr.Value()))
	}
	return kv
}
