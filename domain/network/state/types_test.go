// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestNetInterfaceToDMLSuccess(c *gc.C) {
	dev := getNetInterface()

	dml, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{
		"eth0": "some-device-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(dml, gc.DeepEquals, linkLayerDeviceDML{
		UUID:              "some-device-uuid",
		NetNodeUUID:       "some-node-uuid",
		Name:              "eth0",
		MTU:               ptr(int64(1500)),
		MACAddress:        ptr("00:00:00:00:00:00"),
		DeviceTypeID:      2,
		VirtualPortTypeID: 1,
		IsAutoStart:       true,
		IsEnabled:         true,
		IsDefaultGateway:  true,
		GatewayAddress:    ptr("192.168.0.1"),
		VlanTag:           0,
	})
}

func (s *typesSuite) TestNetInterfaceToDMLBadDeviceTypeError(c *gc.C) {
	dev := getNetInterface()
	dev.Type = "bad-type"

	_, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{
		"eth0": "some-device-uuid",
	})
	c.Assert(err, gc.ErrorMatches, "unsupported device type.*")
}

func (s *typesSuite) TestNetInterfaceToDMLBadVirtualPortTypeError(c *gc.C) {
	dev := getNetInterface()
	dev.VirtualPortType = "bad-type"

	_, err := netInterfaceToDML(dev, "some-node-uuid", map[string]string{
		"eth0": "some-device-uuid",
	})
	c.Assert(err, gc.ErrorMatches, "unsupported virtual port type.*")
}

func getNetInterface() network.NetInterface {
	return network.NetInterface{
		Name:             "eth0",
		MTU:              ptr(int64(1500)),
		MACAddress:       ptr("00:00:00:00:00:00"),
		Type:             corenetwork.EthernetDevice,
		VirtualPortType:  corenetwork.OvsPort,
		IsAutoStart:      true,
		IsEnabled:        true,
		GatewayAddress:   ptr("192.168.0.1"),
		IsDefaultGateway: true,
		VLANTag:          0,

		// TODO (manadart 2025-05-05): Handle the translations below as
		// additional *DML types.

		ParentDeviceName: "",
		ProviderID:       nil,
		DNSSearchDomains: nil,
		DNSAddresses:     nil,
	}
}

func ptr[T any](v T) *T {
	return &v
}
