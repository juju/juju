// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		clock:  clock,
		logger: logger,
	})
}

// ExportService defines the machine service used to export machines to
// another controller model to this controller.
type ExportService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(ctx context.Context) ([]coremachine.Name, error)
	// InstanceID returns the cloud specific instance id for this machine.
	// If the machine is not provisioned, it returns a NotProvisionedError.
	InstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID coremachine.UUID) (*instance.HardwareCharacteristics, error)
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns a [github.com/juju/juju/domain/machine/errors.MachineNotFound]
	// if the machine does not exist.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
	clock   clock.Clock
	logger  logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export machines"
}

func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(state.NewState(scope.ModelDB(), e.clock, e.logger))
	return nil
}

func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {

	for _, machine := range model.Machines() {
		// TODO(nvinuesa): We must check if the machine cloud instance is already
		// set and in that case don't overwrite anything. This check can be removed
		// once we fully move machines over to dqlite.
		if instance := machine.Instance(); instance != nil {
			continue
		}

		machineName := coremachine.Name(machine.Id())
		machineUUID, err := e.service.GetMachineUUID(ctx, machineName)
		if err != nil {
			return errors.Errorf("retrieving instance ID for machine %q: %w", machineName, err)
		}
		instanceID, err := e.service.InstanceID(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			// TODO(nvinuesa): Here we should remove the machine from the
			// exported model because we should not migrate non-provisioned
			// machines. This used to be checked in model.Description, but was
			// removed in https://github.com/juju/description/pull/157.
			// We should revisit this once we finish migrating machines over to
			// dqlite (by not adding the machine to the exported model to begin
			// with if it's not provisioned).
			continue
		}
		if err != nil {
			return errors.Errorf("retrieving instance ID for machine %q: %w", machineName, err)
		}
		instanceArgs := description.CloudInstanceArgs{
			InstanceId: instanceID.String(),
		}
		hardwareCharacteristics, err := e.service.HardwareCharacteristics(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			continue
		}
		if err != nil {
			return errors.Errorf("retrieving hardware characteristics for machine %q: %w", machineName, err)
		}
		if hardwareCharacteristics.Arch != nil {
			instanceArgs.Architecture = *hardwareCharacteristics.Arch
		}
		if hardwareCharacteristics.Mem != nil {
			instanceArgs.Memory = *hardwareCharacteristics.Mem
		}
		if hardwareCharacteristics.RootDisk != nil {
			instanceArgs.RootDisk = *hardwareCharacteristics.RootDisk
		}
		if hardwareCharacteristics.RootDiskSource != nil {
			instanceArgs.RootDiskSource = *hardwareCharacteristics.RootDiskSource
		}
		if hardwareCharacteristics.CpuCores != nil {
			instanceArgs.CpuCores = *hardwareCharacteristics.CpuCores
		}
		if hardwareCharacteristics.CpuPower != nil {
			instanceArgs.CpuPower = *hardwareCharacteristics.CpuPower
		}
		if hardwareCharacteristics.Tags != nil {
			instanceArgs.Tags = *hardwareCharacteristics.Tags
		}
		if hardwareCharacteristics.AvailabilityZone != nil {
			instanceArgs.AvailabilityZone = *hardwareCharacteristics.AvailabilityZone
		}
		if hardwareCharacteristics.VirtType != nil {
			instanceArgs.VirtType = *hardwareCharacteristics.VirtType
		}
		machine.SetInstance(instanceArgs)
		instance := machine.Instance()
		instance.SetStatus(description.StatusArgs{})
	}
	return nil
}
