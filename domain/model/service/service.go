// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// ModelActivator describes a closure type that must be called after creating a
// new model to indicate that all model creation operations have been performed
// and the model is active within the controller.
//
// This type may return an error satisfying [modelerrors.AlreadyActivated] if
// the model in question has been activated already.
type ModelActivator func(context.Context) error

// ModelTypeState represents the state required for determining the type of model
// based on the cloud being set for it.
type ModelTypeState interface {
	// CloudType is responsible for reporting the type for a given cloud name.
	// If no cloud exists for the provided name then an error of
	// [clouderrors.NotFound] will be returned.
	CloudType(context.Context, string) (string, error)
}

// State is the model state required by this service.
type State interface {
	ModelTypeState

	// Create creates a new model with all of its associated metadata.
	Create(context.Context, coremodel.UUID, coremodel.ModelType, model.GlobalModelCreationArgs) error

	// Activate is responsible for setting a model as fully constructed and
	// indicates the final system state for the model is ready for use.
	// If no model exists for the provided id then a [modelerrors.NotFound] will be
	// returned. If the model has previously been activated a
	// [modelerrors.AlreadyActivated] error will be returned.
	Activate(context.Context, coremodel.UUID) error

	// CloudSupportsAuthType is a check that allows the caller to find out if a
	// cloud supports a specific auth type. If the cloud doesn't support the
	// authtype then false is return with a nil error.
	// If no cloud exists for the supplied name an error satisfying
	// [github.com/juju/juju/domain/cloud/errors.NotFound] is returned.
	CloudSupportsAuthType(context.Context, string, cloud.AuthType) (bool, error)

	// GetModel returns the model associated with the provided uuid.
	GetModel(context.Context, coremodel.UUID) (coremodel.Model, error)

	// GetModelByName returns the model associated with the given user and name.
	// If no model exists for the provided user or model name then an error of
	// [modelerrors.NotFound] will be returned.
	GetModelByName(context.Context, coreuser.Name, string) (coremodel.Model, error)

	// GetModelType returns the model type for a model with the provided uuid.
	GetModelType(context.Context, coremodel.UUID) (coremodel.ModelType, error)

	// GetControllerModel returns the model the controller is running in.
	// If no controller model exists then an error satisfying
	// [modelerrors.NotFound] is returned.
	GetControllerModel(ctx context.Context) (coremodel.Model, error)

	// GetControllerModelUUID returns the model uuid for the controller model.
	// If no controller model exists then an error satisfying
	// [modelerrors.NotFound] is returned.
	GetControllerModelUUID(context.Context) (coremodel.UUID, error)

	// GetModelCloudNameAndCredential returns the cloud name and credential id
	// for a model identified by the model uuid. If no model exists for the
	// provided name and user a [modelerrors.NotFound] error is returned.
	GetModelCloudNameAndCredential(context.Context, coremodel.UUID) (string, credential.Key, error)

	// Delete removes a model and all of it's associated data from Juju.
	Delete(context.Context, coremodel.UUID) error

	// ListAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	ListAllModels(context.Context) ([]coremodel.Model, error)

	// ListModelIDs returns a list of all model UUIDs.
	ListModelIDs(context.Context) ([]coremodel.UUID, error)

	// ListModelsForUser returns a slice of models owned by the user
	// specified by user id. If no user or models are found an empty slice is
	// returned.
	ListModelsForUser(context.Context, coreuser.UUID) ([]coremodel.Model, error)

	// GetModelUsers will retrieve basic information about all users with
	// permissions on the given model UUID.
	// If the model cannot be found it will return modelerrors.NotFound.
	GetModelUsers(context.Context, coremodel.UUID) ([]coremodel.ModelUserInfo, error)

	// ListModelSummariesForUser returns a slice of model summaries for a given
	// user. If no models are found an empty slice is returned.
	ListModelSummariesForUser(context.Context, coreuser.Name) ([]coremodel.UserModelSummary, error)

	// ListAllModelSummaries returns a slice of model summaries for all models
	// known to the controller.
	ListAllModelSummaries(ctx context.Context) ([]coremodel.ModelSummary, error)

	// UpdateCredential updates a model's cloud credential.
	UpdateCredential(context.Context, coremodel.UUID, credential.Key) error

	// GetActivatedModelUUIDs returns the subset of model UUIDS from the supplied list that are activated.
	// If no model uuids are activated then an empty slice is returned.
	GetActivatedModelUUIDs(ctx context.Context, uuids []coremodel.UUID) ([]coremodel.UUID, error)

	// InitialWatchActivatedModelsStatement returns a SQL statement that will get all the activated models UUIDS in the controller.
	InitialWatchActivatedModelsStatement() string
}

