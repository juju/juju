// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/uuid"
)

type linkLayerImportSuite struct {
	linkLayerBaseSuite
}

func TestLinkLayerImportSuite(t *testing.T) {
	tc.Run(t, &linkLayerImportSuite{})
}

func (s *linkLayerImportSuite) TestAllMachinesAndNetNodes(c *tc.C) {
	// Arrange
	netNodeUUID := s.addNetNode(c)
	machineName := "73"
	s.addMachine(c, machineName, netNodeUUID)
	netNodeUUID2 := s.addNetNode(c)
	machineName2 := "42"
	s.addMachine(c, machineName2, netNodeUUID2)
	expected := map[string]string{
		machineName:  netNodeUUID,
		machineName2: netNodeUUID2,
	}

	// Act
	obtained, err := s.state.AllMachinesAndNetNodes(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *linkLayerImportSuite) TestImportLinkLayerDevices(c *tc.C) {
	// Arrange:
	ctx := c.Context()

	// Arrange: prior imported items required for link layer devices.
	netNodeUUID := s.addNetNode(c)
	machineName := "73"
	s.addMachine(c, machineName, netNodeUUID)
	netNodeUUID2 := s.addNetNode(c)
	machineName2 := "42"
	s.addMachine(c, machineName2, netNodeUUID2)

	// Arrange: data to be imported, parent LLD has no MTU, MACAddress,
	// nor ProviderID to ensure null for tests.
	importData := []internal.ImportLinkLayerDevice{
		{
			UUID:             uuid.MustNewUUID().String(),
			NetNodeUUID:      netNodeUUID,
			Name:             "test",
			MTU:              ptr(int64(1500)),
			Type:             corenetwork.EthernetDevice,
			VirtualPortType:  corenetwork.NonVirtualPort,
			MachineID:        machineName,
			ParentDeviceName: "parent",
			ProviderID:       ptr("one"),
			MACAddress:       ptr("00:16:3e:ad:4e:01"),
		}, {
			UUID:            uuid.MustNewUUID().String(),
			NetNodeUUID:     netNodeUUID,
			Name:            "parent",
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.NonVirtualPort,
			MachineID:       machineName,
		}, {
			// This LLD should not be matched as the parent LLD of test.
			UUID:            uuid.MustNewUUID().String(),
			NetNodeUUID:     netNodeUUID2,
			Name:            "parent",
			MTU:             ptr(int64(4328)),
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.NonVirtualPort,
			MachineID:       machineName2,
			ProviderID:      ptr("two"),
			MACAddress:      ptr("00:16:3e:ad:4e:88"),
		},
	}
	expectedLLDRows := transformImportArgToResult(c, importData)

	// Act
	err := s.state.ImportLinkLayerDevices(ctx, importData)

	// Assert
	c.Check(err, tc.ErrorIsNil)
	s.checkRowCount(c, "link_layer_device", 3)
	s.checkRowCount(c, "link_layer_device_parent", 1)
	s.checkRowCount(c, "provider_link_layer_device", 2)
	obtainedLLDRows := s.readLinkLayerDevices(c)
	c.Check(obtainedLLDRows, tc.SameContents, expectedLLDRows)
	obtainedParentRow := s.readLinkLayerDeviceParent(c)
	c.Check(obtainedParentRow, tc.SameContents, []linkLayerDeviceParent{
		{
			DeviceUUID: importData[0].UUID,
			ParentUUID: importData[1].UUID,
		},
	})
	obtainedProviderRows := s.readProviderLinkLayerDevice(c)
	c.Check(obtainedProviderRows, tc.SameContents, []providerLinkLayerDevice{
		{
			DeviceUUID: importData[0].UUID,
			ProviderID: *importData[0].ProviderID,
		}, {
			DeviceUUID: importData[2].UUID,
			ProviderID: *importData[2].ProviderID,
		},
	})
}

func (s *linkLayerImportSuite) TestDeleteImportedRelations(c *tc.C) {
	// Arrange:
	ctx := c.Context()

	// Arrange: prior imported items required for link layer devices.
	netNodeUUID := s.addNetNode(c)
	machineName := "73"
	s.addMachine(c, machineName, netNodeUUID)

	// Arrange: import some data
	importData := []internal.ImportLinkLayerDevice{
		{
			UUID:             uuid.MustNewUUID().String(),
			NetNodeUUID:      netNodeUUID,
			Name:             "test",
			MTU:              ptr(int64(1500)),
			Type:             corenetwork.EthernetDevice,
			VirtualPortType:  corenetwork.NonVirtualPort,
			MachineID:        machineName,
			ParentDeviceName: "parent",
			ProviderID:       ptr("one"),
			MACAddress:       ptr("00:16:3e:ad:4e:01"),
		},
		{
			UUID:            uuid.MustNewUUID().String(),
			NetNodeUUID:     netNodeUUID,
			Name:            "parent",
			MTU:             ptr(int64(1500)),
			Type:            corenetwork.EthernetDevice,
			VirtualPortType: corenetwork.NonVirtualPort,
			MachineID:       machineName,
			ProviderID:      ptr("two"),
			MACAddress:      ptr("00:16:3e:ad:4e:88"),
		},
	}
	err := s.state.ImportLinkLayerDevices(ctx, importData)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	err = s.state.DeleteImportedLinkLayerDevices(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkRowCount(c, "link_layer_device", 0)
	s.checkRowCount(c, "link_layer_device_parent", 0)
	s.checkRowCount(c, "provider_link_layer_device", 0)
}

func (s *linkLayerImportSuite) readLinkLayerDevices(c *tc.C) []linkLayerDeviceDML {
	var (
		rows []linkLayerDeviceDML
		err  error
	)
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		stmt, err := s.state.Prepare(`
SELECT * AS &linkLayerDeviceDML.*
FROM link_layer_device
`, linkLayerDeviceDML{})
		if err != nil {
			return err
		}
		return tx.Query(ctx, stmt).GetAll(&rows)
	})
	if !c.Check(err, tc.ErrorIsNil) {
		return nil
	}
	return rows
}

