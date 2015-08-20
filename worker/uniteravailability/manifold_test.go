// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniteravailability_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/uniteravailability"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func kill(w worker.Worker) error {
	w.Kill()
	return w.Wait()
}

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.manifold = uniteravailability.Manifold(uniteravailability.ManifoldConfig{
		AgentName: "agent-name",
	})
	s.getResource = dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{}},
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
	s.SetErrors(errors.New("no file for you"))

	s.manifold = uniteravailability.PatchedManifold(uniteravailability.ManifoldConfig{
		AgentName: "agent-name",
	}, s.readStateFile(true), s.writeStateFile)
	worker, err := s.manifold.Start(s.getResource)

	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no file for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "readStateFile",
		Args:     []interface{}{"/path/to/data/dir/unit-wordpress-1"},
	}})
}

func (s *ManifoldSuite) readStateFile(toReturn bool) func(filename string) (bool, error) {
	return func(filename string) (bool, error) {
		s.AddCall("readStateFile", filename)
		return toReturn, s.NextErr()
	}
}

func (s *ManifoldSuite) writeStateFile(filename string, value bool) error {
	s.AddCall("writeStateFile", filename, value)
	return s.NextErr()
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.manifold = uniteravailability.PatchedManifold(uniteravailability.ManifoldConfig{
		AgentName: "agent-name",
	}, s.readStateFile(true), s.writeStateFile)
	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "readStateFile",
		Args:     []interface{}{"/path/to/data/dir/unit-wordpress-1"},
	}})
	defer kill(worker)

	var state uniteravailability.UniterAvailabilityState
	err = s.manifold.Output(worker, &state)
	c.Check(err, jc.ErrorIsNil)
	c.Check(state.Available(), gc.Equals, true)
}

func (s *ManifoldSuite) setupWorker(c *gc.C, path string) worker.Worker {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{dir: path}},
	})

	worker, err := s.manifold.Start(getResource)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(worker, gc.NotNil)
	return worker
}

func (s *ManifoldSuite) TestOutputIsSaved(c *gc.C) {
	dataDir := c.MkDir()
	worker := s.setupWorker(c, dataDir)
	var state uniteravailability.UniterAvailabilityState
	err := s.manifold.Output(worker, &state)
	c.Check(err, jc.ErrorIsNil)
	// initial value should be false
	c.Check(state.Available(), gc.Equals, false)
	err = state.SetAvailable(true)
	c.Check(err, jc.ErrorIsNil)
	c.Check(state.Available(), gc.Equals, true)
	err = kill(worker)
	c.Assert(err, jc.ErrorIsNil)

	worker = s.setupWorker(c, dataDir)
	defer kill(worker)
	err = s.manifold.Output(worker, &state)
	c.Check(err, jc.ErrorIsNil)
	c.Check(state.Available(), gc.Equals, true)
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	dir := c.MkDir()
	worker := s.setupWorker(c, dir)
	defer kill(worker)
	var state interface{}
	err := s.manifold.Output(worker, &state)
	c.Check(err.Error(), gc.Equals, "expected *uniteravailability.uniterStateWorker->*uniteravailability.UniterAvailabilityState; got *uniteravailability.uniterStateWorker->*interface {}")
	c.Check(state, gc.IsNil)
}

func (s *ManifoldSuite) TestWriteFailKills(c *gc.C) {
	s.SetErrors(nil, errors.New("no save for you"))
	// Manifold with a write function that fails.
	s.manifold = uniteravailability.PatchedManifold(uniteravailability.ManifoldConfig{
		AgentName: "agent-name",
	}, s.readStateFile(false), s.writeStateFile)

	dir := "/path/to/data/dir"
	worker := s.setupWorker(c, dir)
	c.Check(worker, gc.NotNil)

	var state uniteravailability.UniterAvailabilityState
	err := s.manifold.Output(worker, &state)
	c.Check(err, jc.ErrorIsNil)
	c.Check(state.Available(), gc.Equals, false)

	// Try to set the available state, which triggers a write.
	err = state.SetAvailable(true)
	c.Check(err, gc.ErrorMatches, "no save for you")

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "readStateFile",
		Args:     []interface{}{"/path/to/data/dir/unit-wordpress-1"},
	}, {
		FuncName: "writeStateFile",
		Args:     []interface{}{fmt.Sprintf("%s/unit-wordpress-1", dir), true},
	}})

	// The failure to write should have killed the worker.
	ok := make(chan struct{}, 1)
	go func() {
		err = worker.Wait()
		c.Check(err, gc.ErrorMatches, "no save for you")
		ok <- struct{}{}
	}()

	select {
	case <-ok:
	case <-time.After(jujutesting.LongWait):
		// Lets not block the test forever if this fails.
		c.Fatal("timed out waiting for worker to die")
	}

	// Start a new worker.
	worker = s.setupWorker(c, dir)
	c.Check(worker, gc.NotNil)
	defer kill(worker)

	// Make sure the value was not changed.
	err = s.manifold.Output(worker, &state)
	c.Check(err, jc.ErrorIsNil)
	c.Check(state.Available(), gc.Equals, false)

}

type dummyAgent struct {
	agent.Agent
	dir string
}

func (a dummyAgent) CurrentConfig() agent.Config {
	return &dummyAgentConfig{dir: a.dir}
}

type dummyAgentConfig struct {
	agent.Config
	dir string
}

func (a dummyAgentConfig) UniterStateDir() string {
	if a.dir == "" {
		return "/path/to/data/dir"
	} else {
		return a.dir
	}
}

func (_ dummyAgentConfig) Tag() names.Tag {
	return names.NewUnitTag("wordpress/1")
}

type dummyWorker struct {
	worker.Worker
}
