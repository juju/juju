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
		"    juju-status:\n"+
		"      current: started\n"+
		"    dns-name: 10.0.0.1\n"+
		"    instance-id: juju-badd06-0\n"+
		"    series: trusty\n"+
		"    hardware: availability-zone=us-east-1\n"+
		"  \"1\":\n"+
		"    juju-status:\n"+
		"      current: started\n"+
		"    dns-name: 10.0.0.2\n"+
		"    instance-id: juju-badd06-1\n"+
		"    series: trusty\n"+
		"    containers:\n"+
		"      1/lxd/0:\n"+
		"        juju-status:\n"+
		"          current: pending\n"+
		"        dns-name: 10.0.0.3\n"+
		"        instance-id: juju-badd06-1-lxd-0\n"+
		"        series: trusty\n")
}
func (s *MachineShowCommandSuite) TestShowSingleMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"model: dummyenv\n"+
		"machines:\n"+
		"  \"0\":\n"+
		"    juju-status:\n"+
		"      current: started\n"+
		"    dns-name: 10.0.0.1\n"+
		"    instance-id: juju-badd06-0\n"+
		"    series: trusty\n"+
		"    hardware: availability-zone=us-east-1\n")
}

func (s *MachineShowCommandSuite) TestShowTabularMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "--format", "tabular", "0", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"MACHINE  STATE    DNS       INS-ID               SERIES  AZ\n"+
		"0        started  10.0.0.1  juju-badd06-0        trusty  us-east-1\n"+
		"1        started  10.0.0.2  juju-badd06-1        trusty  \n"+
		"1/lxd/0  pending  10.0.0.3  juju-badd06-1-lxd-0  trusty  \n"+
		"\n")
}

func (s *MachineShowCommandSuite) TestShowJsonMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineShowCommand(), "--format", "json", "0", "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"{\"model\":\"dummyenv\",\"machines\":{\"0\":{\"juju-status\":{\"current\":\"started\"},\"dns-name\":\"10.0.0.1\",\"instance-id\":\"juju-badd06-0\",\"machine-status\":{},\"series\":\"trusty\",\"hardware\":\"availability-zone=us-east-1\"},\"1\":{\"juju-status\":{\"current\":\"started\"},\"dns-name\":\"10.0.0.2\",\"instance-id\":\"juju-badd06-1\",\"machine-status\":{},\"series\":\"trusty\",\"containers\":{\"1/lxd/0\":{\"juju-status\":{\"current\":\"pending\"},\"dns-name\":\"10.0.0.3\",\"instance-id\":\"juju-badd06-1-lxd-0\",\"machine-status\":{},\"series\":\"trusty\"}}}}}\n")
}
