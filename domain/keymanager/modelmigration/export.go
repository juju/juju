// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/user"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/keymanager/service"
	"github.com/juju/juju/domain/keymanager/state"
	"github.com/juju/juju/internal/errors"
)

// exportOperation is the type used to describe the export operation for a
// model's authorized keys.
type exportOperation struct {
	modelmigration.BaseOperation

	service     ExportService
	userService UserService
}

// ExportService represents the service methods needed for exporting the
// authorized keys of a model during migration.
type ExportService interface {
	// GetAllUserPublicKeys returns all of the public keys in the model for each
	// user grouped by [user.UUID].
	GetAllUsersPublicKeys(context.Context) (map[user.UUID][]string, error)
}

// Execute the migration of the model's authorized keys.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	usersKeys, err := e.service.GetAllUsersPublicKeys(ctx)
	if err != nil {
		return errors.Errorf(
			"cannot export authorized keys for model while getting all user keys: %w",
			err,
		)
	}

	userIds := make([]user.UUID, 0, len(usersKeys))
	for userId := range usersKeys {
		userIds = append(userIds, userId)
	}

	userIdNameMap, err := e.userService.GetUsernamesForIds(ctx, userIds...)
	if err != nil {
		return errors.Errorf(
			"cannot export authorized keys for model while mapping user ids to user names: %w",
			err,
		)
	}

	for userId, userKeys := range usersKeys {
		model.AddAuthorizedKeys(description.UserAuthorizedKeysArgs{
			Username:       userIdNameMap[userId].Name(),
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
	e.service = service.NewService(
		state.NewState(scope.ModelDB()),
	)
	e.userService = accessservice.NewUserService(accessstate.NewUserState(scope.ControllerDB()))
	return nil
}
