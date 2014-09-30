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
