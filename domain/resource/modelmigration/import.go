// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

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
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

// ImportService provides a subset of the resource domain service methods
// needed for resource import.
type ImportService interface {
	// ImportResources sets resources imported in migration. It first builds all the
	// resources to insert from the arguments, then inserts them at the end so as to
	// wait as long as possible before turning into a write transaction.
	ImportResources(ctx context.Context, args resource.ImportResourcesArgs) error

	// DeleteImportedResources deletes all imported resource associated with the
	// given applications during an import rollback.
	DeleteImportedResources(
		ctx context.Context,
		appNames []string,
	) error
}

type importOperation struct {
	modelmigration.BaseOperation

	resourceService ImportService

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
			if appRevision := res.ApplicationRevision(); appRevision != nil {
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
				arg, err := importResourceRevision(res.Name(), unitRevision)
				if err != nil {
					return errors.Errorf("importing unit resource %q: %w", res.Name(), err)
				}
				appArgs.UnitResources = append(appArgs.UnitResources, resource.ImportUnitResourceInfo{
					ImportResourceInfo: arg,
					UnitName:           unit.Name(),
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

// Rollback the resource import operation by deleting all imported resources
// associated with the imported applications.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	apps := model.Applications()
	var appNames []string
	for _, app := range apps {
		appNames = append(appNames, app.Name())
	}
	err := i.resourceService.DeleteImportedResources(ctx, appNames)
	if err != nil {
		return errors.Errorf("resource import rollback failed: %w", err)
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
		revision = -1
	default:
		return resource.ImportResourceInfo{}, errors.Errorf(
			"unexpected origin %s: %w", origin, resourceerrors.OriginNotValid,
		)
	}
	return resource.ImportResourceInfo{
		Name:      name,
		Origin:    origin,
		Revision:  revision,
		Timestamp: rev.Timestamp(),
	}, nil
}
