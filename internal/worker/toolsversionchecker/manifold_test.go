// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine/enginetest"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/toolsversionchecker"
	"github.com/juju/juju/rpc/params"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	newCalled bool
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.newCalled = false
	s.PatchValue(&toolsversionchecker.New,
		func(api toolsversionchecker.Facade, params *toolsversionchecker.VersionCheckerParams) worker.Worker {
			s.newCalled = true
			return nil
		},
	)
}

func (s *ManifoldSuite) TestMachine(c *tc.C) {
	config := toolsversionchecker.ManifoldConfig(enginetest.AgentAPIManifoldTestConfig())
	_, err := enginetest.RunAgentAPIManifold(
		toolsversionchecker.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		mockAPICaller(model.JobManageModel))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.newCalled, tc.IsTrue)
}

func (s *ManifoldSuite) TestMachineNotModelManagerErrors(c *tc.C) {
	config := toolsversionchecker.ManifoldConfig(enginetest.AgentAPIManifoldTestConfig())
	_, err := enginetest.RunAgentAPIManifold(
		toolsversionchecker.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		mockAPICaller(model.JobHostUnits))
	c.Assert(err, tc.Equals, dependency.ErrMissing)
	c.Assert(s.newCalled, tc.IsFalse)
}

func (s *ManifoldSuite) TestNonMachineAgent(c *tc.C) {
	config := toolsversionchecker.ManifoldConfig(enginetest.AgentAPIManifoldTestConfig())
	_, err := enginetest.RunAgentAPIManifold(
		toolsversionchecker.Manifold(config),
		&fakeAgent{tag: names.NewUnitTag("foo/0")},
		mockAPICaller(""))
	c.Assert(err, tc.ErrorMatches, "this manifold may only be used inside a machine agent")
	c.Assert(s.newCalled, tc.IsFalse)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}

func mockAPICaller(job model.MachineJob) apitesting.APICallerFunc {
	return apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if res, ok := result.(*params.AgentGetEntitiesResults); ok {
			res.Entities = []params.AgentGetEntitiesResult{
				{Jobs: []model.MachineJob{
					job,
				}}}
		}
		return nil
	})
}
