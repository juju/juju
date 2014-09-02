package agent_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type agentSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machine0  *state.Machine
	machine1  *state.Machine
	container *state.Machine
	agent     *agent.API
}

var _ = gc.Suite(&agentSuite{})

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	s.machine1, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	s.container, err = s.State.AddMachineInsideMachine(template, s.machine1.Id(), instance.LXC)
	c.Assert(err, gc.IsNil)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })
	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}

	// Create a machiner API for machine 1.
	s.agent, err = agent.NewAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *agentSuite) TestAgentFailsWithNonAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUserTag("admin")
	api, err := agent.NewAPI(s.State, s.resources, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	auth := s.authorizer
	auth.Tag = names.NewUnitTag("foosball/1")
	_, err := agent.NewAPI(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)
}

func (s *agentSuite) TestGetEntities(c *gc.C) {
	err := s.container.Destroy()
	c.Assert(err, gc.IsNil)
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-1-lxc-0"},
			{Tag: "machine-42"},
		},
	}
	results := s.agent.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "alive",
				Jobs: []params.MachineJob{params.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now login as the machine agent of the container and try again.
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	containerAgent, err := agent.NewAPI(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)

	results = containerAgent.GetEntities(args)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{
				Life:          "dying",
				Jobs:          []params.MachineJob{params.JobHostUnits},
				ContainerType: instance.LXC,
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetNotFoundEntity(c *gc.C) {
	// Destroy the container first, so we can destroy its parent.
	err := s.container.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.container.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.container.Remove()
	c.Assert(err, gc.IsNil)

	err = s.machine1.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine1.Remove()
	c.Assert(err, gc.IsNil)
	results := s.agent.GetEntities(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

func (s *agentSuite) TestSetPasswords(c *gc.C) {
	results, err := s.agent.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-0", Password: "xxx-12345678901234567890"},
			{Tag: "machine-1", Password: "yyy-12345678901234567890"},
			{Tag: "machine-42", Password: "zzz-12345678901234567890"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.machine1.Refresh()
	c.Assert(err, gc.IsNil)
	changed := s.machine1.PasswordValid("yyy-12345678901234567890")
	c.Assert(changed, gc.Equals, true)
}

func (s *agentSuite) TestShortSetPasswords(c *gc.C) {
	results, err := s.agent.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "machine-1", Password: "yyy"},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"password is only 3 bytes long, and is not a valid Agent password")
}
