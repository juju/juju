// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type mergeLinkLayerSuite struct {
	linkLayerBaseSuite
}

func TestMergeLinkLayerSuite(t *testing.T) {
	tc.Run(t, &mergeLinkLayerSuite{})
}

func (s *mergeLinkLayerSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
}

// Txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *mergeLinkLayerSuite) Txn(
	c *tc.C, state *State, fn func(ctx context.Context, tx *sqlair.TX) error,
) error {
	db, err := state.DB()
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// State returns a new State for testing.
func (s *mergeLinkLayerSuite) State(c *tc.C) *State {
	return NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

// TestMergeLinkLayerDeviceNoExistingDevices tests the case where there are no
// existing devices for the machine.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceNoExistingDevices(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create non empty incoming devices (placeholder)
	incoming := []network.NetInterface{{}}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID,
		incoming)

	// Asser: Expect no error, but no changes either (noop)
	c.Assert(err, tc.ErrorIsNil)
}

// TestMergeLinkLayerDeviceIncomingProviderIDDuplicated verifies that merging
// incoming devices with duplicated provider IDs results in an appropriate error.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceIncomingProviderIDDuplicated(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create two existing devices
	device1UUID := s.addLinkLayerDevice(
		c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice,
	)
	device2UUID := s.addLinkLayerDevice(
		c, netNodeUUID, "eth1",
		"00:11:22:33:44:66", corenetwork.EthernetDevice,
	)

	// Add provider IDs to the devices
	s.addProviderLinkLayerDevice(c, "provider-id-1", device1UUID)
	s.addProviderLinkLayerDevice(c, "provider-id-2", device2UUID)

	// Create incoming devices with updated the same provider id
	incoming := []network.NetInterface{
		s.createNetInterface(
			"eth0", "00:11:22:33:44:55", "new-provider-id",
			[]network.NetAddr{},
		),
		s.createNetInterface(
			"eth1", "00:11:22:33:44:66", "new-provider-id",
			[]network.NetAddr{},
		),
	}

	// Act
	err := st.MergeLinkLayerDevice(
		c.Context(), netNodeUUID,
		incoming,
	)

	// Assert
	c.Assert(err, tc.ErrorMatches, "unable to set provider IDs .*new-provider-id.* for multiple devices")
}

// TestMergeLinkLayerDeviceBridgeAndEthernet verifies the merging behavior of
// link-layer devices, specifically bridge and Ethernet types with same MAC
// address.
// It ensures that incoming device details are applied only to the ethernet
// device if no names are provided.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceBridgeAndEthernet(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create two existing devices
	macAddress := "00:11:22:33:44:55"
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0", macAddress, corenetwork.EthernetDevice)
	bridgeUUID := s.addLinkLayerDevice(c, netNodeUUID, "bridge", macAddress, corenetwork.BridgeDevice)

	// Create incoming devices with updated the same provider id
	incoming := []network.NetInterface{
		s.createNetInterface(
			"", macAddress, "new-provider-id",
			[]network.NetAddr{},
		),
	}

	// Act
	err := st.MergeLinkLayerDevice(
		c.Context(), netNodeUUID,
		incoming,
	)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchLinkLayerDevices(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerDevice{
			{
				UUID:       deviceUUID,
				Name:       "eth0",
				ProviderID: "new-provider-id",
				MacAddress: macAddress,
			},
			{
				UUID:       bridgeUUID,
				Name:       "bridge",
				MacAddress: macAddress,
			},
		})
}

