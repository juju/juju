// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/machinelock"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
	lock        *fslock.Lock
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.manifold = machinelock.Manifold(machinelock.ManifoldConfig{
		AgentName: "agent-name",
	})
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{}},
	})

	lock, err := fslock.NewLock(c.MkDir(), "test-lock")
	c.Assert(err, jc.ErrorIsNil)
	s.lock = lock
	s.PatchValue(machinelock.CreateLock, func(dataDir string) (*fslock.Lock, error) {
		s.AddCall("createLock", dataDir)
		if err := s.NextErr(); err != nil {
			return nil, err
		}
		return s.lock, nil
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name"})
}

func (s *ManifoldSuite) TestStartMissingAgent(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
	s.CheckCalls(c, nil)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	s.SetErrors(errors.New("no lock for you"))
	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no lock for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "createLock",
		Args:     []interface{}{"/path/to/data/dir"},
	}})
}

func (s *ManifoldSuite) setupWorkerTest(c *gc.C) worker.Worker {
	worker, err := s.manifold.Start(s.getResource)
	c.Check(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		worker.Kill()
		err := worker.Wait()
		c.Check(err, jc.ErrorIsNil)
	})
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "createLock",
		Args:     []interface{}{"/path/to/data/dir"},
	}})
	return worker
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.setupWorkerTest(c)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var lock *fslock.Lock
	err := s.manifold.Output(worker, &lock)
	c.Check(err, jc.ErrorIsNil)
	c.Check(lock, gc.Equals, s.lock)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var lock *fslock.Lock
	err := s.manifold.Output(&dummyWorker{}, &lock)
	c.Check(err, gc.ErrorMatches, "in should be a \\*valueWorker; is .*")
	c.Check(lock, gc.IsNil)
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	worker := s.setupWorkerTest(c)
	var lock int
	err := s.manifold.Output(worker, &lock)
	c.Check(err, gc.ErrorMatches, "cannot output into \\*int")
	c.Check(lock, gc.Equals, 0)
}

type dummyAgent struct {
	agent.Agent
}

func (_ dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{}
}

type dummyAgentConfig struct {
	agent.Config
}

func (_ dummyAgentConfig) DataDir() string {
	return "/path/to/data/dir"
}

type dummyWorker struct {
	worker.Worker
}
