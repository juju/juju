// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/globalclockupdater"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	stub   testing.Stub
	config globalclockupdater.ManifoldConfig
	worker worker.Worker
	logger loggo.Logger
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.logger = loggo.GetLogger("globalclockupdater_test")
	s.config = globalclockupdater.ManifoldConfig{
		Clock:          fakeClock{},
		RaftName:       "raft",
		FSM:            stubFSM{},
		NewWorker:      s.newWorker,
		UpdateInterval: time.Second,
		Logger:         s.logger,
	}
	s.worker = worker.NewRunner(worker.RunnerParams{})

	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, s.worker) })
}

func (s *ManifoldSuite) newWorker(config globalclockupdater.Config) (worker.Worker, error) {
	s.stub.AddCall("NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	expectInputs := []string{"raft"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestStartValidateClock(c *gc.C) {
	s.config.Clock = nil
	s.testStartValidateConfig(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestStartValidateFSM(c *gc.C) {
	s.config.FSM = nil
	s.testStartValidateConfig(c, "nil FSM not valid")
}

func (s *ManifoldSuite) TestStartValidateRaftName(c *gc.C) {
	s.config.RaftName = ""
	s.testStartValidateConfig(c, "empty RaftName not valid")
}

func (s *ManifoldSuite) TestStartValidateUpdateInterval(c *gc.C) {
	s.config.UpdateInterval = 0
	s.testStartValidateConfig(c, "non-positive UpdateInterval not valid")
}

func (s *ManifoldSuite) testStartValidateConfig(c *gc.C, expect string) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"raft": nil,
	})
	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingRaft(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"raft": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerSuccess(c *gc.C) {
	_, err := s.startManifoldWithContext(map[string]interface{}{
		"clock": fakeClock{},
		"raft":  new(raft.Raft),
	})
	c.Check(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "NewWorker")

	config := s.stub.Calls()[0].Args[0].(globalclockupdater.Config)
	c.Assert(config.NewUpdater, gc.NotNil)

	config.NewUpdater = nil
	c.Assert(config, jc.DeepEquals, globalclockupdater.Config{
		LocalClock:     fakeClock{},
		UpdateInterval: s.config.UpdateInterval,
		Logger:         s.logger,
	})
}

func (s *ManifoldSuite) startManifoldWithContext(data map[string]interface{}) (worker.Worker, error) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, data)
	worker, err := manifold.Start(context)
	if err != nil {
		return nil, err
	}
	return worker, nil
}

type fakeClock struct {
	clock.Clock
}

type stubFSM struct{}

func (stubFSM) GlobalTime() time.Time {
	return time.Now()
}
