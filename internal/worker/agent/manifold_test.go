// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/worker/agent"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)
	c.Check(manifold.Inputs, gc.IsNil)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)

	agentWorker, err := manifold.Start(context.Background(), nil)
	c.Check(err, jc.ErrorIsNil)
	defer assertStop(c, agentWorker)

	var outputAgent coreagent.Agent
	err = manifold.Output(agentWorker, &outputAgent)
	c.Check(err, jc.ErrorIsNil)
	c.Check(outputAgent, jc.DeepEquals, inputAgent)
}

func (s *ManifoldSuite) TestOutputStoppedWorker(c *gc.C) {
	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)

	agentWorker, err := manifold.Start(context.Background(), nil)
	c.Check(err, jc.ErrorIsNil)
	// The lifetime is irrelevant -- the output func will still function
	// whether the (degenerate) worker is alive or not -- so no defer here.
	assertStop(c, agentWorker)

	var outputAgent coreagent.Agent
	err = manifold.Output(agentWorker, &outputAgent)
	c.Check(err, jc.ErrorIsNil)
	c.Check(outputAgent, jc.DeepEquals, inputAgent)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)

	var badWorker worker.Worker

	var outputAgent coreagent.Agent
	err := manifold.Output(badWorker, &outputAgent)
	c.Check(err.Error(), gc.Equals, "expected *agent.agentWorker->*agent.Agent; got <nil>->*agent.Agent")
}

func (s *ManifoldSuite) TestOutputBadTarget(c *gc.C) {
	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)

	agentWorker, err := manifold.Start(context.Background(), nil)
	c.Check(err, jc.ErrorIsNil)
	defer assertStop(c, agentWorker)

	var outputNonsense interface{}
	err = manifold.Output(agentWorker, &outputNonsense)
	c.Check(err.Error(), gc.Equals, "expected *agent.agentWorker->*agent.Agent; got *agent.agentWorker->*interface {}")
}

func (s *ManifoldSuite) TestReport(c *gc.C) {
	s.PatchValue(&jujuversion.Current, semversion.MustParse("3.2.1"))

	inputAgent := &dummyAgent{}
	manifold := agent.Manifold(inputAgent)

	agentWorker, err := manifold.Start(context.Background(), nil)
	c.Check(err, jc.ErrorIsNil)
	defer assertStop(c, agentWorker)

	reporter, ok := agentWorker.(worker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(reporter.Report(), jc.DeepEquals, map[string]interface{}{
		"agent":      "machine-42",
		"model-uuid": "model-uuid",
		"version":    "3.2.1",
	})
}

type dummyAgent struct {
	coreagent.Agent
}

func (dummyAgent) CurrentConfig() coreagent.Config {
	return fakeConfig{}
}

func assertStop(c *gc.C, w worker.Worker) {
	c.Assert(worker.Stop(w), jc.ErrorIsNil)
}

type fakeConfig struct {
	coreagent.Config
}

func (fakeConfig) Model() names.ModelTag {
	return names.NewModelTag("model-uuid")
}

func (fakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}
