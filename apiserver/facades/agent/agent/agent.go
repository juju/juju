// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent implements the API interfaces
// used by the machine agent.

package agent

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// AgentPasswordService defines the methods required to set an agent password
// hash.
type AgentPasswordService interface {
	// SetUnitPassword sets the password hash for the given unit.
	SetUnitPassword(context.Context, unit.Name, string) error
	// SetMachinePassword sets the password hash for the given machine.
	SetMachinePassword(context.Context, machine.Name, string) error
	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(ctx context.Context, machineName machine.Name) (bool, error)
}

// ControllerConfigService is the interface that gets ControllerConfig form DB.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgentsInPreferredOrder returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgentsInPreferredOrder(ctx context.Context) ([]string, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	// WatchCloud returns a watcher that observes changes to the specified cloud.
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)

	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	GetUnitLife(ctx context.Context, name unit.Name) (life.Value, error)
}

// MachineService is an interface that defines methods for managing machines.
type MachineService interface {
	// RequireMachineReboot sets the machine referenced by its UUID as requiring
	// a reboot.
	RequireMachineReboot(ctx context.Context, uuid machine.UUID) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by
	// its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid machine.UUID) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID
	// requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid machine.UUID) (bool, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or
	// shutdown.
	ShouldRebootOrShutdown(ctx context.Context, uuid machine.UUID) (machine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)

	// GetMachineLife returns the GetMachineLife status of the specified machine.
	// It returns a NotFound if the given machine doesn't exist.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(ctx context.Context, machineName machine.Name) (bool, error)
}

// ExternalControllerService defines the methods that the controller
// facade needs from the controller state.
type ExternalControllerService interface {
	// ControllerForModel returns the controller record that's associated
	// with the modelUUID.
	ControllerForModel(ctx context.Context, modelUUID string) (*crossmodel.ControllerInfo, error)

	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}

// AgentAPI implements the version 3 of the API provided to an agent.
type AgentAPI struct {
	*common.PasswordChanger
	*common.RebootFlagClearer
	*commonmodel.ModelConfigWatcher
	*common.ControllerConfigAPI

	credentialService       CredentialService
	controllerConfigService ControllerConfigService
	applicationService      ApplicationService
	machineService          MachineService
	st                      *state.State
	auth                    facade.Authorizer
	resources               facade.Resources
}

// NewAgentAPI returns an agent API facade.
func NewAgentAPI(
	auth facade.Authorizer,
	resources facade.Resources,
	st *state.State,
	agentPasswordService AgentPasswordService,
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	externalControllerService ExternalControllerService,
	machineService MachineService,
	modelConfigService ModelConfigService,
	applicationService ApplicationService,
	watcherRegistry facade.WatcherRegistry,
) *AgentAPI {
	getCanChange := func(context.Context) (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}

	return &AgentAPI{
		PasswordChanger:    common.NewPasswordChanger(agentPasswordService, st, getCanChange),
		RebootFlagClearer:  common.NewRebootFlagClearer(machineService, getCanChange),
		ModelConfigWatcher: commonmodel.NewModelConfigWatcher(modelConfigService, watcherRegistry),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			st,
			controllerConfigService,
			controllerNodeService,
			externalControllerService,
		),
		controllerConfigService: controllerConfigService,
		applicationService:      applicationService,
		machineService:          machineService,
		st:                      st,
		auth:                    auth,
		resources:               resources,
	}
}

