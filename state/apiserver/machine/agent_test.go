package machine_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/machine"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type agentSuite struct {
	commonSuite
	agent *machine.AgentAPI
}

var _ = gc.Suite(&agentSuite{})

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.commonSuite.SetUpTest(c)

	// Create a machiner API for machine 1.
	api, err := machine.NewAgentAPI(s.State, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.agent = api
}

func (s *agentSuite) TestAgentFailsWithNonMachineAgentUser(c *gc.C) {
	auth := s.authorizer
	auth.MachineAgent = false
	api, err := machine.NewAgentAPI(s.State, auth)
	c.Assert(err, gc.NotNil)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *agentSuite) TestGetMachines(c *gc.C) {
	err := s.machine1.Destroy()
	c.Assert(err, gc.IsNil)
	results := s.agent.GetMachines(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-42"},
		},
	})
	c.Assert(results, gc.DeepEquals, params.MachineAgentGetMachinesResults{
		Machines: []params.MachineAgentGetMachinesResult{
			{
				Life: "dying",
				Jobs: []params.MachineJob{params.JobHostUnits},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentSuite) TestGetNotFoundMachine(c *gc.C) {
	err := s.machine1.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine1.Remove()
	c.Assert(err, gc.IsNil)
	results := s.agent.GetMachines(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.MachineAgentGetMachinesResults{
		Machines: []params.MachineAgentGetMachinesResult{{
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
