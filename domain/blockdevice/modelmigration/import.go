// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"slices"

	"github.com/juju/description/v11"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ImportService provides a subset of the block device domain
// service methods needed for block device import.
type ImportService interface {
	// SetBlockDevicesForMachineByName overrides all current block devices on
	// the named machine.
	SetBlockDevicesForMachineByName(
		ctx context.Context, machineName machine.Name,
		devices []blockdevice.BlockDevice,
	) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import block devices"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	i.service = service.NewService(
		state.NewState(scope.ModelDB()), i.logger)
	return nil
}

// Execute the import on the block devices contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	machinesToBlockDevices := i.getMachineBlockDevices(model.Machines())
	volumeBlockDevices := i.getMachineBlockDevicesFromVolumeAttachments(model.Volumes())

	// Prefer block devices defined for machines as they will have more data
	// filled out usually. Only add block devices from volumeBlockDevices which
	// have not been found with the machine.
	for machineName, blockDevices := range volumeBlockDevices {
		unique := slices.DeleteFunc(blockDevices, func(bd blockdevice.BlockDevice) bool {
			return slices.ContainsFunc(
				machinesToBlockDevices[machineName],
				func(machineBD blockdevice.BlockDevice) bool {
					return domainblockdevice.SameDevice(bd, machineBD)
				})
		})
		machinesToBlockDevices[machineName] = append(machinesToBlockDevices[machineName], unique...)
	}

	for machineName, blockDevices := range machinesToBlockDevices {
		err := i.service.SetBlockDevicesForMachineByName(
			ctx, machine.Name(machineName), blockDevices)
		if err != nil {
			return errors.Errorf(
				"importing block devices for machine %q: %w",
				machineName, err,
			)
		}
	}

	return nil
}

// getMachineBlockDevices returns the block devices found for each machine.
func (i *importOperation) getMachineBlockDevices(
	machines []description.Machine,
) map[string][]blockdevice.BlockDevice {
	machinesToBlockDevices := make(map[string][]blockdevice.BlockDevice)
	for _, m := range machines {
		modelBlockDevices := m.BlockDevices()
		if len(modelBlockDevices) == 0 {
			continue
		}

		machineBlockDevices := make([]blockdevice.BlockDevice, len(modelBlockDevices))
		for n, bd := range modelBlockDevices {
			machineBlockDevices[n] = blockdevice.BlockDevice{
				DeviceName:      bd.Name(),
				DeviceLinks:     bd.Links(),
				FilesystemLabel: bd.Label(),
				FilesystemUUID:  bd.UUID(),
				HardwareId:      bd.HardwareID(),
				SerialId:        bd.SerialID(),
				WWN:             bd.WWN(),
				BusAddress:      bd.BusAddress(),
				SizeMiB:         bd.Size(),
				FilesystemType:  bd.FilesystemType(),
				InUse:           bd.InUse(),
				MountPoint:      bd.MountPoint(),
			}
		}

		machinesToBlockDevices[m.Id()] = machineBlockDevices
	}

	return machinesToBlockDevices
}

// getMachineBlockDevicesFromVolumeAttachments returns block devices found in
// volume attachment plans for volume attachments. Partial block devices will
// be resolved by the storage provisioner or registry workers.
func (i *importOperation) getMachineBlockDevicesFromVolumeAttachments(
	volumes []description.Volume,
) map[string][]blockdevice.BlockDevice {
	if len(volumes) == 0 {
		return nil
	}

	machineVolumeBlockDevices := map[string][]blockdevice.BlockDevice{}
	for _, v := range volumes {
		for _, p := range v.AttachmentPlans() {
			planBlockDevice := p.BlockDevice()
			if planBlockDevice == nil {
				continue
			}
			blockDevice := transformPlanToBlockDeviceStruct(planBlockDevice)
			if domainblockdevice.IsEmpty(blockDevice) {
				continue
			}
			machineVolumeBlockDevices[p.Machine()] = append(
				machineVolumeBlockDevices[p.Machine()], blockDevice)
		}

		for _, attach := range v.Attachments() {
			machineName, ok := attach.HostMachine()
			if !ok {
				continue
			}
			blockDevice := transformAttachmentToBlockDevice(attach)
			if domainblockdevice.IsEmpty(blockDevice) {
				continue
			}
			// Check to see if already been added via attachment plans.
			// If not, add.
			machineBDs, _ := machineVolumeBlockDevices[machineName]
			contains := slices.ContainsFunc(machineBDs, func(bd blockdevice.BlockDevice) bool {
				return domainblockdevice.SameDevice(bd, blockDevice)
			})
			if !contains {
				machineVolumeBlockDevices[machineName] = append(machineBDs, blockDevice)
			}
		}
	}

	return machineVolumeBlockDevices
}

func transformPlanToBlockDeviceStruct(bd description.BlockDevice) blockdevice.BlockDevice {
	return blockdevice.BlockDevice{
		DeviceName:      bd.Name(),
		DeviceLinks:     bd.Links(),
		FilesystemLabel: bd.Label(),
		FilesystemUUID:  bd.UUID(),
		HardwareId:      bd.HardwareID(),
		SerialId:        bd.SerialID(),
		WWN:             bd.WWN(),
		BusAddress:      bd.BusAddress(),
	}
}

func transformAttachmentToBlockDevice(attach description.VolumeAttachment) blockdevice.BlockDevice {
	var links []string

	if attach.DeviceLink() != "" {
		links = []string{attach.DeviceLink()}
	}

	return blockdevice.BlockDevice{
		DeviceName:  attach.DeviceName(),
		DeviceLinks: links,
		BusAddress:  attach.BusAddress(),
	}
}
