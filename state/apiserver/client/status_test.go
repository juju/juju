// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

type statusSuite struct {
	baseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	return machine
}

// Complete testing of status functionality happens elsewhere in the codebase,
// these tests just sanity-check the api itself.

func (s *statusSuite) TestFullStatus(c *gc.C) {
	machine := s.addMachine(c)
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, gc.IsNil)
	c.Check(status.EnvironmentName, gc.Equals, "dummyenv")
	c.Check(status.Services, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	c.Check(status.Networks, gc.HasLen, 0)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Series, gc.Equals, machine.Series())
}

func (s *statusSuite) TestLegacyStatus(c *gc.C) {
	machine := s.addMachine(c)
	instanceId := "i-fakeinstance"
	err := machine.SetProvisioned(instance.Id(instanceId), "fakenonce", nil)
	c.Assert(err, gc.IsNil)
	client := s.APIState.Client()
	status, err := client.LegacyStatus()
	c.Assert(err, gc.IsNil)
	c.Check(status.Machines, gc.HasLen, 1)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.InstanceId, gc.Equals, instanceId)
}
