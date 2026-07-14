// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"time"

	"github.com/juju/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
)

// NewClientFunc is the function signature for creating a new client.
type NewClientFunc func(
	context.Context,
	coretrace.TaggedTracerNamespace,
	string, string, string,
	bool,
	float64,
	time.Duration,
	logger.Logger,
) (Client, ClientTracerProvider, ClientTracer, error)

type tracer struct {
	tomb tomb.Tomb

	namespace          coretrace.TaggedTracerNamespace
	client             Client
	clientProvider     ClientTracerProvider
	clientTracer       ClientTracer
	stackTracesEnabled bool
	logger             logger.Logger
}

// NewTracerWorker returns a new tracer worker.
func NewTracerWorker(
	ctx context.Context,
	namespace coretrace.TaggedTracerNamespace,
	httpEndpoint, grpcEndpoint, caCertificate string,
	insecureSkipVerify bool,
	stackTracesEnabled bool,
	sampleRatio float64,
	tailSamplingThreshold time.Duration,
	logger logger.Logger,
	newClient NewClientFunc,
) (TrackedTracer, error) {
	client, clientProvider, clientTracer, err := newClient(ctx,
		namespace,
		httpEndpoint,
		grpcEndpoint,
		caCertificate,
		insecureSkipVerify,
		sampleRatio,
		tailSamplingThreshold,
		logger,
	)
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
	select {
	case <-t.tomb.Dying():
		return ctx, coretrace.NoopSpan{}
	default:
	}

	o := coretrace.NewTracerOptions()
	for _, opt := range opts {
		opt(o)
	}

	ctx = t.buildRequestContext(ctx)
	originalCtx := ctx

	// Tie the lifetime of the internal tracing context to the worker without
	// allowing tracer shutdown to cancel the caller's operation context.
	tracerCtx := t.tomb.Context(ctx)
	if t.isDyingContext(originalCtx) {
		return originalCtx, coretrace.NoopSpan{}
	}

	// Grab any attributes from the options and add them to the span.
	attrs := attributes(o.Attributes())
	attrs = append(attrs,
		attribute.String("namespace", t.namespace.Namespace),
		attribute.String("namespace.short", t.namespace.ShortNamespace()),
		attribute.Stringer("namespace.tag", t.namespace.Tag),
		attribute.Stringer("namespace.kind", t.namespace.Kind),
	)

	var span ClientSpan
	tracerCtx, span = t.clientTracer.Start(tracerCtx, name, trace.WithAttributes(attrs...))
	if t.isDyingContext(originalCtx) {
		span.End()
		return originalCtx, coretrace.NoopSpan{}
	}

	// If the span is sampled then we should log the trace and span ID.
	if t.logger.IsLevelEnabled(logger.TRACE) {
		if spanContext := span.SpanContext(); spanContext.IsSampled() {
			t.logger.Tracef(tracerCtx, "SpanContext: trace-id %s, span-id %s", spanContext.TraceID(), spanContext.SpanID())
		}
	}

	managed := &managedSpan{
		span:               span,
		scope:              managedScope{span: span},
		stackTracesEnabled: t.requiresStackTrace(o.StackTrace()),
	}
	return contextWithSpan(originalCtx, span, &limitedSpan{
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
			t.logger.Infof(ctx, "failed to flush client: %v", err)
		}

		if err := t.client.Stop(ctx); err != nil {
			t.logger.Infof(ctx, "failed to stop client: %v", err)
		}

		if err := t.clientProvider.Shutdown(ctx); err != nil {
			t.logger.Infof(ctx, "failed to shutdown provider: %v", err)
		}
	}()

	<-t.tomb.Dying()
	return tomb.ErrDying
}

func (t *tracer) isDyingContext(originalCtx context.Context) bool {
	if originalCtx.Err() != nil {
		return false
	}
	select {
	case <-t.tomb.Dying():
		return true
	default:
		return false
	}
}

func contextWithSpan(ctx context.Context, span ClientSpan, coreSpan coretrace.Span) context.Context {
	if otelSpan, ok := span.(trace.Span); ok {
		ctx = trace.ContextWithSpan(ctx, otelSpan)
	}
	spanContext := span.SpanContext()
	if spanContext.TraceID().IsValid() {
		ctx = coretrace.WithTraceID(ctx, spanContext.TraceID().String())
	}
	return coretrace.WithSpan(ctx, coreSpan)
}

// buildRequestContext returns a context that may contain a remote span context.
func (t *tracer) buildRequestContext(ctx context.Context) context.Context {
	traceHex, spanHex, flags, ok := coretrace.ScopeFromContext(ctx)
	if !ok {
		return ctx
	}

	traceID, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		// There is clearly something wrong with the trace ID, so we
		// should remove it from all future requests. That way we don't attempt
		// to parse it again.
		return coretrace.WithTraceScope(ctx, "", "", 0)
	}
	spanID, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		// There is clearly something wrong with the span ID, so we
		// should remove it from all future requests. That way we don't attempt
		// to parse it again.
		return coretrace.WithTraceScope(ctx, "", "", 0)
	}

	var traceFlags trace.TraceFlags
	traceFlags = traceFlags.WithSampled((flags & 1) == 1)

	// It might be wise to encode more additional information into the context.
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: traceFlags,
	})

	// We have a remote span context, so we should use it. We should then remove
	// the traceID and spanID from the context so that we don't attempt to parse
	// them again.
	ctx = coretrace.RemoveTraceScope(ctx)

	// Add the trace ID to the context so that we can log it.
	ctx = coretrace.WithTraceID(ctx, traceID.String())
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

type managedSpan struct {
	span               ClientSpan
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
	s.span.SetAttributes(attributes(attrs)...)
	s.span.End(trace.WithStackTrace(s.stackTracesEnabled))
}

type managedScope struct {
	span ClientSpan
}

// TraceID returns the trace ID of the span.
func (s managedScope) TraceID() string {
	return s.span.SpanContext().TraceID().String()
}

// SpanID returns the span ID of the span.
func (s managedScope) SpanID() string {
	return s.span.SpanContext().SpanID().String()
}

// TraceFlags returns the trace flags of the span.
func (s managedScope) TraceFlags() int {
	return int(s.span.SpanContext().TraceFlags())
}

// IsSampled returns if the span is sampled.
func (s managedScope) IsSampled() bool {
	return s.span.SpanContext().IsSampled()
}

// limitedSpan prevents you shooting yourself in the foot by ending a span that
// you don't own.
type limitedSpan struct {
	coretrace.Span
	logger logger.Logger
}

func (s *limitedSpan) End(attrs ...coretrace.Attribute) {
	s.logger.Warningf(context.Background(), "attempted to end a span that you don't own")
}

func attributes(attrs []coretrace.Attribute) []attribute.KeyValue {
	kv := make([]attribute.KeyValue, len(attrs))
	for i, attr := range attrs {
		kv[i] = attribute.String(attr.Key(), attr.Value())
	}
	return kv
}