// TestMergeLinkLayerDevice tests the case where one device is updated and
// one is untouched.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDevice(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create two existing devices
	device1UUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)
	device2UUID := s.addLinkLayerDevice(c, netNodeUUID, "eth1",
		"00:11:22:33:44:66", corenetwork.EthernetDevice)
	toRelinquishUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth2",
		"00:11:22:33:44:77", corenetwork.EthernetDevice)

	// Add provider IDs to the devices
	s.addProviderLinkLayerDevice(c, "old-provider-id-1", device1UUID)
	s.addProviderLinkLayerDevice(c, "provider-id-2", device2UUID)
	s.addProviderLinkLayerDevice(c, "provider-id-3", toRelinquishUUID)

	// Add Ips addresses
	eth01 := s.addIPAddress(c, device1UUID, netNodeUUID, "192.168.1.1/24")
	eth11 := s.addIPAddress(c, device2UUID, netNodeUUID, "100.168.1.1/24")
	eth21 := s.addIPAddress(c, toRelinquishUUID, netNodeUUID, "10.168.1.1/24")

	s.addProviderIPAddress(c, eth01, "provider-ip-1")
	s.addProviderIPAddress(c, eth11, "old-provider-ip-2")
	s.addProviderIPAddress(c, eth21, "old-provider-ip-3")

	// Create incoming devices with updated provider ID for eth0
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "new-provider-id-1",
			[]network.NetAddr{
				s.createNetAddr("192.168.1.1/24", "provider-ip-1"),
			}),
		s.createNetInterface("eth1", "00:11:22:33:44:66", "provider-id-2",
			[]network.NetAddr{
				s.createNetAddr("100.168.1.1/24", "new-provider-ip-2"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID,
		incoming)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchLinkLayerDevices(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerDevice{
			{
				UUID:       device1UUID,
				Name:       "eth0",
				ProviderID: "new-provider-id-1",
				MacAddress: "00:11:22:33:44:55",
			},
			{
				UUID:       device2UUID,
				Name:       "eth1",
				ProviderID: "provider-id-2",
				MacAddress: "00:11:22:33:44:66",
			},
			{
				UUID:       toRelinquishUUID,
				Name:       "eth2",
				ProviderID: "",
				MacAddress: "00:11:22:33:44:77",
			},
		})
	c.Check(s.fetchLinkLayerAddresses(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerAddress{{
			UUID:       eth01,
			Address:    "192.168.1.1/24",
			ProviderID: "provider-ip-1",
			Origin:     "provider",
		}, {
			UUID:       eth11,
			Address:    "100.168.1.1/24",
			ProviderID: "new-provider-ip-2",
			Origin:     "provider",
		}, {
			UUID:       eth21,
			Address:    "10.168.1.1/24",
			ProviderID: "",
			Origin:     "machine",
		}})
}

// testNoAddressToRelinquish tests the case where a provider without
// addresses is put to relinquish.
func (s *mergeLinkLayerSuite) TestApplyLinkLayerChangesNoAddressToRelinquish(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device with an address
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Add provider IDs to the device and address
	s.addProviderLinkLayerDevice(c, "provider-id-1", deviceUUID)

	lldChanges := mergeLinkLayerDevicesChanges{
		deviceToRelinquish: []string{deviceUUID},
	}
	addressChanges := mergeAddressesChanges{}

	// Act
	err := s.Txn(c, st, func(ctx context.Context, tx *sqlair.TX) error {
		return st.applyMergeLinkLayerChanges(ctx, tx, lldChanges,
			addressChanges)
	})

	// Assert: Verify that the provider ID has been removed
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchLinkLayerDevices(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerDevice{
			{
				UUID:       deviceUUID,
				Name:       "eth0",
				ProviderID: "",
				MacAddress: "00:11:22:33:44:55",
			},
		})
}

// TestApplyLinkLayerChanges tests the general case for applying
// linkLayerchanges
func (s *mergeLinkLayerSuite) TestApplyLinkLayerChanges(c *tc.C) {
	// Arrange
	st := s.State(c)
	netNodeUUID := s.addNetNode(c)

	// Create devices:

	eth0UUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)
	eth1UUID := s.addLinkLayerDevice(c, netNodeUUID, "eth1",
		"00:11:22:33:44:66", corenetwork.EthernetDevice)
	eth2UUID := s.addLinkLayerDevice(c, netNodeUUID, "eth2",
		"00:11:22:33:44:77", corenetwork.EthernetDevice)

	// Add provider IDs to the device and address
	// eth0 will have its provider id updated
	// eth1 will stay the same
	// eth2 will be relinquished
	s.addProviderLinkLayerDevice(c, "old-provider-eth0", eth0UUID)
	s.addProviderLinkLayerDevice(c, "provider-eth1", eth1UUID)
	s.addProviderLinkLayerDevice(c, "relinquished-provider-eth2", eth2UUID)

	// Create addresses for each devices:
	// eth0 with two addresses, one will have its provider updated
	// eth1 with two addresses, one will be relinquished
	// eth2 with one addresses, one will be removed (reliquished)
	eth01 := s.addIPAddress(c, eth0UUID, netNodeUUID, "192.168.1.1/24")
	eth02 := s.addIPAddress(c, eth0UUID, netNodeUUID, "192.168.2.1/24")
	eth11 := s.addIPAddress(c, eth1UUID, netNodeUUID, "100.168.1.1/24")
	eth12 := s.addIPAddress(c, eth1UUID, netNodeUUID, "100.168.2.1/24")
	eth21 := s.addIPAddress(c, eth2UUID, netNodeUUID, "10.168.2.1/24")

	s.addProviderIPAddress(c, eth01, "old-eth0-ip-1")
	s.addProviderIPAddress(c, eth02, "eth0-ip-2")
	s.addProviderIPAddress(c, eth11, "old-eth1-ip-1")
	s.addProviderIPAddress(c, eth12, "eth1-ip-2")
	s.addProviderIPAddress(c, eth21, "eth2-ip-1")

	lldChanges := mergeLinkLayerDevicesChanges{
		toAddOrUpdate: map[string]string{
			"new-provider-eth0": eth0UUID,
		},
		deviceToRelinquish:  []string{eth2UUID},
		addressToRelinquish: []string{eth21},
		newDevices: []mergeLinkLayerDevice{
			{
				UUID:       "new-device",
				Name:       "eth4",
				MACAddress: "gyver",
			},
		},
	}
	addressChanges := mergeAddressesChanges{
		providerIDsToAddOrUpdate: map[string]string{
			"new-eth0-ip-1": eth01,
			"new-eth1-ip-1": eth11,
		},
		toRelinquish: []string{eth12},
	}

	// Act
	err := s.Txn(c, st, func(ctx context.Context, tx *sqlair.TX) error {
		return st.applyMergeLinkLayerChanges(ctx, tx, lldChanges,
			addressChanges)
	})

	// Assert: Verify that the provider ID has been removed
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.fetchLinkLayerDevices(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerDevice{
			{
				UUID:       eth0UUID,
				Name:       "eth0",
				MacAddress: "00:11:22:33:44:55",
				ProviderID: "new-provider-eth0",
			},
			{
				UUID:       eth1UUID,
				Name:       "eth1",
				MacAddress: "00:11:22:33:44:66",
				ProviderID: "provider-eth1",
			},
			{
				UUID:       eth2UUID,
				Name:       "eth2",
				MacAddress: "00:11:22:33:44:77",
				ProviderID: "",
			},
		})
	c.Check(s.fetchLinkLayerAddresses(c, netNodeUUID), tc.SameContents,
		[]mergedLinkLayerAddress{
			{
				UUID:       eth01,
				Address:    "192.168.1.1/24",
				ProviderID: "new-eth0-ip-1",
				Origin:     "provider",
			},
			{
				UUID:       eth02,
				Address:    "192.168.2.1/24",
				ProviderID: "eth0-ip-2",
				Origin:     "provider",
			},
			{
				UUID:       eth11,
				Address:    "100.168.1.1/24",
				ProviderID: "new-eth1-ip-1",
				Origin:     "provider",
			},
			{
				UUID:       eth12,
				Address:    "100.168.2.1/24",
				ProviderID: "",
				Origin:     "machine",
			},
			{
				UUID:       eth21,
				Address:    "10.168.2.1/24",
				ProviderID: "",
				Origin:     "machine",
			},
		})
}

// TestComputeMergeAddressChangesNotToBeUpdated tests the case where some
// addresses  are not to be updated.
func (s *mergeLinkLayerSuite) TestComputeMergeAddressChangesNotToBeUpdated(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices with addresses
	existingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
			},
		},
	}

	// Create incoming devices with the same addresses
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
			},
		},
	}

	// Act
	changes := st.computeMergeAddressChanges(incomingDevices, existingDevices)

	// Assert: Verify that no changes are made
	c.Check(changes.providerIDsToAddOrUpdate, tc.HasLen, 0)
	c.Check(changes.toRelinquish, tc.HasLen, 0)
}

