// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"
	"runtime"
	"strings"
)

// Option are options that can be passed to the Tracer.Start() method.
type Option func(*TracerOption)

type TracerOption struct {
	name       string
	attributes func() map[string]string
	stackTrace bool
}

// Name returns the name of the span.
func (t *TracerOption) Name() string {
	return t.name
}

// Attributes returns the attributes on the span.
func (t *TracerOption) Attributes() func() map[string]string {
	return t.attributes
}

// StackTrace returns if the stack trace is enabled on the span on errors.
func (t *TracerOption) StackTrace() bool {
	return t.stackTrace
}

// WithAttributes returns a Option that sets the attributes on the span.
func WithAttributes(attributes func() map[string]string) Option {
	return func(o *TracerOption) {
		o.attributes = attributes
	}
}

// WithName returns a Option that sets the name on the span.
func WithName(name string) Option {
	return func(o *TracerOption) {
		o.name = name
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
	Start(context.Context, string, ...Option) (context.Context, Span)
}

// Span is the individual component of a trace. It represents a single named
// and timed operation of a workflow that is traced. A Tracer is used to
// create a Span and it is then up to the operation the Span represents to
// properly end the Span when the operation itself ends.
type Span interface {
	// RecordError will record err as an exception span event for this span. If
	// this span is not being recorded or err is nil then this method does
	// nothing.
	// The attributes is lazy and only called if the span is recording.
	RecordError(error, func() map[string]string)

	// End completes the Span. The Span is considered complete and ready to be
	// delivered through the rest of the telemetry pipeline after this method
	// is called. Therefore, updates to the Span are not allowed after this
	// method has been called.
	End()
}

// Start returns a new context with the given tracer.
func Start(ctx context.Context, options ...Option) (context.Context, Span) {
	// Get caller frame.
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	if n < 1 {
		// TODO (stickupkid): Log a warning when this happens.
		return ctx, noopSpan{}
	}

	fn := runtime.FuncForPC(pcs[0])
	name := fn.Name()
	if lastSlash := strings.LastIndexByte(name, '/'); lastSlash > 0 {
		name = name[lastSlash+1:]
	}

	// Tracer is always guaranteed to be returned here. If there is no tracer
	// available it will return a noop tracer.
	tracer := FromContext(ctx)
	return tracer.Start(ctx, name, options...)
}
