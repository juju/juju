// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
)

type contextKey string

const traceContextKey contextKey = "trace"

// FromContext returns a tracer from the context. If no tracer is found,
// an empty tracer is returned.
func FromContext(ctx context.Context) Tracer {
	value := ctx.Value(traceContextKey)
	if value == nil {
		return NoopTracer{}
	}
	tracer, ok := value.(Tracer)
	if !ok {
		return NoopTracer{}
	}
	return tracer
}

// WithTracer returns a new context with the given tracer.
func WithTracer(ctx context.Context, tracer Tracer) context.Context {
	if tracer == nil {
		tracer = NoopTracer{}
	}
	return context.WithValue(ctx, traceContextKey, tracer)
}

// NoopTracer is a tracer that does nothing.
type NoopTracer struct{}

func (NoopTracer) Start(ctx context.Context, name string, options ...Option) (context.Context, Span) {
	return ctx, NoopSpan{}
}

// NoopSpan is a span that does nothing.
type NoopSpan struct{}

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
