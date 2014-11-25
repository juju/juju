package agent_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
)

// Definition of reusable V0 tests.

type factoryV0 func(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error)

func (s *baseSuite) testAgentFailsWithNonAgentV0(c *gc.C, factory factoryV0) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	api, err := factory(s.State, s.resources, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *baseSuite) testAgentSucceedsWithUnitAgentV0(c *gc.C, factory factoryV0) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	_, err := factory(s.State, s.resources, auth)
	c.Assert(err, jc.ErrorIsNil)
}

type getEntitiesV0 interface {
	GetEntities(args params.Entities) params.AgentGetEntitiesResults
}

func (s *baseSuite) testGetEntitiesV0(c *gc.C, api getEntitiesV0) {
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxc-0"},
			{Tag: "machine-42"},
		},
	}
	results := api.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "alive",
				Jobs: []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *baseSuite) testGetEntitiesContainerV0(c *gc.C, api getEntitiesV0) {
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxc-0"},
			{Tag: "machine-42"},
		},
	}
	results := api.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{
				Life:          "dying",
				Jobs:          []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
				ContainerType: instance.LXC,
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *baseSuite) testGetEntitiesNotFoundV0(c *gc.C, api getEntitiesV0) {
	// Destroy the container first, so we can destroy its parent.
	err := s.container.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.container.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine1.Remove()
	c.Assert(err, jc.ErrorIsNil)
	results := api.GetEntities(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

type setPasswordsV0 interface {
	SetPasswords(args params.EntityPasswords) (params.ErrorResults, error)
}

func (s *baseSuite) testSetPasswordsV0(c *gc.C, api setPasswordsV0) {
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-0", Password: "xxx-12345678901234567890"},
			{Tag: "machine-1", Password: "yyy-12345678901234567890"},
			{Tag: "machine-42", Password: "zzz-12345678901234567890"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, jc.IsTrue)
}

func (s *baseSuite) testSetPasswordsShortV0(c *gc.C, api setPasswordsV0) {
	results, err := api.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-1", Password: "yyy"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}

// V0 test suite.

func factoryWrapperV0(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return agent.NewAgentAPIV0(st, resources, auth)
}

type agentSuiteV0 struct {
	baseSuite
}

var _ = gc.Suite(&agentSuiteV0{})

func (s *agentSuiteV0) TestAgentFailsWithNonAgent(c *gc.C) {
	s.testAgentFailsWithNonAgentV0(c, factoryWrapperV0)
}

func (s *agentSuiteV0) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	s.testAgentSucceedsWithUnitAgentV0(c, factoryWrapperV0)
}

func (s *agentSuiteV0) TestGetEntities(c *gc.C) {
	s.testGetEntitiesV0(c, s.newAPI(c))
}

func (s *agentSuiteV0) TestGetEntitiesContainer(c *gc.C) {
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	api, err := agent.NewAgentAPIV0(s.State, s.resources, auth)
	c.Assert(err, jc.ErrorIsNil)
	s.testGetEntitiesContainerV0(c, api)
}

func (s *agentSuiteV0) TestGetEntitiesNotFound(c *gc.C) {
	s.testGetEntitiesNotFoundV0(c, s.newAPI(c))
}

func (s *agentSuiteV0) TestSetPasswords(c *gc.C) {
	s.testSetPasswordsV0(c, s.newAPI(c))
}

func (s *agentSuiteV0) TestSetPasswordsShort(c *gc.C) {
	s.testSetPasswordsShortV0(c, s.newAPI(c))
}

func (s *agentSuiteV0) newAPI(c *gc.C) *agent.AgentAPIV0 {
	api, err := agent.NewAgentAPIV0(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api
}
