// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/runcmd"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/names"
)

type runcmdSuite struct {
	jujutesting.JujuConnSuite
	runcmd *runcmd.Client
}

var _ = gc.Suite(&runcmdSuite{})

func (s *runcmdSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.runcmd = runcmd.NewClient(s.APIState)
	c.Assert(s.runcmd, gc.NotNil)
}

func (s *runcmdSuite) TestRunOnAllMachines(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")
	s.addMachineWithAddress(c, "10.3.2.2")
	s.addMachineWithAddress(c, "10.3.2.3")

	s.mockSSH(c, echoInput)

	runparams := params.RunParamsV1{
		Commands: "hostname",
		Timeout:  testing.LongWait,
	}
	results, err := s.runcmd.RunOnAllMachines(runparams)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 3)
	var expectedResults []params.RunResult
	for i := 0; i < 3; i++ {
		expectedResults = append(expectedResults,
			params.RunResult{
				ExecResponse: exec.ExecResponse{Stdout: []byte("juju-run --no-context 'hostname'\n")},
				MachineId:    fmt.Sprint(i),
			})
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runcmdSuite) TestRunMachineAndService(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	charm := s.AddTestingCharm(c, "dummy")
	owner := s.Factory.MakeUser(c, nil).Tag()
	magic, err := s.State.AddService("magic", owner.String(), charm, nil)
	c.Assert(err, gc.IsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.mockSSH(c, echoInput)

	results, err := s.runcmd.Run(
		params.RunParamsV1{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Targets:  []string{names.NewMachineTag("0").String(), magic.Tag().String()},
		})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 3)
	expectedResults := []params.RunResult{
		params.RunResult{
			ExecResponse: exec.ExecResponse{Stdout: []byte("juju-run --no-context 'hostname'\n")},
			MachineId:    "0",
		},
		params.RunResult{
			ExecResponse: exec.ExecResponse{Stdout: []byte("juju-run magic/0 'hostname'\n")},
			MachineId:    "1",
			UnitId:       "magic/0",
		},
		params.RunResult{
			ExecResponse: exec.ExecResponse{Stdout: []byte("juju-run magic/1 'hostname'\n")},
			MachineId:    "2",
			UnitId:       "magic/1",
		},
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runcmdSuite) TestRunWithContext(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	owner := s.Factory.MakeUser(c, nil).Tag()
	charm := s.AddTestingCharm(c, "wordpress")
	wordpress, err := s.State.AddService("wordpress", owner.String(), charm, nil)
	c.Assert(err, gc.IsNil)
	unit := s.addUnit(c, wordpress)

	s.addRelatedService(c, "wordpress", "mysql", unit)
	s.mockSSH(c, echoInput)

	results, err := s.runcmd.Run(
		params.RunParamsV1{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Targets:  []string{wordpress.Tag().String()},
			Context:  &params.RunContext{Relation: "mysql"},
		})
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.HasLen, 1)
	expectedResults := []params.RunResult{
		params.RunResult{
			ExecResponse: exec.ExecResponse{Stdout: []byte("juju-run --relation 0 wordpress/0 'hostname'\n")},
			MachineId:    "1",
			UnitId:       "wordpress/0",
		},
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runcmdSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	return machine
}

func (s *runcmdSuite) mockSSH(c *gc.C, cmd string) {
	testbin := c.MkDir()
	fakessh := filepath.Join(testbin, "ssh")
	s.PatchEnvPathPrepend(testbin)
	err := ioutil.WriteFile(fakessh, []byte(cmd), 0755)
	c.Assert(err, gc.IsNil)
}

func (s *runcmdSuite) addMachineWithAddress(c *gc.C, address string) *state.Machine {
	machine := s.addMachine(c)
	machine.SetAddresses(network.NewAddress(address, network.ScopeUnknown))
	return machine
}

func (s *runcmdSuite) addUnit(c *gc.C, service *state.Service) *state.Unit {
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
	mId, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(mId)
	c.Assert(err, gc.IsNil)
	machine.SetAddresses(network.NewAddress("10.3.2.1", network.ScopeUnknown))
	return unit
}

func (s *runcmdSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Relation, *state.Service, *state.Unit) {
	relatedService := s.AddTestingService(c, relatedSvc, s.AddTestingCharm(c, relatedSvc))
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.addUnit(c, relatedService)
	relatedUnit, err := s.State.Unit(relatedSvc + "/0")
	c.Assert(err, gc.IsNil)
	return rel, relatedService, relatedUnit
}

func (s *runcmdSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

var echoInputShowArgs = `#!/bin/bash
# Write the args to stderr
echo "$*" >&2
# And echo stdin to stdout
while read line
do echo $line
done <&0
`

var echoInput = `#!/bin/bash
# And echo stdin to stdout
while read line
do echo $line
done <&0
`
