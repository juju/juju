// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
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
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ModelService defines the model service used to import models from another
// controller to this one.
type ModelService interface {
	// ImportModel is responsible for creating a new model that is being
	// imported.
	ImportModel(context.Context, domainmodel.ModelImportArgs) (func(context.Context) error, error)

	// DeleteModel is responsible for removing a model from the system.
	DeleteModel(context.Context, coremodel.UUID, ...domainmodel.DeleteModelOption) error
}

// ReadOnlyModelService defines a service for interacting with the read only
// model information found in a model database.
type ReadOnlyModelService interface {
	// CreateModel is responsible for creating a new read only model
	// that is being imported.
	CreateModel(context.Context, uuid.UUID) error

	// DeleteModel is responsible for removing a read only model from the system.
	DeleteModel(context.Context) error
}

// ReadOnlyModelServiceFunc is responsible for creating and returning a
// [ReadOnlyModelService] for the specified model id. We use this func so that
// we can late bind the service during the import operation.
type ReadOnlyModelServiceFunc = func(coremodel.UUID) ReadOnlyModelService

// UserService defines the user service used for model migration.
type UserService interface {
	// GetUserByName will find active users specified by the user name and
	// return the associated user object.
	GetUserByName(context.Context, coreuser.Name) (coreuser.User, error)
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

	modelService             ModelService
	readOnlyModelServiceFunc ReadOnlyModelServiceFunc
	userService              UserService
	controllerConfigService  ControllerConfigService

	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import model"
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.modelService = modelservice.NewService(
		modelstate.NewState(scope.ControllerDB()),
		scope.ModelDeleter(),
		modelservice.DefaultAgentBinaryFinder(),
		i.logger,
	)
	i.readOnlyModelServiceFunc = func(id coremodel.UUID) ReadOnlyModelService {
		return modelservice.NewModelService(
			id,
			modelstate.NewState(scope.ControllerDB()),
			modelstate.NewModelState(scope.ModelDB(), i.logger),
		)
	}
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
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	modelName, modelID, err := i.getModelNameAndID(model)
	if err != nil {
		return errors.Errorf("importing model during migration %w", coreerrors.NotValid)
	}

	user, err := i.userService.GetUserByName(ctx, coreuser.NameFromTag(model.Owner()))
	if errors.Is(err, accesserrors.UserNotFound) {
		return errors.Errorf("cannot import model %q with uuid %q, %w for name %q",
			modelName, modelID, accesserrors.UserNotFound, model.Owner().Name())

	} else if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q during migration, finding user %q: %w",
			modelName, modelID, model.Owner().Name(), err)

	}

	cred := credential.Key{}
	// CloudCredential could be nil.
	if model.CloudCredential() != nil {
		cred.Name = model.CloudCredential().Name()
		cred.Cloud = model.CloudCredential().Cloud()
		cred.Owner, err = coreuser.NewName(model.CloudCredential().Owner())
		if err != nil {
			return errors.Errorf(
				"cannot import model %q with uuid %q: model cloud credential owner: %w",
				modelName, modelID, err)

		}
	}

	// TODO: handle this magic in juju/description, preferably sending the agent-version
	// over the wire as a top-level field on the model, removing it from model config.
	agentVersionStr, ok := model.Config()[config.AgentVersionKey].(string)
	if !ok {
		return errors.Errorf(
			"cannot import model %q with uuid %q: agent-version missing from model config",
			modelName, modelID)

	}
	agentVersion, err := version.Parse(agentVersionStr)
	if err != nil {
		return errors.Errorf(
			"cannot import model %q with uuid %q: cannot parse agent-version: %w",
			modelName, modelID, err)

	}

	args := domainmodel.ModelImportArgs{
		ModelCreationArgs: domainmodel.ModelCreationArgs{
			AgentVersion: agentVersion,
			Cloud:        model.Cloud(),
			CloudRegion:  model.CloudRegion(),
			Credential:   cred,
			Name:         modelName,
			Owner:        user.UUID,
		},
		ID: modelID,
	}

	controllerConfig, err := i.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q during migration, getting controller uuid: %w",
			modelName, modelID, err)

	}

	// NOTE: Try to get all things that can fail before creating the model in
	// the database.
	activator, err := i.modelService.ImportModel(ctx, args)
	if err != nil {
		return errors.Errorf(
			"importing model %q with id %q during migration: %w",
			modelName, modelID, err)

	}

	// NOTE: If we add any more steps to the import operation, we should
	// consider adding a rollback operation to undo the changes made by the
	// import operation.

	// activator needs to be called as the last operation to say that we are
	// happy that the model is ready to rock and roll.
	if err := activator(ctx); err != nil {
		return errors.Errorf(
			"activating imported model %q with uuid %q: %w", modelName, modelID, err)

	}

	// When importing a model, we need to move the model from the prior
	// controller to the current controller. This is done, during the import
	// operation, so it never changes once the model is up and running.

	controllerUUID, err := uuid.UUIDFromString(controllerConfig.ControllerUUID())
	if err != nil {
		return errors.Errorf("parsing controller uuid %q: %w", controllerConfig.ControllerUUID(), err)
	}

	// We need to establish the read only model information in the model database.
	err = i.readOnlyModelServiceFunc(modelID).CreateModel(ctx, controllerUUID)
	if err != nil {
		return errors.Errorf(
			"importing read only model %q with uuid %q during migration: %w",
			modelName, controllerUUID, err)

	}

	return nil
}

// Rollback will attempt to roll back the import operation if it was
// unsuccessful.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	// Attempt to roll back the model database if it was created.
	modelName, modelID, err := i.getModelNameAndID(model)
	if err != nil {
		return errors.Errorf("rollback of model during migration %w", coreerrors.NotValid)
	}

	// If the model is not found, or the underlying db is not found, we can
	// ignore the error.
	if err := i.readOnlyModelServiceFunc(modelID).DeleteModel(ctx); err != nil &&
		!errors.Is(err, modelerrors.NotFound) &&
		!errors.Is(err, coredatabase.ErrDBNotFound) {
		return errors.Errorf(
			"rollback of read only model %q with uuid %q during migration: %w",
			modelName, modelID, err)

	}

	// If the model isn't found, we can simply ignore the error.
	if err := i.modelService.DeleteModel(ctx, modelID, domainmodel.WithDeleteDB()); err != nil &&
		!errors.Is(err, modelerrors.NotFound) &&
		!errors.Is(err, coredatabase.ErrDBNotFound) {
		return errors.Errorf(
			"rollback of model %q with uuid %q during migration: %w",
			modelName, modelID, err)

	}

	return nil
}

func (i *importOperation) getModelNameAndID(model description.Model) (string, coremodel.UUID, error) {
	modelConfig := model.Config()
	if modelConfig == nil {
		return "", "", errors.New("model config is empty")
	}

	modelNameI, exists := modelConfig[config.NameKey]
	if !exists {
		return "", "", errors.Errorf("no model name found in model config")
	}

	modelNameS, ok := modelNameI.(string)
	if !ok {
		return "", "", errors.Errorf("establishing model name type as string. Got unknown type")
	}

	uuidI, exists := modelConfig[config.UUIDKey]
	if !exists {
		return "", "", errors.Errorf("no model uuid found in model config")
	}

	uuidS, ok := uuidI.(string)
	if !ok {
		return "", "", errors.Errorf("establishing model uuid type as string. Got unknown type")
	}

	return modelNameS, coremodel.UUID(uuidS), nil
}
