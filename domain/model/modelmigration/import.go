// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	constraintsmigration "github.com/juju/juju/domain/constraints/modelmigration"
	domainmodel "github.com/juju/juju/domain/model"
	modelservice "github.com/juju/juju/domain/model/service"
	modelmigrationservice "github.com/juju/juju/domain/model/service/migration"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	statemodel "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterModelImport register's a new model migration importer into the
// supplied coordinator.
func RegisterModelImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	// The model import operation must always come first!
	coordinator.Add(&importModelOperation{
		clock:  clock,
		logger: logger,
	})
	coordinator.Add(&importModelConstraintsOperation{
		logger: logger,
	})
}

// RegisterModelActivationImport register's a new model migration importer that
// only handles model activation into the supplied coordinator.
func RegisterModelActivationImport(coordinator Coordinator, logger logger.Logger) {
	coordinator.Add(&importModelActivatorOperation{
		logger: logger,
	})
}

// ModelImportService defines the model service used to import models from
// another controller to this one.
type ModelImportService interface {
	// ImportModel is responsible for creating a new model that is being
	// imported.
	ImportModel(context.Context, domainmodel.ModelImportArgs) error

	// ActivateModel marks the model as active after a successful import or
	// migration.
	ActivateModel(context.Context, coremodel.UUID) error
}

// ModelDetailService defines a service for interacting with the
// model information found in a model database.
type ModelDetailService interface {
	// within the model database using the specified agent version and agent stream.
	//
	// The following error types can be expected to be returned:
	// - [modelerrors.AlreadyExists] when the model uuid is already in use.
	// - [modelerrors.AgentVersionNotSupported] when the agent version is not
	// supported.
	// - [coreerrors.NotValid] when the agent stream is not valid.
	CreateModelWithAgentVersionStream(context.Context, semversion.Number, agentbinary.AgentStream) error

	// CreateImportingModelWithAgentVersionStream is responsible for creating a new
	// model within the model database during model import, using the input agent
	// version and agent stream. This method creates the model and marks it as
	// importing in a single atomic transaction.
	//
	// The following error types can be expected to be returned:
	// - [modelerrors.AlreadyExists] when the model uuid is already in use.
	// - [modelerrors.AgentVersionNotSupported] when the agent version is not
	// supported.
	// - [coreerrors.NotValid] when the agent stream is not valid.
	CreateImportingModelWithAgentVersionStream(context.Context, semversion.Number, agentbinary.AgentStream) error

	// SetModelConstraints sets the model constraints to the new values removing
	// any previously set constraints.
	//
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/network/errors.SpaceNotFound]: when the space
	// being set in the model constraint doesn't exist.
	// - [github.com/juju/juju/domain/machine/errors.InvalidContainerType]: when
	// the container type being set in the model constraint isn't valid.
	SetModelConstraints(context.Context, coreconstraints.Value) error
}

// ModelDetailServiceFunc is responsible for creating and returning a
// [ModelDetailService] for the specified model id. We use this func so that
// we can late bind the service during the import operation.
type ModelDetailServiceFunc = func(coremodel.UUID) ModelDetailService

// UserService defines the user service used for model migration.
type UserService interface {
	// GetUserByName will find active users specified by the user name and
	// return the associated user object.
	GetUserByName(context.Context, coreuser.Name) (coreuser.User, error)
}

// importModelOperation implements the steps to import a model from another
// controller into the current controller. importModelOperation assumes that
// data related to the model such as cloud credentials and users have already
// been imported or created in the system.
type importModelOperation struct {
	modelmigration.BaseOperation

	modelImportService     ModelImportService
	modelDetailServiceFunc ModelDetailServiceFunc
	userService            UserService

	clock  clock.Clock
	logger logger.Logger
}

