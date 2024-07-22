// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
	logger  logger.Logger
}

// ImportService defines the machine service used to import machines from
// another controller model to this controller.
type ImportService interface {
	// CreateMachine creates the specified machine.
	CreateMachine(ctx context.Context, machineName machine.Name) (string, error)
	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(ctx context.Context, machineUUID string, instanceID instance.Id, hardwareCharacteristics *instance.HardwareCharacteristics) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import machines"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB(), i.logger))
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, m := range model.Machines() {
		// We need skeleton machines in dqlite.
		machineUUID, err := i.service.CreateMachine(ctx, machine.Name(m.Id()))
		if err != nil {
			return errors.Annotatef(err, "importing machine %q", m.Id())
		}

		// Import the machine's cloud instance.
		cloudInstance := m.Instance()
		if cloudInstance != nil {
			arch := cloudInstance.Architecture()
			mem := cloudInstance.Memory()
			rootDisk := cloudInstance.RootDisk()
			rootDiskSource := cloudInstance.RootDiskSource()
			cpuCores := cloudInstance.CpuCores()
			cpuPower := cloudInstance.CpuPower()
			tags := cloudInstance.Tags()
			virtType := cloudInstance.VirtType()
			hardwareCharacteristics := &instance.HardwareCharacteristics{
				Arch:           &arch,
				Mem:            &mem,
				RootDisk:       &rootDisk,
				RootDiskSource: &rootDiskSource,
				CpuCores:       &cpuCores,
				CpuPower:       &cpuPower,
				Tags:           &tags,
				VirtType:       &virtType,
			}
			// Only add the availability zone if it is not empty. It can be empty
			// because we are deserializing from a description.Model and we
			// lose the pointer reference since it returns a string.
			if availabilityZone := cloudInstance.AvailabilityZone(); availabilityZone != "" {
				hardwareCharacteristics.AvailabilityZone = &availabilityZone
			}
			if err := i.service.SetMachineCloudInstance(
				ctx,
				machineUUID,
				instance.Id(cloudInstance.InstanceId()),
				hardwareCharacteristics,
			); err != nil {
				return errors.Annotatef(err, "importing machine cloud instance %q", m.Id())
			}
		}
	}
	return nil
}
