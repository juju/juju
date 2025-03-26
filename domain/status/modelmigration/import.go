// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(
	coordinator Coordinator,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService

	clock  clock.Clock
	logger logger.Logger
}

// ImportService provides a subset of the status domain service methods needed
// for importing status.
type ImportService interface {
	// SetApplicationStatus saves the given application status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.ApplicationNotFound] if the application doesn't exist.
	SetApplicationStatus(context.Context, string, corestatus.StatusInfo) error

	// SetUnitWorkloadStatus sets the workload status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the unit
	// doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// SetUnitAgentStatus sets the agent status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the unit
	// doesn't exist.
	SetUnitAgentStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import status"
}

// Setup the import operation.
// This will create a new service instance.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB(), i.clock, i.logger),
		i.clock,
		i.logger,
		// TODO(jack): This is currently the wrong logger. We should construct
		// the StatusHistory using the model logger, however, at the moment, we
		// cannot get the model logger until the model has been imported. Once
		// this has changed, refactor this to use the model logger.
		domain.NewStatusHistory(i.logger, i.clock),
	)
	return nil
}

// Execute the import, loading the statuses of the various entities out of the
// description representation, into the domain.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		appStatus := i.importStatus(app.Status())
		if err := i.service.SetApplicationStatus(ctx, app.Name(), appStatus); err != nil {
			return err
		}

		for _, unit := range app.Units() {
			unitName, err := coreunit.NewName(unit.Name())
			if err != nil {
				return err
			}
			unitAgentStatus := i.importStatus(unit.AgentStatus())
			if err := i.service.SetUnitAgentStatus(ctx, unitName, unitAgentStatus); err != nil {
				return err
			}

			unitWorkloadStatus := i.importStatus(unit.WorkloadStatus())
			if err := i.service.SetUnitWorkloadStatus(ctx, unitName, unitWorkloadStatus); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *importOperation) importStatus(s description.Status) corestatus.StatusInfo {
	// Older versions of Juju would pass through NeverSet() on the status
	// description for application statuses that hadn't been explicitly
	// set by the lead unit. If that is the case, we make the status what
	// the new code expects.
	if s == nil || s.NeverSet() {
		return corestatus.StatusInfo{
			Status: corestatus.Unset,
		}
	}

	return corestatus.StatusInfo{
		Status:  corestatus.Status(s.Value()),
		Message: s.Message(),
		Data:    s.Data(),
		Since:   ptr(s.Updated()),
	}
}

func ptr[T any](v T) *T {
	return &v
}
