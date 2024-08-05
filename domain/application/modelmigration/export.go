// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v8"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/internal/charm"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the application domain
// service methods needed for application export.
type ExportService interface {
	// GetCharmID returns a charm ID by name. It returns an error if the charm
	// can not be found by the name.
	// This can also be used as a cheap way to see if a charm exists without
	// needing to load the charm metadata.
	GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error)

	// GetCharm returns the charm metadata for the given charm ID.
	GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, error)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export applications"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ModelDB(), e.logger),
		nil,
		e.logger,
	)
	return nil
}

// Execute the export, adding the application to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	// We don't currently export applications, that'll be done in a future.
	// For now we need to ensure that we write the charms on the applications.

	for _, app := range model.Applications() {
		// For every application, ensure that the charm is written to the model.
		// This will still be required in the future, it'll just be done in
		// one step.

		metadata := app.CharmMetadata()
		if metadata != nil {
			// The application already has a charm, nothing to do.
			continue
		}

		// To locate a charm, we currently need to know the charm URL of the
		// application. This is not going to work like this in the future,
		// we can use the charm_uuid instead.

		curl, err := internalcharm.ParseURL(app.CharmURL())
		if err != nil {
			return fmt.Errorf("cannot parse charm URL %q: %v", app.CharmURL(), err)
		}

		charmID, err := e.service.GetCharmID(ctx, charm.GetCharmArgs{
			Name:     curl.Name,
			Revision: &curl.Revision,
		})
		if err != nil {
			return fmt.Errorf("cannot get charm ID for %q: %v", app.CharmURL(), err)
		}

		charm, err := e.service.GetCharm(ctx, charmID)
		if err != nil {
			return fmt.Errorf("cannot get charm %q: %v", charmID, err)
		}

		// TODO (stickupkid): Export the charm to the model description.
		_ = charm
	}
	return nil
}
