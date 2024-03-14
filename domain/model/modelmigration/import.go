// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v5"
	"github.com/juju/errors"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/model"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	usererrors "github.com/juju/juju/domain/user/errors"
	userservice "github.com/juju/juju/domain/user/service"
	userstate "github.com/juju/juju/domain/user/state"
	"github.com/juju/juju/environs/config"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// ImportService defines the model service used to import models from another
// controller to this one.
type ImportService interface {
	// CreateModel is responsible for creating a new model that is being imported.
	CreateModel(context.Context, model.ModelCreationArgs) (coremodel.UUID, error)
}

// UserService defines the user service used for model migration.
type UserService interface {
	// GetUserByName will find active users specified by the user name and
	// return the associated user object.
	GetUserByName(context.Context, string) (coreuser.User, error)
}

// importOperation implements the steps to import models from another controller
// into the current controller. importOperation assumes that data related to the
// model such as cloud credentials and users have already been imported or
// created in the system.
type importOperation struct {
	modelmigration.BaseOperation

	service     ImportService
	userService UserService
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ControllerDB()),
		service.DefaultAgentBinaryFinder(),
	)

	i.userService = userservice.NewService(userstate.NewState(scope.ControllerDB()))

	return nil
}

// Execute will attempt to import the model into the current system  based on
// the description received.
//
// If model name or uuid are undefined or are not strings in the model config an
// error satisfying [errors.NotValid] will be returned.
// If the user specified for the model cannot be found an error satisfying
// [usererrors.NotFound] will be returned.
func (i importOperation) Execute(ctx context.Context, model description.Model) error {
	modelConfig := model.Config()
	if modelConfig == nil {
		return errors.New("model config is empty")
	}

	modelNameI, exists := modelConfig[config.NameKey]
	if !exists {
		return fmt.Errorf(
			"importing model during migration %w, no model name found in model config",
			errors.NotValid,
		)
	}

	modelNameS, ok := modelNameI.(string)
	if !ok {
		return fmt.Errorf(
			"importing model during migration %w, establishing model name type as string. Got unknown type",
			errors.NotValid,
		)
	}

	uuidI, exists := modelConfig[config.UUIDKey]
	if !exists {
		return fmt.Errorf(
			"importing model during migration %w, no model uuid found in model config",
			errors.NotValid,
		)
	}

	uuidS, ok := uuidI.(string)
	if !ok {
		return fmt.Errorf(
			"importing model during migration %w, establishing model uuid type as string. Got unknown type",
			errors.NotValid,
		)
	}

	user, err := i.userService.GetUserByName(ctx, model.Owner().Name())
	if errors.Is(err, usererrors.NotFound) {
		return fmt.Errorf("cannot import model %q with uuid %q, %w for name %q",
			modelNameS, uuidS, usererrors.NotFound, model.Owner().Name(),
		)
	} else if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration, finding user %q: %w",
			modelNameS, uuidS, model.Owner().Name(), err,
		)
	}

	credential := credential.ID{}
	// CloudCredential could be nil
	if model.CloudCredential() != nil {
		credential.Name = model.CloudCredential().Name()
		credential.Cloud = model.CloudCredential().Cloud()
		credential.Owner = model.CloudCredential().Owner()
	}

	args := domainmodel.ModelCreationArgs{
		AgentVersion: model.LatestToolsVersion(),
		Cloud:        model.Cloud(),
		CloudRegion:  model.CloudRegion(),
		Credential:   credential,
		Name:         modelNameS,
		Owner:        user.UUID,
		Type:         coremodel.ModelType(model.Type()),
		UUID:         coremodel.UUID(uuidS),
	}

	_, err = i.service.CreateModel(ctx, args)
	if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration: %w",
			modelNameS, uuidS, err,
		)
	}

	return nil
}
