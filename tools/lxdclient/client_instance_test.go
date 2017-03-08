// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdclient"
)

type addressesSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&addressesSuite{})

type addressTester struct {
	// Stub out all the APIs so we conform to the interface,
	// we only implement the ones that we are going to be testing
	lxdclient.RawInstanceClient

	ContainerStateResult *lxdapi.ContainerState
}

func (a *addressTester) ContainerState(name string) (*lxdapi.ContainerState, error) {
	return a.ContainerStateResult, nil
}

var _ lxdclient.RawInstanceClient = (*addressTester)(nil)

// containerStateSample was captured from a real response
var containerStateSample = lxdapi.ContainerState{
	Status:     "Running",
	StatusCode: lxdapi.Running,
	Disk:       map[string]lxdapi.ContainerStateDisk{},
	Memory: lxdapi.ContainerStateMemory{
		Usage:         66486272,
		UsagePeak:     92405760,
		SwapUsage:     0,
		SwapUsagePeak: 0,
	},
	Network: map[string]lxdapi.ContainerStateNetwork{
		"eth0": lxdapi.ContainerStateNetwork{
			Addresses: []lxdapi.ContainerStateNetworkAddress{
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet",
					Address: "10.0.8.173",
					Netmask: "24",
					Scope:   "global",
				},
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet6",
					Address: "fe80::216:3eff:fe3b:e582",
					Netmask: "64",
					Scope:   "link",
				},
			},
			Counters: lxdapi.ContainerStateNetworkCounters{
				BytesReceived:   16352,
				BytesSent:       6238,
				PacketsReceived: 69,
				PacketsSent:     59,
			},
			Hwaddr:   "00:16:3e:3b:e5:82",
			HostName: "vethYIEDPS",
			Mtu:      1500,
			State:    "up",
			Type:     "broadcast",
		},
		"lo": lxdapi.ContainerStateNetwork{
			Addresses: []lxdapi.ContainerStateNetworkAddress{
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet",
					Address: "127.0.0.1",
					Netmask: "8",
					Scope:   "local",
				},
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet6",
					Address: "::1",
					Netmask: "128",
					Scope:   "local",
				},
			},
			Counters: lxdapi.ContainerStateNetworkCounters{
				BytesReceived:   0,
				BytesSent:       0,
				PacketsReceived: 0,
				PacketsSent:     0,
			},
			Hwaddr:   "",
			HostName: "",
			Mtu:      65536,
			State:    "up",
			Type:     "loopback",
		},
		"lxcbr0": lxdapi.ContainerStateNetwork{
			Addresses: []lxdapi.ContainerStateNetworkAddress{
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet",
					Address: "10.0.5.12",
					Netmask: "24",
					Scope:   "global",
				},
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet6",
					Address: "fe80::216:3eff:fe3b:e432",
					Netmask: "64",
					Scope:   "link",
				},
			},
			Counters: lxdapi.ContainerStateNetworkCounters{
				BytesReceived:   0,
				BytesSent:       500,
				PacketsReceived: 0,
				PacketsSent:     6,
			},
			Hwaddr:   "5e:9b:b2:af:4c:f2",
			HostName: "",
			Mtu:      1500,
			State:    "up",
			Type:     "broadcast",
		},
		"lxdbr0": lxdapi.ContainerStateNetwork{
			Addresses: []lxdapi.ContainerStateNetworkAddress{
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet",
					Address: "10.0.6.17",
					Netmask: "24",
					Scope:   "global",
				},
				lxdapi.ContainerStateNetworkAddress{
					Family:  "inet6",
					Address: "fe80::5c9b:b2ff:feaf:4cf2",
					Netmask: "64",
					Scope:   "link",
				},
			},
			Counters: lxdapi.ContainerStateNetworkCounters{
				BytesReceived:   0,
				BytesSent:       500,
				PacketsReceived: 0,
				PacketsSent:     6,
			},
			Hwaddr:   "52:54:00:5c:54:8f",
			HostName: "",
			Mtu:      1500,
			State:    "up",
			Type:     "broadcast",
		},
	},
	Pid:       46072,
	Processes: 19,
}

func (s *addressesSuite) TestAddresses(c *gc.C) {
	raw := &addressTester{
		ContainerStateResult: &containerStateSample,
	}
	client := lxdclient.NewInstanceClient(raw)
	addrs, err := client.Addresses("test")
	c.Assert(err, jc.ErrorIsNil)
	// We should filter out the MachineLocal addresses 127.0.0.1 and [::1]
	// and filter out the LinkLocal address [fe80::216:3eff:fe3b:e582]
	c.Check(addrs, jc.DeepEquals, []network.Address{
		{
			Value: "10.0.8.173",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
	})
}

type devicesSuite struct {
	lxdclient.BaseSuite
}

var _ = gc.Suite(&devicesSuite{})

func (s *devicesSuite) TestAttachDisk(c *gc.C) {
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.AttachDisk("instance", "device", lxdclient.DiskDevice{
		Source: "source-value",
		Path:   "path-value",
		Pool:   "pool-value",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []testing.StubCall{
		{"ContainerDeviceAdd", []interface{}{"instance", "device", "disk", []string{
			"path=path-value", "source=source-value", "pool=pool-value",
		}}},
		{"WaitForSuccess", []interface{}{""}},
	})
}

func (s *devicesSuite) TestAttachDiskReadOnly(c *gc.C) {
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.AttachDisk("instance", "device", lxdclient.DiskDevice{
		Source:   "source-value",
		Path:     "path-value",
		ReadOnly: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCall(c, 0, "ContainerDeviceAdd", "instance", "device", "disk", []string{
		"path=path-value", "source=source-value", "readonly=true",
	})
}

func (s *devicesSuite) TestAttachDiskSyncError(c *gc.C) {
	s.Stub.SetErrors(errors.New("sync error"))
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.AttachDisk("instance", "device", lxdclient.DiskDevice{})
	c.Assert(err, gc.ErrorMatches, "sync error")
}

func (s *devicesSuite) TestAttachDiskAsyncError(c *gc.C) {
	s.Stub.SetErrors(nil, errors.New("async error"))
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.AttachDisk("instance", "device", lxdclient.DiskDevice{})
	c.Assert(err, gc.ErrorMatches, "async error")
}

func (s *devicesSuite) TestRemoveDevice(c *gc.C) {
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.RemoveDevice("instance", "device")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []testing.StubCall{
		{"ContainerDeviceDelete", []interface{}{"instance", "device"}},
		{"WaitForSuccess", []interface{}{""}},
	})
}

func (s *devicesSuite) TestRemoveDeviceSyncError(c *gc.C) {
	s.Stub.SetErrors(errors.New("sync error"))
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.RemoveDevice("instance", "device")
	c.Assert(err, gc.ErrorMatches, "sync error")
}

func (s *devicesSuite) TestRemoveDeviceAsyncError(c *gc.C) {
	s.Stub.SetErrors(nil, errors.New("async error"))
	client := lxdclient.NewInstanceClient(s.Client)
	err := client.RemoveDevice("instance", "device")
	c.Assert(err, gc.ErrorMatches, "async error")
}
