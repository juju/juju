// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager"
	modelmanagererrors "github.com/juju/juju/domain/modelmanager/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// ModelActivatorFunc is a func type for activating a model once it has been
// fully established within the controller. A caller may only call this function
// once for a newly created model.
//
// The following errors can be expected:
// - [modelmanagererrors.AlreadyActivated] when the model has already been
// activated.
type ModelActivatorFunc func(context.Context) error

// ModelRemover is an interface for removing model resources within a controller
// that are not directly accessible from the constroller's state.
type ModelRemover interface {
	// DeleteDB is responsible for removing the model database associated with
	// the supplied model uuid.
	DeleteDB(coremodel.UUID) error
}

// State defines the controller state methods required by this service for
// managing models in the controller.
type State interface {
	// ModelTypeState defines the functions required for determining a new
	// model's type based off of the cloud.
	ModelTypeState

	// ActivateModel activates the model identified by the model uuid. This
	// tells the controller that the model is ready for use.
	//
	// The following errors can be expected:
	// - [modelmanagererrors.AlreadyActivated] when the model has already been
	// activated.
	// - [modelerrors.NotFound] when no model exists for the supplied model uuid.
	ActivateModel(context.Context, coremodel.UUID) error

	// CheckCloudSupportsAuthType checks to see if the given cloud supports a
	// given [cloud.AuthType]. False is returned if the cloud does not support the
	// authentication type.
	//
	// The following errors can be expected:
	// - [clouderrors.NotFound] when the supplied cloud does not exist.
	CheckCloudSupportsAuthType(context.Context, string, cloud.AuthType) (bool, error)

	// CheckModelExists is a check that allows the caller to find out if a model
	// exists and is active within the controller. True or false is returned
	// indicating if the model exists.
	CheckModelExists(context.Context, coremodel.UUID) (bool, error)

	// CreateModel creates a new model in the controller with the supplied
	// creation arguments.
	//
	// The following errors can be expected:
	// - [modelmanagererrors.AlreadyExists] if a
	// model for the supplied uuid or a model with the same name and owner
	// already exists.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] if the owner
	// of the model does not exist.
	// [secretbackenderrors.NotFound] if the secret backend for the new model
	// does not exist.
	// - [modelerrors.CredentialNotValid] when the cloud credential for the
	// model is not valid. This means that either the credential is not
	// supported with the cloud or the cloud doesn't support having an empty
	// credential.
	// - [clouderrors.NotFound] when the cloud does not exist.
	// - [github.com/juju/juju/domain/credential/errors.NotFound] when the
	// credential does not exist.
	CreateModel(
		context.Context,
		coremodel.UUID,
		coremodel.ModelType,
		modelmanager.CreationArgs,
	) error

	// GetControllerModelUUID returns the model uuid for the model that is
	// running the current controller.
	//
	// The following errors can be expected:
	// - [modelerrors.NotFound] when no controller model exists.
	GetControllerModelUUID(context.Context) (coremodel.UUID, error)

	// GetModelCloudNameAndRegion returns the cloud name and cloud region name
	// for a model identified by the model uuid. If no cloud region is in use by
	// the model then a zero value string is returned.
	//
	// The following errors can be expected:
	// - [modelerrors.NotFound] when no model exists for the supplied uuid.
	GetModelCloudNameAndRegion(context.Context, coremodel.UUID) (string, string, error)

	// GetModelUUIDForNameAndOwner returns the model uuid for the model that
	// exists for name and owned by the supplied user name.
	//
	// The following errors can be expected:
	// - [modelerrors.NotFound] when no model exists for the supplied name and
	// owner.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when no user
	// exists for the supplied user name.
	GetModelUUIDForNameAndOwner(context.Context, string, coreuser.Name) (coremodel.UUID, error)

	// ListModelUUIDs returns a list of all model UUIDs in the controller that
	// are active. If no models exist then a zero value slice is returned.
	ListModelUUIDs(context.Context) ([]coremodel.UUID, error)

	// ListModelUUIDsForUser returns a slice of model UUIDs that the supplied
	// user has access to. If the user has no models that they can access a zero
	// value slice is returned.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user
	// does not exist.
	ListModelUUIDsForUser(context.Context, coreuser.UUID) ([]coremodel.UUID, error)

	// RemoveNonActivatedModel removes a model and all of it's associated data
	// from the controller. The model must not have been activated.
	//
	// The following errors can be expected:
	// - [modelerrors.NotFound] when no model exists for the supplied uuid.
	// - [modelmanagererrors.AlreadyActivated
	// when the model has already been activated and cannot be removed.
	RemoveNonActivatedModel(context.Context, coremodel.UUID) error
}

