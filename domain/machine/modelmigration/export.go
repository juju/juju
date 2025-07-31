// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain"
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
		domain.NewStatusHistory(e.logger, e.clock),
		e.clock,
		e.logger,
	)
	return nil
}

func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exportMachines, err := e.service.GetMachines(ctx)
	if err != nil {
		return errors.Errorf("retrieving machines for export: %w", err)
	}
	exportContainers := []machine.ExportMachine{}
	machines := map[string]description.Machine{}

	for _, m := range exportMachines {
		if m.Name.IsContainer() {
			exportContainers = append(exportContainers, m)
			continue
		}
		machine := model.AddMachine(description.MachineArgs{
			Id:        m.Name.String(),
			Placement: m.Placement,
			Base:      m.Base,
		})
		err := e.exportMachine(ctx, m, machine)
		if err != nil {
			return errors.Errorf("exporting machine %q: %w", m.Name, err)
		}
		machines[m.Name.String()] = machine
	}

	for _, c := range exportContainers {
		parentName := c.Name.Parent()
		parent, ok := machines[parentName.String()]
		if !ok {
			return errors.Errorf("parent machine %q not exported", parentName)
		}
		container := parent.AddContainer(description.MachineArgs{
			Id:        c.Name.String(),
			Placement: c.Placement,
			Base:      c.Base,
		})
		err := e.exportMachine(ctx, c, container)
		if err != nil {
			return errors.Errorf("exporting container machine %q: %w", c.Name, err)
		}
	}
	return nil
}

func (e *exportOperation) exportMachine(ctx context.Context, m machine.ExportMachine, machine description.Machine) error {
	machine.SetNonce(m.Nonce)
	machine.SetPasswordHash(m.PasswordHash)

	instanceArgs := description.CloudInstanceArgs{
		InstanceId: m.InstanceID,
	}
	hardwareCharacteristics, err := e.service.GetHardwareCharacteristics(ctx, m.UUID)
	if errors.Is(err, machineerrors.NotProvisioned) {
		return nil
	}
	if err != nil {
		return errors.Errorf("retrieving hardware characteristics for machine %q: %w", m.Name, err)
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
	return nil
}
