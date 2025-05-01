// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/status/service"
	"github.com/juju/juju/domain/status/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&exportOperation{
		clock:  clock,
		logger: logger,
	})
}

// ExportService provides a subset of the status domain
// service methods needed for status export.
type ExportService interface {
	// ExportUnitStatuses returns the workload and agent statuses of all the units in
	// in the model, indexed by unit name.
	ExportUnitStatuses(ctx context.Context) (map[coreunit.Name]corestatus.StatusInfo, map[coreunit.Name]corestatus.StatusInfo, error)

	// ExportApplicationStatuses returns the statuses of all applications in the model,
	// indexed by application name, if they have a status set.
	ExportApplicationStatuses(ctx context.Context) (map[string]corestatus.StatusInfo, error)

	// ExportRelationStatuses returns the statuses of all relations in the model,
	// indexed by relation id, if they have a status set.
	ExportRelationStatuses(ctx context.Context) (map[int]corestatus.StatusInfo, error)
}

type exportOperation struct {
	modelmigration.BaseOperation

	serviceGetter func(model.UUID) ExportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export status"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.serviceGetter = func(modelUUID model.UUID) ExportService {
		return service.NewService(
			state.NewState(scope.ModelDB(), e.clock, e.logger),
			modelUUID,
			// TODO(jack): This is currently the wrong logger. We should construct
			// the StatusHistory using the model logger, however, at the moment, we
			// cannot get the model logger until the model has been imported. Once
			// this has changed, refactor this to use the model logger.
			domain.NewStatusHistory(e.logger, e.clock),
			func() (service.StatusHistoryReader, error) {
				return nil, errors.Errorf("status history reader not available")
			},
			e.clock,
			e.logger,
		)
	}
	return nil
}

// Execute the export operation, loading the statuses of the various entities in
// the model onto their description representation.
func (e *exportOperation) Execute(ctx context.Context, m description.Model) error {
	modelUUID := model.UUID(m.UUID())
	service := e.serviceGetter(modelUUID)

	err := e.exportApplicationAndUnitStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("exporting application and unit status: %w", err)
	}

	err = e.exportRelationStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("exporting reltaion status: %w", err)
	}

	return nil
}

func (e *exportOperation) exportApplicationAndUnitStatus(
	ctx context.Context,
	service ExportService,
	m description.Model,
) error {
	appStatuses, err := service.ExportApplicationStatuses(ctx)
	if err != nil {
		return errors.Errorf("retrieving application statuses: %w", err)
	}

	unitWorkloadStatuses, unitAgentStatuses, err := service.ExportUnitStatuses(ctx)
	if err != nil {
		return errors.Errorf("retrieving unit statuses: %w", err)
	}

	for _, app := range m.Applications() {
		appName := app.Name()

		// Application statuses are optional, so set this to NeverSet if there
		// is no status.
		if appStatus, ok := appStatuses[appName]; ok {
			app.SetStatus(e.exportStatus(appStatus))
		} else {
			app.SetStatus(description.StatusArgs{
				NeverSet: true,
			})
		}

		for _, unit := range app.Units() {
			unitName := coreunit.Name(unit.Name())
			agentStatus, ok := unitAgentStatuses[unitName]
			if !ok {
				return errors.Errorf("unit %q has no agent status", unitName)
			}
			unit.SetAgentStatus(e.exportStatus(agentStatus))

			workloadStatus, ok := unitWorkloadStatuses[unitName]
			if !ok {
				return errors.Errorf("unit %q has no workload status", unitName)
			}
			unit.SetWorkloadStatus(e.exportStatus(workloadStatus))
		}
	}

	return nil
}

func (e *exportOperation) exportRelationStatus(
	ctx context.Context,
	service ExportService,
	m description.Model,
) error {
	relStatuses, err := service.ExportRelationStatuses(ctx)
	if err != nil {
		return errors.Errorf("retrieving relation statuses: %w", err)
	}

	for _, relation := range m.Relations() {
		if relationStatus, ok := relStatuses[relation.Id()]; ok {
			relation.SetStatus(e.exportStatus(relationStatus))
		} else {
			relation.SetStatus(description.StatusArgs{
				NeverSet: true,
			})
		}
	}
	return nil
}

func (e *exportOperation) exportStatus(status corestatus.StatusInfo) description.StatusArgs {
	now := e.clock.Now().UTC()
	if status.Since != nil {
		now = *status.Since
	}

	return description.StatusArgs{
		Value:   status.Status.String(),
		Message: status.Message,
		Data:    status.Data,
		Updated: now,
	}
}
