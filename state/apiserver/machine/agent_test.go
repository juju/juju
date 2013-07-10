package machine_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/machine"
)

type agentSuite struct {
	commonSuite
	agent *machine.AgentAPI
}

var _ = Suite(&agentSuite{})

func (s *agentSuite) SetUpTest(c *C) {
	s.commonSuite.SetUpTest(c)

	// Create a machiner API for machine 1.
	api, err := machine.NewAgentAPI(
		s.State,
		s.authorizer,
	)
	c.Assert(err, IsNil)
	s.agent = api
}

func (s *agentSuite) TestAgentFailsWithNonMachineAgentUser(c *C) {
	auth := s.authorizer
	auth.MachineAgent = false
	api, err := machine.NewAgentAPI(s.State, auth)
	c.Assert(err, NotNil)
	c.Assert(api, IsNil)
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *agentSuite) TestGetMachines(c *C) {
	err := s.machine1.Destroy()
	c.Assert(err, IsNil)
	results := s.agent.GetMachines(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-1"},
			{Tag: "machine-0"},
			{Tag: "machine-42"},
		},
	})
	c.Assert(results, DeepEquals, params.MachineAgentGetMachinesResults{
		Machines: []params.MachineAgentGetMachinesResult{{
			Life: "dying",
			Jobs: []params.MachineJob{params.JobHostUnits},
		}, {
			Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "permission denied",
			},
		}},
	})
}

func (s *agentSuite) TestGetNotFoundMachine(c *C) {
	err := s.machine1.Destroy()
	c.Assert(err, IsNil)
	err = s.machine1.EnsureDead()
	c.Assert(err, IsNil)
	err = s.machine1.Remove()
	c.Assert(err, IsNil)
	results := s.agent.GetMachines(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1"}},
	})
	c.Assert(err, IsNil)
	c.Assert(results, DeepEquals, params.MachineAgentGetMachinesResults{
		Machines: []params.MachineAgentGetMachinesResult{{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "machine 1 not found",
			},
		}},
	})
}

func (s *agentSuite) TestSetPasswords(c *C) {
	results, err := s.agent.SetPasswords(params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: "machine-0", Password: "xxx"},
			{Tag: "machine-1", Password: "yyy"},
			{Tag: "machine-42", Password: "zzz"},
		},
	})
	c.Assert(err, IsNil)
	unauth := &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	c.Assert(results, DeepEquals, params.ErrorResults{
		Errors: []*params.Error{unauth, nil, unauth},
	})
	err = s.machine1.Refresh()
	c.Assert(err, IsNil)
	changed := s.machine1.PasswordValid("yyy")
	c.Assert(changed, Equals, true)
}
