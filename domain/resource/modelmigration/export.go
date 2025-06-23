// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/domain/resource/service"
	"github.com/juju/juju/domain/resource/state"
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

// ExportService provides a subset of the resource domain service methods
// needed for resource export.
type ExportService interface {
	// ExportResources returns the list of application and unit resources to
	// export for the given application.
	//
	// If the application exists but doesn't have any resources, no error are
	// returned, the result just contains an empty list.
	ExportResources(ctx context.Context, name string) (
		resource.ExportedResources,
		error,
	)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export resources"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		nil,
		e.logger,
	)
	return nil
}

// Execute the export. Go through each of the applications on the model and add
// their resources, and unit resources.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		exported, err := e.service.ExportResources(ctx, app.Name())
		if err != nil {
			return errors.Errorf("getting resource of application %q: %w", app.Name(), err)
		}

		for _, res := range exported.Resources {
			r := app.AddResource(description.ResourceArgs{
				Name: res.Name,
			})
			r.SetApplicationRevision(description.ResourceRevisionArgs{
				Type:        res.Type.String(),
				Origin:      res.Origin.String(),
				Timestamp:   res.Timestamp,
				Revision:    res.Revision,
				SHA384:      res.Fingerprint.String(),
				Size:        res.Size,
				RetrievedBy: res.RetrievedBy,
			})
		}

		if len(exported.UnitResources) == 0 {
			continue
		}

		unitNameToResources := make(map[string][]coreresource.Resource)
		for _, unitRes := range exported.UnitResources {
			unitNameToResources[unitRes.Name.String()] = unitRes.Resources
		}

		for _, unit := range app.Units() {
			unitResources := unitNameToResources[unit.Name()]
			for _, unitRes := range unitResources {
				unit.AddResource(description.UnitResourceArgs{
					Name: unitRes.Name,
					RevisionArgs: description.ResourceRevisionArgs{
						Type:        unitRes.Type.String(),
						Origin:      unitRes.Origin.String(),
						Timestamp:   unitRes.Timestamp,
						Revision:    unitRes.Revision,
						SHA384:      unitRes.Fingerprint.String(),
						Size:        unitRes.Size,
						RetrievedBy: unitRes.RetrievedBy,
					},
				})
			}
		}

	}
	return nil
}
