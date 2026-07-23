// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type linkLayerSuite struct {
	linkLayerBaseSuite
}

func TestLinkLayerSuite(t *testing.T) {
	tc.Run(t, &linkLayerSuite{})
}

func (s *linkLayerSuite) TestGetMachineNetNodeUUID(c *tc.C) {
	db := s.DB()

	// Arrange
	nodeUUID := "net-node-uuid"
	machineUUID := "machine-uuid"

	ctx := c.Context()

	_, err := db.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	q := "INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, ?)"
	_, err = db.ExecContext(ctx, q, machineUUID, "666", nodeUUID, 0)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	gotUUID, err := s.state.GetMachineNetNodeUUID(ctx, machineUUID)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, nodeUUID)
}

func (s *linkLayerSuite) TestGetMachineNetNodeUUIDNotFoundError(c *tc.C) {
	_, err := s.state.GetMachineNetNodeUUID(c.Context(), "machine-uuid")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *linkLayerSuite) TestGetAllLinkLayerDevicesByNetNodeUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	ctx := c.Context()

	// Arrange
	// Create two net nodes
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)

	// Create three link layer devices (2 for node1, 1 for node2)
	eth0UUID := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", corenetwork.EthernetDevice)
	bridgeUUID := s.addLinkLayerDevice(c, nodeUUID1, "eth0-bridge", "00:11:22:33:44:66", corenetwork.BridgeDevice)
	eth1UUID := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:77", corenetwork.EthernetDevice)
	s.setLinkLayerDeviceParent(c, bridgeUUID, eth0UUID)

	// Create DNS domains for each device
	s.addDNSDomains(c, eth0UUID, "search1.maas.net", "search2.maas.net")
	s.addDNSDomains(c, bridgeUUID, "search3.maas.net")
	s.addDNSDomains(c, eth1UUID, "search4.maas.net", "search5.maas.net")

	// Create DNS addresses for each device
	s.addDNSAddresses(c, eth0UUID, "8.8.8.8", "8.8.4.4")
	s.addDNSAddresses(c, bridgeUUID, "1.1.1.1")
	s.addDNSAddresses(c, eth1UUID, "9.9.9.9", "4.4.4.4")

	// Create subnets for IP addresses
	subnet1UUID := s.addSubnet(c, "192.168.1.0/24", corenetwork.AlphaSpaceId.String())
	subnet2UUID := s.addSubnet(c, "192.168.2.0/24", corenetwork.AlphaSpaceId.String())
	subnet3UUID := s.addSubnet(c, "192.168.3.0/24", corenetwork.AlphaSpaceId.String())

	// Create IP addresses for each device
	insertIPAddress := `
INSERT INTO ip_address (uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid, net_node_uuid) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	s.query(c, insertIPAddress, "ip-uuid-1", eth0UUID, "192.168.1.10/24", 0, 2, 0, 4, subnet1UUID, nodeUUID1)
	s.query(c, insertIPAddress, "ip-uuid-2", eth0UUID, "192.168.1.11/24", 0, 2, 0, 4, subnet1UUID, nodeUUID1)
	s.query(c, insertIPAddress, "ip-uuid-3", bridgeUUID, "192.168.2.10/24", 0, 2, 0, 4, subnet2UUID, nodeUUID1)
	s.query(c, insertIPAddress, "ip-uuid-4", eth1UUID, "192.168.3.10/24", 0, 2, 0, 4, subnet3UUID, nodeUUID2)
	s.query(c, insertIPAddress, "ip-uuid-5", eth1UUID, "192.168.3.11/24", 0, 2, 0, 4, subnet3UUID, nodeUUID2)

	// Act
	result, err := st.GetAllLinkLayerDevicesByNetNodeUUIDs(ctx)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)

	// Check that we have both nodes as keys
	c.Assert(result, tc.HasLen, 2)

	// Check node1 has 2 devices
	c.Assert(result[nodeUUID1], tc.HasLen, 2)

	// Check node2 has 1 device
	c.Assert(result[nodeUUID2], tc.HasLen, 1)

	// Check device details for node1
	var eth0, eth1, bridge network.NetInterface
	eth1 = result[nodeUUID2][0]
	for _, dev := range result[nodeUUID1] {
		if dev.Name == "eth0" {
			eth0 = dev
		} else if dev.Name == "eth0-bridge" {
			bridge = dev
		} else {
			// Unexpected device for node1
			c.Fail()
		}
	}

	// Filter not checked fields for addresses and devices
	filterNetInterface := func(dev network.NetInterface) network.NetInterface {
		dev.DNSSearchDomains = nil
		dev.DNSAddresses = nil
		dev.Addrs = nil
		dev.MTU = nil
		return network.NetInterface{
			Name:             dev.Name,
			MACAddress:       dev.MACAddress,
			Type:             dev.Type,
			ParentDeviceName: dev.ParentDeviceName,
		}
	}
	filterNetAddr := func(addrs []network.NetAddr) []network.NetAddr {
		return transform.Slice(addrs, func(in network.NetAddr) network.NetAddr {
			return network.NetAddr{
				InterfaceName: in.InterfaceName,
				AddressValue:  in.AddressValue,
				Space:         in.Space,
			}
		})
	}

	// Check eth0 details
	c.Check(filterNetInterface(eth0), tc.DeepEquals, network.NetInterface{
		Name:       "eth0",
		MACAddress: new("00:11:22:33:44:55"),
		Type:       corenetwork.EthernetDevice,
	})
	c.Check(filterNetAddr(eth0.Addrs), tc.SameContents, []network.NetAddr{{
		InterfaceName: "eth0",
		AddressValue:  "192.168.1.10/24",
		Space:         "alpha",
	}, {
		InterfaceName: "eth0",
		AddressValue:  "192.168.1.11/24",
		Space:         "alpha",
	}})
	c.Check(eth0.DNSSearchDomains, tc.SameContents, []string{"search1.maas.net", "search2.maas.net"})
	c.Check(eth0.DNSAddresses, tc.SameContents, []string{"8.8.8.8", "8.8.4.4"})

	// Check bridge details
	c.Check(filterNetInterface(bridge), tc.DeepEquals, network.NetInterface{
		Name:             "eth0-bridge",
		MACAddress:       new("00:11:22:33:44:66"),
		Type:             corenetwork.BridgeDevice,
		ParentDeviceName: "eth0",
	})
	c.Check(filterNetAddr(bridge.Addrs), tc.SameContents, []network.NetAddr{
		{
			InterfaceName: "eth0-bridge",
			AddressValue:  "192.168.2.10/24",
			Space:         "alpha",
		},
	})
	c.Check(bridge.DNSSearchDomains, tc.SameContents, []string{"search3.maas.net"})
	c.Check(bridge.DNSAddresses, tc.SameContents, []string{"1.1.1.1"})

	// Check eth1 details
	c.Check(filterNetInterface(eth1), tc.DeepEquals, network.NetInterface{
		Name:       "eth1",
		MACAddress: new("00:11:22:33:44:77"),
		Type:       corenetwork.EthernetDevice,
	})
	c.Check(filterNetAddr(eth1.Addrs), tc.SameContents, []network.NetAddr{
		{
			InterfaceName: "eth1",
			AddressValue:  "192.168.3.10/24",
			Space:         "alpha",
		},
		{
			InterfaceName: "eth1",
			AddressValue:  "192.168.3.11/24",
			Space:         "alpha",
		},
	})
	c.Check(eth1.DNSSearchDomains, tc.SameContents, []string{"search4.maas.net", "search5.maas.net"})
	c.Check(eth1.DNSAddresses, tc.SameContents, []string{"9.9.9.9", "4.4.4.4"})
}