// modelDetailServiceGetter constructs a [ModelDetailServiceFunc] from the
// supplied [modelmigration.Scope].
func modelDetailServiceGetter(
	scope modelmigration.Scope,
	logger logger.Logger,
) ModelDetailServiceFunc {
	return func(id coremodel.UUID) ModelDetailService {
		return modelservice.NewModelService(
			id,
			statecontroller.NewState(scope.ControllerDB()),
			statemodel.NewState(scope.ModelDB(), logger),
			modelservice.EnvironVersionProviderGetter(),
			modelservice.DefaultAgentBinaryFinder(),
		)
	}
}

// Name returns the name of this operation.
func (i *importModelOperation) Name() string {
	return "import model"
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importModelOperation) Setup(scope modelmigration.Scope) error {
	i.modelImportService = modelmigrationservice.NewMigrationService(
		statecontroller.NewState(scope.ControllerDB()),
		i.logger,
	)

	i.modelDetailServiceFunc = modelDetailServiceGetter(scope, i.logger)
	i.userService = accessservice.NewService(accessstate.NewState(scope.ControllerDB(), i.clock, i.logger), i.clock)
	return nil
}

// Execute will attempt to import the model into the current system  based on
// the description received.
//
// If model name or uuid are undefined or are not strings in the model config an
// error satisfying [errors.NotValid] will be returned.
// If the user specified for the model cannot be found an error satisfying
// [accesserrors.NotFound] will be returned.
func (i *importModelOperation) Execute(ctx context.Context, model description.Model) error {
	modelName, modelUUID, err := getModelNameAndUUID(model)
	if err != nil {
		return errors.Errorf("%w", err).Add(coreerrors.NotValid)
	}

	owner, err := coreuser.NewName(model.Owner())
	if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q: invalid owner: %w",
			modelName, modelUUID, err)

	}
	user, err := i.userService.GetUserByName(ctx, owner)
	if errors.Is(err, accesserrors.UserNotFound) {
		return errors.Errorf("importing model %q with uuid %q, %w for name %q",
			modelName, modelUUID, accesserrors.UserNotFound, model.Owner(),
		)
	} else if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q during migration, finding user %q: %w",
			modelName, modelUUID, model.Owner(), err,
		)
	}

	cred := credential.Key{}
	// CloudCredential could be nil.
	if model.CloudCredential() != nil {
		cred.Name = model.CloudCredential().Name()
		cred.Cloud = model.CloudCredential().Cloud()
		cred.Owner, err = coreuser.NewName(model.CloudCredential().Owner())
		if err != nil {
			return errors.Errorf(
				"importing model %q with uuid %q: model cloud credential owner: %w",
				modelName, modelUUID, err)
		}
	}

	// TODO: handle this magic in juju/description, preferably sending the agent-version
	// over the wire as a top-level field on the model, removing it from model config.
	agentVersionStr, ok := model.Config()[config.AgentVersionKey].(string)
	if !ok {
		return errors.Errorf(
			"importing model %q with uuid %q: agent-version missing from model config",
			modelName, modelUUID)
	}
	agentVersion, err := semversion.Parse(agentVersionStr)
	if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q: cannot parse agent-version: %w",
			modelName, modelUUID, err)
	}

	// If no agent stream exists in the model config we will default to
	// released.
	agentStream := agentbinary.AgentStreamReleased
	if agentStreamStr, ok := model.Config()[config.AgentStreamKey].(string); ok {
		agentStream = agentbinary.AgentStream(agentStreamStr)
	}

	args := domainmodel.ModelImportArgs{
		GlobalModelCreationArgs: domainmodel.GlobalModelCreationArgs{
			Cloud:       model.Cloud(),
			CloudRegion: model.CloudRegion(),
			Credential:  cred,
			Name:        modelName,
			Qualifier:   coremodel.QualifierFromUserTag(names.NewUserTag(model.Owner())),
			AdminUsers:  []coreuser.UUID{user.UUID},
		},
		UUID: modelUUID,
	}

	if err := i.modelImportService.ImportModel(ctx, args); err != nil {
		return errors.Errorf(
			"importing model %q with id %q during migration: %w",
			modelName, modelUUID, err,
		)
	}

	// NOTE: If we add any more steps to the import operation, we should
	// consider adding a rollback operation to undo the changes made by the
	// import operation.

	// We need to establish the read only model information in the model database.
	// This also marks the model as importing in the model_migrating table so that
	// charm uploads during the migration can be properly handled.
	err = i.modelDetailServiceFunc(modelUUID).CreateImportingModelWithAgentVersionStream(ctx, agentVersion, agentStream)
	if err != nil {
		return errors.Errorf(
			"importing read only model %q with uuid %q during migration: %w",
			modelName, args.UUID, err,
		)
	}

	return nil
}

