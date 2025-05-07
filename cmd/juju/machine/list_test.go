// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package machine_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MachineListCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&MachineListCommandSuite{})

func newMachineListCommand() cmd.Command {
	return machine.NewListCommandForTest(&fakeStatusAPI{})
}

type fakeStatusAPI struct{}

func (*fakeStatusAPI) Status(ctx context.Context, args *client.StatusArgs) (*params.FullStatus, error) {
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
				DNSName: "10.0.0.1",
				IPAddresses: []string{
					"10.0.0.1",
					"10.0.1.1",
				},
				InstanceId: "juju-badd06-0",
				Base:       params.Base{Name: "ubuntu", Channel: "22.04"},
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{
							"10.0.0.1",
							"10.0.1.1",
						},
						MACAddress: "aa:bb:cc:dd:ee:ff",
						IsUp:       true,
					},
				},
				Constraints: "mem=3584M",
				Hardware:    "availability-zone=us-east-1",
			},
			"1": {
				Id: "1",
				AgentStatus: params.DetailedStatus{
					Status: "started",
				},
				DNSName: "10.0.0.2",
				IPAddresses: []string{
					"10.0.0.2",
					"10.0.1.2",
				},
				InstanceId: "juju-badd06-1",
				Base:       params.Base{Name: "ubuntu", Channel: "22.04"},
				NetworkInterfaces: map[string]params.NetworkInterface{
					"eth0": {
						IPAddresses: []string{
							"10.0.0.2",
							"10.0.1.2",
						},
						MACAddress: "aa:bb:cc:dd:ee:ff",
						IsUp:       true,
					},
				},
				Containers: map[string]params.MachineStatus{
					"1/lxd/0": {
						Id: "1/lxd/0",
						AgentStatus: params.DetailedStatus{
							Status: "pending",
						},
						DNSName: "10.0.0.3",
						IPAddresses: []string{
							"10.0.0.3",
							"10.0.1.3",
						},
						InstanceId: "juju-badd06-1-lxd-0",
						Base:       params.Base{Name: "ubuntu", Channel: "22.04"},
						NetworkInterfaces: map[string]params.NetworkInterface{
							"eth0": {
								IPAddresses: []string{
									"10.0.0.3",
									"10.0.1.3",
								},
								MACAddress: "aa:bb:cc:dd:ee:ff",
								IsUp:       true,
							},
						},
					},
				},
				LXDProfiles: map[string]params.LXDProfile{
					"lxd-profile-name": {
						Config: map[string]string{
							"environment.http_proxy": "",
						},
						Description: "lxd-profile description",
						Devices: map[string]map[string]string{
							"tun": {
								"path": "/dev/net/tun",
								"type": "unix-char",
							},
						},
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

func (s *MachineListCommandSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *MachineListCommandSuite) TestMachine(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, newMachineListCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Machine  State    Address   Inst id              Base          AZ         Message\n"+
		"0        started  10.0.0.1  juju-badd06-0        ubuntu@22.04  us-east-1  \n"+
		"1        started  10.0.0.2  juju-badd06-1        ubuntu@22.04             \n"+
		"1/lxd/0  pending  10.0.0.3  juju-badd06-1-lxd-0  ubuntu@22.04             \n")
}

func (s *MachineListCommandSuite) TestListMachineYaml(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, newMachineListCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"model: dummyenv\n"+
		"machines:\n"+
		"  \"0\":\n"+
		"    juju-status:\n"+
		"      current: started\n"+
		"    dns-name: 10.0.0.1\n"+
		"    ip-addresses:\n"+
		"    - 10.0.0.1\n"+
		"    - 10.0.1.1\n"+
		"    instance-id: juju-badd06-0\n"+
		"    base:\n"+
		"      name: ubuntu\n"+
		"      channel: \"22.04\"\n"+
		"    network-interfaces:\n"+
		"      eth0:\n"+
		"        ip-addresses:\n"+
		"        - 10.0.0.1\n"+
		"        - 10.0.1.1\n"+
		"        mac-address: aa:bb:cc:dd:ee:ff\n"+
		"        is-up: true\n"+
		"    constraints: mem=3584M\n"+
		"    hardware: availability-zone=us-east-1\n"+
		"  \"1\":\n"+
		"    juju-status:\n"+
		"      current: started\n"+
		"    dns-name: 10.0.0.2\n"+
		"    ip-addresses:\n"+
		"    - 10.0.0.2\n"+
		"    - 10.0.1.2\n"+
		"    instance-id: juju-badd06-1\n"+
		"    base:\n"+
		"      name: ubuntu\n"+
		"      channel: \"22.04\"\n"+
		"    network-interfaces:\n"+
		"      eth0:\n"+
		"        ip-addresses:\n"+
		"        - 10.0.0.2\n"+
		"        - 10.0.1.2\n"+
		"        mac-address: aa:bb:cc:dd:ee:ff\n"+
		"        is-up: true\n"+
		"    containers:\n"+
		"      1/lxd/0:\n"+
		"        juju-status:\n"+
		"          current: pending\n"+
		"        dns-name: 10.0.0.3\n"+
		"        ip-addresses:\n"+
		"        - 10.0.0.3\n"+
		"        - 10.0.1.3\n"+
		"        instance-id: juju-badd06-1-lxd-0\n"+
		"        base:\n"+
		"          name: ubuntu\n"+
		"          channel: \"22.04\"\n"+
		"        network-interfaces:\n"+
		"          eth0:\n"+
		"            ip-addresses:\n"+
		"            - 10.0.0.3\n"+
		"            - 10.0.1.3\n"+
		"            mac-address: aa:bb:cc:dd:ee:ff\n"+
		"            is-up: true\n"+
		"    lxd-profiles:\n"+
		"      lxd-profile-name:\n"+
		"        config:\n"+
		"          environment.http_proxy: \"\"\n"+
		"        description: lxd-profile description\n"+
		"        devices:\n"+
		"          tun:\n"+
		"            path: /dev/net/tun\n"+
		"            type: unix-char\n",
	)
}

func (s *MachineListCommandSuite) TestListMachineJson(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, newMachineListCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	// Make the test more readable by putting all the JSON in a expanded form.
	// Then to test it, marshal it back into json, so that map equality ordering
	// doesn't matter.
	expectedJSON, err := unmarshalStringAsJSON("" +
		"{" +
		"	\"model\":\"dummyenv\"," +
		"	\"machines\":{" +
		"	   \"0\":{" +
		"		  \"juju-status\":{" +
		"			 \"current\":\"started\"" +
		"		  }," +
		"		  \"dns-name\":\"10.0.0.1\"," +
		"		  \"ip-addresses\":[" +
		"			 \"10.0.0.1\"," +
		"			 \"10.0.1.1\"" +
		"		  ]," +
		"		  \"instance-id\":\"juju-badd06-0\"," +
		"		  \"machine-status\":{}," +
		"		  \"modification-status\":{}," +
		"		  \"base\":{\"name\":\"ubuntu\",\"channel\":\"22.04\"}," +
		"		  \"network-interfaces\":{" +
		"			 \"eth0\":{" +
		"				\"ip-addresses\":[" +
		"				   \"10.0.0.1\"," +
		"				   \"10.0.1.1\"" +
		"				]," +
		"				\"mac-address\":\"aa:bb:cc:dd:ee:ff\"," +
		"				\"is-up\":true" +
		"			 }" +
		"		  }," +
		"		  \"constraints\":\"mem=3584M\"," +
		"		  \"hardware\":\"availability-zone=us-east-1\"" +
		"	   }," +
		"	   \"1\":{" +
		"		  \"juju-status\":{" +
		"			 \"current\":\"started\"" +
		"		  }," +
		"		  \"dns-name\":\"10.0.0.2\"," +
		"		  \"ip-addresses\":[" +
		"			 \"10.0.0.2\"," +
		"			 \"10.0.1.2\"" +
		"		  ]," +
		"		  \"instance-id\":\"juju-badd06-1\"," +
		"		  \"machine-status\":{}," +
		"		  \"modification-status\":{}," +
		"		  \"base\":{\"name\":\"ubuntu\",\"channel\":\"22.04\"}," +
		"		  \"network-interfaces\":{" +
		"			 \"eth0\":{" +
		"				\"ip-addresses\":[" +
		"				   \"10.0.0.2\"," +
		"				   \"10.0.1.2\"" +
		"				]," +
		"				\"mac-address\":\"aa:bb:cc:dd:ee:ff\"," +
		"				\"is-up\":true" +
		"			 }" +
		"		  }," +
		"		  \"containers\":{" +
		"			 \"1/lxd/0\":{" +
		"				\"juju-status\":{" +
		"				   \"current\":\"pending\"" +
		"				}," +
		"				\"dns-name\":\"10.0.0.3\"," +
		"				\"ip-addresses\":[" +
		"				   \"10.0.0.3\"," +
		"				   \"10.0.1.3\"" +
		"				]," +
		"				\"instance-id\":\"juju-badd06-1-lxd-0\"," +
		"				\"machine-status\":{}," +
		"				\"modification-status\":{}," +
		"		        \"base\":{\"name\":\"ubuntu\",\"channel\":\"22.04\"}," +
		"				\"network-interfaces\":{" +
		"				   \"eth0\":{" +
		"					  \"ip-addresses\":[" +
		"						 \"10.0.0.3\"," +
		"						 \"10.0.1.3\"" +
		"					  ]," +
		"					  \"mac-address\":\"aa:bb:cc:dd:ee:ff\"," +
		"					  \"is-up\":true" +
		"				   }" +
		"				}" +
		"			 }" +
		"		  }," +
		"		  \"lxd-profiles\":{" +
		"			 \"lxd-profile-name\":{" +
		"				\"config\":{" +
		"				   \"environment.http_proxy\":\"\"" +
		"				}," +
		"				\"description\":\"lxd-profile description\"," +
		"				\"devices\":{" +
		"				   \"tun\":{" +
		"					  \"path\":\"/dev/net/tun\"," +
		"					  \"type\":\"unix-char\"" +
		"				   }" +
		"				}" +
		"			 }" +
		"		  }" +
		"	   }" +
		"	}" +
		" }\n")
	c.Assert(err, tc.ErrorIsNil)
	actualJSON, err := unmarshalStringAsJSON(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(actualJSON, tc.DeepEquals, expectedJSON)
}

func (s *MachineListCommandSuite) TestListMachineArgsError(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, newMachineListCommand(), "0")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["0"\]`)
}