// Service defines the contract for managing models within this controller.
type Service struct {
	logger logger.Logger

	// modelRemover is used for deleting model resources that are not directly
	// accessible via [State].
	modelRemover ModelRemover

	// st is the state interface that is used for managing models in the
	// controller.
	st State
}

// WatchableService extends [Service] providing methods for watching models
// being managed by the controller.
type WatchableService struct {
	// Service provides the methods required for managing models that are not
	// related to watching models.
	Service

	// st is the state interface that is used for watching models in the
	// controller.
	st WatchableState

	// watcherFactory provides the ability to construct new watchers.
	watcherFactory WatcherFactory
}

// WatchableState defines the controller state methods required by the
// [WatchableService] for watching models in the controller.
type WatchableState interface {
	// State defines the controller state funcs that are used as part of the
	// composition of [WatchableService] and [Service].
	State

	// IdentifyActiveModelsFromList takes a list of model uuids for the
	// controller and returns the subset of the list where the model identified
	// by the uuid is active in the controller. If no models are active then an
	// empty slice is returned.
	//
	// Order of model uuids is not maintained in the returned slice.
	IdentifyActiveModelsFromList(context.Context, []coremodel.UUID) ([]coremodel.UUID, error)

	// InitialWatchActivatedModelsStatement returns the table name and SQL
	// statement that will get all the activated models UUIDS in the controller.
	InitialWatchActivatedModelsStatement() (string, string)
}

// WatcherFactory describes the set of functions required for making watchers to
// manage controller models.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new namespace watcher for events
	// based on the input change mask. The initialStateQuery ensures the watcher
	// starts with the current state of the system, preventing data loss from
	// prior events.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// NewService constructs a new [Service] instance.
func NewService(
	modelRemover ModelRemover,
	st State,
	logger logger.Logger,
) *Service {
	return &Service{
		logger:       logger,
		modelRemover: modelRemover,
		st:           st,
	}
}

// NewWatchableService constructs a new [WatchableService] instance.
func NewWatchableService(
	modelRemover ModelRemover,
	st WatchableState,
	watcherFactory WatcherFactory,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			logger:       logger,
			modelRemover: modelRemover,
			st:           st,
		},
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// CheckModelExists checks to see if a model exists in the controller and is
// activated. True is returned if the model exists.
//
// The following errors can be expected:
// - [coreerrors.NotValid] when the model uuid is not valid.
func (s *Service) CheckModelExists(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return false, errors.Errorf("validating model uuid: %w", err)
	}
	return s.st.CheckModelExists(ctx, modelUUID)
}

// CreateModel is responsible for creating a new model from start to finish with
// its associated metadata. As part of creating a new model a new model uuid
// will be generated and returned to the caller.
//
// If the caller has not prescribed a secret backend to use then one will be
// determined for the new model based on the cloud that has been chosen.
//
// Models created by this function must be activated before they can be used in
// the controller. Models are activated by calling the [ModelActivatorFunc] that
// is returned.
//
// The following error types can be expected:
// - [github.com/juju/juju/domain/access/errors.UserNotFound] if the owner
// of the model does not exist.
// - [coreerrors.NotValid] when the supplied creation arguments are not valid.
// - [github.com/juju/juju/domain/cloud/errors.NotFound] when the cloud to be
// used for the new model does not exist.
// - [github.com/juju/juju/domain/credential/errors.NotFound] when the
// credential does not exist.
// - [modelerrors.CredentialNotValid] when either the credential supplied for
// the new model is not valid or cannot be used. This also gets raised when no
// credential has been supplied and the cloud does not support empty auth types.
// - [modelmanagererrors.AlreadyExists] if a model for the supplied uuid or a
// model with the same name and owner already exists.
// - [modelmanagererrors.ModelNameNotValid] when the model name is not valid.
// - [secretbackenderrors.NotFound] when either the secret backend specified for
// the new model does not exist or a default cannot be determined.
func (s *Service) CreateModel(
	ctx context.Context,
	args modelmanager.CreationArgs,
) (coremodel.UUID, ModelActivatorFunc, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelUUID, err := coremodel.NewUUID()
	if err != nil {
		return "", nil, errors.Errorf(
			"generating new model uuid: %w", err,
		)
	}

	activator, err := s.createModel(ctx, modelUUID, args)
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	return modelUUID, activator, nil
}