// ModelDeleter is an interface for deleting models.
type ModelDeleter interface {
	// DeleteDB is responsible for removing a model from Juju and all of it's
	// associated metadata.
	DeleteDB(string) error
}

// Service defines a service for interacting with the underlying state based
// information of a model.
type Service struct {
	st           State
	modelDeleter ModelDeleter
	logger       logger.Logger
}

var (
	caasCloudTypes = []string{cloud.CloudTypeKubernetes}
)

// NewService returns a new Service for interacting with a models state.
func NewService(
	st State,
	modelDeleter ModelDeleter,
	logger logger.Logger,
) *Service {
	return &Service{
		st:           st,
		modelDeleter: modelDeleter,
		logger:       logger,
	}
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new namespace watcher for events based on the input change mask.
	// The initialStateQuery ensures the watcher starts with the current state of the system,
	// preventing data loss from prior events.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery, mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService extends Service to provide interactions with model state
// and integrates a watcher factory for monitoring changes.
type WatchableService struct {
	// Service is the inherited model service to extend upon.
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService provides a new Service for interacting with the underlying
// state and the ability to create watchers.
func NewWatchableService(st State,
	modelDeleter ModelDeleter,
	logger logger.Logger,
	watcherFactory WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:           st,
			modelDeleter: modelDeleter,
			logger:       logger,
		},
		watcherFactory: watcherFactory,
	}
}

// DefaultModelCloudNameAndCredential returns the default cloud name and
// credential that should be used for newly created models that haven't had
// either cloud or credential specified. If no default credential is available
// the zero value of [credential.UUID] will be returned.
//
// The defaults that are sourced come from the controller's default model. If
// there is a no controller model a [modelerrors.NotFound] error will be
// returned.
func (s *Service) DefaultModelCloudNameAndCredential(
	ctx context.Context,
) (string, credential.Key, error) {
	ctrlUUID, err := s.st.GetControllerModelUUID(ctx)
	if err != nil {
		return "", credential.Key{}, errors.Errorf(
			"getting controller model uuid: %w", err,
		)
	}
	cloudName, cred, err := s.st.GetModelCloudNameAndCredential(ctx, ctrlUUID)

	if err != nil {
		return "", credential.Key{}, errors.Errorf(
			"getting controller model %q cloud name and credential: %w",
			ctrlUUID, err,
		)
	}
	return cloudName, cred, nil
}

// CreateModel is responsible for creating a new model from start to finish with
// its associated metadata. The function will return the created model's id.
//
// If the caller has not prescribed a specific agent version to use for the
// model the current controllers supported agent version will be used.
//
// If no secret backend is defined for the created model then one will be
// determined for the new model.
//
// Models created by this function must be activated using the returned
// ModelActivator.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not exist.
// - [github.com/juju/juju/domain/access/errors.NotFound]: When the owner of the
// model can not be found.
// - [modelerrors.AgentVersionNotSupported]: When the prescribed agent version
// cannot be used with this controller.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
// - [modelerrors.CredentialNotValid]: When the cloud credential for the model
// is not valid. This means that either the credential is not supported with
// the cloud or the cloud doesn't support having an empty credential.
func (s *Service) CreateModel(
	ctx context.Context,
	args model.GlobalModelCreationArgs,
) (coremodel.UUID, func(context.Context) error, error) {
	if err := args.Validate(); err != nil {
		return "", nil, errors.Errorf(
			"cannot validate model creation args: %w", err,
		)
	}

	modelID, err := coremodel.NewUUID()
	if err != nil {
		return "", nil, errors.Errorf(
			"cannot generate id for model %q: %w", args.Name, err,
		)
	}

	activator, err := s.createModel(ctx, modelID, args)
	return modelID, activator, err
}

