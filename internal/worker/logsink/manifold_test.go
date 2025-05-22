// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"maps"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type ManifoldSuite struct {
	jujutesting.BaseSuite

	manifold dependency.Manifold
	getter   dependency.Getter

	logger logger.Logger

	clock clock.Clock
	stub  testhelpers.Stub
}

func TestManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = clock.WallClock

	s.stub.ResetCalls()

	s.logger = loggertesting.WrapCheckLog(c)

	s.getter = s.newGetter(c, nil)
	s.manifold = Manifold(ManifoldConfig{
		LogSink:   loggertesting.WrapCheckLogSink(c),
		Clock:     s.clock,
		NewWorker: s.newWorker,
		NewModelLogger: func(logger.LogSink, model.UUID, names.Tag) (worker.Worker, error) {
			return nil, nil
		},
	})
}

func (s *ManifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.LogSink = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewModelLogger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		LogSink:   loggertesting.WrapCheckLogSink(c),
		NewWorker: s.newWorker,
		NewModelLogger: func(logger.LogSink, model.UUID, names.Tag) (worker.Worker, error) {
			return nil, nil
		},
		Clock: clock.WallClock,
	}
}

func (s *ManifoldSuite) newGetter(c *tc.C, overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"clock": s.clock,
	}
	maps.Copy(resources, overlay)
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) newWorker(config Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{
		Name: "log-sink",
	})
}

var expectedInputs = []string{}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *tc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(c, map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(c.Context(), getter)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, tc.HasLen, 1)
	c.Check(args[0], tc.FitsTypeOf, Config{})

	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "NewWorker")
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
