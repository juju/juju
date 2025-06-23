// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v10"

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

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&exportOperation{
		logger: logger,
	})
}

// ExportService provides a subset of the access domain
// service methods needed for model permissions export.
type ExportService interface {
	// ReadAllUserAccessForTarget return a slice of user access for all users
	// with access to the given target.
	// An [errors.NotValid] error is returned if the target is not valid. Any
	// errors from the state layer are passed through.
	// An [accesserrors.PermissionNotFound] error is returned if no permissions
	// can be found on the target.
	ReadAllUserAccessForTarget(ctx context.Context, target corepermission.ID) ([]corepermission.UserAccess, error)
	// LastModelLogin will return the last login time of the specified user.
	// The following error types are possible from this function:
	// - [accesserrors.UserNameNotValid] when the username is not valid.
	// - [accesserrors.UserNotFound] when the user cannot be found.
	// - [modelerrors.NotFound] if no model by the given modelUUID exists.
	// - [accesserrors.UserNeverAccessedModel] if there is no record of the user
	// accessing the model.
	LastModelLogin(ctx context.Context, name user.Name, modelUUID coremodel.UUID) (time.Time, error)
}

// exportOperation describes a way to execute a migration for
// exporting model user permissions.
type exportOperation struct {
	modelmigration.BaseOperation

	logger  logger.Logger
	service ExportService
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export model user permissions"
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(
		state.NewState(scope.ControllerDB(), e.logger),
	)
	return nil
}

// Execute the export, adding the model user permissions to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	modelUUID := model.UUID()
	userAccesses, err := e.service.ReadAllUserAccessForTarget(ctx, corepermission.ID{
		ObjectType: corepermission.Model,
		Key:        modelUUID,
	})
	if err != nil {
		return errors.Errorf("getting user access on model: %w", err)
	}
	for _, userAccess := range userAccesses {
		lastModelLogin, err := e.service.LastModelLogin(ctx, userAccess.UserName, coremodel.UUID(modelUUID))
		if err != nil && !errors.Is(err, accesserrors.UserNeverAccessedModel) {
			return errors.Errorf("getting user last login on model: %w", err)
		}
		userName := userAccess.UserName.Name()
		var createdBy string
		if !userAccess.CreatedBy.IsZero() {
			createdBy = userAccess.CreatedBy.Name()
		}
		arg := description.UserArgs{
			Name:           userName,
			DisplayName:    userAccess.DisplayName,
			CreatedBy:      createdBy,
			DateCreated:    userAccess.DateCreated,
			LastConnection: lastModelLogin,
			Access:         string(userAccess.Access),
		}
		model.AddUser(arg)
	}
	return nil
}