// TestComputeMergeAddressChangesToBeRelinquished tests the case where some addresses
// are to be relinquished.
func (s *mergeLinkLayerSuite) TestComputeMergeAddressChangesToBeRelinquished(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices with addresses
	existingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					UUID:       "address1-uuid",
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
				{
					UUID:       "no-matching-uuid",
					Value:      "192.168.1.2/24",
					ProviderID: "no-matching-provider-id",
				},
			},
		},
	}

	// Create incoming devices with only one of the addresses
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
			},
		},
	}

	// Act
	changes := st.computeMergeAddressChanges(incomingDevices, existingDevices)

	// Assert: Verify that the second address is relinquished
	c.Check(changes.providerIDsToAddOrUpdate, tc.HasLen, 0)
	c.Check(changes.toRelinquish, tc.SameContents,
		[]string{"no-matching-uuid"})
}

// TestComputeMergeAddressChangesProviderIDUpdated tests the case where some
// addresses have their provider ID updated.
func (s *mergeLinkLayerSuite) TestComputeMergeAddressChangesProviderIDUpdated(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices with addresses
	existingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					UUID:       "address1-uuid",
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
			},
		},
	}

	// Create incoming devices with updated provider ID for the address
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name: "eth0",
			Addresses: []mergeAddress{
				{
					Value:      "192.168.1.1/24",
					ProviderID: "new-provider-ip-1",
				},
			},
		},
	}

	// Act
	changes := st.computeMergeAddressChanges(incomingDevices, existingDevices)

	// Assert: Verify that the address provider ID is updated
	c.Check(changes.providerIDsToAddOrUpdate, tc.DeepEquals,
		map[string]string{"new-provider-ip-1": "address1-uuid"})
	c.Check(changes.toRelinquish, tc.HasLen, 0)
}

