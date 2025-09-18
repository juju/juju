// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/blockdevice/service"
	"github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the block device domain
// service methods needed for block device export.
type ExportService interface {
	// AllBlockDevices retrieves block devices for all machines.
	AllBlockDevices(
		ctx context.Context,
	) (map[machine.Name][]blockdevice.BlockDevice, error)
}

// exportOperation describes a way to execute a migration for
// exporting block devices.
type exportOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export block devices"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	// We must not use a watcher during migration, so it's safe to pass a
	// nil watcher factory.
	e.service = service.NewService(
		state.NewState(scope.ModelDB()), e.logger)
	return nil
}

// Execute the export, adding the block devices to the model.
func (e *exportOperation) Execute(
	ctx context.Context, model description.Model,
) error {
	blockDevices, err := e.service.AllBlockDevices(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	for machineName, devices := range blockDevices {
		for _, dev := range devices {
			desc := description.BlockDeviceArgs{
				Name:           dev.DeviceName,
				Links:          dev.DeviceLinks,
				Label:          dev.FilesystemLabel,
				UUID:           dev.FilesystemUUID,
				HardwareID:     dev.HardwareId,
				SerialID:       dev.SerialId,
				WWN:            dev.WWN,
				BusAddress:     dev.BusAddress,
				Size:           dev.SizeMiB,
				FilesystemType: dev.FilesystemType,
				InUse:          dev.InUse,
				MountPoint:     dev.MountPoint,
			}
			err := model.AddBlockDevice(machineName.String(), desc)
			if err != nil {
				return errors.Errorf(
					"adding block device for machine %q: %w",
					machineName, err,
				)
			}
		}
	}
	return nil
}
