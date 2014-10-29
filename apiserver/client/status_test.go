// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
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

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
	*factory.Factory
}

func (s *statusUnitTestSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	// State gets reset per test, so must the factory.
	s.Factory = factory.NewFactory(s.State)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	host := s.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("0")})
	container := s.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): []*state.Machine{host, container},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	containerStatus := client.MakeMachineStatus(container)
	c.Check(statuses[host.Id()].Containers[container.Id()].Id, gc.Equals, containerStatus.Id)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {

	host := s.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("1")})
	lxcHost := s.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): []*state.Machine{
			host,
			lxcHost,
			s.MakeMachineNested(c, lxcHost.Id(), nil),
			s.MakeMachineNested(c, host.Id(), nil),
		},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	hostContainer := statuses[host.Id()].Containers
	c.Check(hostContainer, gc.HasLen, 2)
	c.Check(hostContainer[lxcHost.Id()].Containers, gc.HasLen, 1)
}
