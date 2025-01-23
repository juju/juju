// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the model domain
// service methods needed for model export.
type ExportService interface {
	// GetEnvironVersion retrieves the version of the environment provider
	// associated with the model.
	GetEnvironVersion(context.Context) (int, error)
}

// exportOperation describes a way to execute a migration for
// exporting model.
type exportOperation struct {
	modelmigration.BaseOperation

	serviceGetter func(modelUUID model.UUID) ExportService
	logger        logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export model"
}

// Setup the export operation, this will ensure the service is created
// and ready to be used.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.serviceGetter = func(modelUUID model.UUID) ExportService {
		return service.NewModelService(
			modelUUID,
			state.NewState(scope.ControllerDB()),
			state.NewModelState(scope.ModelDB(), e.logger),
			service.EnvironVersionProviderGetter(),
		)
	}
	return nil
}

// Execute the export and sets the environ version of the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := coremodel.UUID(model.Tag().Id())
	exportService := e.serviceGetter(modelUUID)
	environVersion, err := exportService.GetEnvironVersion(ctx)
	if err != nil {
		return errors.Errorf(
			"exporting environ version for model: %w",
			err,
		)
	}
	model.SetEnvironVersion(environVersion)
	return nil
}
