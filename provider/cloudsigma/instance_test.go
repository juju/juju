// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"strings"

	"github.com/altoros/gosigma"
	"github.com/altoros/gosigma/data"
	"github.com/altoros/gosigma/mock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type instanceSuite struct {
	testing.BaseSuite
	inst          *sigmaInstance
	instWithoutIP *sigmaInstance
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	mock.Start()
}

func (s *instanceSuite) TearDownSuite(c *gc.C) {
	mock.Stop()
	s.BaseSuite.TearDownSuite(c)
}

func (s *instanceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	cli, err := gosigma.NewClient(mock.Endpoint(""), mock.TestUser, mock.TestPassword, nil)
	c.Assert(err, gc.IsNil)

	mock.ResetServers()

	ds, err := data.ReadServer(strings.NewReader(jsonInstanceData))
	c.Assert(err, gc.IsNil)
	mock.AddServer(ds)

	mock.AddServer(&data.Server{
		Resource: data.Resource{URI: "uri", UUID: "uuid-no-ip"},
	})

	server, err := cli.Server("f4ec5097-121e-44a7-a207-75bc02163260")
	c.Assert(err, gc.IsNil)
	c.Assert(server, gc.NotNil)
	s.inst = &sigmaInstance{server}

	server, err = cli.Server("uuid-no-ip")
	c.Assert(err, gc.IsNil)
	c.Assert(server, gc.NotNil)
	s.instWithoutIP = &sigmaInstance{server}
}

func (s *instanceSuite) TearDownTest(c *gc.C) {
	mock.ResetServers()
	s.BaseSuite.TearDownTest(c)
}

func (s *instanceSuite) TestInstanceId(c *gc.C) {
	c.Check(s.inst.Id(), gc.Equals, instance.Id("f4ec5097-121e-44a7-a207-75bc02163260"))
}

func (s *instanceSuite) TestInstanceStatus(c *gc.C) {
	c.Check(s.inst.Status(), gc.Equals, "running")
}

func (s *instanceSuite) TestInstanceRefresh(c *gc.C) {
	c.Check(s.inst.Status(), gc.Equals, "running")

	mock.SetServerStatus("f4ec5097-121e-44a7-a207-75bc02163260", "stopped")

	err := s.inst.Refresh()
	c.Check(err, gc.IsNil)

	c.Check(s.inst.Status(), gc.Equals, "stopped")
}

func (s *instanceSuite) TestInstanceAddresses(c *gc.C) {
	addrs, err := s.inst.Addresses()
	c.Check(addrs, gc.HasLen, 1)
	c.Check(err, gc.IsNil)
	a := addrs[0]
	c.Check(a.Value, gc.Equals, "178.22.70.33")
	c.Check(a.Type, gc.Equals, network.IPv4Address)
	c.Check(a.Scope, gc.Equals, network.ScopePublic)
	c.Check(a.NetworkName, gc.Equals, "")

	addrs, err = s.instWithoutIP.Addresses()
	c.Check(err, gc.IsNil)
	c.Check(len(addrs), gc.Equals, 0)
}

func (s *instanceSuite) TestInstancePorts(c *gc.C) {
	c.Check(s.inst.OpenPorts("", nil), gc.ErrorMatches, "OpenPorts not implemented")
	c.Check(s.inst.ClosePorts("", nil), gc.ErrorMatches, "ClosePorts not implemented")

	_, err := s.inst.Ports("")
	c.Check(err, gc.ErrorMatches, "Ports not implemented")
}

func (s *instanceSuite) TestInstanceHardware(c *gc.C) {
	hw, err := s.inst.hardware("64", 1000000)
	c.Assert(err, gc.IsNil)
	c.Assert(hw, gc.NotNil)

	c.Check(*hw.Arch, gc.Equals, "64")

	c.Check(hw.Mem, gc.NotNil)
	if hw.Mem != nil {
		c.Check(*hw.Mem, gc.Equals, uint64(2048))
	}

	c.Check(hw.RootDisk, gc.IsNil)

	c.Check(hw.CpuCores, gc.NotNil)
	if hw.CpuCores != nil {
		c.Check(*hw.CpuCores, gc.Equals, uint64(1))
	}

	c.Check(hw.CpuPower, gc.NotNil)
	c.Check(*hw.CpuPower, gc.Equals, uint64(2000))

	c.Check(hw.Tags, gc.IsNil)
}

