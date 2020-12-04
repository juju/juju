// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/globalclock"
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
		Clock:            fakeClock{},
		LeaseManagerName: "lease-manager",
		RaftName:         "raft",
		NewWorker:        s.newWorker,
		UpdateInterval:   time.Second,
		Logger:           s.logger,
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
	expectInputs := []string{"lease-manager", "raft"}
	c.Check(manifold.Inputs, jc.SameContents, expectInputs)
}

func (s *ManifoldSuite) TestStartValidateClock(c *gc.C) {
	s.config.Clock = nil
	s.testStartValidateConfig(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestStartValidateLeaseManagerName(c *gc.C) {
	s.config.LeaseManagerName = ""
	s.testStartValidateConfig(c, "empty LeaseManagerName not valid")
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
		"raft":          nil,
		"lease-manager": nil,
	})
	worker, err := manifold.Start(context)
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingLeaseManager(c *gc.C) {
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"lease-manager": dependency.ErrMissing,
		"raft":          nil,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartMissingRaft(c *gc.C) {
	updater := fakeUpdater{}
	manifold := globalclockupdater.Manifold(s.config)
	context := dt.StubContext(nil, map[string]interface{}{
		"lease-manager": &updater,
		"raft":          dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestStartNewWorkerSuccessWithLeaseManager(c *gc.C) {
	updater := fakeUpdater{}
	s.config.LeaseManagerName = "lease-manager"
	s.config.RaftName = "raft"
	worker, err := s.startManifoldWithContext(c, map[string]interface{}{
		"clock":         fakeClock{},
		"lease-manager": &updater,
		"raft":          nil,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)

	s.stub.CheckCallNames(c, "NewWorker")
	config := s.stub.Calls()[0].Args[0].(globalclockupdater.Config)
	c.Assert(config.NewUpdater, gc.NotNil)
	actualUpdater, err := config.NewUpdater()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actualUpdater, gc.Equals, &updater)
	config.NewUpdater = nil
	c.Assert(config, jc.DeepEquals, globalclockupdater.Config{
		LocalClock:     fakeClock{},
		UpdateInterval: s.config.UpdateInterval,
		Logger:         s.logger,
	})
}

func (s *ManifoldSuite) startManifoldWithContext(c *gc.C, data map[string]interface{}) (worker.Worker, error) {
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

type fakeUpdater struct {
	globalclock.Updater
}