func (api *AgentAPI) GetEntities(ctx context.Context, args params.Entities) params.AgentGetEntitiesResults {
	results := params.AgentGetEntitiesResults{
		Entities: make([]params.AgentGetEntitiesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Entities[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Allow only for the owner agent.
		// Note: having a bulk API call for this is utter madness, given that
		// this check means we can only ever return a single object.
		if !api.auth.AuthOwner(tag) {
			results.Entities[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		// Handle units using the domain service.
		// Eventually all entities will be supported via dqlite.
		results.Entities[i] = api.getOneEntity(ctx, tag)
		if err != nil {
			results.Entities[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results
}

func (api *AgentAPI) getOneEntity(ctx context.Context, tag names.Tag) params.AgentGetEntitiesResult {
	switch tag.Kind() {
	case names.UnitTagKind:
		unitName, err := unit.NewName(tag.Id())
		if err != nil {
			return params.AgentGetEntitiesResult{
				Error: apiservererrors.ServerError(err),
			}
		}
		lifeValue, err := api.applicationService.GetUnitLife(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			err = errors.NotFoundf("unit %s", unitName)
		}

		return params.AgentGetEntitiesResult{
			Life:  lifeValue,
			Error: apiservererrors.ServerError(err),
		}

	case names.MachineTagKind:
		machineName := machine.Name(tag.Id())

		lifeValue, err := api.machineService.GetMachineLife(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return params.AgentGetEntitiesResult{
				Error: apiservererrors.ServerError(errors.NotFoundf("machine %q", machineName)),
			}
		} else if err != nil {
			return params.AgentGetEntitiesResult{
				Error: apiservererrors.ServerError(err),
			}
		}

		isController, err := api.machineService.IsMachineController(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return params.AgentGetEntitiesResult{
				Error: apiservererrors.ServerError(errors.NotFoundf("machine %q", machineName)),
			}
		} else if err != nil {
			return params.AgentGetEntitiesResult{
				Error: apiservererrors.ServerError(err),
			}
		}

		// All machines can host units, so we just need to work out if it's
		// a controller machine, then it can host models.
		jobs := []model.MachineJob{model.JobHostUnits}
		if isController {
			jobs = append(jobs, model.JobManageModel)
		}

		return params.AgentGetEntitiesResult{
			Life:          lifeValue,
			Jobs:          jobs,
			ContainerType: instance.LXD,
			Error:         apiservererrors.ServerError(err),
		}

	default:
		return params.AgentGetEntitiesResult{
			Error: apiservererrors.ServerError(apiservererrors.NotSupportedError(tag, "entities")),
		}
	}
}

func (api *AgentAPI) StateServingInfo(ctx context.Context) (result params.StateServingInfo, err error) {
	if !api.auth.AuthController() {
		err = apiservererrors.ErrPerm
		return
	}
	info, err := api.st.StateServingInfo()
	if err != nil {
		return params.StateServingInfo{}, errors.Trace(err)
	}

	result = params.StateServingInfo{
		APIPort:        info.APIPort,
		StatePort:      info.StatePort,
		Cert:           info.Cert,
		PrivateKey:     info.PrivateKey,
		CAPrivateKey:   info.CAPrivateKey,
		SharedSecret:   info.SharedSecret,
		SystemIdentity: info.SystemIdentity,
	}

	return result, nil
}

// MongoIsMaster is called by the IsMaster API call
// instead of mongo.IsMaster. It exists so it can
// be overridden by tests.
var MongoIsMaster = mongo.IsMaster

func (api *AgentAPI) IsMaster(ctx context.Context) (params.IsMasterResult, error) {
	if !api.auth.AuthController() {
		return params.IsMasterResult{}, apiservererrors.ErrPerm
	}

	switch tag := api.auth.GetAuthTag().(type) {
	case names.MachineTag:
		machine, err := api.st.Machine(tag.Id())
		if err != nil {
			return params.IsMasterResult{}, apiservererrors.ErrPerm
		}

		session := api.st.MongoSession()
		isMaster, err := MongoIsMaster(session, machine)
		return params.IsMasterResult{Master: isMaster}, err
	default:
		return params.IsMasterResult{}, errors.Errorf("authenticated entity is not a Machine")
	}
}

// WatchCredentials watches for changes to the specified credentials.
func (api *AgentAPI) WatchCredentials(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	if !api.auth.AuthController() {
		return params.NotifyWatchResults{}, apiservererrors.ErrPerm
	}

	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		credentialTag, err := names.ParseCloudCredentialTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watch, err := api.credentialService.WatchCredential(ctx, credential.KeyFromTag(credentialTag))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Consume the initial event. Technically, API calls to Watch
		// 'transmit' the initial event in the Watch response. But
		// NotifyWatchers have no state to transmit.
		if _, ok := <-watch.Changes(); ok {
			results.Results[i].NotifyWatcherId = api.resources.Register(watch)
		} else {
			watch.Kill()
			results.Results[i].Error = apiservererrors.ServerError(watch.Wait())
		}
	}
	return results, nil
}
