// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	time "time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.opentelemetry.io/otel/trace"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
)

type tracerSuite struct {
	baseSuite
}

func TestTracerSuite(t *testing.T) {
	tc.Run(t, &tracerSuite{})
}

var _ coretrace.Tracer = (*tracer)(nil)

func (s *tracerSuite) TestTracer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(c.Context(), "foo")
	c.Check(ctx, tc.NotNil)
	c.Check(span, tc.NotNil)

	defer span.End()
}

func (s *tracerSuite) TestTracerStartContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	select {
	case <-ctx.Done():
		c.Fatalf("context should not be done")
	default:
	}
}

func (s *tracerSuite) TestTracerStartContextShouldBeCanceled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, cancel := context.WithCancel(c.Context())

	// cancel the context straight away
	cancel()

	ctx, span := tracer.Start(ctx, "foo")
	defer span.End()

	select {
	case <-ctx.Done():
	default:
		c.Fatalf("context should be done")
	}
}

func (s *tracerSuite) TestTracerAddEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	s.span.EXPECT().AddEvent(gomock.Any(), gomock.Any()).AnyTimes()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.AddEvent("bar")
	span.AddEvent("baz", coretrace.StringAttr("qux", "quux"))
}

func (s *tracerSuite) TestTracerRecordErrorWithNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.RecordError(nil)
}

func (s *tracerSuite) TestTracerRecordError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	s.span.EXPECT().RecordError(gomock.Any(), gomock.Any()).AnyTimes()
	s.span.EXPECT().SetStatus(gomock.Any(), gomock.Any()).AnyTimes()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(c.Context(), "foo")
	defer span.End()

	span.RecordError(errors.Errorf("boom"))
}

func (s *tracerSuite) TestBuildRequestContextWithBackgroundContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := w.(*tracer).buildRequestContext(c.Context())
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContextWithBrokenTraceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "foo", "bar", 0)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContextWithBrokenSpanID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "80f198ee56343ba864fe8b2a57d3eff7", "bar", 0)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)
}

func (s *tracerSuite) TestBuildRequestContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClient()

	w := s.newTracer(c)
	defer workertest.CleanKill(c, w)

	ctx := coretrace.WithTraceScope(c.Context(), "80f198ee56343ba864fe8b2a57d3eff7", "ff00000000000000", 1)

	ctx = w.(*tracer).buildRequestContext(ctx)
	c.Check(ctx, tc.NotNil)

	traceID, spanID, flags, ok := coretrace.ScopeFromContext(ctx)
	c.Assert(ok, tc.IsFalse)
	c.Check(traceID, tc.Equals, "")
	c.Check(spanID, tc.Equals, "")
	c.Check(flags, tc.Equals, 0)

	span := trace.SpanContextFromContext(ctx)
	c.Check(span.IsRemote(), tc.IsTrue)
}

func (s *tracerSuite) newTracer(c *tc.C) TrackedTracer {
	ns := coretrace.Namespace("agent", "controller").WithTagAndKind(names.NewMachineTag("0"), coretrace.KindController)
	newClient := func(context.Context, coretrace.TaggedTracerNamespace, string, bool, float64, time.Duration, logger.Logger) (Client, ClientTracerProvider, ClientTracer, error) {
		return s.client, s.clientTracerProvider, s.clientTracer, nil
	}
	tracer, err := NewTracerWorker(c.Context(), ns, "http://meshuggah.com", false, false, 0.42, time.Second, s.logger, newClient)
	c.Assert(err, tc.ErrorIsNil)
	return tracer
}
