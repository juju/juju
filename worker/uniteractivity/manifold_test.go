// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniteractivity_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/uniteractivity"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold    dependency.Manifold
	getResource dependency.GetResourceFunc
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.manifold = uniteractivity.Manifold(uniteractivity.ManifoldConfig{
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
	s.PatchValue(uniteractivity.ReadStateFile, s.readStateFile(true))
	s.SetErrors(errors.New("no file for you"))
	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no file for you")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "readStateFile",
		Args:     []interface{}{"/path/to/data/dir/wordpress-1"},
	}})
}

func (s *ManifoldSuite) readStateFile(toReturn bool) func(filename string) (bool, error) {
	return func(filename string) (bool, error) {
		s.AddCall("readStateFile", filename)
		return toReturn, s.NextErr()
	}
}

func (s *ManifoldSuite) writeStateFile() func(filename string, value bool) error {
	return func(filename string, value bool) error {
		s.AddCall("writeStateFile", filename, value)
		return s.NextErr()
	}
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	s.PatchValue(uniteractivity.ReadStateFile, s.readStateFile(true))
	s.SetErrors(nil)
	worker, err := s.manifold.Start(s.getResource)
	c.Check(worker, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "readStateFile",
		Args:     []interface{}{"/path/to/data/dir/wordpress-1"},
	}})

	var started uniteractivity.UniterState
	err = s.manifold.Output(worker, &started)
	c.Check(err, jc.ErrorIsNil)
	c.Check(started.GetStarted(), gc.Equals, true)
}

func (s *ManifoldSuite) setupWorker(c *gc.C, path string) worker.Worker {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Output: &dummyAgent{dir: path}},
	})

	worker, err := s.manifold.Start(getResource)
	c.Assert(worker, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	return worker
}

func (s *ManifoldSuite) TestOutputIsSaved(c *gc.C) {
	dataDir := c.MkDir()
	worker := s.setupWorker(c, dataDir)
	var started uniteractivity.UniterState
	err := s.manifold.Output(worker, &started)
	c.Check(err, jc.ErrorIsNil)
	// initial value should be false
	c.Check(started.GetStarted(), gc.Equals, false)
	err = started.SetStarted(true)
	c.Check(err, jc.ErrorIsNil)
	c.Check(started.GetStarted(), gc.Equals, true)
	worker.Kill()

	worker = s.setupWorker(c, dataDir)
	err = s.manifold.Output(worker, &started)
	c.Check(err, jc.ErrorIsNil)
	c.Check(started.GetStarted(), gc.Equals, true)
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	dir := c.MkDir()
	worker := s.setupWorker(c, dir)
	var started interface{}
	err := s.manifold.Output(worker, &started)
	c.Check(err.Error(), gc.Equals, "expected *uniteractivity.uniterStateWorker->*uniteractivity.UniterState; got *uniteractivity.uniterStateWorker->*interface {}")
	c.Check(started, gc.IsNil)
}

func (s *ManifoldSuite) TestWriteFailKills(c *gc.C) {
	// Patch the manifold with a write function that fails.
	s.PatchValue(uniteractivity.WriteStateFile, s.writeStateFile())
	s.SetErrors(errors.New("no save for you"))
	dir := c.MkDir()

	worker := s.setupWorker(c, dir)
	c.Check(worker, gc.NotNil)

	var started uniteractivity.UniterState
	err := s.manifold.Output(worker, &started)
	c.Check(err, jc.ErrorIsNil)
	c.Check(started.GetStarted(), gc.Equals, false)

	// Try to set the started state, which triggers a write.
	err = started.SetStarted(true)
	c.Check(err, gc.ErrorMatches, "no save for you")

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "writeStateFile",
		Args:     []interface{}{fmt.Sprintf("%s/wordpress-1", dir), true},
	}})

	// The failure to write should have killed the worker.
	err = worker.Wait()
	c.Check(err, gc.ErrorMatches, "no save for you")

	// Start a new worker.
	worker = s.setupWorker(c, dir)
	c.Check(worker, gc.NotNil)

	// Make sure the value was not changed.
	err = s.manifold.Output(worker, &started)
	c.Check(err, jc.ErrorIsNil)
	c.Check(started.GetStarted(), gc.Equals, false)

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
