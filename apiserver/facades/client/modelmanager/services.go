// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"

	"github.com/juju/description/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/model"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
)

// ServiceFactory is a factory for creating model info services.
type ServiceFactory interface {
	ModelInfo() ModelInfoService
	Config(modeldefaultsservice.ModelDefaultsProviderFunc) ModelConfigService
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
	// CreateModel creates a model.
	CreateModel(context.Context, model.ModelCreationArgs) (coremodel.UUID, error)
	// DefaultModelCloudNameAndCredential returns the default cloud name and
	// credential that should be used for newly created models that haven't had
	// either cloud or credential specified.
	DefaultModelCloudNameAndCredential(context.Context) (string, credential.Key, error)
	// ModelType returns the type of the given model.
	ModelType(context.Context, coremodel.UUID) (coremodel.ModelType, error)
	// DeleteModel deletes the give model.
	DeleteModel(context.Context, coremodel.UUID) error
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
	CreateModel(context.Context, model.ReadOnlyModelCreationArgs) error
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
	// subject (user) on the given target.
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error)
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	// BackendSummaryInfo returns a summary of the secret backends.
	BackendSummaryInfo(ctx context.Context, reveal, all bool, names ...string) ([]*secretbackendservice.SecretBackendInfo, error)
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

func (s serviceFactory) Config(defaults modeldefaultsservice.ModelDefaultsProviderFunc) ModelConfigService {
	return s.serviceFactory.Config(defaults)
}