// createModel is responsible for creating a new model from start to finish with
// its associated metadata. The function takes the model id to be used as part
// of the creation. This helps serve both new model creation and model
// importing.
//
// If the caller has not prescribed a specific agent version to use for the
// model the current controllers supported agent version will be used.
//
// If no secret backend is defined for the created model then one will be
// determined for the new model.
//
// Models created by this function must be activated using the returned
// ModelActivator.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a
// model with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not
// exist.
// - [github.com/juju/juju/domain/access/errors.NotFound]: When the owner of the
// model can not be found.
// - [modelerrors.AgentVersionNotSupported]: When the prescribed agent version
// cannot be used with this controller.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
// - [modelerrors.CredentialNotValid]: When the cloud credential for the model
// is not valid. This means that either the credential is not supported with
// the cloud or the cloud doesn't support having an empty credential.
func (s *Service) createModel(
	ctx context.Context,
	id coremodel.UUID,
	args model.GlobalModelCreationArgs,
) (func(context.Context) error, error) {
	modelType, err := ModelTypeForCloud(ctx, s.st, args.Cloud)
	if err != nil {
		return nil, errors.Errorf(
			"determining model type when creating model %q: %w",
			args.Name, err,
		)
	}

	if args.SecretBackend == "" && modelType == coremodel.CAAS {
		args.SecretBackend = kubernetessecrets.BackendName
	} else if args.SecretBackend == "" && modelType == coremodel.IAAS {
		args.SecretBackend = jujusecrets.BackendName
	} else if args.SecretBackend == "" {
		return nil, errors.Errorf(
			"%w for model type %q when creating model with name %q",
			secretbackenderrors.NotFound,
			modelType,
			args.Name,
		)
	}

	if args.Credential.IsZero() {
		supports, err := s.st.CloudSupportsAuthType(ctx, args.Cloud, cloud.EmptyAuthType)
		if err != nil {
			return nil, errors.Errorf(
				"checking if cloud %q support empty authentication for new model %q: %w",
				args.Cloud, args.Name, err,
			)
		}

		if !supports {
			return nil, errors.Errorf(
				"new model %q cloud %q does not support empty authentication, a credential needs to be specified",
				args.Name, args.Cloud,
			).Add(modelerrors.CredentialNotValid)
		}
	}

	activator := ModelActivator(func(ctx context.Context) error {
		return s.st.Activate(ctx, id)
	})

	return activator, s.st.Create(ctx, id, modelType, args)
}

// ImportModel is responsible for importing an existing model into this Juju
// controller. The caller must explicitly specify the agent version that is in
// use for the imported model.
//
// Models created by this function must be activated using the returned
// ModelActivator.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a
// model with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not
// exist.
// - [github.com/juju/juju/domain/access/errors.NotFound]: When the owner of the
// model can not be found.
// - [modelerrors.AgentVersionNotSupported]: When the prescribed agent version
// cannot be used with this controller or the agent version is set to zero.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
func (s *Service) ImportModel(
	ctx context.Context,
	args model.ModelImportArgs,
) (func(context.Context) error, error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Errorf(
			"cannot validate model import args: %w", err,
		)
	}

	// If we are importing a model we need to know the agent version in use to
	// make sure we have tools to support the model and it will work with this
	// controller.
	if args.AgentVersion == version.Zero {
		return nil, errors.Errorf(
			"cannot import model with id %q, agent version cannot be zero: %w",
			args.ID, modelerrors.AgentVersionNotSupported,
		)
	}

	return s.createModel(ctx, args.ID, args.GlobalModelCreationArgs)
}

// ControllerModel returns the model used for housing the Juju controller.
// Should no model exist for the controller an error of [modelerrors.NotFound]
// will be returned.
func (s *Service) ControllerModel(ctx context.Context) (coremodel.Model, error) {
	return s.st.GetControllerModel(ctx)
}

// Model returns the model associated with the provided uuid.
// The following error types can be expected to be returned:
// - [modelerrors.ModelNotFound]: When the model does not exist.
func (s *Service) Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error) {
	if err := uuid.Validate(); err != nil {
		return coremodel.Model{}, errors.Errorf("model uuid: %w", err)
	}

	return s.st.GetModel(ctx, uuid)
}

// ModelType returns the current model type based on the cloud name being used
// for the model.
func (s *Service) ModelType(ctx context.Context, uuid coremodel.UUID) (coremodel.ModelType, error) {
	if err := uuid.Validate(); err != nil {
		return "", errors.Errorf("model type uuid: %w", err)
	}

	return s.st.GetModelType(ctx, uuid)
}