// createModel is responsible for creating a new model from start to finish with
// its associated metadata. The function takes the model uuid to be used as part
// of the creation. This helps serve both new model creation and that of
// importing existing models.
//
// If the caller has not prescribed a secret backend to use then one will be
// determined for the new model based on the cloud that has been chosen.
//
// Models created by this function must be activated before they can be used in
// the controller by calling the [ModelActivatorFunc] that is returned.
//
// The following error types can be expected:
// - [github.com/juju/juju/domain/access/errors.UserNotFound] if the owner
// of the model does not exist.
// - [coreerrors.NotValid] when the supplied creation arguments are not valid.
// - [github.com/juju/juju/domain/cloud/errors.NotFound] when the cloud to be
// used for the new model does not exist.
// - [github.com/juju/juju/domain/credential/errors.NotFound] when the
// credential does not exist.
// - [modelerrors.CredentialNotValid] when either the credential supplied for
// the new model is not valid or cannot be used. This also gets raised when no
// credential has been supplied and the cloud does not support empty auth types.
// - [modelmanagererrors.AlreadyExists] if a model for the supplied uuid or a
// model with the same name and owner already exists.
// - [modelmanagererrors.ModelNameNotValid] when the model name is not valid.
// - [secretbackenderrors.NotFound] when either the secret backend specified for
// the new model does not exist or a default cannot be determined.
func (s *Service) createModel(
	ctx context.Context,
	modelUUID coremodel.UUID,
	args modelmanager.CreationArgs,
) (ModelActivatorFunc, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := args.Validate(); err != nil {
		return nil, errors.Errorf(
			"validating model creationg arguments: %w", err,
		)
	}

	if !coremodel.IsValidModelName(args.Name) {
		return nil, errors.Errorf(
			"model name for new model is not valid",
		).Add(modelmanagererrors.ModelNameNotValid)
	}

	modelType, err := DetermineModelTypeForCloud(ctx, s.st, args.Cloud)
	if err != nil {
		return nil, errors.Errorf(
			"determining model type when creating model: %w", err,
		)
	}

	// Determine the secret backend for a model if one has not been supplied.
	if args.SecretBackend == "" && modelType == coremodel.CAAS {
		args.SecretBackend = kubernetessecrets.BackendName
	} else if args.SecretBackend == "" && modelType == coremodel.IAAS {
		args.SecretBackend = jujusecrets.BackendName
	} else if args.SecretBackend == "" {
		return nil, errors.Errorf(
			"no default secret backend can be found for new model of type %q",
			modelType,
		).Add(secretbackenderrors.NotFound)
	}

	if args.Credential.IsZero() {
		supports, err := s.st.CheckCloudSupportsAuthType(
			ctx,
			args.Cloud,
			cloud.EmptyAuthType,
		)
		if errors.Is(err, clouderrors.NotFound) {
			return nil, errors.Errorf(
				"cloud %q does not exist", args.Cloud,
			).Add(clouderrors.NotFound)
		} else if err != nil {
			return nil, errors.Errorf(
				"checking if cloud %q supports empty authentication: %w",
				args.Cloud, err,
			)
		}

		if !supports {
			return nil, errors.Errorf(
				"cloud %q does not support empty authentication, a credential must be supplied",
				args.Cloud,
			).Add(modelerrors.CredentialNotValid)
		}
	}

	err = s.st.CreateModel(ctx, modelUUID, modelType, args)
	if err != nil {
		return nil, errors.Capture(err)
	}

	activator := ModelActivatorFunc(func(ctx context.Context) error {
		return s.st.ActivateModel(ctx, modelUUID)
	})

	return activator, nil
}

