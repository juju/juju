// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v9"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ImportService provides a subset of the access domain
// service methods needed for model permissions import.
type ImportService interface {
	// CreatePermission gives the user access per the provided spec.
	// If the user provided does not exist or is marked removed,
	// [accesserrors.PermissionNotFound] is returned.
	// If the user provided exists but is marked disabled,
	// [accesserrors.UserAuthenticationDisabled] is returned.
	// If a permission for the user and target key already exists,
	// [accesserrors.PermissionAlreadyExists] is returned.
	CreatePermission(ctx context.Context, spec corepermission.UserAccessSpec) (corepermission.UserAccess, error)
	// SetLastModelLogin will set the last login time for the user to the given
	// value. The following error types are possible from this function:
	// [accesserrors.UserNameNotValid] when the username supplied is not valid.
	// [accesserrors.UserNotFound] when the user cannot be found.
	// [modelerrors.NotFound] if no model by the given modelUUID exists.
	SetLastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID, time time.Time) error
}

type importOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import model user permissions"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ControllerDB(), i.logger))
	return nil
}

// Execute the import on the model user permissions contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := model.UUID()
	for _, u := range model.Users() {
		name, err := user.NewName(u.Name())
		if err != nil {
			return errors.Errorf("importing access for user %q: %w", u.Name(), err)
		}
		access := corepermission.Access(u.Access())
		if err := access.Validate(); err != nil {
			return errors.Errorf("importing access for user %q: %w", name, err)
		}
		_, err = i.service.CreatePermission(ctx, corepermission.UserAccessSpec{
			AccessSpec: corepermission.AccessSpec{
				Target: corepermission.ID{
					ObjectType: corepermission.Model,
					Key:        modelUUID,
				},
				Access: access,
			},
			User: name,
		})
		if err != nil && !errors.Is(err, accesserrors.PermissionAlreadyExists) {
			// If the permission already exists then it must be the model owner
			// who is granted admin access when the model is created.
			return errors.Errorf("creating permission for user %q: %w", name, err)
		}

		lastLogin := u.LastConnection()
		if !lastLogin.IsZero() {
			err := i.service.SetLastModelLogin(ctx, name, coremodel.UUID(modelUUID), lastLogin)
			if err != nil {
				return errors.Errorf("setting model last login for user %q: %w", name, err)
			}
		}

	}
	return nil
}