// TestComputeMergeLLDChangesWithMatchingNameDifferentProviderID
// tests the case where a device has a matching name but different provider ID.
func (s *mergeLinkLayerSuite) TestComputeMergeLLDChangesWithMatchingNameDifferentProviderID(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices
	existingDevices := []mergeLinkLayerDevice{
		{
			UUID:       "device1-uuid",
			Name:       "eth0",
			ProviderID: "provider-id-1",
		},
	}

	// Create incoming devices with the same name but different provider ID
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name:       "eth0",
			ProviderID: "new-provider-id-1",
		},
	}

	// Create nameless hardware addresses
	namelessHWAddrs := set.NewStrings()

	// Act
	ctx := c.Context()
	changes := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices,
		incomingDevices, namelessHWAddrs)

	// Assert: Verify that the provider ID is updated
	c.Check(changes.toAddOrUpdate, tc.DeepEquals,
		map[string]string{"new-provider-id-1": "device1-uuid"})
	c.Check(changes.deviceToRelinquish, tc.HasLen, 0)
	c.Check(changes.addressToRelinquish, tc.HasLen, 0)
	c.Check(changes.newDevices, tc.HasLen, 0)
}

// TestComputeMergeLLDChangesWithMatchingNameSameProviderID tests the case
// where a device has a matching name and same provider ID.
func (s *mergeLinkLayerSuite) TestComputeMergeLLDChangesWithMatchingNameSameProviderID(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices
	existingDevices := []mergeLinkLayerDevice{
		{
			UUID:       "device1-uuid",
			Name:       "eth0",
			ProviderID: "provider-id-1",
		},
	}

	// Create incoming devices with the same name and same provider ID
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name:       "eth0",
			ProviderID: "provider-id-1",
		},
	}

	// Create nameless hardware addresses
	namelessHWAddrs := set.NewStrings()

	// Act
	ctx := c.Context()
	changes := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices,
		incomingDevices, namelessHWAddrs)

	// Assert: Verify that no changes are made
	c.Check(changes.toAddOrUpdate, tc.HasLen, 0)
	c.Check(changes.deviceToRelinquish, tc.HasLen, 0)
	c.Check(changes.addressToRelinquish, tc.HasLen, 0)
	c.Check(changes.newDevices, tc.HasLen, 0)
}

// TestComputeMergeLLDChangesWithNoMatchingNameMatchingHWAddr tests
// the case where a device has no matching name but matching hardware address.
func (s *mergeLinkLayerSuite) TestComputeMergeLLDChangesWithNoMatchingNameMatchingHWAddr(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices
	existingDevices := []mergeLinkLayerDevice{
		{
			UUID:       "device1-uuid",
			Name:       "eth0",
			MACAddress: "00:11:22:33:44:55",
			ProviderID: "provider-id-1",
		},
	}

	// Create incoming devices with a different name
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name:       "eth1",
			MACAddress: "00:11:22:33:44:55",
			ProviderID: "provider-id-2",
		},
	}

	// Create nameless hardware addresses with the matching hardware address
	namelessHWAddrs := set.NewStrings("00:11:22:33:44:55")

	// Act
	ctx := c.Context()
	changes := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices,
		incomingDevices, namelessHWAddrs)

	// Assert: Verify that the device is not relinquished
	c.Check(changes.toAddOrUpdate, tc.HasLen, 0)
	c.Check(changes.deviceToRelinquish, tc.HasLen, 0)
	c.Check(changes.addressToRelinquish, tc.HasLen, 0)
	c.Check(changes.newDevices, tc.HasLen, 1)
	c.Check(changes.newDevices[0].Name, tc.Equals, "eth1")
}