const jsonInstanceData = `{
    "context": true,
    "cpu": 2000,
    "cpu_model": null,
    "cpus_instead_of_cores": false,
    "drives": [
        {
            "boot_order": 1,
            "dev_channel": "0:0",
            "device": "virtio",
            "drive": {
                "resource_uri": "/api/2.0/drives/f968dc48-25a0-4d46-8f16-e12e073e1fe6/",
                "uuid": "f968dc48-25a0-4d46-8f16-e12e073e1fe6"
            },
            "runtime": {
                "io": {
                    "bytes_read": 82980352,
                    "bytes_written": 189440,
                    "count_flush": 0,
                    "count_read": 3952,
                    "count_written": 19,
                    "total_time_ns_flush": 0,
                    "total_time_ns_read": 4435322816,
                    "total_time_ns_write": 123240430
                }
            }
        }
    ],
    "enable_numa": false,
    "hv_relaxed": false,
    "hv_tsc": false,
    "jobs": [],
    "mem": 2147483648,
    "meta": {
        "description": "test-description",
        "ssh_public_key": "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDDiwTGBsmFKBYHcKaVy5IgsYBR4XVYLS6KP/NKClE7gONlIGURE3+/45BX8TfHJHM5WTN8NBqJejKDHqwfyueR1f2VGoPkJxODGt/X/ZDNftLZLYwPd2DfDBs27ahOadZCk4Cl5l7mU0aoE74UnIcQoNPl6w7axkIFTIXr8+0HMk8DFB0iviBSJK118p1RGwhsoA1Hudn1CsgqARGPmNn6mxwvmQfQY7hZxZoOH9WMcvkNZ7rAFrwS/BuvEpEXkoC95K/JDPvmQVVJk7we+WeHfTYSmApkDFcSaypyjL2HOV8pvE+VntcIIhZccHiOubyjsBAx5aoTI+ueCsoz5AL1 maxim.perenesenko@altoros.com"
    },
    "name": "LiveTest-srv-17-54-51-999999999",
    "nics": [
        {
            "boot_order": null,
            "firewall_policy": null,
            "ip_v4_conf": {
                "conf": "dhcp",
                "ip": null
            },
            "ip_v6_conf": null,
            "mac": "22:ab:bf:26:e1:be",
            "model": "virtio",
            "runtime": {
                "interface_type": "public",
                "io": {
                    "bytes_recv": 0,
                    "bytes_sent": 17540,
                    "packets_recv": 0,
                    "packets_sent": 256
                },
                "ip_v4": {
                    "resource_uri": "/api/2.0/ips/178.22.70.33/",
                    "uuid": "178.22.70.33"
                },
                "ip_v6": null
            },
            "vlan": null
        },
        {
            "boot_order": null,
            "firewall_policy": null,
            "ip_v4_conf": null,
            "ip_v6_conf": null,
            "mac": "22:9e:e7:d7:86:94",
            "model": "virtio",
            "runtime": {
                "interface_type": "private",
                "io": {
                    "bytes_recv": 0,
                    "bytes_sent": 1046,
                    "packets_recv": 0,
                    "packets_sent": 13
                },
                "ip_v4": null,
                "ip_v6": null
            },
            "vlan": {
                "resource_uri": "/api/2.0/vlans/5bc05e7e-6555-4f40-add8-3b8e91447702/",
                "uuid": "5bc05e7e-6555-4f40-add8-3b8e91447702"
            }
        }
    ],
    "owner": {
        "resource_uri": "/api/2.0/user/c25eb0ed-161f-44f4-ac1d-d584ce3a5312/",
        "uuid": "c25eb0ed-161f-44f4-ac1d-d584ce3a5312"
    },
    "requirements": [],
    "resource_uri": "/api/2.0/servers/f4ec5097-121e-44a7-a207-75bc02163260/",
    "runtime": {
        "active_since": "2014-04-24T14:56:58+00:00",
        "nics": [
            {
                "interface_type": "public",
                "io": {
                    "bytes_recv": 0,
                    "bytes_sent": 17540,
                    "packets_recv": 0,
                    "packets_sent": 256
                },
                "ip_v4": {
                    "resource_uri": "/api/2.0/ips/178.22.70.33/",
                    "uuid": "178.22.70.33"
                },
                "ip_v6": null,
                "mac": "22:ab:bf:26:e1:be"
            },
            {
                "interface_type": "private",
                "io": {
                    "bytes_recv": 0,
                    "bytes_sent": 1046,
                    "packets_recv": 0,
                    "packets_sent": 13
                },
                "ip_v4": null,
                "ip_v6": null,
                "mac": "22:9e:e7:d7:86:94"
            }
        ]
    },
    "smp": 1,
    "status": "running",
    "tags": [],
    "uuid": "f4ec5097-121e-44a7-a207-75bc02163260",
    "vnc_password": "test-vnc-password"
}`
