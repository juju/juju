// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"time"

	"github.com/juju/description/v8"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/model"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

// ServiceFactory is a factory for creating model info services.
type ServiceFactory interface {
	ModelInfo() ModelInfoService
	Config() ModelConfigService
	Network() NetworkService
}

// ServiceFactoryGetter is a factory for creating model services.
type ServiceFactoryGetter interface {
	ServiceFactoryForModel(coremodel.UUID) ServiceFactory
}

// ModelConfigServiceGetter provides a means to fetch the model config service
// for a given model uuid.
type ModelConfigServiceGetter func(coremodel.UUID) (ModelConfigService, error)

// ModelConfigService describes the set of functions needed for working with a
// model's config.
type ModelConfigService interface {
	// SetModelConfig sets the models config.
	SetModelConfig(context.Context, map[string]any) error
}

// ModelService defines a interface for interacting with the underlying state.
type ModelService interface {
	// CreateModel creates a model returning the resultant model's new id.
	CreateModel(context.Context, model.ModelCreationArgs) (coremodel.UUID, func(context.Context) error, error)

	// DefaultModelCloudNameAndCredential returns the default cloud name and
	// credential that should be used for newly created models that haven't had
	// either cloud or credential specified.
	DefaultModelCloudNameAndCredential(context.Context) (string, credential.Key, error)

	// DeleteModel deletes the give model.
	DeleteModel(context.Context, coremodel.UUID, ...model.DeleteModelOption) error

	// ListModelsForUser returns a list of models for the given user.
	ListModelsForUser(context.Context, coreuser.UUID) ([]coremodel.Model, error)

	// ListAllModels returns a list of all models.
	ListAllModels(context.Context) ([]coremodel.Model, error)
}

// ModelDefaultsService defines a interface for interacting with the model
// defaults.
type ModelDefaultsService interface {
	// ModelDefaultsProvider provides a [ModelDefaultsProviderFunc] scoped to the
	// supplied model. This can be used in the construction of
	// [github.com/juju/juju/domain/modelconfig/service.Service]. If no model exists
	// for the specified UUID then the [ModelDefaultsProviderFunc] will return a
	// error that satisfies
	// [github.com/juju/juju/domain/model/errors.NotFound].
	ModelDefaultsProvider(
		uuid coremodel.UUID,
	) modeldefaultsservice.ModelDefaultsProviderFunc
}

// ModelInfoService defines a interface for interacting with the underlying
// state.
type ModelInfoService interface {
	// CreateModel is responsible for creating a new read only model
	// that is being imported.
	CreateModel(context.Context, uuid.UUID) error

	// DeleteModel is responsible for deleting a model during model migration.
	DeleteModel(context.Context) error
}

// ModelExporter defines a interface for exporting models.
type ModelExporter interface {
	// ExportModelPartial exports the current model into a partial description
	// model. This can be serialized into yaml and then imported.
	ExportModelPartial(context.Context, state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	common.CloudService
	// ListAll return all clouds.
	ListAll(ctx context.Context) ([]jujucloud.Cloud, error)
}

// CredentialService exposes State methods needed by credential manager.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given key.
	CloudCredential(ctx context.Context, id credential.Key) (jujucloud.Credential, error)
	// InvalidateCloudCredential marks the cloud credential for the given name, cloud, owner as invalid.
	InvalidateCredential(ctx context.Context, id credential.Key, reason string) error
}

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// GetUserByName returns a User for the given name.
	GetUserByName(context.Context, string) (coreuser.User, error)
	// ReadUserAccessLevelForTarget returns the Access level for the given
	// subject (user) on the given target (model).
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error)
	// UpdatePermission updates the access level for a user of the model.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// LastModelLogin will return the last login time of the specified
	// user. An accesserrors.UserNeverAccessedModel error will be returned if
	// there is no record of the user logging in to this model.
	LastModelLogin(context.Context, string, coremodel.UUID) (time.Time, error)
	// GetModelUsers will retrieve basic information about all users with
	// permissions on the given model UUID.
	// If the model cannot be found it will return modelerrors.NotFound.
	// If no permissions can be found on the model it will return
	// accesserrors.PermissionNotValid.
	GetModelUsers(ctx context.Context, apiUser string, modelUUID coremodel.UUID) ([]access.ModelUserInfo, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	ReloadSpaces(ctx context.Context) error
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	// BackendSummaryInfoForModel returns a summary of the secret backends for a model.
	BackendSummaryInfoForModel(ctx context.Context, modelUUID coremodel.UUID) ([]*secretbackendservice.SecretBackendInfo, error)
}

// Services holds the services needed by the model manager api.
type Services struct {
	// ServiceFactoryGetter is an interface for interacting with a factory for
	// creating model services.
	ServiceFactoryGetter ServiceFactoryGetter
	// CloudServices is an interface for interacting with the cloud service.
	CloudService CloudService
	// CredentialService is an interface for interacting with the credential
	// service.
	CredentialService CredentialService
	// ModelService is an interface for interacting with the model service.
	ModelService ModelService
	// ModelDefaultsService is an interface for interacting with the model
	// defaults service.
	ModelDefaultsService ModelDefaultsService
	// AccessService is an interface for interacting with the access service
	// covering user and permission.
	AccessService AccessService
	// ObjectStore is an interface for interacting with a full object store.
	ObjectStore objectstore.ObjectStore
	// SecretBackendService is an interface for interacting with secret backend
	// service.
	SecretBackendService SecretBackendService
	NetworkService       NetworkService
}

type serviceFactoryGetter struct {
	ctx facade.MultiModelContext
}

func (s serviceFactoryGetter) ServiceFactoryForModel(uuid coremodel.UUID) ServiceFactory {
	return serviceFactory{serviceFactory: s.ctx.ServiceFactoryForModel(uuid)}
}

type serviceFactory struct {
	serviceFactory servicefactory.ServiceFactory
}

func (s serviceFactory) ModelInfo() ModelInfoService {
	return s.serviceFactory.ModelInfo()
}

func (s serviceFactory) Config() ModelConfigService {
	return s.serviceFactory.Config()
}

func (s serviceFactory) Network() NetworkService {
	return s.serviceFactory.Network()
}