// DeleteModel is responsible for removing a model from Juju and all of it's
// associated metadata.
// - errors.NotValid: When the model uuid is not valid.
// - modelerrors.ModelNotFound: When the model does not exist.
func (s *Service) DeleteModel(
	ctx context.Context,
	uuid coremodel.UUID,
	opts ...model.DeleteModelOption,
) error {
	options := model.DefaultDeleteModelOptions()
	for _, fn := range opts {
		fn(options)
	}

	if err := uuid.Validate(); err != nil {
		return errors.Errorf("delete model, uuid: %w", err)
	}

	// Delete common items from the model. This helps to ensure that the
	// model is cleaned up correctly.
	if err := s.st.Delete(ctx, uuid); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("delete model: %w", err)
	}

	// If the db should not be deleted then we can return early.
	if !options.DeleteDB() {
		s.logger.Infof(ctx, "skipping model deletion, model database will still be present")
		return nil
	}

	// Delete the db completely from the system. Currently, this will remove
	// the db from the dbaccessor, but it will not drop the db (currently not
	// supported in dqlite). For now we do a best effort to remove all items
	// with in the db.
	if err := s.modelDeleter.DeleteDB(uuid.String()); err != nil {
		return errors.Errorf("delete model: %w", err)
	}

	return nil
}

// ListModelIDs returns a list of all model UUIDs in the system that have not been
// deleted. This list does not represent one or more lifecycle states for
// models.
func (s *Service) ListModelIDs(ctx context.Context) ([]coremodel.UUID, error) {
	uuids, err := s.st.ListModelIDs(ctx)
	if err != nil {
		return nil, errors.Errorf("getting list of model id's: %w", err)
	}
	return uuids, nil
}

// ListAllModels  lists all models in the controller. If no models exist then
// an empty slice is returned.
func (s *Service) ListAllModels(ctx context.Context) ([]coremodel.Model, error) {
	return s.st.ListAllModels(ctx)
}

// ListModelsForUser lists the models that are either owned by the user or
// accessible  by the user specified by the user id. If no user or models exists
// an empty slice of models will be returned.
func (s *Service) ListModelsForUser(ctx context.Context, userID coreuser.UUID) ([]coremodel.Model, error) {
	if err := userID.Validate(); err != nil {
		return nil, errors.Errorf("listing models owned by user: %w", err)
	}

	return s.st.ListModelsForUser(ctx, userID)
}

// ModelTypeForCloud is responsible returning the model type based on the cloud
// name being used for the model. If no cloud exists for the provided name then
// an error of [clouderrors.NotFound] will be returned.
func ModelTypeForCloud(
	ctx context.Context,
	state ModelTypeState,
	cloudName string,
) (coremodel.ModelType, error) {
	cloudType, err := state.CloudType(ctx, cloudName)
	if err != nil {
		return "", errors.Errorf("determining model type from cloud: %w", err)
	}

	if set.NewStrings(caasCloudTypes...).Contains(cloudType) {
		return coremodel.CAAS, nil
	}
	return coremodel.IAAS, nil
}

// GetModelUsers will retrieve basic information about users with permissions on
// the given model UUID.
// If the model cannot be found it will return [modelerrors.NotFound].
func (s *Service) GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error) {
	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	modelUserInfo, err := s.st.GetModelUsers(ctx, modelUUID)
	if err != nil {
		return nil, errors.Errorf("getting users for model %q: %w", modelUUID, err)
	}
	return modelUserInfo, nil
}

// GetModelUser will retrieve basic information about the specified model user.
// If the model cannot be found it will return [modelerrors.NotFound].
// If the user cannot be found it will return [modelerrors.UserNotFoundOnModel].
func (s *Service) GetModelUser(ctx context.Context, modelUUID coremodel.UUID, name coreuser.Name) (coremodel.ModelUserInfo, error) {
	if name.IsZero() {
		return coremodel.ModelUserInfo{}, errors.New(
			"empty username not allowed",
		).Add(accesserrors.UserNameNotValid)
	}
	if err := modelUUID.Validate(); err != nil {
		return coremodel.ModelUserInfo{}, errors.Capture(err)
	}
	modelUserInfo, err := s.st.GetModelUsers(ctx, modelUUID)
	if err != nil {
		return coremodel.ModelUserInfo{}, errors.Errorf(
			"getting info of user %q on model %q: %w",
			name, modelUUID, err,
		)
	}

	for _, mui := range modelUserInfo {
		if mui.Name == name {
			return mui, nil
		}
	}
	return coremodel.ModelUserInfo{}, errors.Errorf(
		"getting info of user %q on model %q: %w",
		name, modelUUID, err,
	)
}

