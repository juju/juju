// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
)

type contextKey string

const (
	traceContextKey contextKey = "trace"
	spanContextKey  contextKey = "span"

	traceIDContextKey    contextKey = "traceID"
	spanIDContextKey     contextKey = "spanID"
	traceFlagsContextKey contextKey = "traceFlags"
)

// TracerFromContext returns a tracer from the context. If no tracer is found,
// an empty tracer is returned.
func TracerFromContext(ctx context.Context) (Tracer, bool) {
	value := ctx.Value(traceContextKey)
	if value == nil {
		return NoopTracer{}, false
	}
	tracer, ok := value.(Tracer)
	if !ok {
		return NoopTracer{}, false
	}
	return tracer, tracer.Enabled()
}

// SpanFromContext returns a span from the context. If no span is found,
// an empty span is returned.
func SpanFromContext(ctx context.Context) Span {
	value := ctx.Value(spanContextKey)
	if value == nil {
		return NoopSpan{}
	}
	span, ok := value.(Span)
	if !ok {
		return NoopSpan{}
	}
	return span
}

// WithTracer returns a new context with the given tracer.
func WithTracer(ctx context.Context, tracer Tracer) context.Context {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return context.WithValue(ctx, traceContextKey, tracer)
}

// InjectTracerIfRequired returns a new context with the given tracer if one
// isn't already set on the context.
func InjectTracerIfRequired(ctx context.Context, tracer Tracer) context.Context {
	// If the tracer is nil, we'll just pass back the context, as that will
	// either have a tracer or it won't. Using nil tracer could invalidate the
	// parent one, so just send it back.
	if tracer == nil {
		return ctx
	}

	// If the parent tracer is parent tracer is not nil, then use that one.
	if value := ctx.Value(traceContextKey); value != nil {
		return ctx
	}
	// If the tracer isn't already found, then inject the new one.
	return context.WithValue(ctx, traceContextKey, tracer)
}

// WithSpan returns a new context with the given span.
func WithSpan(ctx context.Context, span Span) context.Context {
	if span == nil {
		span = NoopSpan{}
	}
	return context.WithValue(ctx, spanContextKey, span)
}

// WithTraceScope returns a new context with the given trace scope (traceID and
// spanID).
func WithTraceScope(ctx context.Context, traceID, spanID string, flags int) context.Context {
	ctx = context.WithValue(ctx, traceFlagsContextKey, flags)
	ctx = context.WithValue(ctx, spanIDContextKey, spanID)
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

// ScopeFromContext returns the traceID, spanID and the flags from the context.
// Both traceID and spanID can be in the form of a hex string or a raw
// string.
func ScopeFromContext(ctx context.Context) (string, string, int) {
	traceID, _ := ctx.Value(traceIDContextKey).(string)
	spanID, _ := ctx.Value(spanIDContextKey).(string)
	flags, _ := ctx.Value(traceFlagsContextKey).(int)
	return traceID, spanID, flags
}

// TraceIDFromContext returns the traceID from the context.
func TraceIDFromContext(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDContextKey).(string)
	return traceID
}

// NoopTracer is a tracer that does nothing.
type NoopTracer struct{}

func (NoopTracer) Start(ctx context.Context, name string, options ...Option) (context.Context, Span) {
	return ctx, NoopSpan{}
}

func (NoopTracer) Enabled() bool {
	return false
}

// NoopSpan is a span that does nothing.
type NoopSpan struct{}

// Scope returns the scope of the span.
func (NoopSpan) Scope() Scope {
	return NoopScope{}
}

// AddEvent will record an event for this span. This is a manual mechanism
// for recording an event, it is useful to log information about what
// happened during the lifetime of a span.
// This is not the same as a log attached to a span, unfortunately the
// OpenTelemetry API does not have a way to record logs yet.
func (NoopSpan) AddEvent(string, ...Attribute) {}

// RecordError will record err as an exception span event for this span. If
// this span is not being recorded or err is nil then this method does
// nothing.
// The attributes is lazy and only called if the span is recording.
func (NoopSpan) RecordError(error, ...Attribute) {}

// End completes the Span. The Span is considered complete and ready to be
// delivered through the rest of the telemetry pipeline after this method
// is called. Therefore, updates to the Span are not allowed after this
// method has been called.
func (NoopSpan) End(...Attribute) {}

// NoopScope is a scope that does nothing.
type NoopScope struct{}

// TraceID returns the trace ID of the span.
func (NoopScope) TraceID() string {
	return ""
}

// SpanID returns the span ID of the span.
func (NoopScope) SpanID() string {
	return ""
}

// TraceFlags returns the trace flags of the span.
func (NoopScope) TraceFlags() int {
	return 0
}

// IsSampled returns if the span is sampled.
func (NoopScope) IsSampled() bool {
	return false
}
