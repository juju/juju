// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coreinstance "github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	domainmodel "github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger) {
	// The model import operation must always come first!
	coordinator.Add(&importModelOperation{
		logger: logger,
	})

	coordinator.Add(&importModelConstraintsOperation{
		logger: logger,
	})
}

// ModelImportService defines the model service used to import models from
// another controller to this one.
type ModelImportService interface {
	// ImportModel is responsible for creating a new model that is being
	// imported.
	ImportModel(context.Context, domainmodel.ModelImportArgs) (func(context.Context) error, error)

	// DeleteModel is responsible for removing a model from the system.
	DeleteModel(context.Context, coremodel.UUID, ...domainmodel.DeleteModelOption) error
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

	// DeleteModel is responsible for removing a read only model from the system.
	DeleteModel(context.Context) error

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

	logger logger.Logger
}

// importModelConstraintsOperation implements the steps to import a model's
// constraints.
type importModelConstraintsOperation struct {
	modelmigration.BaseOperation
	modelDetailServiceFunc ModelDetailServiceFunc
	logger                 logger.Logger
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
			modelstate.NewState(scope.ControllerDB()),
			modelstate.NewModelState(scope.ModelDB(), logger),
			modelservice.EnvironVersionProviderGetter(),
			modelservice.DefaultAgentBinaryFinder(),
		)
	}
}

// Name returns the name of this operation.
func (i *importModelOperation) Name() string {
	return "import model"
}

// Name returns the name of this operation.
func (i *importModelConstraintsOperation) Name() string {
	return "import model constraints"
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import.
func (i *importModelOperation) Setup(scope modelmigration.Scope) error {
	i.modelImportService = modelservice.NewService(
		modelstate.NewState(scope.ControllerDB()),
		scope.ModelDeleter(),
		i.logger,
	)

	i.modelDetailServiceFunc = modelDetailServiceGetter(scope, i.logger)
	i.userService = accessservice.NewService(accessstate.NewState(scope.ControllerDB(), i.logger))
	return nil
}

// Setup is responsible for taking the model migration scope and creating the
// needed services used during import of a model's constraints.
func (i *importModelConstraintsOperation) Setup(scope modelmigration.Scope) error {
	i.modelDetailServiceFunc = modelDetailServiceGetter(scope, i.logger)
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
	modelName, modelID, err := i.getModelNameAndID(model)
	if err != nil {
		return errors.Errorf("importing model during migration %w", coreerrors.NotValid)
	}

	owner, err := coreuser.NewName(model.Owner())
	if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q: invalid owner: %w",
			modelName, modelID, err)

	}
	user, err := i.userService.GetUserByName(ctx, owner)
	if errors.Is(err, accesserrors.UserNotFound) {
		return errors.Errorf("importing model %q with uuid %q, %w for name %q",
			modelName, modelID, accesserrors.UserNotFound, model.Owner(),
		)
	} else if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q during migration, finding user %q: %w",
			modelName, modelID, model.Owner(), err,
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
				modelName, modelID, err)
		}
	}

	// TODO: handle this magic in juju/description, preferably sending the agent-version
	// over the wire as a top-level field on the model, removing it from model config.
	agentVersionStr, ok := model.Config()[config.AgentVersionKey].(string)
	if !ok {
		return errors.Errorf(
			"importing model %q with uuid %q: agent-version missing from model config",
			modelName, modelID)
	}
	agentVersion, err := semversion.Parse(agentVersionStr)
	if err != nil {
		return errors.Errorf(
			"importing model %q with uuid %q: cannot parse agent-version: %w",
			modelName, modelID, err)
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
		UUID: modelID,
	}

	// NOTE: Try to get all things that can fail before creating the model in
	// the database.
	activator, err := i.modelImportService.ImportModel(ctx, args)
	if err != nil {
		return errors.Errorf(
			"importing model %q with id %q during migration: %w",
			modelName, modelID, err,
		)
	}

	// NOTE: If we add any more steps to the import operation, we should
	// consider adding a rollback operation to undo the changes made by the
	// import operation.

	// activator needs to be called as the last operation to say that we are
	// happy that the model is ready to rock and roll.
	if err := activator(ctx); err != nil {
		return errors.Errorf(
			"activating imported model %q with uuid %q: %w", modelName, modelID, err,
		)
	}

	// We need to establish the read only model information in the model database.
	err = i.modelDetailServiceFunc(modelID).CreateModelWithAgentVersionStream(ctx, agentVersion, agentStream)
	if err != nil {
		return errors.Errorf(
			"importing read only model %q with uuid %q during migration: %w",
			modelName, args.UUID, err,
		)
	}

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

	cons := coreconstraints.Value{}
	if allocatePublicIP := descCons.AllocatePublicIP(); allocatePublicIP {
		cons.AllocatePublicIP = &allocatePublicIP
	}
	if arch := descCons.Architecture(); arch != "" {
		cons.Arch = &arch
	}
	if container := coreinstance.ContainerType(descCons.Container()); container != "" {
		cons.Container = &container
	}
	if cores := descCons.CpuCores(); cores != 0 {
		cons.CpuCores = &cores
	}
	if power := descCons.CpuPower(); power != 0 {
		cons.CpuPower = &power
	}
	if inst := descCons.InstanceType(); inst != "" {
		cons.InstanceType = &inst
	}
	if mem := descCons.Memory(); mem != 0 {
		cons.Mem = &mem
	}
	if disk := descCons.RootDisk(); disk != 0 {
		cons.RootDisk = &disk
	}
	if source := descCons.RootDiskSource(); source != "" {
		cons.RootDiskSource = &source
	}
	if spaces := descCons.Spaces(); len(spaces) > 0 {
		cons.Spaces = &spaces
	}
	if tags := descCons.Tags(); len(tags) > 0 {
		cons.Tags = &tags
	}
	if virt := descCons.VirtType(); virt != "" {
		cons.VirtType = &virt
	}
	if zones := descCons.Zones(); len(zones) > 0 {
		cons.Zones = &zones
	}

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

// Rollback will attempt to roll back the import operation if it was
// unsuccessful.
func (i *importModelOperation) Rollback(ctx context.Context, model description.Model) error {
	// Attempt to roll back the model database if it was created.
	modelName, modelID, err := i.getModelNameAndID(model)
	if err != nil {
		return errors.Errorf("rollback of model during migration %w", coreerrors.NotValid)
	}

	// If the model is not found, or the underlying db is not found, we can
	// ignore the error.
	if err := i.modelDetailServiceFunc(modelID).DeleteModel(ctx); err != nil &&
		!errors.Is(err, modelerrors.NotFound) &&
		!errors.Is(err, coredatabase.ErrDBNotFound) {
		return errors.Errorf(
			"rollback of read only model %q with uuid %q during migration: %w",
			modelName, modelID, err,
		)
	}

	// If the model isn't found, we can simply ignore the error.
	if err := i.modelImportService.DeleteModel(ctx, modelID, domainmodel.WithDeleteDB()); err != nil &&
		!errors.Is(err, modelerrors.NotFound) &&
		!errors.Is(err, coredatabase.ErrDBNotFound) {
		return errors.Errorf(
			"rollback of model %q with uuid %q during migration: %w",
			modelName, modelID, err,
		)
	}

	return nil
}

func (i *importModelOperation) getModelNameAndID(model description.Model) (string, coremodel.UUID, error) {
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
