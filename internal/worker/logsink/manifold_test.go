// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"context"
	"maps"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	model "github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type ManifoldSuite struct {
	jujutesting.BaseSuite

	manifold dependency.Manifold
	getter   dependency.Getter

	logger logger.Logger

	clock clock.Clock
	stub  testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
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

func (s *ManifoldSuite) TestValidateConfig(c *gc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.LogSink = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewModelLogger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) getConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		LogSink:   loggertesting.WrapCheckLogSink(c),
		NewWorker: s.newWorker,
		NewModelLogger: func(logger.LogSink, model.UUID, names.Tag) (worker.Worker, error) {
			return nil, nil
		},
		Clock: clock.WallClock,
	}
}

func (s *ManifoldSuite) newGetter(c *gc.C, overlay map[string]any) dependency.Getter {
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

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(c, map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Check(args[0], gc.FitsTypeOf, Config{})

	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "NewWorker")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
