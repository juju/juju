// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"fmt"

	"github.com/juju/description/v5"
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	domainmodel "github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/environs/config"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ModelService defines the model service used to import models from another
// controller to this one.
type ModelService interface {
	// CreateModel is responsible for creating a new model that is being
	// imported.
	CreateModel(context.Context, domainmodel.ModelCreationArgs) (coremodel.UUID, func(context.Context) error, error)
	// ModelType returns the type of the model.
	ModelType(context.Context, coremodel.UUID) (coremodel.ModelType, error)
	// DeleteModel is responsible for removing a model from the system.
	DeleteModel(context.Context, coremodel.UUID) error
}

type ReadOnlyModelService interface {
	// CreateModel is responsible for creating a new read only model
	// that is being imported.
	CreateModel(context.Context, domainmodel.ReadOnlyModelCreationArgs) error

	// DeleteModel is responsible for removing a read only model from the system.
	DeleteModel(context.Context, coremodel.UUID) error
}

// UserService defines the user service used for model migration.
type UserService interface {
	// GetUserByName will find active users specified by the user name and
	// return the associated user object.
	GetUserByName(context.Context, string) (coreuser.User, error)
}

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// ControllerConfigService defines the controller config service used for model
// migration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// importOperation implements the steps to import models from another controller
// into the current controller. importOperation assumes that data related to the
// model such as cloud credentials and users have already been imported or
// created in the system.
type importOperation struct {
	modelmigration.BaseOperation

	modelService            ModelService
	readOnlyModelService    ReadOnlyModelService
	userService             UserService
	controllerConfigService ControllerConfigService

	logger Logger
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.modelService = modelservice.NewService(
		modelstate.NewState(scope.ControllerDB()),
		modelservice.DefaultAgentBinaryFinder(),
	)
	i.readOnlyModelService = modelservice.NewModelService(
		modelstate.NewModelState(scope.ModelDB()),
	)
	i.userService = accessservice.NewService(accessstate.NewState(scope.ControllerDB(), i.logger))
	i.controllerConfigService = controllerconfigservice.NewService(
		controllerconfigstate.NewState(scope.ControllerDB()),
	)
	return nil
}

// Execute will attempt to import the model into the current system  based on
// the description received.
//
// If model name or uuid are undefined or are not strings in the model config an
// error satisfying [errors.NotValid] will be returned.
// If the user specified for the model cannot be found an error satisfying
// [accesserrors.NotFound] will be returned.
func (i importOperation) Execute(ctx context.Context, model description.Model) error {
	modelName, uuid, err := i.getModelNameAndUUID(model)
	if err != nil {
		return fmt.Errorf("importing model during migration %w", errors.NotValid)
	}

	user, err := i.userService.GetUserByName(ctx, model.Owner().Name())
	if errors.Is(err, accesserrors.UserNotFound) {
		return fmt.Errorf("cannot import model %q with uuid %q, %w for name %q",
			modelName, uuid, accesserrors.UserNotFound, model.Owner().Name(),
		)
	} else if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration, finding user %q: %w",
			modelName, uuid, model.Owner().Name(), err,
		)
	}

	credential := credential.Key{}
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
		Name:         modelName,
		Owner:        user.UUID,
		UUID:         coremodel.UUID(uuid),
	}

	controllerConfig, err := i.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration, getting controller uuid: %w",
			modelName, uuid, err,
		)
	}

	// NOTE: Try to get all things that can fail before creating the model in
	// the database.

	createdModelUUID, finaliser, err := i.modelService.CreateModel(ctx, args)
	if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration: %w",
			modelName, uuid, err,
		)
	}
	if createdModelUUID != args.UUID {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration, created model uuid %q does not match expected uuid %q",
			modelName, uuid, createdModelUUID, args.UUID,
		)
	}

	// When importing a model, we need to move the model from the prior
	// controller to the current controller. This is done, during the import
	// operation, so it never changes once the model is up and running.
	controllerUUID := coremodel.UUID(controllerConfig.ControllerUUID())

	modelType, err := i.modelService.ModelType(ctx, createdModelUUID)
	if err != nil {
		return fmt.Errorf(
			"importing model %q with uuid %q during migration, getting model type: %w",
			modelName, uuid, err,
		)
	}

	// If the model is read only, we need to create a read only model.
	err = i.readOnlyModelService.CreateModel(ctx, args.AsReadOnly(controllerUUID, modelType))
	if err != nil {
		return fmt.Errorf(
			"importing read only model %q with uuid %q during migration: %w",
			modelName, uuid, err,
		)
	}

	// NOTE: If we add any more steps to the import operation, we should
	// consider adding a rollback operation to undo the changes made by the
	// import operation.

	// finaliser needs to be called as the last operation to say that we are
	// happy that the model is ready to rock and roll.
	if err := finaliser(ctx); err != nil {
		return fmt.Errorf(
			"finalising imported model %q with uuid %q: %w", modelName, uuid, err,
		)
	}

	return nil
}

// Rollback will attempt to rollback the import operation if it was
// unsuccessful.
func (i importOperation) Rollback(ctx context.Context, model description.Model) error {
	// Attempt to rollback the model database if it was created.
	modelName, uuid, err := i.getModelNameAndUUID(model)
	if err != nil {
		return fmt.Errorf("rollback of model during migration %w", errors.NotValid)
	}

	modelUUID := coremodel.UUID(uuid)

	// If the model isn't found, we can simply ignore the error.
	if err := i.modelService.DeleteModel(ctx, modelUUID); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return fmt.Errorf(
			"rollback of model %q with uuid %q during migration: %w",
			modelName, uuid, err,
		)
	}

	// If the read only model isn't found, we can simply ignore the error.
	if err := i.readOnlyModelService.DeleteModel(ctx, modelUUID); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return fmt.Errorf(
			"rollback of read only model %q with uuid %q during migration: %w",
			modelName, uuid, err,
		)
	}

	return nil
}

func (i importOperation) getModelNameAndUUID(model description.Model) (string, string, error) {
	modelConfig := model.Config()
	if modelConfig == nil {
		return "", "", errors.New("model config is empty")
	}

	modelNameI, exists := modelConfig[config.NameKey]
	if !exists {
		return "", "", fmt.Errorf("no model name found in model config")
	}

	modelNameS, ok := modelNameI.(string)
	if !ok {
		return "", "", fmt.Errorf("establishing model name type as string. Got unknown type")
	}

	uuidI, exists := modelConfig[config.UUIDKey]
	if !exists {
		return "", "", fmt.Errorf("no model uuid found in model config")
	}

	uuidS, ok := uuidI.(string)
	if !ok {
		return "", "", fmt.Errorf("establishing model uuid type as string. Got unknown type")
	}

	return modelNameS, uuidS, nil
}
