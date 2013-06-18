package machine_test

import (
	. "launchpad.net/gocheck"
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
	auth.machineAgent = false
	api, err := machine.NewAgentAPI(s.State, auth)
	c.Assert(err, NotNil)
	c.Assert(api, IsNil)
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *agentSuite) TestAgentFailsWhenNotLoggedIn(c *C) {
	auth := s.authorizer
	auth.loggedIn = false
	api, err := machine.NewAgentAPI(s.State, auth)
	c.Assert(err, NotNil)
	c.Assert(api, IsNil)
	c.Assert(err, ErrorMatches, "not logged in")
}

