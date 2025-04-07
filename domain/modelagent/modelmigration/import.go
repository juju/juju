// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	modelagentservice "github.com/juju/juju/domain/modelagent/service"
	modelagentstate "github.com/juju/juju/domain/modelagent/state"
	"github.com/juju/juju/internal/errors"
)

// baseAgentBinaryImportOperation describes the base set of operation
// characteristics shared between import operations of this package.
// Specifically the common need for the import service.
type baseAgentBinaryImportOperation struct {
	modelmigration.BaseOperation
	importService ImportService
	logger        logger.Logger
}

// Coordinator is the interface that is used to add operats to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// ImportService describes the service required for importing agent binary
// versions of a model into a new controller.
type ImportService interface {
	// SetMachineReportedAgentVersion sets the reported agent version for the
	// supplied machine name. Reported agent version is the version that the agent
	// binary on this machine has reported it is running.
	//
	// The following errors are possible:
	// - [github.com/juju/juju/core/errors.NotValid] if the reportedVersion is
	// not valid or the machine name is not valid.
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not supported.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when the
	// machine does not exist.
	// - [github.com/juju/juju/domain/machine/errors.MachineDead] when the
	// machine is dead.
	SetMachineReportedAgentVersion(
		ctx context.Context,
		machineName coremachine.Name,
		reportedVersion coreagentbinary.Version,
	) error

	// SetUnitReportedAgentVersion sets the reported agent version for the
	// supplied unit name. Reported agent version is the version that the agent
	// binary on this unit has reported it is running.
	//
	// The following errors are possible:
	// - [github.com/juju/juju/core/errors.NotValid] when the reportedVersion
	// is not valid.
	// - [github.com/juju/juju/core/errors.NotSupported] when the architecture
	// is not supported.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when the
	// unit does not exist.
	// - [github.com/juju/juju/domain/application/errors.UnitIsDead] when the
	// unit is dead.
	SetUnitReportedAgentVersion(
		ctx context.Context,
		unitName coreunit.Name,
		reportedVersion coreagentbinary.Version,
	) error
}

// importMachineAgentBinaryOperation is a model migration import operation for
// setting the reported agent binary version for each machine being imported as
// part of a model.
type importMachineAgentBinaryOperation struct {
	baseAgentBinaryImportOperation
}

// importUnitAgentBinaryOperation is a model migration import operationg for
// setting the reported agent binary version for each unit being imported as
// part of a model.
type importUnitAgentBinaryOperation struct {
	baseAgentBinaryImportOperation
}

// Execute will attempt to set the reported tools/agentbinary version for each
// machine in the model. This operation assumes that prior to this being
// performed all of the machines in the model have been imported into this
// controller.
//
// The following errors can be expected:
// - [github.com/juju/juju/core/errors.NotValid] if the reportedVersion is
// not valid or the machine name is not valid.
// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
// not supported.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when the
// machine does not exist.
// - [github.com/juju/juju/domain/machine/errors.MachineDead] when the
// machine is dead.
func (i *importMachineAgentBinaryOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	for _, machine := range model.Machines() {
		tools := machine.Tools()

		mName := coremachine.Name(machine.Id())
		binVer, err := semversion.ParseBinary(tools.Version())
		if err != nil {
			return errors.Errorf(
				"parsing agent version for machine %q during migration: %w",
				mName.String(), err,
			)
		}
		agentBinaryVersion := coreagentbinary.Version{
			Number: binVer.Number,
			Arch:   binVer.Arch,
		}

		err = i.importService.SetMachineReportedAgentVersion(
			ctx, mName, agentBinaryVersion,
		)

		if err != nil {
			return errors.Errorf(
				"setting reported agent version for machine %q during migration: %w",
				mName.String(), err,
			)
		}
	}

	return nil
}

// Execute will attempt to set the reported tools/agentbinary version for each
// unit of each application in the model. This operation assumes that priort to
// this being performed all of the applications and units in the model have been
// imported into this controller.
//
// The following errors are possible:
// - [github.com/juju/juju/core/errors.NotValid] when the reportedVersion
// is not valid.
// - [github.com/juju/juju/core/errors.NotSupported] when the architecture
// is not supported.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when the
// unit does not exist.
// - [github.com/juju/juju/domain/application/errors.UnitIsDead] when the
// unit is dead.
func (i *importUnitAgentBinaryOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	for _, application := range model.Applications() {
		for _, unit := range application.Units() {
			tools := unit.Tools()

			uName := coreunit.Name(unit.Name())
			binVer, err := semversion.ParseBinary(tools.Version())
			if err != nil {
				return errors.Errorf(
					"parsing agent version for unit %q during migration: %w",
					uName.String(), err,
				)
			}
			agentBinaryVersion := coreagentbinary.Version{
				Number: binVer.Number,
				Arch:   binVer.Arch,
			}

			err = i.importService.SetUnitReportedAgentVersion(
				ctx, uName, agentBinaryVersion,
			)

			if err != nil {
				return errors.Errorf(
					"setting reported agent version for unit %q during migration: %w",
					uName.String(), err,
				)
			}
		}
	}

	return nil
}

// Name returns the unique descriptive name for this import operation.
func (*importMachineAgentBinaryOperation) Name() string {
	return "import agent binary version for machines"
}

// Name returns the unique descriptive name for this import operation.
func (*importUnitAgentBinaryOperation) Name() string {
	return "import agent binary version for units"
}

// RegisterImport register's a set of new model migration imports into the
// supplied coordinator. Specifically this registers import operations for
// setting machine and unit agent binary versions on the entities in the
// model's state.
//
// It is expected that this Register is called after the operations that are
// responsible for putting machines and units into the controller's state.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importMachineAgentBinaryOperation{
		baseAgentBinaryImportOperation: baseAgentBinaryImportOperation{
			logger: logger,
		},
	})

	coordinator.Add(&importUnitAgentBinaryOperation{
		baseAgentBinaryImportOperation: baseAgentBinaryImportOperation{
			logger: logger,
		},
	})
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import of a model agents information.
func (b *baseAgentBinaryImportOperation) Setup(scope modelmigration.Scope) error {
	b.importService = modelagentservice.NewService(
		modelagentstate.NewState(scope.ModelDB()),
	)
	return nil
}