// GetControllerModelUUID returns the model uuid used for hosting the Juju
// controller.
//
// The following errors can be expected:
// - [modelerrors.NotFound] when no controller model exists.
func (s *Service) GetControllerModelUUID(ctx context.Context) (coremodel.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetControllerModelUUID(ctx)
}

// GetDefaultModelCloudInfo returns the default cloud name and region for this
// controller. This information can be used by external callers to offer UX
// components for choosing a default cloud and region for models that have not
// been created with a specified cloud.
//
// The defaults that are sourced from the controller's default model.
//
// The following errors can be expected:
// - [modelerrors.NotFound] when the default model for the controller doesn't
// exist.
func (s *Service) GetDefaultModelCloudInfo(
	ctx context.Context,
) (string, string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ctrlUUID, err := s.st.GetControllerModelUUID(ctx)
	if errors.Is(err, modelerrors.NotFound) {
		return "", "", errors.New(
			"controller model does not exist to determine the default cloud and region for the controller",
		).Add(modelerrors.NotFound)
	} else if err != nil {
		return "", "", errors.Errorf(
			"getting controller model uuid: %w", err,
		)
	}

	cloudName, regionName, err := s.st.GetModelCloudNameAndRegion(ctx, ctrlUUID)
	if errors.Is(err, modelerrors.NotFound) {
		return "", "", errors.New(
			"controller model does not exist to determine the default cloud and region for the controller",
		).Add(modelerrors.NotFound)
	} else if err != nil {
		return "", "", errors.Errorf(
			"getting controller model %q to determin default cloud and region: %w",
			ctrlUUID, err,
		)
	}
	return cloudName, regionName, nil
}

// GetModelUUIDForNameAndOwner returns the model uuid for the model that exists
// for name and owned by the supplied user.
//
// The following errors can be expected:
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when no user
// exists for the supplied user name.
// - [coreerrors.NotValid] when the user name supplied is not valid.
// - [modelerrors.NotFound] when no model exists for the supplied name and
// - [modelmanagererrors.ModelNameNotValid] when the model name is not valid.
// owner.
func (s *Service) GetModelUUIDForNameAndOwner(
	ctx context.Context,
	modelName string,
	ownerName coreuser.Name,
) (coremodel.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if ownerName.IsZero() {
		return "", errors.New("owner name is required").Add(coreerrors.NotValid)
	}

	if !coremodel.IsValidModelName(modelName) {
		return "", errors.New("model name is not valid").Add(
			modelmanagererrors.ModelNameNotValid,
		)
	}

	return s.st.GetModelUUIDForNameAndOwner(ctx, modelName, ownerName)
}

// getWatchActivatedModelsMapper provides a mapper for filtering out model uuid
// changes to just those models that are activated in the controller. This func
// will maintain the order changes received in.
func getWatchActivatedModelsMapper(st WatchableState) eventsource.Mapper {
	return func(
		ctx context.Context,
		changes []changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		modelUUIDs := make([]coremodel.UUID, 0, len(changes))
		for _, change := range changes {
			modelUUIDs = append(modelUUIDs, coremodel.UUID(change.Changed()))
		}

		filteredUUIDChanges, err := st.IdentifyActiveModelsFromList(ctx, modelUUIDs)
		if err != nil {
			return nil, errors.Errorf(
				"identifying active models from change set: %w", err,
			)
		}

		// Create a map of filtered UUIDs so we can look them up quickly.
		filteredUUIDChangesMap := make(map[string]struct{}, len(filteredUUIDChanges))
		for _, filteredUUID := range filteredUUIDChanges {
			filteredUUIDChangesMap[filteredUUID.String()] = struct{}{}
		}

		// We can now go through the set of changes and only keep the changes
		// for which their exists an activated model. We must maintain the order
		// we received the changes in.
		rval := make([]changestream.ChangeEvent, 0, len(filteredUUIDChanges))
		for _, change := range changes {
			if _, exists := filteredUUIDChangesMap[change.Changed()]; exists {
				rval = append(rval, change)
			}
		}

		return rval, nil
	}
}

