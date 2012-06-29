package main

import (
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type ProvisioningSuite struct {
	logging testing.LoggingSuite
	zkSuite
	st *state.State
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

var veryShortAttempt = environs.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 80 * time.Millisecond,
}

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.logging.SetUpTest(c)

	// Create the operations channel with more than enough space
	// for those tests that don't listen on it.
	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	env, err := environs.NewEnviron(map[string]interface{}{
		"type":      "dummy",
		"zookeeper": true,
		"name":      "testing",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	s.zkSuite.SetUpTest(c)
	s.st, err = state.Open(s.zkInfo)
	c.Assert(err, IsNil)

	// Make sure that zkInfo holds exactly the info we're passing to
	// Newservice.Provisioner,
	s.zkInfo, err = env.StateInfo()
	c.Assert(err, IsNil)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.zkSuite.TearDownTest()
	s.logging.TearDownTest(c)
}

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}

func initProvisioningAgent() (*ProvisioningAgent, error) {
	args := []string{"--zookeeper-servers", zkAddr}
	c := &ProvisioningAgent{}
	return c, initCmd(c, args)
}
