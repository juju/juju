package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju/testing"
	"reflect"
	"time"
)

type ProvisioningSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ProvisioningSuite{})

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
	op := make(chan dummy.Operation, 200)
	dummy.Listen(op)

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

	// Add one unit to a service; it should get allocated a machine
	// and then its ports should be opened.
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.Conn.AddService("test-service", charm)
	c.Assert(err, IsNil)
	err = svc.SetExposed()
	c.Assert(err, IsNil)
	units, err := s.Conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	err = units[0].OpenPort("tcp", 999)
	c.Assert(err, IsNil)

	c.Check(opRecvTimeout(c, op, dummy.OpStartInstance{}), NotNil)
	c.Check(opRecvTimeout(c, op, dummy.OpOpenPorts{}), NotNil)

	err = a.Stop()
	c.Assert(err, IsNil)

	select {
	case err := <-done:
		c.Assert(err, IsNil)
	case <-time.After(2 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *C, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
	for {
		select {
		case op := <-opc:
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					return op
				}
			}
			c.Logf("discarding unknown event %#v", op)
		case <-time.After(2 * time.Second):
			c.Fatalf("time out wating for operation")
		}
	}
	panic("not reached")
}