// importModelConstraintsOperation implements the steps to import a model's
// constraints.
type importModelConstraintsOperation struct {
	modelmigration.BaseOperation
	modelDetailServiceFunc ModelDetailServiceFunc
	logger                 logger.Logger
}

// Name returns the name of this operation.
func (i *importModelConstraintsOperation) Name() string {
	return "import model constraints"
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import of a model's constraints.
func (i *importModelConstraintsOperation) Setup(scope modelmigration.Scope) error {
	i.modelDetailServiceFunc = modelDetailServiceGetter(scope, i.logger)
	return nil
}

// Execute will attempt to import the model constraints from description. If no
// constraints have been set on the description then Execute will not attempt to
// set any constraints for the model.
func (i *importModelConstraintsOperation) Execute(
	ctx context.Context,
	model description.Model,
) error {
	descCons := model.Constraints()

	// It is possible that the constraints interface from description can be nil
	// if it was never set. This isn't a documented contract of the description
	// package but we include this check here for safety.
	if descCons == nil {
		return nil
	}
	cons := constraintsmigration.DecodeConstraints(descCons)
	// If no constraints are set we will noop from here.
	if coreconstraints.IsEmpty(&cons) {
		return nil
	}

	modelUUID := coremodel.UUID(model.UUID())
	err := i.modelDetailServiceFunc(modelUUID).SetModelConstraints(ctx, cons)

	if err != nil {
		return errors.Errorf("importing model %q constraints: %w", modelUUID, err)
	}

	return nil
}

type importModelActivatorOperation struct {
	modelmigration.BaseOperation

	modelImportService ModelImportService

	logger logger.Logger
}

// Name returns the name of this operation.
func (i *importModelActivatorOperation) Name() string {
	return "import model activator"
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importModelActivatorOperation) Setup(scope modelmigration.Scope) error {
	i.modelImportService = modelmigrationservice.NewMigrationService(
		statecontroller.NewState(scope.ControllerDB()),
		i.logger,
	)
	return nil
}

// Execute will attempt to activate the model in the current system based on
// the description received.
//
// If model name or uuid are undefined or are not strings in the model config an
// error satisfying [errors.NotValid] will be returned.
func (i *importModelActivatorOperation) Execute(ctx context.Context, model description.Model) error {
	modelName, modelUUID, err := getModelNameAndUUID(model)
	if err != nil {
		return errors.Errorf("importing model during migration %w", coreerrors.NotValid)
	}

	if err := i.modelImportService.ActivateModel(ctx, modelUUID); err != nil {
		return errors.Errorf(
			"activating model %q with id %q during migration: %w",
			modelName, modelUUID, err,
		)
	}

	return nil
}

func getModelNameAndUUID(model description.Model) (string, coremodel.UUID, error) {
	modelConfig := model.Config()
	if modelConfig == nil {
		return "", "", errors.New("model config is empty")
	}

	modelName, ok := modelConfig[config.NameKey].(string)
	if !ok {
		return "", "", errors.Errorf("no model name found in model config")
	}

	uuid, ok := modelConfig[config.UUIDKey].(string)
	if !ok {
		return "", "", errors.Errorf("no model uuid found in model config")
	}

	return modelName, coremodel.UUID(uuid), nil
}
