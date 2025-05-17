// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"time"

	"github.com/juju/description/v9"

	"github.com/juju/juju/apiserver/facade"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/modeldefaults"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/state"
)

// ModelDomainServices is a factory for creating model info services.
type ModelDomainServices interface {
	// Agent returns the model's agent service.
	Agent() ModelAgentService

	// Config returns the model config service.
	Config() ModelConfigService

	// ModelInfo returns the model service for the model.
	// Note: This should be called model, but we have naming conflicts with
	// the model service. As this is only for model information, we
	// can rename it to the more obscure version.
	ModelInfo() ModelInfoService

	// Network returns the space service.
	Network() NetworkService

	// BlockCommand returns the block command service.
	BlockCommand() BlockCommandService

	// Machine returns the machine service.
	Machine() MachineService

	// Status returns the status service.
	Status() StatusService
}

// DomainServicesGetter is a factory for creating model services.
type DomainServicesGetter interface {
	DomainServicesForModel(context.Context, coremodel.UUID) (ModelDomainServices, error)
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

// ModelService defines an interface for interacting with the model service.
type ModelService interface {
	// CreateModel creates a model returning the resultant model's new ID.
	CreateModel(context.Context, model.GlobalModelCreationArgs) (coremodel.UUID, func(context.Context) error, error)

	// UpdateCredential is responsible for updating the cloud credential
	// associated with a model. The cloud credential must be of the same cloud type
	// as that of the model.
	// The following error types can be expected to be returned:
	// - modelerrors.NotFound: When the model does not exist.
	// - errors.NotFound: When the cloud or credential cannot be found.
	// - errors.NotValid: When the cloud credential is not of the same cloud as the
	// model or the model uuid is not valid.
	UpdateCredential(ctx context.Context, uuid coremodel.UUID, key credential.Key) error

	// Model returns the model associated with the provided uuid.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)

	// DefaultModelCloudInfo returns the default cloud name and region name
	// that should be used for newly created models that haven't had
	// either cloud or credential specified.
	DefaultModelCloudInfo(context.Context) (string, string, error)

	// DeleteModel deletes the give model.
	DeleteModel(context.Context, coremodel.UUID, ...model.DeleteModelOption) error

	// ListAllModels returns a list of all models.
	ListAllModels(context.Context) ([]coremodel.Model, error)

	// ListModelsForUser returns a list of models for the given user.
	ListModelsForUser(context.Context, coreuser.UUID) ([]coremodel.Model, error)

	// ListModelUUIDs returns a list of all model UUIDs in the controller that
	// are active.
	ListModelUUIDs(context.Context) ([]coremodel.UUID, error)

	// ListModelUUIDsForUser returns a list of model UUIDs that the supplied
	// user has access to. If the user supplied does not have access to any
	// models then an empty slice is returned.
	// The following errors can be expected:
	// - [github.com/juju/juju/core/errors.NotValid] when the user uuid supplied
	// is not valid.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user
	// does not exist.
	ListModelUUIDsForUser(context.Context, coreuser.UUID) ([]coremodel.UUID, error)

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
}

// ModelDefaultsService defines a interface for interacting with the model
// defaults.
type ModelDefaultsService interface {
	// CloudDefaults returns the default attribute details for a specified cloud.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	CloudDefaults(ctx context.Context, cloudName string) (modeldefaults.ModelDefaultAttributes, error)

	// UpdateCloudDefaults saves the specified default attribute details for a cloud.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	UpdateCloudDefaults(ctx context.Context, cloudName string, updateAttrs map[string]any) error

	// UpdateCloudRegionDefaults saves the specified default attribute details for a cloud region.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	UpdateCloudRegionDefaults(ctx context.Context, cloudName, regionName string, updateAttrs map[string]any) error

	// RemoveCloudDefaults deletes the specified default attribute details for a cloud.
	// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't exist.
	RemoveCloudDefaults(ctx context.Context, cloudName string, removeAttrs []string) error

	// RemoveCloudRegionDefaults deletes the specified default attributes for a
	// cloud region. It returns an error satisfying [clouderrors.NotFound] if
	// the cloud doesn't exist.
	RemoveCloudRegionDefaults(ctx context.Context, cloudName, regionName string, removeAttrs []string) error
}

// ModelInfoService defines a interface for interacting with the underlying
// state.
type ModelInfoService interface {
	// CreateModel is responsible for creating a new model within the model
	// database. Upon creating the model any information required in the model's
	// provider will be initialised.
	//
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.AlreadyExists] when the model
	// uuid is already in use.
	CreateModel(context.Context) error

	// CreateModelWithAgentVersion is responsible for creating a new model within
	// the model database using the specified agent version. Upon creating the
	// model any information required in the model's provider will be
	// initialised.
	//
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.AlreadyExists] when the model
	// uuid is already in use.
	// - [github.com/juju/juju/domain/model/errors.AgentVersionNotSupported]
	// when the agent version is not supported.
	CreateModelWithAgentVersion(context.Context, semversion.Number) error

	// CreateModelWithAgentVersionStream is responsible for creating a new model
	// within the model database using the specified agent version and agent
	// stream. Upon creating the model any information required in the model's
	// provider will be initialised.
	//
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.AlreadyExists] when the model
	// uuid is already in use.
	// - [github.com/juju/juju/domain/model/errors.AgentVersionNotSupported]
	// when the agent version is not supported.
	// - [github.com/juju/juju/core/errors.NotValid] when the agent stream is
	// not valid.
	CreateModelWithAgentVersionStream(
		context.Context, semversion.Number, agentbinary.AgentStream,
	) error

	// DeleteModel is responsible for deleting a model.
	DeleteModel(context.Context) error

	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)

	// GetModelSummary returns a summary of the current model as a
	// [coremodel.ModelSummary] type.
	// The following error types can be expected:
	// - [modelerrors.NotFound] when the model does not exist.
	GetModelSummary(ctx context.Context) (coremodel.ModelSummary, error)

	// GetUserModelSummary returns a summary of the current model from the
	// provided user's perspective.
	// The following error types can be expected:
	// - [modelerrors.NotFound] when the model does not exist.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user
	// is not found for the given user uuid.
	// - [github.com/juju/juju/domain/access/errors.AccessNotFound] when the
	// user does not have access to the model.
	GetUserModelSummary(ctx context.Context, userUUID coreuser.UUID) (coremodel.UserModelSummary, error)

	// IsControllerModel returns true if the model is the controller model.
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] when the model does not exist.
	IsControllerModel(context.Context) (bool, error)
}

