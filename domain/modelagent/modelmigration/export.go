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
	GetMachineReportedAgentVersion(
		ctx context.Context,
		machineName coremachine.Name,
	) (coreagentbinary.Version, error)

	GetUnitReportedAgentVersion(
		ctx context.Context,
		unitName coreunit.Name,
	) (coreagentbinary.Version, error)
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
	for _, machine := range model.Machines() {
		mName := coremachine.Name(machine.Id())
		agentVersion, err := e.exportService.GetMachineReportedAgentVersion(ctx, mName)

		if err != nil {
			return errors.Errorf(
				"getting reported agent version for machine %q: %w",
				mName, err,
			)
		}

		exportVer := semversion.Binary{
			Number:  agentVersion.Number,
			Arch:    agentVersion.Arch,
			Release: coreostype.Ubuntu.String(),
		}

		// We purposely don't record the other information on agent tools that
		// is asked for in model description. Specifically because Juju has
		// never had an association between version running and binaries in
		// store.
		machine.SetTools(description.AgentToolsArgs{
			Version: exportVer.String(),
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
	for _, application := range model.Applications() {
		for _, unit := range application.Units() {
			uName := coreunit.Name(unit.Name())
			agentVersion, err := e.exportService.GetUnitReportedAgentVersion(ctx, uName)

			if err != nil {
				return errors.Errorf(
					"getting reported agent version for unit %q: %w",
					uName.String(), err,
				)
			}

			exportVer := semversion.Binary{
				Number:  agentVersion.Number,
				Arch:    agentVersion.Arch,
				Release: coreostype.Ubuntu.String(),
			}

			// We purposely don't record the other information on agent tools
			// that is asked for in model description. Specifically because Juju
			// has never had an association between version running and binaries in
			// store.
			unit.SetTools(description.AgentToolsArgs{
				Version: exportVer.String(),
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
