// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coretrace "github.com/juju/juju/core/trace"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTracerWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName: "agent",
		Clock:     s.clock,
		Logger:    s.logger,
		NewTracerWorker: func(context.Context, string, string, bool, bool, Logger) (TrackedTracer, error) {
			return nil, nil
		},
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent": s.agent,
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	type tracerGetter interface {
		GetTracer(string) (coretrace.Tracer, error)
	}

	test := func(enabled bool) {
		defer s.setupMocks(c).Finish()

		s.expectCurrentConfig(false)

		w, err := Manifold(s.getConfig()).Start(s.getContext())
		c.Assert(err, jc.ErrorIsNil)
		defer workertest.CleanKill(c, w)

		tracer, err := w.(tracerGetter).GetTracer("foo")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tracer, gc.NotNil)

		// This shouldn't panic, if it's a noop tracer.
		ctx, span := tracer.Start(context.Background(), "foo")
		c.Assert(ctx, gc.NotNil)
		c.Assert(span, gc.NotNil)
	}

	// Test the noop and real tracer.
	for _, enabled := range []bool{true, false} {
		c.Logf("enabled: %v", enabled)
		test(enabled)
	}
}
