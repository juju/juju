// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracing

import (
	"context"
)

// Option are options that can be passed to the Tracer.Start() method.
type Option func(*TracerOption)

type TracerOption struct {
	attributes []Attribute
	stackTrace bool
}

// Attributes returns a slice of attributes for creating a span.
func (t *TracerOption) Attributes() []Attribute {
	return t.attributes
}

// StackTrace returns if the stack trace is enabled on the span on errors.
func (t *TracerOption) StackTrace() bool {
	return t.stackTrace
}

// WithAttributes returns a Option that sets the attributes on the span.
func WithAttributes(attributes ...Attribute) Option {
	return func(o *TracerOption) {
		o.attributes = attributes
	}
}

// WithStackTrace returns a Option that sets the stack trace on the span.
func WithStackTrace() Option {
	return func(o *TracerOption) {
		o.stackTrace = true
	}
}

// NewTracerOptions returns a new tracerOption.
func NewTracerOptions() *TracerOption {
	return &TracerOption{
		stackTrace: true,
	}
}

// Tracer is the interface that all tracers must implement.
type Tracer interface {
	// Start creates a span and a context.Context containing the newly-created
	// span.
	//
	// If the context.Context provided in `ctx` contains a Span then the
	// newly-created Span will be a child of that span, otherwise it will be a
	// root span.
	//
	// Any Span that is created MUST also be ended. This is the responsibility
	// of the user. Implementations of this API may leak memory or other
	// resources if Spans are not ended.
	Start(context.Context, string, ...Option) (context.Context, Span)
}

// Span is the individual component of a trace. It represents a single named
// and timed operation of a workflow that is traced. A Tracer is used to
// create a Span and it is then up to the operation the Span represents to
// properly end the Span when the operation itself ends.
type Span interface {
	// AddEvent will record an event for this span. This is a manual mechanism
	// for recording an event, it is useful to log information about what
	// happened during the lifetime of a span.
	// This is not the same as a log attached to a span, unfortunately the
	// OpenTelemetry API does not have a way to record logs yet.
	AddEvent(string, ...Attribute)

	// RecordError will record err as an exception span event for this span. If
	// this span is not being recorded or err is nil then this method does
	// nothing.
	// The attributes is lazy and only called if the span is recording.
	RecordError(error, ...Attribute)

	// End completes the Span. The Span is considered complete and ready to be
	// delivered through the rest of the telemetry pipeline after this method
	// is called. Therefore, updates to the Span are not allowed after this
	// method has been called.
	End(...Attribute)
}
