// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTracerWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Kind = coretrace.Kind("")
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName: "agent",
		Clock:     s.clock,
		Logger:    s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			return nil, nil
		},
		Kind: coretrace.KindController,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent": s.agent,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	test := func(enabled bool) {
		defer s.setupMocks(c).Finish()

		s.expectCurrentConfig(enabled)

		if enabled {
			s.expectOpenTelemetry()
		}

		w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
		c.Assert(err, tc.ErrorIsNil)
		workertest.CleanKill(c, w)
	}

	// Test the noop and real tracer.
	for _, enabled := range []bool{true, false} {
		c.Logf("enabled: %v", enabled)
		test(enabled)
	}
}

func (s *manifoldSuite) expectOpenTelemetry() {
	s.config.EXPECT().Tag().Return(names.NewControllerAgentTag("0"))
	s.config.EXPECT().OpenTelemetryEndpoint().Return("blah")
	s.config.EXPECT().OpenTelemetryInsecure().Return(false)
	s.config.EXPECT().OpenTelemetryStackTraces().Return(true)
	s.config.EXPECT().OpenTelemetrySampleRatio().Return(0.5)
	s.config.EXPECT().OpenTelemetryTailSamplingThreshold().Return(time.Second)
}
