// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
) {
	coordinator.Add(&exportOperation{})
}

// ExportService provides a subset of the leadership domain
// service methods needed for application export.
type ExportService interface {
	// GetApplicationLeadershipForModel returns the leadership information for the
	// model applications.
	GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (map[string]string, error)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export application leases"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewMigrationService(
		state.NewMigrationState(scope.ControllerDB()),
	)
	return nil
}

// Execute the export, adding the application to the model.
// The export also includes all the charm metadata, manifest, config and
// actions. Along with units and resources.
func (e *exportOperation) Execute(ctx context.Context, m description.Model) error {
	leases, err := e.service.GetApplicationLeadershipForModel(ctx, model.UUID(m.UUID()))
	if err != nil {
		return errors.Errorf("getting application leadership: %w", err)
	}

	for _, app := range m.Applications() {
		holder, ok := leases[app.Name()]
		if !ok {
			return errors.Errorf("application %q has no leadership", app.Name())
		}
		app.SetLeader(holder)
	}

	return nil
}