// ImportModel is responsible for importing a new model from start to finish
// with its associated metadata. The function takes model import arguments that
// describes both the model uuid for the import and also the metadata for the
// model.
//
// If the caller has not prescribed a secret backend to use then one will be
// determined for the new model based on the cloud that has been chosen.
//
// Models imported by this function must be activated by the caller before they
// can be used in the controller by calling the [ModelActivatorFunc] that is
// returned.
//
// The following error types can be expected:
// - [github.com/juju/juju/domain/access/errors.UserNotFound] if the owner
// of the model does not exist.
// - [coreerrors.NotValid] when the supplied import arguments are not valid.
// This includes an invalid model uuid.
// - [clouderrors.NotFound] when the cloud to be used for the new model does not
// exist.
// - [github.com/juju/juju/domain/credential/errors.NotFound] when the
// credential does not exist.
// - [modelerrors.CredentialNotValid] when either the credential supplied for
// the new model is not valid or cannot be used. This also gets raised when no
// credential has been supplied and the cloud does not support empty auth types.
// - [modelmanagererrors.AlreadyExists] if a model for the supplied uuid or a
// model with the same name and owner already exists.
// - [modelmanagererrors.ModelNameNotValid] when the model name is not valid.
// - [secretbackenderrors.NotFound] when either the secret backend specified for
// the new model does not exist or a default cannot be determined.
func (s *Service) ImportModel(
	ctx context.Context,
	args modelmanager.ImportArgs,
) (ModelActivatorFunc, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := args.UUID.Validate(); err != nil {
		return nil, errors.Errorf("validating model uuid: %w", err)
	}

	return s.createModel(ctx, args.UUID, args.CreationArgs)
}

// ListModelUUIDs returns a list of all model UUIDs in the controller that are
// active. If no models exist a zero value slice is returned.
func (s *Service) ListModelUUIDs(ctx context.Context) ([]coremodel.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.ListModelUUIDs(ctx)
}

// ListModelUUIDsForUser returns a list of model UUIDs that the supplied user
// has access to. If the user supplied does not have access to any models then
// a zero value slice is returned.
//
// The following errors can be expected:
// - [coreerrors.NotValid] when the user uuid supplied is not valid.
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user does
// not exist.
func (s *Service) ListModelUUIDsForUser(
	ctx context.Context,
	userUUID coreuser.UUID,
) ([]coremodel.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating user uuid: %w", err)
	}

	return s.st.ListModelUUIDsForUser(ctx, userUUID)
}

// RemoveNonActivatedModel is responsible for removing a non activated model
// from the controller and all of it's associated metadata. To remove an
// activated model the removal service needs to be used.
//
// This method exists to cleanup a model that has failed to been created
// and activated within the controller.
//
// The following error types can be expected:
// - [coreerrors.NotValid] when the model uuid supplied is not valid.
// - [modelerrors.NotFound] when no model exists for the supplied model uuid.
// - [modelmanagererrors.AlreadyActivated] when the model has already been
// activated and cannot be removed.
func (s *Service) RemoveNonActivatedModel(
	ctx context.Context,
	modelUUID coremodel.UUID,
	opts ...modelmanager.RemoveModelOption,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	options := modelmanager.DefaultRemoveModelOptions()
	for _, fn := range opts {
		fn(options)
	}

	if err := s.st.RemoveNonActivatedModel(ctx, modelUUID); err != nil {
		return errors.Capture(err)
	}

	if options.DeleteDB() {
		s.logger.Infof(
			ctx,
			"skipping model %q resource deletion, the model database is still present",
			modelUUID,
		)
		return nil
	}

	if err := s.modelRemover.DeleteDB(modelUUID); err != nil {
		return errors.Errorf(
			"deleting model %q resources and database: %w", modelUUID, err,
		)
	}

	return nil
}

// WatchActivatedModels returns a [watcher.StringsWatcher] that emits an event
// containing the model UUID when a model in the controller becomes activated or
// an activated model receives an update. Newly created models will not be
// reported since they are not activated at creation. Deletion of activated
// models is also not reported.
func (s *WatchableService) WatchActivatedModels(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mapper := getWatchActivatedModelsMapper(s.st)
	tableName, stmt := s.st.InitialWatchActivatedModelsStatement()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		mapper,
		eventsource.NamespaceFilter(tableName, changestream.Changed),
	)
}
