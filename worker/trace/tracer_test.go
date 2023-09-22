// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coretrace "github.com/juju/juju/core/trace"
)

type tracerSuite struct {
	baseSuite
}

var _ = gc.Suite(&tracerSuite{})

func (s *tracerSuite) TestTracer(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(context.Background(), "foo")
	c.Check(ctx, gc.NotNil)
	c.Check(span, gc.NotNil)

	defer span.End()
}

func (s *tracerSuite) TestTracerStartContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, span := tracer.Start(context.Background(), "foo")
	defer span.End()

	select {
	case <-ctx.Done():
		c.Fatalf("context should not be done")
	default:
	}
}

func (s *tracerSuite) TestTracerStartContextShouldBeCanceled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	ctx, cancel := context.WithCancel(context.Background())

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

func (s *tracerSuite) TestTracerAddEvent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(context.Background(), "foo")
	defer span.End()

	span.AddEvent("bar")
	span.AddEvent("baz", coretrace.StringAttr("qux", "quux"))
}

func (s *tracerSuite) TestTracerRecordErrorWithNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(context.Background(), "foo")
	defer span.End()

	span.RecordError(nil)
}

func (s *tracerSuite) TestTracerRecordError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tracer := s.newTracer(c)
	defer workertest.CleanKill(c, tracer)

	_, span := tracer.Start(context.Background(), "foo")
	defer span.End()

	span.RecordError(errors.Errorf("boom"))
}

func (s *tracerSuite) newTracer(c *gc.C) TrackedTracer {
	tracer, err := NewTracerWorker(context.Background(), coretrace.Namespace("agent", "controller").WithTag(names.NewMachineTag("0")), "http://meshuggah.com", false, false, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	return tracer
}
