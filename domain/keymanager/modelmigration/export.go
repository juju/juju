// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/keymanager/state"
	"github.com/juju/juju/internal/errors"
)

type exportServiceGetterFunc func(model.UUID) ExportService

// exportOperation is the type used to describe the export operation for a
// model's authorized keys.
type exportOperation struct {
	modelmigration.BaseOperation

	serviceGetter exportServiceGetterFunc
}

// ExportService represents the service methods needed for exporting the
// authorized keys of a model during migration.
type ExportService interface {
	// GetAllUserPublicKeys returns all of the public keys in the model for each
	// user grouped by [user.UUID].
	GetAllUsersPublicKeys(context.Context) (map[user.Name][]string, error)
}

// Execute the migration of the model's authorized keys.
func (e *exportOperation) Execute(ctx context.Context, m description.Model) error {
	modelUUID := model.UUID(m.UUID())
	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf(
			"exporting authorized keys for model %q: %w",
			m.UUID(), err,
		)
	}

	usersKeys, err := e.serviceGetter(modelUUID).GetAllUsersPublicKeys(ctx)
	if err != nil {
		return errors.Errorf(
			"exporting authorized keys for model while getting all users keys: %w",
			err,
		)
	}

	for userName, userKeys := range usersKeys {
		m.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
			Username:       userName.String(),
			AuthorizedKeys: userKeys,
		})
	}
	return nil
}

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// Name returns  the user readable name for this export operation.
func (e *exportOperation) Name() string {
	return "export model authorized keys"
}

// Setup the export operation, this will ensure the service is created and ready
// to be used.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.serviceGetter = func(modelUUID model.UUID) ExportService {
		return service.NewService(
			modelUUID,
			state.NewState(scope.ControllerDB()),
		)
	}
	return nil
}
