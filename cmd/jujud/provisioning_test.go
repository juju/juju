package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/tomb"
	"time"
)

func assertAlive(c *C, a *ProvisioningAgent, alive bool) {
	start := time.Now()
	for time.Since(start) < 500*time.Millisecond {
		time.Sleep(50 * time.Millisecond)
		perr := a.provisioner.Err()
		fwerr := a.firewaller.Err()
		if alive {
			// Provisioner and firewaller have to be alive.
			if perr == tomb.ErrStillAlive && fwerr == tomb.ErrStillAlive {
				return
			}
		} else {
			// Provisioner and firewaller have to be stopped.
			if perr != tomb.ErrStillAlive && fwerr != tomb.ErrStillAlive {
				c.Succeed()
				return
			}
		}
	}
	c.Fatalf("timed out")
	return
}

type ProvisioningSuite struct {
	coretesting.LoggingSuite
	testing.JujuConnSuite
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.JujuConnSuite.SetUpTest(c)
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

	assertAlive(c, a, true)

	err := a.Stop()
	c.Assert(err, IsNil)

	assertAlive(c, a, false)
}
