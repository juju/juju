// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&contextSuite{})

func (contextSuite) TestTracerFromContextEmpty(c *tc.C) {
	tracer, enabled := TracerFromContext(context.Background())
	c.Assert(tracer, tc.NotNil)
	c.Assert(enabled, tc.Equals, false)

	ctx, span := tracer.Start(context.Background(), "test")
	c.Assert(ctx, tc.NotNil)
	c.Assert(span, tc.NotNil)

	c.Check(span, tc.Equals, NoopSpan{})
}

func (contextSuite) TestTracerFromContextTracer(c *tc.C) {
	tracer, enabled := TracerFromContext(WithTracer(context.Background(), stubTracer{}))
	c.Assert(tracer, tc.NotNil)
	c.Assert(enabled, tc.Equals, true)

	ctx, span := tracer.Start(context.Background(), "test")
	c.Assert(ctx, tc.NotNil)
	c.Assert(span, tc.NotNil)

	// Ensure that we get the correct span.
	c.Check(span, tc.Equals, stubSpan{})
	c.Check(span, tc.Not(tc.Equals), NoopSpan{})
}

type stubTracer struct{}

func (stubTracer) Start(ctx context.Context, name string, options ...Option) (context.Context, Span) {
	return ctx, stubSpan{}
}

func (stubTracer) Enabled() bool {
	return true
}

type stubSpan struct{}

// Scope returns the scope of the span.
func (stubSpan) Scope() Scope { return nil }

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
