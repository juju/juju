package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/tomb"
	"reflect"
	"time"
)

type ProvisioningSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)


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
	conn, err := juju.NewConnFromAttrs(map[string]interface{}{
		"name":            "testing",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, IsNil)
	op := make(chan dummy.Operation, 200)
	dummy.Listen(op)
	s.op = filterOps(op, dummy.OpStartInstance{}, dummy.OpOpenPorts{})

	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	a := &ProvisioningAgent{
		Conf: AgentConf{
			JujuDir:   "/var/lib/juju",
			StateInfo: *s.StateInfo(c),
		},
	}
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	// Check that the provisioner is alive by doing a rudimentary check
	// that it responds to state changes.
	st, err := conn.State()
	c.Assert(err, IsNil)

	charm := s.AddTestingCharm(c, "dummy")

	s.checkProvisionerAlive(c)
	s.checkFirewallerAlive(c)

	err = a.Stop()
	c.Assert(err, IsNil)

	err = <-done
	c.Assert(err, IsNil)
}

func (s *ProvisioningSuite) checkProvisionerAlive(c *C) {
	// Allocate a machine and check that we get a StartInstance operation
	_, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	c.Assert(<-s.op, FitsTypeOf, dummy.OpStartInstance{})

}

func (s *ProvisioningSuite) checkFirewallerAlive(c *C) {
	charm := s.AddTestingCharm(c, "dummy")

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	inst, err := s.environ.StartInstance(m.Id(), s.StateInfo(c), nil)
	c.Assert(err, IsNil)
}

// filterOps filters all but the given kinds of operation from
// the operations channel.
func filterOps(c <-chan dummy.Operation, kinds ...dummy.Operation) <-chan dummy.Operation {
	rc := make(chan dummy.Operation)
	go func() {
		for op := range c {
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					rc <- op
					break
				}
			}
		}
		close(rc)
	}()
	return rc
}
