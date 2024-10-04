// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/modeldefaults"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

// ModelDomainServices is a factory for creating model info services.
type ModelDomainServices interface {
	// Agent returns the model's agent service.
	Agent() ModelAgentService

	// Config returns the model config service.
	Config() ModelConfigService

	// ModelInfo returns the model service for the model. The model info
	// contains read-only information about the model.
	// Note: This should be called model, but we have naming conflicts with
	// the model service. As this is only for read-only model information, we
	// can rename it to the more obscure version.
	ModelInfo() ModelInfoService

	// Network returns the space service.
	Network() NetworkService
	BlockCommand() BlockCommandService
}

// DomainServicesGetter is a factory for creating model services.
type DomainServicesGetter interface {
	DomainServicesForModel(coremodel.UUID) ModelDomainServices
}

// ModelConfigServiceGetter provides a means to fetch the model config service
// for a given model uuid.
type ModelConfigServiceGetter func(coremodel.UUID) (ModelConfigService, error)

// ModelConfigService describes the set of functions needed for working with a
// model's config.
type ModelConfigService interface {
	// ModelConfig returns the currently set config on this model.
	ModelConfig(context.Context) (*config.Config, error)

	// SetModelConfig sets the models config.
	SetModelConfig(context.Context, map[string]any) error
}

// ModelService defines an interface for interacting with the model service.
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

	// GetModelUsers will retrieve basic information about users with
	// permissions on the given model UUID.
	// If the model cannot be found it will return
	// [github.com/juju/juju/domain/model/errors.NotFound].
	GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error)

	// GetModelUser will retrieve basic information about the specified model
	// user.
	// If the model cannot be found it will return
	// [github.com/juju/juju/domain/model/errors.NotFound].
	// If the user cannot be found it will return
	//[github.com/juju/juju/domain/model/errors.UserNotFoundOnModel].
	GetModelUser(ctx context.Context, modelUUID coremodel.UUID, name coreuser.Name) (coremodel.ModelUserInfo, error)

	// ListModelSummariesForUser returns a slice of model summaries for a given
	// user. If no models are found an empty slice is returned.
	ListModelSummariesForUser(ctx context.Context, userName coreuser.Name) ([]coremodel.UserModelSummary, error)

	// ListAllModelSummaries returns a slice of model summaries for all models
	// known to the controller.
	ListAllModelSummaries(ctx context.Context) ([]coremodel.ModelSummary, error)
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

	// CloudDefaults returns the default attribute details for a specified cloud.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	CloudDefaults(ctx context.Context, cloudName string) (modeldefaults.ModelDefaultAttributes, error)

	// UpdateModelConfigDefaultValues saves the specified default attribute details for a cloud or region.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	UpdateModelConfigDefaultValues(ctx context.Context, updateAttrs map[string]interface{}, cloudRegion modeldefaults.CloudRegion) error

	// RemoveModelConfigDefaultValues deletes the specified default attribute details for a cloud or region.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	RemoveModelConfigDefaultValues(ctx context.Context, removeAttrs []string, cloudRegion modeldefaults.CloudRegion) error
}

// ModelInfoService defines a interface for interacting with the underlying
// state.
type ModelInfoService interface {
	// CreateModel is responsible for creating a new read only model
	// that is being imported.
	CreateModel(context.Context, uuid.UUID) error

	// DeleteModel is responsible for deleting a model during model migration.
	DeleteModel(context.Context) error

	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (coremodel.ReadOnlyModel, error)
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
	GetUserByName(context.Context, coreuser.Name) (coreuser.User, error)
	// ReadUserAccessLevelForTarget returns the Access level for the given
	// subject (user) on the given target (model).
	// If the access level of a user cannot be found then
	// [github.com/juju/juju/domain/access/errors.AccessNotFound] is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject coreuser.Name, target corepermission.ID) (corepermission.Access, error)
	// UpdatePermission updates the access level for a user of the model.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// LastModelLogin will return the last login time of the specified
	// user.
	// [github.com/juju/juju/domain/access/errors.UserNeverAccessedModel] will
	// be returned if there is no record of the user logging in to this model.
	LastModelLogin(context.Context, coreuser.Name, coremodel.UUID) (time.Time, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelAgentVersion returns the agent version for the current model.
	GetModelAgentVersion(ctx context.Context) (version.Number, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// ReloadSpaces reloads the spaces.
	ReloadSpaces(ctx context.Context) error
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID string) (string, error)
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID string) (string, string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
}

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	// BackendSummaryInfoForModel returns a summary of the secret backends for a model.
	BackendSummaryInfoForModel(ctx context.Context, modelUUID coremodel.UUID) ([]*secretbackendservice.SecretBackendInfo, error)
}

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	// GetSupportedFeatures returns the set of features supported by the service.
	GetSupportedFeatures(ctx context.Context) (assumes.FeatureSet, error)
}

// Services holds the services needed by the model manager api.
type Services struct {
	// DomainServicesGetter is an interface for interacting with a factory for
	// creating model services.
	DomainServicesGetter DomainServicesGetter
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
	// NetworkService is an interface for interacting with the network service.
	NetworkService NetworkService
	// MachineService is an interface for interacting with the machine service.
	MachineService MachineService
	// ApplicationService is an interface for interacting with the application
	// service.
	ApplicationService ApplicationService
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently switched on.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

type domainServicesGetter struct {
	ctx facade.MultiModelContext
}

func (s domainServicesGetter) DomainServicesForModel(uuid coremodel.UUID) ModelDomainServices {
	return domainServices{domainServices: s.ctx.DomainServicesForModel(uuid)}
}

type domainServices struct {
	domainServices services.DomainServices
}

func (s domainServices) Agent() ModelAgentService { return s.domainServices.Agent() }

func (s domainServices) Config() ModelConfigService {
	return s.domainServices.Config()
}

func (s domainServices) ModelInfo() ModelInfoService { return s.domainServices.ModelInfo() }

func (s domainServices) Network() NetworkService {
	return s.domainServices.Network()
}

func (s domainServices) BlockCommand() BlockCommandService {
	return s.domainServices.BlockCommand()
}
