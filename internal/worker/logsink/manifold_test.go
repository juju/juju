// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/worker/logsink"
	jujutesting "github.com/juju/juju/testing"
)

type ManifoldSuite struct {
	jujutesting.BaseSuite

	manifold dependency.Manifold
	getter   dependency.Getter

	clock clock.Clock
	stub  testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.clock = clock.WallClock

	s.stub.ResetCalls()

	s.getter = s.newGetter(c, nil)
	s.manifold = logsink.Manifold(logsink.ManifoldConfig{
		ClockName:   "clock",
		AgentName:   "agent",
		DebugLogger: loggo.GetLogger("test"),
		NewWorker:   s.newWorker,
	})
}

func (s *ManifoldSuite) newGetter(c *gc.C, overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"agent": &fakeAgent{
			logDir: c.MkDir(),
		},
		"clock": s.clock,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) newWorker(config logsink.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{"agent", "clock"}

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
	c.Assert(args[0], gc.FitsTypeOf, logsink.Config{})
	config := args[0].(logsink.Config)
	c.Assert(config.LoggerForModelFunc, gc.NotNil)
	config.LoggerForModelFunc = nil

	expectedConfig := logsink.Config{
		Logger: loggo.GetLogger("test"),
		Clock:  s.clock,
		LogSinkConfig: logsink.LogSinkConfig{
			LoggerBufferSize:    1000,
			LoggerFlushInterval: time.Second,
		},
	}
	workertest.CleanKill(c, w)
	s.stub.CheckCallNames(c, "NewWorker")
	c.Assert(config, jc.DeepEquals, expectedConfig)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

type fakeAgent struct {
	agent.Agent
	agent.Config
	logDir string
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return f
}

func (f *fakeAgent) Tag() names.Tag {
	return names.NewMachineTag("1")
}

func (f *fakeAgent) Value(key string) string {
	return ""
}

func (f *fakeAgent) LogDir() string {
	return f.logDir
}

func (f *fakeAgent) ModelLogfileMaxSizeMB() int {
	return 10
}

func (f *fakeAgent) ModelLogfileMaxBackups() int {
	return 5
}
