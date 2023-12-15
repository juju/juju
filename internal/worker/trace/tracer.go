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

	coretrace "github.com/juju/juju/core/trace"
)

// NewClientFunc is the function signature for creating a new client.
type NewClientFunc func(context.Context, coretrace.TaggedTracerNamespace, string, bool, float64) (Client, ClientTracerProvider, ClientTracer, error)

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
	sampleRatio float64,
	logger Logger,
	newClient NewClientFunc,
) (TrackedTracer, error) {
	client, clientProvider, clientTracer, err := newClient(ctx, namespace, endpoint, insecureSkipVerify, sampleRatio)
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
		span   ClientSpan
	)
	ctx = t.buildRequestContext(ctx)
	ctx, cancel = t.scopedContext(ctx)

	// Grab any attributes from the options and add them to the span.
	attrs := attributes(o.Attributes())
	attrs = append(attrs,
		attribute.String("namespace", t.namespace.Namespace),
		attribute.String("namespace.short", t.namespace.ShortNamespace()),
		attribute.String("namespace.tag", t.namespace.Tag.String()),
		attribute.String("namespace.worker", t.namespace.Worker),
	)

	ctx, span = t.clientTracer.Start(ctx, name, trace.WithAttributes(attrs...))

	if spanContext := span.SpanContext(); spanContext.IsSampled() {
		t.logger.Debugf("SpanContext: trace-id %s, span-id %s", spanContext.TraceID(), spanContext.SpanID())
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
	traceHex, spanHex, flags := coretrace.ScopeFromContext(ctx)
	if traceHex == "" || spanHex == "" {
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
	ctx = coretrace.WithTraceScope(ctx, "", "", 0)
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}

type managedSpan struct {
	span               ClientSpan
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
