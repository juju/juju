// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"time"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
)

type runSuite struct {
	baseSuite
}

var _ = gc.Suite(&runSuite{})

func (s *runSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *runSuite) addMachineWithAddress(c *gc.C, address string) *state.Machine {
	machine := s.addMachine(c)
	machine.SetProviderAddresses(network.NewAddress(address))
	return machine
}

func (s *runSuite) TestRemoteParamsForMachinePopulates(c *gc.C) {
	machine := s.addMachine(c)
	result := client.RemoteParamsForMachine(machine, "command", time.Minute)
	c.Assert(result.Command, gc.Equals, "command")
	c.Assert(result.Timeout, gc.Equals, time.Minute)
	c.Assert(result.MachineId, gc.Equals, machine.Id())
	// Now an empty host isn't particularly useful, but the machine doesn't
	// have an address to use.
	c.Assert(machine.Addresses(), gc.HasLen, 0)
	c.Assert(result.Host, gc.Equals, "")
}

func (s *runSuite) TestRemoteParamsForMachinePopulatesWithAddress(c *gc.C) {
	machine := s.addMachineWithAddress(c, "10.3.2.1")

	result := client.RemoteParamsForMachine(machine, "command", time.Minute)
	c.Assert(result.Command, gc.Equals, "command")
	c.Assert(result.Timeout, gc.Equals, time.Minute)
	c.Assert(result.MachineId, gc.Equals, machine.Id())
	c.Assert(result.Host, gc.Equals, "ubuntu@10.3.2.1")
}

func (s *runSuite) addUnit(c *gc.C, service *state.Service) *state.Unit {
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mId)
	c.Assert(err, jc.ErrorIsNil)
	machine.SetProviderAddresses(network.NewAddress("10.3.2.1"))
	return unit
}

func (s *runSuite) TestGetAllUnitNames(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	owner := s.AdminUserTag(c)
	magic, err := s.State.AddService("magic", owner.String(), charm, nil, nil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	notAssigned, err := s.State.AddService("not-assigned", owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = notAssigned.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddService("no-units", owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	wordpress, err := s.State.AddService("wordpress", owner.String(), s.AddTestingCharm(c, "wordpress"), nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wordpress0 := s.addUnit(c, wordpress)
	_, err = s.State.AddService("logging", owner.String(), s.AddTestingCharm(c, "logging"), nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		message  string
		expected []string
		units    []string
		services []string
		error    string
	}{{
		message: "no units, expected nil slice",
	}, {
		message: "asking for a unit that isn't there",
		units:   []string{"foo/0"},
		error:   `unit "foo/0" not found`,
	}, {
		message:  "asking for a service that isn't there",
		services: []string{"foo"},
		error:    `service "foo" not found`,
	}, {
		message:  "service with no units is not really an error",
		services: []string{"no-units"},
	}, {
		message:  "A service with units not assigned is an error",
		services: []string{"not-assigned"},
		error:    `unit "not-assigned/0" is not assigned to a machine`,
	}, {
		message:  "A service with units",
		services: []string{"magic"},
		expected: []string{"magic/0", "magic/1"},
	}, {
		message:  "Asking for just a unit",
		units:    []string{"magic/0"},
		expected: []string{"magic/0"},
	}, {
		message:  "Asking for just a subordinate unit",
		units:    []string{"logging/0"},
		expected: []string{"logging/0"},
	}, {
		message:  "Asking for a unit, and the service",
		services: []string{"magic"},
		units:    []string{"magic/0"},
		expected: []string{"magic/0", "magic/1"},
	}} {
		c.Logf("%v: %s", i, test.message)
		result, err := client.GetAllUnitNames(s.State, test.units, test.services)
		if test.error == "" {
			c.Check(err, jc.ErrorIsNil)
			var units []string
			for _, unit := range result {
				units = append(units, unit.Name())
			}
			c.Check(units, jc.SameContents, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.error)
		}
	}
}

func (s *runSuite) mockSSH(c *gc.C, cmd string) {
	gitjujutesting.PatchExecutable(c, s, "ssh", cmd)
	gitjujutesting.PatchExecutable(c, s, "scp", cmd)
	client, _ := ssh.NewOpenSSHClient()
	s.PatchValue(&ssh.DefaultClient, client)
}

func (s *runSuite) TestParallelExecuteErrorsOnBlankHost(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Command: "foo",
				Timeout: testing.LongWait,
			},
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "missing host address")
}

func (s *runSuite) TestParallelExecuteAddsIdentity(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Host:    "localhost",
				Command: "foo",
				Timeout: testing.LongWait,
			},
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "")
	c.Assert(string(result.Stderr), jc.Contains, "system-identity")
}

func (s *runSuite) TestParallelExecuteCopiesAcrossMachineAndUnit(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Host:    "localhost",
				Command: "foo",
				Timeout: testing.LongWait,
			},
			MachineId: "machine-id",
			UnitId:    "unit-id",
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "")
	c.Assert(result.MachineId, gc.Equals, "machine-id")
	c.Assert(result.UnitId, gc.Equals, "unit-id")
}

func (s *runSuite) TestRunOnAllMachines(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")
	s.addMachineWithAddress(c, "10.3.2.2")
	s.addMachineWithAddress(c, "10.3.2.3")

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()
	results, err := client.RunOnAllMachines("hostname", testing.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)

	var expectedResults []params.RunResult
	for i := 0; i < 3; i++ {
		expectedResults = append(expectedResults,
			params.RunResult{
				ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[0])},

				MachineId: fmt.Sprint(i),
			})
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runSuite) TestBlockRunOnAllMachines(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")
	s.addMachineWithAddress(c, "10.3.2.2")
	s.addMachineWithAddress(c, "10.3.2.3")

	s.mockSSH(c, echoInput)

	// block all changes
	s.BlockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := s.APIState.Client().RunOnAllMachines("hostname", testing.LongWait)
	s.AssertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestRunMachineAndService(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	charm := s.AddTestingCharm(c, "dummy")
	owner := s.Factory.MakeUser(c, nil).Tag()
	magic, err := s.State.AddService("magic", owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()
	results, err := client.Run(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Machines: []string{"0"},
			Services: []string{"magic"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)

	expectedResults := []params.RunResult{
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[0])},
			MachineId:    "0",
		},
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[1])},
			MachineId:    "1",
			UnitId:       "magic/0",
		},
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[2])},
			MachineId:    "2",
			UnitId:       "magic/1",
		},
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runSuite) TestBlockRunMachineAndService(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	charm := s.AddTestingCharm(c, "dummy")
	owner := s.Factory.MakeUser(c, nil).Tag()
	magic, err := s.State.AddService("magic", owner.String(), charm, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()

	// block all changes
	s.BlockAllChanges(c, "TestBlockRunMachineAndService")
	_, err = client.Run(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Machines: []string{"0"},
			Services: []string{"magic"},
		})
	s.AssertBlocked(c, err, "TestBlockRunMachineAndService")
}
