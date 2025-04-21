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
	coreostype "github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	modelagentservice "github.com/juju/juju/domain/modelagent/service"
	modelagentstate "github.com/juju/juju/domain/modelagent/state"
	"github.com/juju/juju/internal/errors"
)

// baseAgentBinaryExportOperation describes the base set of operation
// characteristics shared between export operations of this package.
// Specifically the common need for the export service.
type baseAgentBinaryExportOperation struct {
	modelmigration.BaseOperation
	exportService ExportService
	logger        logger.Logger
}

// ExportService describes the service required for exporting agent binary
// versions of a model to a new controller.
type ExportService interface {
	// GetMachinesAgentBinaryMetadata returns the agent binary metadata that is
	// running for each machine in the model. This call expects that every
	// machine in the model has their agent binary version set and there exist
	// agent binaries available for each machine and the version that it is
	// running.
	//
	// This is a bulk call to support operations such as model export where it
	// will never provide enough granuality into what machine fails as part of
	// the checks.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/modelagent/errors.MachineAgentVersionNotSet]
	// when one or more machines in the model do not have their agent binary
	// version set.
	// - [github.com/juju/juju/domain/modelagent/errors.MissingAgentBinaries]
	// when the agent binaries don't exist for one or more units in the model.
	GetMachinesAgentBinaryMetadata(
		context.Context,
	) (map[coremachine.Name]coreagentbinary.Metadata, error)

	// GetUnitsAgentBinaryMetadata returns the agent binary metadata that is
	// running for each unit in the model. This call expects that every unit in
	// the model has their agent binary version set and there exist agent
	// binaries available for each unit and the version that it is running.
	//
	// This is a bulk call to support operations such as model export where it
	// will never provide enough granuality into what unit fails as part of the
	// checks.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/modelagent/errors.UnitAgentVersionNotSet]
	// when one or more units in the model do not have their agent binary
	// version set.
	// - [github.com/juju/juju/domain/modelagent/errors.MissingAgentBinaries]
	// when theagent binaries don't exist for one or more units in the model.
	GetUnitsAgentBinaryMetadata(
		context.Context,
	) (map[coreunit.Name]coreagentbinary.Metadata, error)
}

// exportMachineAgentBinaryOperation is a model migration export operation for
// setting the reported agent binary version for each machine being exported as
// part of a model.
type exportMachineAgentBinaryOperation struct {
	baseAgentBinaryExportOperation
}

// exportUnitAgentBinaryOperation is a model migration export operation for
// setting the reported agent binary version for each machine being exported as
// part of a model.
type exportUnitAgentBinaryOperation struct {
	baseAgentBinaryExportOperation
}

// Execute will attempt to set the reported tools/agentbinary version for each
// machine in the model description. This operation assumes that prior to this
// being performed all of the machines in the model have been exported on to
// the model description.
func (e *exportMachineAgentBinaryOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	machinesAgentBinaries, err := e.exportService.GetMachinesAgentBinaryMetadata(ctx)
	if err != nil {
		return errors.Errorf(
			"getting agent binary information for each machine in the model to export: %w",
			err,
		)
	}

	for _, machine := range model.Machines() {
		mName := coremachine.Name(machine.Id())
		machineAgentBinaryMetadata, exists := machinesAgentBinaries[mName]
		if !exists {
			return errors.Errorf(
				"getting agent binary information for machine %q not found during export",
				mName,
			)
		}

		exportVer := semversion.Binary{
			Number:  machineAgentBinaryMetadata.Version.Number,
			Arch:    machineAgentBinaryMetadata.Version.Arch,
			Release: coreostype.Ubuntu.String(),
		}

		// We do not record the path when exporting tools. This was created in
		// description to signal back to other Juju components what the
		// apiserver path was for downloading the agent binaires. This is a
		// leaky abstraction as the api server does not form a contract with the
		// description package.
		//
		// The sha is enough for the model migration master to figure out what
		// to do.
		machine.SetTools(description.AgentToolsArgs{
			Version: exportVer.String(),
			SHA256:  machineAgentBinaryMetadata.SHA256,
			Size:    machineAgentBinaryMetadata.Size,
		})
	}

	return nil
}

// Execute will attempt to set the reported tools/agentbinary version for each
// unit of each application in the model description. This operation assumes
// that prior to this being performed all of the applications and units in the
// model have been exported on to the model description.
func (e *exportUnitAgentBinaryOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	unitsAgentBinaries, err := e.exportService.GetUnitsAgentBinaryMetadata(ctx)
	if err != nil {
		return errors.Errorf(
			"getting agent binary information for each unit in the model to export: %w",
			err,
		)
	}

	for _, application := range model.Applications() {
		for _, unit := range application.Units() {
			uName := coreunit.Name(unit.Name())
			machineAgentBinaryMetadata, exists := unitsAgentBinaries[uName]
			if !exists {
				return errors.Errorf(
					"getting agent binary information for unit %q not found during export",
					uName,
				)
			}

			exportVer := semversion.Binary{
				Number:  machineAgentBinaryMetadata.Version.Number,
				Arch:    machineAgentBinaryMetadata.Version.Arch,
				Release: coreostype.Ubuntu.String(),
			}

			// We do not record the path when exporting tools. This was created
			// in description to signal back to other Juju components what the
			// apiserver path was for downloading the agent binaires. This is a
			// leaky abstraction as the api server does not form a contract with
			// the description package.
			//
			// The sha is enough for the model migration master to figure out what
			// to do.
			unit.SetTools(description.AgentToolsArgs{
				Version: exportVer.String(),
				SHA256:  machineAgentBinaryMetadata.SHA256,
				Size:    machineAgentBinaryMetadata.Size,
			})
		}
	}

	return nil
}

// Name returns the unique descriptive name for this export operation.
func (*exportMachineAgentBinaryOperation) Name() string {
	return "export agent binary version for machines"
}

// Name returns the unique descriptive name for this export operation.
func (*exportUnitAgentBinaryOperation) Name() string {
	return "export agent binary version for units"
}

// RegisterExport registers a set of new model migration exporters into the
// supplied coordinator. Specifically this registers export operations for
// setting machine and unit agent binary versions on the out going model
// description.
//
// It is expected that this Register is called after the operations that are
// responsible for putting machines and units into the description.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportMachineAgentBinaryOperation{
		baseAgentBinaryExportOperation: baseAgentBinaryExportOperation{
			logger: logger,
		},
	})

	coordinator.Add(&exportUnitAgentBinaryOperation{
		baseAgentBinaryExportOperation: baseAgentBinaryExportOperation{
			logger: logger,
		},
	})
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during export of a model agents information.
func (b *baseAgentBinaryExportOperation) Setup(scope modelmigration.Scope) error {
	b.exportService = modelagentservice.NewService(
		modelagentstate.NewState(scope.ModelDB()),
	)
	return nil
}
