// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	blockdevicemodelmigration "github.com/juju/juju/domain/blockdevice/modelmigration"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportBlockDevices(c *tc.C) {
	m0UUID, m0Name := s.addMachine(c)
	m1UUID, m1Name := s.addMachine(c)

	desc := description.NewModel(description.ModelArgs{})
	m0 := desc.AddMachine(description.MachineArgs{
		Id: m0Name.String(),
	})
	m0.AddBlockDevice(description.BlockDeviceArgs{
		Name:           "foo",
		Links:          []string{"a-link"},
		Label:          "label",
		UUID:           "device-uuid",
		HardwareID:     "hardware-id",
		WWN:            "wwn",
		BusAddress:     "bus-address",
		SerialID:       "serial-id",
		Size:           100,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/path/to/here",
	})
	m0.AddBlockDevice(description.BlockDeviceArgs{
		Name:           "bar",
		Links:          []string{"another-link"},
		Label:          "another-label",
		UUID:           "another-device-uuid",
		HardwareID:     "another-hardware-id",
		WWN:            "another-wwn",
		BusAddress:     "another-bus-address",
		SerialID:       "another-serial-id",
		Size:           200,
		FilesystemType: "xfs",
		InUse:          false,
		MountPoint:     "/another/path",
	})

	m1 := desc.AddMachine(description.MachineArgs{
		Id: m1Name.String(),
	})
	m1.AddBlockDevice(description.BlockDeviceArgs{
		Name:           "baz",
		Links:          []string{"baz-link"},
		Label:          "baz-label",
		UUID:           "baz-device-uuid",
		HardwareID:     "baz-hardware-id",
		WWN:            "baz-wwn",
		BusAddress:     "baz-bus-address",
		SerialID:       "baz-serial-id",
		Size:           300,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "/baz/path",
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	blockdevicemodelmigration.RegisterImport(coordinator, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	m0BlockDevices, err := svc.GetBlockDevicesForMachine(c.Context(), m0UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(m0BlockDevices, tc.SameContents, []coreblockdevice.BlockDevice{{
		DeviceName:      "foo",
		DeviceLinks:     []string{"a-link"},
		FilesystemLabel: "label",
		FilesystemUUID:  "device-uuid",
		HardwareId:      "hardware-id",
		WWN:             "wwn",
		BusAddress:      "bus-address",
		SerialId:        "serial-id",
		SizeMiB:         100,
		FilesystemType:  "ext4",
		InUse:           true,
		MountPoint:      "/path/to/here",
	}, {
		DeviceName:      "bar",
		DeviceLinks:     []string{"another-link"},
		FilesystemLabel: "another-label",
		FilesystemUUID:  "another-device-uuid",
		HardwareId:      "another-hardware-id",
		WWN:             "another-wwn",
		BusAddress:      "another-bus-address",
		SerialId:        "another-serial-id",
		SizeMiB:         200,
		FilesystemType:  "xfs",
		InUse:           false,
		MountPoint:      "/another/path",
	}})

	m1BlockDevices, err := svc.GetBlockDevicesForMachine(c.Context(), m1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(m1BlockDevices, tc.SameContents, []coreblockdevice.BlockDevice{{
		DeviceName:      "baz",
		DeviceLinks:     []string{"baz-link"},
		FilesystemLabel: "baz-label",
		FilesystemUUID:  "baz-device-uuid",
		HardwareId:      "baz-hardware-id",
		WWN:             "baz-wwn",
		BusAddress:      "baz-bus-address",
		SerialId:        "baz-serial-id",
		SizeMiB:         300,
		FilesystemType:  "btrfs",
		InUse:           true,
		MountPoint:      "/baz/path",
	}})
}

func (s *importSuite) addMachine(c *tc.C) (machine.UUID, machine.Name) {
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, mNames, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := machineState.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID, mNames[0]
}

func (s *importSuite) setupService(c *tc.C) *service.Service {
	return service.NewService(
		state.NewState(s.TxnRunnerFactory()),
		loggertesting.WrapCheckLog(c),
	)
}
