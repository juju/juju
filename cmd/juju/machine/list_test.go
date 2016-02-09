// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package machine_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/testing"
)

type MachineListCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&MachineListCommandSuite{})

func newMachineListCommand() cmd.Command {
	return machine.NewListCommandForTest(&fakeStatusAPI{})
}

type fakeStatusAPI struct{}

func (*fakeStatusAPI) Status(c []string) (*params.FullStatus, error) {
	result := &params.FullStatus{
		ModelName: "dummyenv",
		Machines: map[string]params.MachineStatus{
			"0": {
				Id: "0",
				Agent: params.AgentStatus{
					Status: "started",
				},
				DNSName:    "10.0.0.1",
				InstanceId: "juju-dummy-machine-0",
				Series:     "trusty",
				Hardware:   "availability-zone=us-east-1",
			},
			"1": {
				Id: "1",
				Agent: params.AgentStatus{
					Status: "pending",
				},
				DNSName:    "10.0.0.2",
				InstanceId: "juju-dummy-machine-1",
				Series:     "trusty",
			},
		},
	}
	return result, nil

}
func (*fakeStatusAPI) Close() error {
	return nil
}

func (s *MachineListCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *MachineListCommandSuite) TestMachine(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineListCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"\n"+
		"[Machines] \n"+
		"ID         STATE   DNS      INS-ID               SERIES AZ        \n"+
		"0          started 10.0.0.1 juju-dummy-machine-0 trusty us-east-1 \n"+
		"1          pending 10.0.0.2 juju-dummy-machine-1 trusty           \n"+
		"\n")
}

func (s *MachineListCommandSuite) TestListMachineYaml(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineListCommand(), "--format", "yaml")
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

func (s *MachineListCommandSuite) TestListMachineJson(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineListCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"{\"model\":\"dummyenv\",\"machines\":{\"0\":{\"agent-state\":\"started\",\"dns-name\":\"10.0.0.1\",\"instance-id\":\"juju-dummy-machine-0\",\"series\":\"trusty\",\"hardware\":\"availability-zone=us-east-1\"},\"1\":{\"agent-state\":\"pending\",\"dns-name\":\"10.0.0.2\",\"instance-id\":\"juju-dummy-machine-1\",\"series\":\"trusty\"}}}\n")
}

func (s *MachineListCommandSuite) TestListMachineArgsError(c *gc.C) {
	_, err := testing.RunCommand(c, newMachineListCommand(), "0")
	c.Assert(err, gc.ErrorMatches, "The list-machines command does not take any arguments")
}
