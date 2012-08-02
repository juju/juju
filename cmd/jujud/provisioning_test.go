package main

import (
	// "fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	// "launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	// "sort"
	// "strings"
	// "time"
)

type ProvisioningSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "testing",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	s.StateSuite.SetUpTest(c)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
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

func (s *ProvisioningSuite) TestRunStop(c *C) {
	a := &ProvisioningAgent{
		Conf: AgentConf{
			JujuDir:   "/var/lib/juju",
			StateInfo: *s.StateInfo(c),
		},
	}

	go func() {
		err := a.Run(nil)
		c.Assert(err, IsNil)
	}()

	err := a.Stop()
	c.Assert(err, IsNil)
}