// ModelExporter defines a interface for exporting models.
type ModelExporter interface {
	// ExportModelPartial exports the current model into a partial description
	// model. This can be serialized into yaml and then imported.
	ExportModelPartial(context.Context, state.ExportConfig, objectstore.ObjectStore) (description.Model, error)
}

// CredentialService exposes State methods needed by credential manager.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given key.
	CloudCredential(ctx context.Context, id credential.Key) (jujucloud.Credential, error)
}

// AccessService defines a interface for interacting the users and permissions
// of a controller.
type AccessService interface {
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when no user
	// exists for the supplied user name.
	// - [github.com/juju/juju/domain/access/errors.UserNameNotValid] when the
	// user name is not valid.
	GetUserUUIDByName(context.Context, coreuser.Name) (coreuser.UUID, error)
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
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// does not exist.
	GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error)
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
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error)
}

// StatusService returns the status of a applications, and units and machines.
type StatusService interface {
	// GetApplicationAndUnitModelStatuses returns the application name and unit
	// count for each model for the model status request.
	GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error)

	// GetModelStatusInfo returns information about the current model for the
	// purpose of reporting its status.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error)

	// GetModelStatus returns the current status of the model.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	GetModelStatus(ctx context.Context) (status.ModelStatus, error)
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
	// ModelAgentService is an interface for interacting with the model agent
	// service.
	ModelAgentService ModelAgentService
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

func (s domainServicesGetter) DomainServicesForModel(ctx context.Context, uuid coremodel.UUID) (ModelDomainServices, error) {
	svc, err := s.ctx.DomainServicesForModel(ctx, uuid)
	if err != nil {
		return nil, err
	}
	return domainServices{domainServices: svc}, nil
}

type domainServices struct {
	domainServices services.DomainServices
}

func (s domainServices) Agent() ModelAgentService {
	return s.domainServices.Agent()
}

func (s domainServices) Config() ModelConfigService {
	return s.domainServices.Config()
}

func (s domainServices) ModelInfo() ModelInfoService {
	return s.domainServices.ModelInfo()
}

func (s domainServices) Network() NetworkService {
	return s.domainServices.Network()
}

func (s domainServices) Machine() MachineService {
	return s.domainServices.Machine()
}

func (s domainServices) BlockCommand() BlockCommandService {
	return s.domainServices.BlockCommand()
}

func (s domainServices) Status() StatusService {
	return s.domainServices.Status()
}
