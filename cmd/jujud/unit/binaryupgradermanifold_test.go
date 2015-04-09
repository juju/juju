// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/unit"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type BinaryUpgraderManifoldSuite struct {
	testing.IsolationSuite
	manifold dependency.Manifold
}

var _ = gc.Suite(&BinaryUpgraderManifoldSuite{})

func (s *BinaryUpgraderManifoldSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.manifold = unit.BinaryUpgraderManifold(
		unit.BinaryUpgraderManifoldConfig{
			AgentName:     "agent-name",
			ApiCallerName: "api-caller-name",
		},
	)
}

func (s *BinaryUpgraderManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *BinaryUpgraderManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *BinaryUpgraderManifoldSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *BinaryUpgraderManifoldSuite) TestStartApiConnMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: &dummyAgent{}},
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *BinaryUpgraderManifoldSuite) TestStartFailure(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: expectAgent},
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})

	newBinaryUpgrader := func(gotAgent agent.Agent, gotApiCaller base.APICaller) (worker.Worker, error) {
		c.Check(gotAgent, gc.Equals, expectAgent)
		c.Check(gotApiCaller, gc.Equals, expectApiCaller)
		return nil, errors.New("some error")
	}
	s.PatchValue(unit.NewBinaryUpgrader, newBinaryUpgrader)

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
}

func (s *BinaryUpgraderManifoldSuite) TestStartSuccess(c *gc.C) {
	expectAgent := &dummyAgent{}
	expectApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"agent-name":      dt.StubResource{Output: expectAgent},
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})

	expectWorker := &dummyWorker{}
	newBinaryUpgrader := func(gotAgent agent.Agent, gotApiCaller base.APICaller) (worker.Worker, error) {
		c.Check(gotAgent, gc.Equals, expectAgent)
		c.Check(gotApiCaller, gc.Equals, expectApiCaller)
		return expectWorker, nil
	}
	s.PatchValue(unit.NewBinaryUpgrader, newBinaryUpgrader)

	worker, err := s.manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expectWorker)
}

type dummyApiCaller struct {
	base.APICaller
}

type dummyAgent struct {
	agent.Agent
}

type dummyWorker struct {
	worker.Worker
}
