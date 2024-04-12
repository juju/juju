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
// models config.
type ModelConfigService interface {
	SetModelConfig(context.Context, map[string]any) error
}

// ModelService defines a interface for interacting with the underlying state.
type ModelService interface {
	CreateModel(context.Context, model.ModelCreationArgs) (coremodel.UUID, error)
	DefaultModelCloudNameAndCredential(context.Context) (string, credential.Key, error)
	ModelType(context.Context, coremodel.UUID) (coremodel.ModelType, error)
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
	CreateModel(context.Context, model.ReadOnlyModelCreationArgs) error
}

// ModelExporter defines a interface for exporting models.
type ModelExporter interface {
	ExportModelPartial(context.Context, state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	common.CloudService
	ListAll(ctx context.Context) ([]jujucloud.Cloud, error)
}

// CredentialService exposes State methods needed by credential manager.
type CredentialService interface {
	CloudCredential(ctx context.Context, id credential.Key) (jujucloud.Credential, error)
	InvalidateCredential(ctx context.Context, id credential.Key, reason string) error
}

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// User
	GetUserByName(context.Context, string) (coreuser.User, error)

	// Permissions
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target corepermission.ID) (corepermission.Access, error)
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	BackendSummaryInfo(ctx context.Context, reveal, all bool, names ...string) ([]*secretbackendservice.SecretBackendInfo, error)
}

// Services holds the services needed by the model manager api.
type Services struct {
	ServiceFactoryGetter ServiceFactoryGetter
	CloudService         CloudService
	CredentialService    CredentialService
	ModelService         ModelService
	ModelDefaultsService ModelDefaultsService
	AccessService        AccessService
	ObjectStore          objectstore.ObjectStore
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
