// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type mergeLinkLayerSuite struct {
	schematesting.ModelSuite
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

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *mergeLinkLayerSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query, args...)
			if err != nil {
				return errors.Errorf("%w: query: %s (args: %s)", err, query,
					args)
			}
			return nil
		})
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Arrange) failed to populate DB: %v",
			errors.ErrorStack(err)))
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
		toRemove:     []string{"provider-id-1"},
		toRelinquish: []string{deviceUUID},
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
		toAdd: map[string]string{
			"new-provider-eth0": eth0UUID,
		},
		toRemove: []string{
			"old-provider-eth0", "relinquished-provider-eth2",
		},
		toRelinquish: []string{eth21},
		newDevices: []mergeLinkLayerDevice{
			{
				UUID:       "new-device",
				Name:       "eth4",
				MACAddress: "gyver",
			},
		},
	}
	addressChanges := mergeAddressesChanges{
		toAdd: map[string]string{
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
	c.Check(changes.toAdd, tc.HasLen, 0)
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
	c.Check(changes.toAdd, tc.HasLen, 0)
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
	c.Check(changes.toAdd, tc.DeepEquals,
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
	c.Check(changes.toAdd, tc.DeepEquals,
		map[string]string{"new-provider-id-1": "device1-uuid"})
	c.Check(changes.toRemove, tc.SameContents, []string{"provider-id-1"})
	c.Check(changes.toRelinquish, tc.HasLen, 0)
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
	c.Check(changes.toAdd, tc.HasLen, 0)
	c.Check(changes.toRemove, tc.HasLen, 0)
	c.Check(changes.toRelinquish, tc.HasLen, 0)
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
	c.Check(changes.toAdd, tc.HasLen, 0)
	c.Check(changes.toRemove, tc.HasLen, 0)
	c.Check(changes.toRelinquish, tc.HasLen, 0)
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
	c.Check(changes.toAdd, tc.DeepEquals, map[string]string{})
	c.Check(changes.toRemove, tc.SameContents, []string{"provider-id-1"})
	c.Check(changes.toRelinquish, tc.SameContents, []string{"address1-uuid"})
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
	c.Check(changes.toAdd, tc.HasLen, 0)
	c.Check(changes.toRemove, tc.HasLen, 0)
	c.Check(changes.toRelinquish, tc.HasLen, 0)
	c.Check(changes.newDevices, tc.HasLen, 1)
	c.Check(changes.newDevices[0].Name, tc.Equals, "eth1")
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

// addProviderLinkLayerDevice adds a provider link layer device to the database.
func (s *mergeLinkLayerSuite) addProviderLinkLayerDevice(
	c *tc.C, providerID, deviceUUID string,
) {
	s.query(c, `
		INSERT INTO provider_link_layer_device (provider_id, device_uuid)
		VALUES (?, ?)
	`, providerID, deviceUUID)
}

// addIPAddress adds an IP address to the database and returns its UUID.
func (s *mergeLinkLayerSuite) addIPAddress(
	c *tc.C, deviceUUID, netNodeUUID, addressValue string,
) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id, is_secondary, is_shadow)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, nil, 0, 4, 1, 0,
		false, false)

	return addressUUID
}

// addProviderIPAddress adds a provider IP address to the database.
func (s *mergeLinkLayerSuite) addProviderIPAddress(
	c *tc.C, addressUUID, providerID string,
) {
	s.query(c, `
		INSERT INTO provider_ip_address (provider_id, address_uuid)
		VALUES (?, ?)
	`, providerID, addressUUID)
}

// createNetInterface creates a network.NetInterface for testing.
func (s *mergeLinkLayerSuite) createNetAddr(value,
	providerID string) network.NetAddr {
	provider := corenetwork.Id(providerID)
	return network.NetAddr{
		ProviderID:   &provider,
		AddressValue: value,
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
SELECT uuid, address_value, provider_id, iao.name as origin
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
				err := rows.Scan(&addr.UUID, &addr.Address,
					&providerID, &addr.Origin)
				if err != nil {
					return err
				}
				addr.ProviderID = providerID.String
				result = append(result, addr)
			}
			return nil
		})
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("(Assert) failed to fetch addresses: %q, "+
			"with netnodeuuid: %q", query, netNodeUUID))

	return result
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
}