// ListModelSummariesForUser returns a slice of model summaries for a given
// user. If no models are found an empty slice is returned.
func (s *Service) ListModelSummariesForUser(ctx context.Context, userName coreuser.Name) ([]coremodel.UserModelSummary, error) {
	if userName.IsZero() {
		return nil, errors.New("empty username").Add(accesserrors.UserNameNotValid)
	}
	return s.st.ListModelSummariesForUser(ctx, userName)
}

// ListAllModelSummaries returns a slice of model summaries for all models
// known to the controller.
func (s *Service) ListAllModelSummaries(ctx context.Context) ([]coremodel.ModelSummary, error) {
	return s.st.ListAllModelSummaries(ctx)
}

// UpdateCredential is responsible for updating the cloud credential
// associated with a model. The cloud credential must be of the same cloud type
// as that of the model.
// The following error types can be expected to be returned:
// - modelerrors.ModelNotFound: When the model does not exist.
// - errors.NotFound: When the cloud or credential cannot be found.
// - errors.NotValid: When the cloud credential is not of the same cloud as the
// model or the model uuid is not valid.
func (s *Service) UpdateCredential(
	ctx context.Context,
	uuid coremodel.UUID,
	key credential.Key,
) error {
	if err := uuid.Validate(); err != nil {
		return errors.Errorf("updating cloud credential model uuid: %w", err)
	}
	if err := key.Validate(); err != nil {
		return errors.Errorf("updating cloud credential: %w", err)
	}

	return s.st.UpdateCredential(ctx, uuid, key)
}

// getWatchActivatedModelsMapper returns a mapper function that filters change events to
// include only those associated with activated models.
// The subset of changes returned is maintained in the same order as they are received.
func getWatchActivatedModelsMapper(st State) func(ctx context.Context, db database.TxnRunner,
	changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {

	return func(ctx context.Context, db database.TxnRunner,
		changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {

		modelUUIDs := make([]coremodel.UUID, len(changes))
		for i, change := range changes {
			modelUUIDs[i] = coremodel.UUID(change.Changed())
		}

		// Retrieve all activate status of all models with associated uuids
		activatedModelUUIDs, err := st.GetActivatedModelUUIDs(ctx, modelUUIDs)

		// There will be no errors returned if there are no activated models found.
		if err != nil {
			return nil, err
		}

		if len(activatedModelUUIDs) == 0 {
			return nil, nil
		}

		activatedModelUUIDToChangeEventMap := make(map[coremodel.UUID]struct{}, len(activatedModelUUIDs))
		for _, activatedModelUUID := range activatedModelUUIDs {
			activatedModelUUIDToChangeEventMap[activatedModelUUID] = struct{}{}
		}

		activatedModelChangeEvents := make([]changestream.ChangeEvent, 0, len(changes))

		// Add all events associated with activated model UUIDs
		for _, change := range changes {
			uuid := coremodel.UUID(change.Changed())
			if _, exists := activatedModelUUIDToChangeEventMap[uuid]; exists {
				activatedModelChangeEvents = append(activatedModelChangeEvents, change)
			}
		}

		return activatedModelChangeEvents, nil
	}
}

// WatchActivatedModels returns a watcher that emits an event containing the model UUID
// when a model becomes activated or an activated model receives an update.
// The events returned are maintained in the same order as they are received.
// Newly created models will not be reported since they are not activated at creation.
// Deletion of activated models is also not reported.
func (s *WatchableService) WatchActivatedModels(ctx context.Context) (watcher.StringsWatcher, error) {
	mapper := getWatchActivatedModelsMapper(s.st)
	query := s.st.InitialWatchActivatedModelsStatement()

	return s.watcherFactory.NewNamespaceMapperWatcher(
		eventsource.InitialNamespaceChanges(query),
		mapper,
		eventsource.NamespaceFilter("model", changestream.Changed),
	)
}
