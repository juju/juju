// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"slices"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/machine"
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
	// GetMachines returns all the machines in the model.
	GetMachines(ctx context.Context) ([]machine.ExportMachine, error)
	// GetInstanceID returns the cloud specific instance id for this machine.
	// If the machine is not provisioned, it returns a NotProvisionedError.
	GetInstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error)
	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID coremachine.UUID) (*instance.HardwareCharacteristics, error)
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
	e.service = service.NewMigrationService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		e.clock,
		e.logger,
	)
	return nil
}

func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machines, err := e.service.GetMachines(ctx)
	if err != nil {
		return errors.Errorf("retrieving machines for export: %w", err)
	}

	for _, m := range model.Machines() {
		index := slices.IndexFunc(machines, func(em machine.ExportMachine) bool {
			return em.Name == coremachine.Name(m.Id())
		})
		if index == -1 {
			// The machine is not in the exported model, so we skip it.
			continue
		}

		machine := machines[index]

		m.SetNonce(machine.Nonce)

		instanceID, err := e.service.GetInstanceID(ctx, machine.UUID)
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
			return errors.Errorf("retrieving instance ID for machine %q: %w", machine.Name, err)
		}
		instanceArgs := description.CloudInstanceArgs{
			InstanceId: instanceID.String(),
		}
		hardwareCharacteristics, err := e.service.GetHardwareCharacteristics(ctx, machine.UUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			continue
		}
		if err != nil {
			return errors.Errorf("retrieving hardware characteristics for machine %q: %w", machine.Name, err)
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

		m.SetInstance(instanceArgs)

		instance := m.Instance()
		instance.SetStatus(description.StatusArgs{})
	}
	return nil
}
