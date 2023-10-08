// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (contextSuite) TestTracerFromContextEmpty(c *gc.C) {
	tracer, enabled := TracerFromContext(context.Background())
	c.Assert(tracer, gc.NotNil)
	c.Assert(enabled, gc.Equals, false)

	ctx, span := tracer.Start(context.Background(), "test")
	c.Assert(ctx, gc.NotNil)
	c.Assert(span, gc.NotNil)

	c.Check(span, gc.Equals, NoopSpan{})
}

func (contextSuite) TestTracerFromContextTracer(c *gc.C) {
	tracer, enabled := TracerFromContext(WithTracer(context.Background(), stubTracer{}))
	c.Assert(tracer, gc.NotNil)
	c.Assert(enabled, gc.Equals, true)

	ctx, span := tracer.Start(context.Background(), "test")
	c.Assert(ctx, gc.NotNil)
	c.Assert(span, gc.NotNil)

	// Ensure that we get the correct span.
	c.Check(span, gc.Equals, stubSpan{})
	c.Check(span, gc.Not(gc.Equals), NoopSpan{})
}

type stubTracer struct{}

func (stubTracer) Start(ctx context.Context, name string, options ...Option) (context.Context, Span) {
	return ctx, stubSpan{}
}

func (stubTracer) Enabled() bool {
	return true
}

type stubSpan struct{}

// AddEvent will record an event for this span. This is a manual mechanism
// for recording an event, it is useful to log information about what
// happened during the lifetime of a span.
// This is not the same as a log attached to a span, unfortunately the
// OpenTelemetry API does not have a way to record logs yet.
func (stubSpan) AddEvent(string, ...Attribute) {}

// RecordError will record err as an exception span event for this span. If
// this span is not being recorded or err is nil then this method does
// nothing.
// The attributes is lazy and only called if the span is recording.
func (stubSpan) RecordError(error, ...Attribute) {}

// End completes the Span. The Span is considered complete and ready to be
// delivered through the rest of the telemetry pipeline after this method
// is called. Therefore, updates to the Span are not allowed after this
// method has been called.
func (stubSpan) End(...Attribute) {}
