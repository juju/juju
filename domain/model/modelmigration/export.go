// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(newExportEnvironVersionOperation(logger))
	coordinator.Add(newExportModelConstraintsOperation(logger))
}

// ExportService provides a subset of the model domain
// service methods needed for model export.
type ExportService interface {
	// GetEnvironVersion retrieves the version of the environment provider
	// associated with the model.
	GetEnvironVersion(context.Context) (int, error)

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	GetModelConstraints(context.Context) (coreconstraints.Value, error)
}

type exportOperation struct {
	modelmigration.BaseOperation

	serviceGetter func(modelUUID coremodel.UUID) ExportService
	logger        logger.Logger
}

// exportEnvironVersionOperation describes a way to execute a migration for
// exporting model.
type exportEnvironVersionOperation struct {
	exportOperation
}

// exportModelConstraintsOperation describes an export operation for a model's
// set constraints.
type exportModelConstraintsOperation struct {
	exportOperation
}

// Name returns the name of this operation.
func (e *exportEnvironVersionOperation) Name() string {
	return "export model environ version"
}

// Name returns the name of this operation.
func (e *exportModelConstraintsOperation) Name() string {
	return "export model constraints"
}

// newExportEnvironVersionOperation constructs a new
// [exportEnvironVersionOperation]
func newExportEnvironVersionOperation(l logger.Logger) *exportEnvironVersionOperation {
	return &exportEnvironVersionOperation{
		exportOperation{
			logger: l,
		},
	}
}

// newExportModelConstraintsOperation constructs a new
// [exportModelConstraintsOperation]
func newExportModelConstraintsOperation(l logger.Logger) *exportModelConstraintsOperation {
	return &exportModelConstraintsOperation{
		exportOperation{
			logger: l,
		},
	}
}

// Setup established the required services needed for retrieving information
// about the model being exported.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.serviceGetter = func(modelUUID coremodel.UUID) ExportService {
		return service.NewModelService(
			modelUUID,
			state.NewState(scope.ControllerDB()),
			state.NewModelState(scope.ModelDB(), e.logger),
			service.EnvironVersionProviderGetter(),
			service.DefaultAgentBinaryFinder(),
		)
	}
	return nil
}

// Execute the export and sets the environ version of the model.
func (e *exportEnvironVersionOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := coremodel.UUID(model.UUID())
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

// Execute the export and sets the model's constraints.
func (e *exportModelConstraintsOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	modelUUID := coremodel.UUID(model.UUID())
	exportService := e.serviceGetter(modelUUID)
	cons, err := exportService.GetModelConstraints(ctx)
	if err != nil {
		return errors.Errorf(
			"exporting model constraints: %w", err,
		)
	}

	model.SetConstraints(description.ConstraintsArgs{
		AllocatePublicIP: deref(cons.AllocatePublicIP),
		Architecture:     deref(cons.Arch),
		Container:        string(deref(cons.Container)),
		CpuCores:         deref(cons.CpuCores),
		CpuPower:         deref(cons.CpuPower),
		ImageID:          deref(cons.ImageID),
		InstanceType:     deref(cons.InstanceType),
		Memory:           deref(cons.Mem),
		RootDisk:         deref(cons.RootDisk),
		RootDiskSource:   deref(cons.RootDiskSource),
		Spaces:           deref(cons.Spaces),
		Tags:             deref(cons.Tags),
		Zones:            deref(cons.Zones),
		VirtType:         deref(cons.VirtType),
	})

	return nil
}

// deref returns the dereferenced value of T if T is not nil. Otherwise the zero
// value of T is returned.
func deref[T any](t *T) T {
	if t == nil {
		return *new(T)
	}
	return *t
}