// TestComputeMergeLLDChangesWithNoMatchingNameNoMatchingHWAddr tests
// the case where a device has no matching name and no matching hardware address.
func (s *mergeLinkLayerSuite) TestComputeMergeLLDChangesWithNoMatchingNameNoMatchingHWAddr(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices
	existingDevices := []mergeLinkLayerDevice{
		{
			UUID:       "device1-uuid",
			Name:       "eth0",
			ProviderID: "provider-id-1",
			Addresses: []mergeAddress{
				{
					UUID:       "address1-uuid",
					Value:      "192.168.1.1/24",
					ProviderID: "provider-ip-1",
				},
			},
		},
	}

	// Create incoming devices with a different name and different hardware address
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name:       "eth1",
			MACAddress: "00:11:22:33:44:66",
			ProviderID: "provider-id-2",
		},
	}

	// Create nameless hardware addresses
	namelessHWAddrs := set.NewStrings()

	// Act
	ctx := c.Context()
	changes := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices,
		incomingDevices, namelessHWAddrs)

	// Assert: Verify that the device is relinquished
	c.Check(changes.toAddOrUpdate, tc.DeepEquals, map[string]string{})
	c.Check(changes.deviceToRelinquish, tc.SameContents, []string{"device1-uuid"})
	c.Check(changes.addressToRelinquish, tc.SameContents, []string{"address1-uuid"})
	c.Check(changes.newDevices, tc.HasLen, 1)
	c.Check(changes.newDevices[0].Name, tc.Equals, "eth1")
}

// TestComputeMergeLLDChangesIncomingWithNoMatchingExisting tests the case
// where an incoming device has no matching existing device.
func (s *mergeLinkLayerSuite) TestComputeMergeLLDChangesIncomingWithNoMatchingExisting(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create existing devices
	existingDevices := []mergeLinkLayerDevice{
		{
			UUID:       "device1-uuid",
			Name:       "eth0",
			ProviderID: "provider-id-1",
		},
	}

	// Create incoming devices with a different name and different hardware address
	incomingDevices := []mergeLinkLayerDevice{
		{
			Name:       "eth0",
			ProviderID: "provider-id-1",
		},
		{
			Name:       "eth1",
			MACAddress: "00:11:22:33:44:66",
			ProviderID: "provider-id-2",
		},
	}

	// Create nameless hardware addresses
	namelessHWAddrs := set.NewStrings()

	// Act
	ctx := c.Context()
	changes := st.computeMergeLinkLayerDeviceChanges(ctx, existingDevices,
		incomingDevices, namelessHWAddrs)

	// Assert: Verify that the new device is added to newDevices
	c.Check(changes.toAddOrUpdate, tc.HasLen, 0)
	c.Check(changes.deviceToRelinquish, tc.HasLen, 0)
	c.Check(changes.addressToRelinquish, tc.HasLen, 0)
	c.Check(changes.newDevices, tc.HasLen, 1)
	c.Check(changes.newDevices[0].Name, tc.Equals, "eth1")
}

