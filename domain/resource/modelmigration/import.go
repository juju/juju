// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v8"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/domain/resource/service"
	"github.com/juju/juju/domain/resource/state"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ResourceService provides a subset of the resource domain service methods
// needed for resource import.
type ResourceService interface {
	// ImportResources sets resources imported in migration. It first builds all the
	// resources to insert from the arguments, then inserts them at the end so as to
	// wait as long as possible before turning into a write transaction.
	ImportResources(ctx context.Context, args resource.ImportResourcesArgs) error
}

type importOperation struct {
	modelmigration.BaseOperation

	resourceService ResourceService

	clock  clock.Clock
	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import resources"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.resourceService = service.NewService(
		state.NewState(
			scope.ModelDB(),
			i.clock,
			i.logger,
		),
		nil,
		i.logger)
	return nil
}

// Execute the import of application resources.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	var args []resource.ImportResourcesArg
	apps := model.Applications()
	for _, app := range apps {
		resources := app.Resources()
		appArgs := resource.ImportResourcesArg{
			ApplicationName: app.Name(),
		}

		for _, res := range resources {
			// The application revision may be unset, in this case we will get a
			// struct with zero values. IsZero is used on the timestamp to check
			// this, as this is the way it was done in 3.6.
			if appRevision := res.ApplicationRevision(); !appRevision.Timestamp().IsZero() {
				arg, err := importResourceRevision(res.Name(), appRevision)
				if err != nil {
					return errors.Errorf("importing resource %q: %w", res.Name(), err)
				}
				appArgs.Resources = append(appArgs.Resources, arg)
			}
		}

		for _, unit := range app.Units() {
			unitResources := unit.Resources()
			for _, res := range unitResources {
				unitRevision := res.Revision()
				appArgs.UnitResources = append(appArgs.UnitResources, resource.ImportUnitResourceInfo{
					ResourceName: res.Name(),
					UnitName:     unit.Name(),
					Timestamp:    unitRevision.Timestamp(),
				})
			}
		}
		args = append(args, appArgs)
	}

	err := i.resourceService.ImportResources(ctx, args)
	if err != nil {
		return errors.Errorf("setting resources: %w", err)
	}

	return nil
}

// importResourceRevision converts a ResourceRevision description into an
// argument for SetResource.
func importResourceRevision(name string, rev description.ResourceRevision) (resource.ImportResourceInfo, error) {
	if name == "" {
		return resource.ImportResourceInfo{}, errors.Errorf("got empty resource name: %w", resourceerrors.ResourceNameNotValid)
	}
	origin, err := charmresource.ParseOrigin(rev.Origin())
	if err != nil {
		return resource.ImportResourceInfo{}, errors.Errorf("parsing origin: %w: %w", resourceerrors.OriginNotValid, err)
	}
	revision := rev.Revision()
	switch origin {
	case charmresource.OriginStore:
		if revision < 0 {
			return resource.ImportResourceInfo{}, errors.Errorf(
				"expected resource with origin %q to have positive revision, got %d: %w",
				charmresource.OriginUpload, revision, resourceerrors.ResourceRevisionNotValid,
			)
		}
	case charmresource.OriginUpload:
		if revision > 0 {
			return resource.ImportResourceInfo{}, errors.Errorf(
				"expected resource with origin %q to have revision unset, got %d: %w",
				charmresource.OriginUpload, revision, resourceerrors.ResourceRevisionNotValid,
			)
		}
	default:
		return resource.ImportResourceInfo{}, errors.Errorf(
			"unexpected origin %s: %w", origin, resourceerrors.OriginNotValid,
		)
	}
	return resource.ImportResourceInfo{
		Name:      name,
		Origin:    origin,
		Revision:  rev.Revision(),
		Timestamp: rev.Timestamp(),
	}, nil
}
