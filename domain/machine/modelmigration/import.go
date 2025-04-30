// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
	clock   clock.Clock
	logger  logger.Logger
}

// ImportService defines the machine service used to import machines from
// another controller model to this controller.
type ImportService interface {
	// CreateMachine creates the specified machine.
	CreateMachine(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(ctx context.Context, machineUUID machine.UUID, instanceID instance.Id, displayName string, hardwareCharacteristics *instance.HardwareCharacteristics) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import machines"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB(), i.clock, i.logger))
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, m := range model.Machines() {
		// We need skeleton machines in dqlite.
		machineUUID, err := i.service.CreateMachine(ctx, machine.Name(m.Id()))
		if err != nil {
			return errors.Errorf("importing machine %q: %w", m.Id(), err)
		}

		// Import the machine's cloud instance.
		cloudInstance := m.Instance()
		if cloudInstance != nil {
			hardwareCharacteristics := &instance.HardwareCharacteristics{
				Arch:             nilZeroPtr(cloudInstance.Architecture()),
				Mem:              nilZeroPtr(cloudInstance.Memory()),
				RootDisk:         nilZeroPtr(cloudInstance.RootDisk()),
				RootDiskSource:   nilZeroPtr(cloudInstance.RootDiskSource()),
				CpuCores:         nilZeroPtr(cloudInstance.CpuCores()),
				CpuPower:         nilZeroPtr(cloudInstance.CpuPower()),
				AvailabilityZone: nilZeroPtr(cloudInstance.AvailabilityZone()),
				VirtType:         nilZeroPtr(cloudInstance.VirtType()),
			}
			if tags := cloudInstance.Tags(); len(tags) != 0 {
				hardwareCharacteristics.Tags = &tags
			}
			if err := i.service.SetMachineCloudInstance(
				ctx,
				machineUUID,
				instance.Id(cloudInstance.InstanceId()),
				cloudInstance.DisplayName(),
				hardwareCharacteristics,
			); err != nil {
				return errors.Errorf("importing machine cloud instance %q: %w", m.Id(), err)
			}
		}
	}
	return nil
}

func nilZeroPtr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