// TestMergeLinkLayerDeviceProviderSubnetIDMatching tests the case where an IP address
// with a provider subnet ID is merged, and the subnet_uuid is updated to match
// the corresponding subnet.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceProviderSubnetIDMatching(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create subnets with provider subnet IDs
	subnet1UUID := s.addSubnet(c, "192.168.1.0/24")
	s.addProviderSubnet(c, "provider-subnet-1", subnet1UUID)

	subnet2UUID := s.addSubnet(c, "10.0.0.0/24")
	s.addProviderSubnet(c, "provider-subnet-2", subnet2UUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address with no subnet
	s.addIPAddress(c, deviceUUID, netNodeUUID, "192.168.1.5/24")

	// Create incoming device with address that has provider subnet ID
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "",
			[]network.NetAddr{
				s.createNetAddrWithSubnet("192.168.1.5/24", "provider-address-1", "provider-subnet-1"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database now has subnet_uuid = "subnet-1"
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet1UUID)
}

// TestMergeLinkLayerDeviceProviderSubnetIDMatchingWithPreviousSubnet verifies
// that the proper subnet is updated when an incoming device has an IP linked
// to a new provider subnet ID. It ensures proper reassignment without affecting
// the previous subnet unnecessarily.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceProviderSubnetIDMatchingWithPreviousSubnet(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create subnets with provider subnet IDs
	subnet1UUID := s.addSubnet(c, "192.168.1.5/32")
	s.addProviderSubnet(c, "provider-subnet-1", subnet1UUID)

	subnet2UUID := s.addSubnet(c, "192.168.1.0/24")
	s.addProviderSubnet(c, "provider-subnet-2", subnet2UUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address on subnet 1
	s.addIPAddressWithSubnet(c, deviceUUID, netNodeUUID, subnet1UUID, "192.168.1.5/24")

	// Create incoming device with address that has provider subnet ID
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "",
			[]network.NetAddr{
				s.createNetAddrWithSubnet("192.168.1.5/24", "provider-address-1", "provider-subnet-2"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database now has subnet_uuid = "subnet-2"
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet2UUID)
	// Verify that the first subnet has not been cleaned up (it has a provider ID)
	c.Check(s.findUUIDInTable(c, "subnet", subnet1UUID), tc.IsTrue)
}

// TestMergeLinkLayerDeviceProviderSubnetIDMatchingWithPreviousPlaceholderSubnet
// verifies that a placeholder subnet with a single IP address is replaced by a
// provider's known subnet, if the IP address matches the provider subnet ID.
// It also check that the placeholder subnet is properly cleaned up after the
// merge.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceProviderSubnetIDMatchingWithPreviousPlaceholderSubnet(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create subnets with provider subnet IDs
	subnet1UUID := s.addSubnet(c, "192.168.1.5/32") // placeholder: no provider_id

	subnet2UUID := s.addSubnet(c, "192.168.1.0/24")
	s.addProviderSubnet(c, "provider-subnet-2", subnet2UUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address on subnet 1
	s.addIPAddressWithSubnet(c, deviceUUID, netNodeUUID, subnet1UUID, "192.168.1.5/24")

	// Create incoming device with address that has provider subnet ID
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "",
			[]network.NetAddr{
				s.createNetAddrWithSubnet("192.168.1.5/24", "provider-address-1", "provider-subnet-2"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database now has subnet_uuid = "subnet-2"
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet2UUID)
	// Verify that the first subnet has been cleaned up (it has a no provider ID and only one address)
	c.Check(s.findUUIDInTable(c, "subnet", subnet1UUID), tc.IsFalse)
}

// TestMergeLinkLayerDeviceNoSubnet tests the case where an IP address
// without a subnet is merged, the subnet shouldn't be rematch
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceNoSubnet(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a subnet with /32 CIDR in the alpha space
	subnet32UUID := s.addSubnet(c, "192.168.1.5/32")

	// Create a subnet with /24 CIDR in the alpha space that does match the IP
	_ = s.addSubnet(c, "192.168.1.0/24")

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address with the /32 subnet
	s.addIPAddressWithSubnet(c, deviceUUID, netNodeUUID, subnet32UUID, "192.168.1.5/24")

	// Create incoming device with address
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "provider-device-1",
			[]network.NetAddr{
				s.createNetAddr("192.168.1.5/24", ""),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database still has subnet_uuid = subnet32UUID (no change)
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet32UUID)
}

// TestMergeLinkLayerDeviceSubnetNotInAlphaSpace tests the case where an IP address
// with a subnet not in the alpha space is merged.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceSubnetNotInAlphaSpace(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a custom space
	customSpaceUUID := s.addSpace(c, "custom-space")

	// Create a subnet with /32 CIDR in the custom space
	subnet32UUID := s.addSubnetWithSpaceUUID(c, "192.168.1.5/32", customSpaceUUID)

	// Create a subnet with /24 CIDR in the custom space
	_ = s.addSubnetWithSpaceUUID(c, "192.168.1.0/24", customSpaceUUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address with the /32 subnet
	s.addIPAddressWithSubnet(c, deviceUUID, netNodeUUID, subnet32UUID, "192.168.1.5/24")

	// Create incoming device with address
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "provider-device-1",
			[]network.NetAddr{
				s.createNetAddr("192.168.1.5/24", ""),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database still has subnet_uuid = subnet32UUID
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet32UUID)
}

// TestMergeLinkLayerDeviceProviderSubnetIDNotFound tests the case where an IP address
// with a provider subnet ID that doesn't exist is merged.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceProviderSubnetIDNotFound(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a subnet with provider subnet ID
	subnet1UUID := s.addSubnet(c, "192.168.1.0/24")
	s.addProviderSubnet(c, "provider-subnet-1", subnet1UUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address with no subnet
	s.addIPAddress(c, deviceUUID, netNodeUUID, "192.168.1.5/24")

	// Create incoming device with address that has a non-existent provider subnet ID
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "provider-device-1",
			[]network.NetAddr{
				s.createNetAddrWithSubnet("192.168.1.5/24", "", "provider-subnet-unknown"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database still has subnet_uuid = NULL (no change)
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, "")
}

// TestMergeLinkLayerDeviceAddressAlreadyHasCorrectSubnet tests the case where an IP address
// already has the correct subnet.
func (s *mergeLinkLayerSuite) TestMergeLinkLayerDeviceAddressAlreadyHasCorrectSubnet(c *tc.C) {
	// Arrange
	st := s.State(c)

	// Create a subnet
	subnet1UUID := s.addSubnet(c, "192.168.1.0/24")
	s.addProviderSubnet(c, "provider-subnet-1", subnet1UUID)

	// Create a net node
	netNodeUUID := s.addNetNode(c)

	// Create a device
	deviceUUID := s.addLinkLayerDevice(c, netNodeUUID, "eth0",
		"00:11:22:33:44:55", corenetwork.EthernetDevice)

	// Create an IP address with the correct subnet
	s.addIPAddressWithSubnet(c, deviceUUID, netNodeUUID, subnet1UUID, "192.168.1.5")

	// Create incoming device with address
	incoming := []network.NetInterface{
		s.createNetInterface("eth0", "00:11:22:33:44:55", "provider-device-1",
			[]network.NetAddr{
				s.createNetAddrWithSubnet("192.168.1.5", "", "provider-subnet-1"),
			}),
	}

	// Act
	err := st.MergeLinkLayerDevice(c.Context(), netNodeUUID, incoming)

	// Assert
	c.Check(err, tc.IsNil)

	// Verify that the IP address in the database still has subnet_uuid = subnet1UUID (no change)
	addresses := s.fetchLinkLayerAddresses(c, netNodeUUID)
	c.Check(addresses, tc.HasLen, 1)
	c.Check(addresses[0].SubnetUUID, tc.Equals, subnet1UUID)
}

// helpers

// addNetNode adds a net_node to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addNetNode(c *tc.C) string {
	nodeUUID := uuid.MustNewUUID().String()
	s.query(c, `
			INSERT INTO net_node (uuid)
			VALUES (?)`, nodeUUID)
	return nodeUUID
}

// addLinkLayerDevice adds a link layer device to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addLinkLayerDevice(
	c *tc.C, netNodeUUID, name, macAddress string,
	deviceType corenetwork.LinkLayerDeviceType,
) string {
	deviceUUID := "device-" + name + "-uuid"

	deviceTypeID, err := encodeDeviceType(deviceType)
	c.Assert(err, tc.ErrorIsNil)

	mtu := int64(1500)

	s.query(c, `
		INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id, is_auto_start, is_enabled, is_default_gateway, gateway_address, vlan_tag)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, deviceUUID, netNodeUUID, name, mtu, macAddress, deviceTypeID, 0, true,
		true, false, nil, 0)

	return deviceUUID
}

// addProviderSubnet adds a provider subnet to the database.
func (s *mergeLinkLayerSuite) addProviderSubnet(
	c *tc.C, providerID, subnetUUID string,
) {
	s.query(c, `
		INSERT INTO provider_subnet (provider_id, subnet_uuid)
		VALUES (?, ?)`, providerID, subnetUUID)
}

// addIPAddressWithSubnet adds an IP address to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addIPAddressWithSubnet(c *tc.C, deviceUUID, netNodeUUID,
	subnetUUID, addressValue string) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id, is_secondary, is_shadow)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, subnetUUID, 0, 4, 1, 0,
		false, false)

	return addressUUID
}

// addSpace adds a space to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addSpace(c *tc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.query(c, `
		INSERT INTO space (uuid, name)
		VALUES (?, ?)`, spaceUUID, name)

	return spaceUUID
}

// addSubnet adds a subnet to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addSubnet(
	c *tc.C, cidr string,
) string {
	return s.addSubnetWithSpaceUUID(c, cidr, corenetwork.AlphaSpaceId.String())
}

// addSubnetWithSpace adds a subnet to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addSubnetWithSpaceUUID(
	c *tc.C, cidr, spaceUUID string,
) string {
	subnetUUID := "subnet-" + cidr + "-uuid"
	s.query(c, `
		INSERT INTO subnet (uuid, cidr, space_uuid)
		VALUES (?, ?, ?)`, subnetUUID, cidr, spaceUUID)
	return subnetUUID
}

// createNetAddr creates a network.NetAddr for testing.
func (s *mergeLinkLayerSuite) createNetAddr(value,
	providerID string) network.NetAddr {
	provider := corenetwork.Id(providerID)
	return network.NetAddr{
		ProviderID:   &provider,
		AddressValue: value,
	}
}

// createNetAddrWithSubnet creates a network.NetAddr with a provider subnet ID for testing.
func (s *mergeLinkLayerSuite) createNetAddrWithSubnet(value, providerID, providerSubnetID string) network.NetAddr {
	provider := corenetwork.Id(providerID)
	providerSubnet := corenetwork.Id(providerSubnetID)
	return network.NetAddr{
		ProviderID:       &provider,
		AddressValue:     value,
		ProviderSubnetID: &providerSubnet,
	}
}

// createNetInterface creates a network.NetInterface for testing.
func (s *mergeLinkLayerSuite) createNetInterface(
	name, macAddress, providerID string, addresses []network.NetAddr,
) network.NetInterface {
	macPtr := &macAddress
	var provIDPtr *corenetwork.Id
	if providerID != "" {
		id := corenetwork.Id(providerID)
		provIDPtr = &id
	}

	return network.NetInterface{
		Name:       name,
		MACAddress: macPtr,
		ProviderID: provIDPtr,
		Type:       corenetwork.EthernetDevice,
		Addrs:      addresses,
	}
}

// fetchLinkLayerDevices fetches link layer devices for a given net node UUID.
// It queries the database to retrieve device attributes like uuid, name, MAC
// address, and an optional provider ID. The function runs within a transaction
// and returns the results as a list of mergedLinkLayerDevice structs.
func (s *mergeLinkLayerSuite) fetchLinkLayerDevices(
	c *tc.C, netNodeUUID string,
) []mergedLinkLayerDevice {
	var result []mergedLinkLayerDevice
	query := `
SELECT uuid, name, mac_address, provider_id
FROM link_layer_device AS lld
LEFT JOIN provider_link_layer_device AS plld ON lld.uuid = plld.device_uuid
WHERE lld.net_node_uuid = ?
`
	err := s.TxnRunner().StdTxn(c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, query, netNodeUUID)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var lld mergedLinkLayerDevice
				var providerID sql.NullString
				err := rows.Scan(&lld.UUID, &lld.Name, &lld.MacAddress,
					&providerID)
				if err != nil {
					return err
				}
				lld.ProviderID = providerID.String
				result = append(result, lld)
			}
			return nil
		})
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Assert) failed to fetch linkLayerDevices: %q, "+
			"with netnodeuuid: %q", query, netNodeUUID))

	return result
}
func (s *mergeLinkLayerSuite) fetchLinkLayerAddresses(
	c *tc.C, netNodeUUID string,
) []mergedLinkLayerAddress {
	var result []mergedLinkLayerAddress

	query := `
SELECT uuid, address_value, provider_id, iao.name as origin, subnet_uuid
FROM ip_address AS ia
LEFT JOIN provider_ip_address AS pia ON ia.uuid = pia.address_uuid
JOIN ip_address_origin AS iao ON ia.origin_id = iao.id
WHERE ia.net_node_uuid = ?
`
	err := s.TxnRunner().StdTxn(c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, query, netNodeUUID)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var addr mergedLinkLayerAddress
				var providerID sql.NullString
				var subnetUUID sql.NullString
				err := rows.Scan(&addr.UUID, &addr.Address,
					&providerID, &addr.Origin, &subnetUUID)
				if err != nil {
					return err
				}
				addr.ProviderID = providerID.String
				addr.SubnetUUID = subnetUUID.String
				result = append(result, addr)
			}
			return nil
		})
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Assert) failed to fetch addresses: %q, "+
			"with netnodeuuid: %q", query, netNodeUUID))

	return result
}

func (s *mergeLinkLayerSuite) findUUIDInTable(c *tc.C, table, uuid string) bool {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE uuid = ?`, table)
	var count int
	err := s.TxnRunner().StdTxn(c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, query, uuid).Scan(&count)
		})
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Assert) failed to check uuid %q exists in table %q, query:%s", uuid, table, query))
	return count > 0
}

// mergedLinkLayerDevice represents a link layer device with additional data.
type mergedLinkLayerDevice struct {
	UUID       string
	Name       string
	MacAddress string
	ProviderID string
}

// mergedLinkLayerAddress represents an IP address with additional data.
type mergedLinkLayerAddress struct {
	UUID       string
	Address    string
	ProviderID string
	Origin     string
	SubnetUUID string
}