func (s *linkLayerImportSuite) readLinkLayerDeviceParent(c *tc.C) []linkLayerDeviceParent {
	var (
		rows []linkLayerDeviceParent
		err  error
	)
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		stmt, err := s.state.Prepare(`
SELECT * AS &linkLayerDeviceParent.*
FROM link_layer_device_parent
`, linkLayerDeviceParent{})
		if err != nil {
			return err
		}
		return tx.Query(ctx, stmt).GetAll(&rows)
	})
	if !c.Check(err, tc.ErrorIsNil) {
		return nil
	}
	return rows
}

func (s *linkLayerImportSuite) readProviderLinkLayerDevice(c *tc.C) []providerLinkLayerDevice {
	var (
		rows []providerLinkLayerDevice
		err  error
	)
	err = s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		stmt, err := s.state.Prepare(`
SELECT * AS &providerLinkLayerDevice.*
FROM provider_link_layer_device
`, providerLinkLayerDevice{})
		if err != nil {
			return err
		}
		return tx.Query(ctx, stmt).GetAll(&rows)
	})
	if !c.Check(err, tc.ErrorIsNil) {
		return nil
	}
	return rows
}

func transformImportArgToResult(
	c *tc.C, importData []internal.ImportLinkLayerDevice,
) []linkLayerDeviceDML {
	return transform.Slice[internal.ImportLinkLayerDevice, linkLayerDeviceDML](importData,
		func(in internal.ImportLinkLayerDevice) linkLayerDeviceDML {
			typeID, err := encodeDeviceType(in.Type)
			c.Check(err, tc.ErrorIsNil)
			portTypeID, err := encodeVirtualPortType(in.VirtualPortType)
			c.Check(err, tc.ErrorIsNil)
			return linkLayerDeviceDML{
				UUID:              in.UUID,
				NetNodeUUID:       in.NetNodeUUID,
				Name:              in.Name,
				MTU:               in.MTU,
				MACAddress:        in.MACAddress,
				DeviceTypeID:      typeID,
				VirtualPortTypeID: portTypeID,
				IsAutoStart:       in.IsAutoStart,
				IsEnabled:         in.IsEnabled,
				IsDefaultGateway:  false,
				GatewayAddress:    nil,
				VlanTag:           0,
			}
		})

}
