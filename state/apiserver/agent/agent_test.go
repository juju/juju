package agent_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/agent"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type agentSuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer

	machine0 *state.Machine
	machine1 *state.Machine
	agent    *agent.API
}

var _ = gc.Suite(&agentSuite{})

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobManageEnviron, state.JobManageState)
	c.Assert(err, gc.IsNil)

	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:          s.machine1.Tag(),
		LoggedIn:     true,
		Manager:      false,
		MachineAgent: true,
	}

	// Create a machiner API for machine 1.
	s.agent, err = agent.NewAPI(s.State, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *agentSuite) TestAgentFailsWithClientUser(c *gc.C) {
	auth := s.authorizer
	auth.Client = true
	api, err := agent.NewAPI(s.State, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestGetEntities(c *gc.C) {
	err := s.machine1.Destroy()
	c.Assert(err, gc.IsNil)
	results := s.agent.GetEntities(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-42"},
		},
	})
	c.Assert(results, gc.DeepEquals, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{
			{
				Life: "dying",
				Jobs: []params.MachineJob{params.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetNotFoundEntity(c *gc.C) {
	err := s.machine1.Destroy()
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
	results, err := s.agent.SetPasswords(params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: "machine-0", Password: "xxx"},
			{Tag: "machine-1", Password: "yyy"},
			{Tag: "machine-42", Password: "zzz"},
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
	changed := s.machine1.PasswordValid("yyy")
	c.Assert(changed, gc.Equals, true)
}
