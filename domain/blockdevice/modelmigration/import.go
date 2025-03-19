// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
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
	UpdateBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error
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
	machines := model.Machines()

	for _, m := range machines {
		modelBlockDevices := m.BlockDevices()
		if len(modelBlockDevices) == 0 {
			continue
		}
		machineBlockDevices := make([]blockdevice.BlockDevice, len(modelBlockDevices))
		for n, bd := range modelBlockDevices {
			machineBlockDevices[n] = blockdevice.BlockDevice{
				DeviceName:     bd.Name(),
				DeviceLinks:    bd.Links(),
				Label:          bd.Label(),
				UUID:           bd.UUID(),
				HardwareId:     bd.HardwareID(),
				SerialId:       bd.SerialID(),
				WWN:            bd.WWN(),
				BusAddress:     bd.BusAddress(),
				SizeMiB:        bd.Size(),
				FilesystemType: bd.FilesystemType(),
				InUse:          bd.InUse(),
				MountPoint:     bd.MountPoint(),
			}
		}
		if err := i.service.UpdateBlockDevices(ctx, m.Id(), machineBlockDevices...); err != nil {
			return errors.Errorf("importing block devices for machine %q: %w", m.Id(), err)
		}
	}
	return nil
}
