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
		Model: params.ModelStatusInfo{
			Name:    "dummyenv",
			Version: "1.2.3",
		},
		Machines: map[string]params.MachineStatus{
			"0": {
				Id: "0",
				AgentStatus: params.DetailedStatus{
					Status: "started",
				},
				DNSName:    "10.0.0.1",
				InstanceId: "juju-badd06-0",
				Series:     "trusty",
				Hardware:   "availability-zone=us-east-1",
			},
			"1": {
				Id: "1",
				AgentStatus: params.DetailedStatus{
					Status: "started",
				},
				DNSName:    "10.0.0.2",
				InstanceId: "juju-badd06-1",
				Series:     "trusty",
				Containers: map[string]params.MachineStatus{
					"1/lxd/0": {
						Id: "1/lxd/0",
						AgentStatus: params.DetailedStatus{
							Status: "pending",
						},
						DNSName:    "10.0.0.3",
						InstanceId: "juju-badd06-1-lxd-0",
						Series:     "trusty",
					},
				},
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
		"MACHINE  STATE    DNS       INS-ID               SERIES  AZ\n"+
		"0        started  10.0.0.1  juju-badd06-0        trusty  us-east-1\n"+
		"1        started  10.0.0.2  juju-badd06-1        trusty  \n"+
		"1/lxd/0  pending  10.0.0.3  juju-badd06-1-lxd-0  trusty  \n"+
		"\n")
}

func (s *MachineListCommandSuite) TestListMachineYaml(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineListCommand(), "--format", "yaml")
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

func (s *MachineListCommandSuite) TestListMachineJson(c *gc.C) {
	context, err := testing.RunCommand(c, newMachineListCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"{\"model\":\"dummyenv\",\"machines\":{\"0\":{\"juju-status\":{\"current\":\"started\"},\"dns-name\":\"10.0.0.1\",\"instance-id\":\"juju-badd06-0\",\"machine-status\":{},\"series\":\"trusty\",\"hardware\":\"availability-zone=us-east-1\"},\"1\":{\"juju-status\":{\"current\":\"started\"},\"dns-name\":\"10.0.0.2\",\"instance-id\":\"juju-badd06-1\",\"machine-status\":{},\"series\":\"trusty\",\"containers\":{\"1/lxd/0\":{\"juju-status\":{\"current\":\"pending\"},\"dns-name\":\"10.0.0.3\",\"instance-id\":\"juju-badd06-1-lxd-0\",\"machine-status\":{},\"series\":\"trusty\"}}}}}\n")
}

func (s *MachineListCommandSuite) TestListMachineArgsError(c *gc.C) {
	_, err := testing.RunCommand(c, newMachineListCommand(), "0")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["0"\]`)
}
