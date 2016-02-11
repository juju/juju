// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package machine_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/testing"
)

type MachineShowCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&MachineShowCommandSuite{})

func newMachineShowCommand() cmd.Command {
	return machine.NewShowCommandForTest(&fakeStatusAPI{})
}

func (s *MachineShowCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *MachineShowCommandSuite) TestShowMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"model: dummyenv\n"+
		"machines:\n"+
		"  \"0\":\n"+
		"    agent-state: started\n"+
		"    dns-name: 10.0.0.1\n"+
		"    instance-id: juju-dummy-machine-0\n"+
		"    series: trusty\n"+
		"    hardware: availability-zone=us-east-1\n"+
		"  \"1\":\n"+
		"    agent-state: pending\n"+
		"    dns-name: 10.0.0.2\n"+
		"    instance-id: juju-dummy-machine-1\n"+
		"    series: trusty\n")
}
func (s *MachineShowCommandSuite) TestShowSingleMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"model: dummyenv\n"+
		"machines:\n"+
		"  \"0\":\n"+
		"    agent-state: started\n"+
		"    dns-name: 10.0.0.1\n"+
		"    instance-id: juju-dummy-machine-0\n"+
		"    series: trusty\n"+
		"    hardware: availability-zone=us-east-1\n")
}

func (s *MachineShowCommandSuite) TestShowTabularMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "--format", "tabular", "0", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"\n"+
		"[Machines] \n"+
		"ID         STATE   DNS      INS-ID               SERIES AZ        \n"+
		"0          started 10.0.0.1 juju-dummy-machine-0 trusty us-east-1 \n"+
		"1          pending 10.0.0.2 juju-dummy-machine-1 trusty           \n"+
		"\n")
}

func (s *MachineShowCommandSuite) TestShowJsonMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "--format", "json", "0", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"{\"model\":\"dummyenv\",\"machines\":{\"0\":{\"agent-state\":\"started\",\"dns-name\":\"10.0.0.1\",\"instance-id\":\"juju-dummy-machine-0\",\"series\":\"trusty\",\"hardware\":\"availability-zone=us-east-1\"},\"1\":{\"agent-state\":\"pending\",\"dns-name\":\"10.0.0.2\",\"instance-id\":\"juju-dummy-machine-1\",\"series\":\"trusty\"}}}\n")

}
