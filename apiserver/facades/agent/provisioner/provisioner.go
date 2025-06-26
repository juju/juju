// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	commonnetwork "github.com/juju/juju/apiserver/common/network"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/constraints"
	corecontainer "github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/network/containerizer"
	"github.com/juju/juju/internal/ssh"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
)

// ProvisionerAPI provides access to the Provisioner API facade.
type ProvisionerAPI struct {
	*common.ControllerConfigAPI
	*common.DeadEnsurer
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser
	*commonmodel.ModelConfigWatcher
	*commonmodel.ModelMachinesWatcher
	*common.InstanceIdGetter
	*common.ToolsGetter
	*commonnetwork.NetworkConfigAPI

	networkService            NetworkService
	st                        *state.State
	controllerConfigService   ControllerConfigService
	cloudImageMetadataService CloudImageMetadataService
	agentProvisionerService   AgentProvisionerService
	keyUpdaterService         KeyUpdaterService
	modelConfigService        ModelConfigService
	modelInfoService          ModelInfoService
	machineService            MachineService
	statusService             StatusService
	applicationService        ApplicationService
	resources                 facade.Resources
	authorizer                facade.Authorizer
	storageProviderRegistry   storage.ProviderRegistry
	storagePoolGetter         StoragePoolGetter
	configGetter              environs.EnvironConfigGetter
	getAuthFunc               common.GetAuthFunc
	getCanModify              common.GetAuthFunc
	toolsFinder               common.ToolsFinder
	logger                    logger.Logger
	clock                     clock.Clock

	// Hold on to the controller UUID, as we'll reuse it for a lot of
	// calls.
	controllerUUID string

	// Used for MaybeWriteLXDProfile()
	mu sync.Mutex
}

// MakeProvisionerAPI creates a new server-side ProvisionerAPI facade.
func MakeProvisionerAPI(stdCtx context.Context, ctx facade.ModelContext) (*ProvisionerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	getAuthFunc := func(context.Context) (common.AuthFunc, error) {
		isModelManager := authorizer.AuthController()
		isMachineAgent := authorizer.AuthMachineAgent()
		authEntityTag := authorizer.GetAuthTag()

		return func(tag names.Tag) bool {
			if isMachineAgent && tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			switch tag := tag.(type) {
			case names.MachineTag:
				parentId := corecontainer.ParentId(tag.Id())
				if parentId == "" {
					// All top-level machines are accessible by the controller.
					return isModelManager
				}
				// All containers with the authenticated machine as a
				// parent are accessible by it.
				// TODO(dfc) sometimes authEntity tag is nil, which is fine because nil is
				// only equal to nil, but it suggests someone is passing an authorizer
				// with a nil tag.
				return isMachineAgent && names.NewMachineTag(parentId) == authEntityTag
			default:
				return false
			}
		}, nil
	}
	getCanModify := func(context.Context) (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getAuthOwner := func(context.Context) (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServices := ctx.DomainServices()

	agentService := domainServices.Agent()
	agentBinaryService := domainServices.AgentBinary()
	applicationService := domainServices.Application()
	machineService := domainServices.Machine()
	statusService := domainServices.Status()
	modelInfoService := domainServices.ModelInfo()
	cloudService := domainServices.Cloud()
	credentialService := domainServices.Credential()
	modelConfigService := domainServices.Config()
	controllerNodeService := domainServices.ControllerNode()
	agentPasswordService := domainServices.AgentPassword()
	controllerConfigService := domainServices.ControllerConfig()
	networkService := domainServices.Network()
	storageService := domainServices.Storage()
	keyUpdaterService := domainServices.KeyUpdater()
	cloudImageMetadataService := domainServices.CloudImageMetadata()
	agentProvisionerService := domainServices.AgentProvisioner()
	externalControllerService := domainServices.ExternalController()

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:              model,
		CloudService:       cloudService,
		CredentialService:  credentialService,
		ModelConfigService: modelConfigService,
	}

	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	isCaasModel := modelInfo.Type == coremodel.CAAS

	var env storage.ProviderRegistry
	if isCaasModel {
		env, err = stateenvirons.GetNewCAASBrokerFunc(caas.New)(model, cloudService, credentialService, modelConfigService)
	} else {
		env, err = environs.GetEnviron(stdCtx, configGetter, environs.NoopCredentialInvalidator(), environs.New)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageProviderRegistry := provider.NewStorageProviderRegistry(env)

	netConfigAPI, err := commonnetwork.NewNetworkConfigAPI(
		stdCtx, st, domainServices.ModelInfo(), domainServices.Network(), getCanModify)
	if err != nil {
		return nil, errors.Annotate(err, "instantiating network config API")
	}

	controllerNodeServices := domainServices.ControllerNode()
	urlGetter := common.NewToolsURLGetter(string(modelInfo.UUID), controllerNodeServices)

	watcherRegistry := ctx.WatcherRegistry()
	modelConfigWatcher := commonmodel.NewModelConfigWatcher(modelConfigService, watcherRegistry)

	resources := ctx.Resources()

	api := &ProvisionerAPI{
		DeadEnsurer:          common.NewDeadEnsurer(st, getAuthFunc, machineService),
		PasswordChanger:      common.NewPasswordChanger(agentPasswordService, st, getAuthFunc),
		LifeGetter:           common.NewLifeGetter(applicationService, machineService, st, getAuthFunc, ctx.Logger()),
		APIAddresser:         common.NewAPIAddresser(controllerNodeService, watcherRegistry),
		ModelConfigWatcher:   modelConfigWatcher,
		ModelMachinesWatcher: commonmodel.NewModelMachinesWatcher(st, resources, authorizer),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			st,
			controllerConfigService,
			controllerNodeServices,
			externalControllerService,
		),
		NetworkConfigAPI:          netConfigAPI,
		networkService:            networkService,
		st:                        st,
		controllerConfigService:   controllerConfigService,
		agentProvisionerService:   agentProvisionerService,
		cloudImageMetadataService: cloudImageMetadataService,
		keyUpdaterService:         keyUpdaterService,
		modelConfigService:        modelConfigService,
		modelInfoService:          modelInfoService,
		machineService:            machineService,
		statusService:             statusService,
		applicationService:        applicationService,
		resources:                 resources,
		authorizer:                authorizer,
		configGetter:              configGetter,
		storageProviderRegistry:   storageProviderRegistry,
		storagePoolGetter:         storageService,
		getAuthFunc:               getAuthFunc,
		getCanModify:              getCanModify,
		controllerUUID:            ctx.ControllerUUID(),
		logger:                    ctx.Logger().Child("provisioner"),
		clock:                     ctx.Clock(),
	}
	if isCaasModel {
		return api, nil
	}

	api.InstanceIdGetter = common.NewInstanceIdGetter(machineService, getAuthFunc)

	api.toolsFinder = common.NewToolsFinder(
		st, urlGetter,
		ctx.ControllerObjectStore(),
		agentBinaryService,
	)
	api.ToolsGetter = common.NewToolsGetter(agentService, st, urlGetter, api.toolsFinder, getAuthOwner)
	return api, nil
}

// ProvisionerAPIV11 provides v10 of the provisioner facade.
// It relies on agent-set origin when calling SetHostMachineNetworkConfig.
type ProvisionerAPIV11 struct {
	*ProvisionerAPI
}

func (api *ProvisionerAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (*state.Machine, error) {
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	return entity.(*state.Machine), nil
}

// getInstanceID returns the instance ID for the given machine.
func (api *ProvisionerAPI) getInstanceID(ctx context.Context, machineID string) (instance.Id, error) {
	machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", apiservererrors.ServerError(err)
	}
	if err != nil {
		return "", err
	}
	return api.machineService.GetInstanceID(ctx, machineUUID)
}

func (api *ProvisionerAPI) watchOneMachineContainers(ctx context.Context, arg params.WatchContainer) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	tag, err := names.ParseMachineTag(arg.MachineTag)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	if !canAccess(tag) {
		return nothing, apiservererrors.ErrPerm
	}
	machine, err := api.st.Machine(tag.Id())
	if err != nil {
		return nothing, err
	}
	var watch state.StringsWatcher
	if arg.ContainerType != "" {
		watch = machine.WatchContainers(instance.ContainerType(arg.ContainerType))
	} else {
		watch = machine.WatchAllContainers()
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: api.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchContainers starts a StringsWatcher to watch containers deployed to
// any machine passed in args.
func (api *ProvisionerAPI) WatchContainers(ctx context.Context, args params.WatchContainers) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Params)),
	}
	for i, arg := range args.Params {
		watcherResult, err := api.watchOneMachineContainers(ctx, arg)
		result.Results[i] = watcherResult
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// WatchAllContainers starts a StringsWatcher to watch all containers deployed to
// any machine passed in args.
func (api *ProvisionerAPI) WatchAllContainers(ctx context.Context, args params.WatchContainers) (params.StringsWatchResults, error) {
	return api.WatchContainers(ctx, args)
}

// SetSupportedContainers updates the list of containers supported by the
// machines passed in args.
// Deprecated: This method doesn't do anything and can be removed in the future.
func (api *ProvisionerAPI) SetSupportedContainers(ctx context.Context, args params.MachineContainersParams) (params.ErrorResults, error) {
	return params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}, nil
}

// SupportedContainers returns the list of containers supported by the machines
// passed in args.
func (api *ProvisionerAPI) SupportedContainers(ctx context.Context, args params.Entities) (params.MachineContainerResults, error) {
	result := params.MachineContainerResults{
		Results: make([]params.MachineContainerResult, len(args.Entities)),
	}

	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFound)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		containerTypes, err := api.machineService.GetSupportedContainersTypes(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFound)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Container types will always return a non-empty slice of `[lxd]`.
		// We can make this guarantee at the moment, because we only support
		// deployment to ubuntu machines. If this ever changes, we will need
		// to ratify this logic.
		// As a result we can set the determined field to true.
		result.Results[i].ContainerTypes = containerTypes
		result.Results[i].Determined = true
	}
	return result, nil
}

// ContainerManagerConfig returns information from the model config that is
// needed for configuring the container manager.
func (api *ProvisionerAPI) ContainerManagerConfig(ctx context.Context, args params.ContainerManagerConfigParams) (params.ContainerManagerConfig, error) {
	var result params.ContainerManagerConfig

	cfg, err := api.agentProvisionerService.ContainerManagerConfigForType(ctx, args.Type)
	if err != nil {
		return result, fmt.Errorf("cannot get container manager config: %w", err)
	}

	containerNetworkingMethod, err := api.agentProvisionerService.ContainerNetworkingMethod(ctx)
	if err != nil {
		return result, fmt.Errorf("cannot get container networking method: %w", err)
	}

	result.ManagerConfig = make(map[string]string)
	result.ManagerConfig[container.ConfigModelUUID] = cfg.ModelID.String()
	result.ManagerConfig[config.LXDSnapChannel] = cfg.LXDSnapChannel
	if cfg.ImageMetadataURL != "" {
		result.ManagerConfig[config.ContainerImageMetadataURLKey] = cfg.ImageMetadataURL
	}
	if cfg.MetadataDefaultsDisabled {
		result.ManagerConfig[config.ContainerImageMetadataDefaultsDisabledKey] = "true"
	}
	result.ManagerConfig[config.ContainerImageStreamKey] = cfg.ImageStream
	result.ManagerConfig[config.ContainerNetworkingMethodKey] = containerNetworkingMethod.String()

	return result, nil
}

// ContainerConfig returns information from the model config that is
// needed for container cloud-init.
func (api *ProvisionerAPI) ContainerConfig(ctx context.Context) (params.ContainerConfig, error) {
	containerConfig, err := api.agentProvisionerService.ContainerConfig(ctx)
	if err != nil {
		return params.ContainerConfig{}, fmt.Errorf("cannot get container config: %w", err)
	}

	// Add authorised keys to container config
	containerKeys, err := api.keyUpdaterService.GetInitialAuthorisedKeysForContainer(ctx)
	if err != nil {
		return params.ContainerConfig{}, fmt.Errorf("cannot get authorised keys for container config: %w", err)
	}
	authorizedKeys := ssh.MakeAuthorizedKeysString(containerKeys)

	return params.ContainerConfig{
		ProviderType:               containerConfig.ProviderType,
		AuthorizedKeys:             authorizedKeys,
		SSLHostnameVerification:    containerConfig.SSLHostnameVerification,
		LegacyProxy:                containerConfig.LegacyProxy,
		JujuProxy:                  containerConfig.JujuProxy,
		AptProxy:                   containerConfig.AptProxy,
		SnapProxy:                  containerConfig.SnapProxy,
		SnapStoreAssertions:        containerConfig.SnapStoreAssertions,
		SnapStoreProxyID:           containerConfig.SnapStoreProxyID,
		SnapStoreProxyURL:          containerConfig.SnapStoreProxyURL,
		AptMirror:                  containerConfig.AptMirror,
		CloudInitUserData:          containerConfig.CloudInitUserData,
		ContainerInheritProperties: containerConfig.ContainerInheritProperties,
		UpdateBehavior: &params.UpdateBehavior{
			EnableOSRefreshUpdate: containerConfig.EnableOSRefreshUpdate,
			EnableOSUpgrade:       containerConfig.EnableOSUpgrade,
		},
	}, nil
}

// MachinesWithTransientErrors returns status data for machines with provisioning
// errors which are transient.
func (api *ProvisionerAPI) MachinesWithTransientErrors(ctx context.Context) (params.StatusResults, error) {
	var results params.StatusResults
	canAccessFunc, err := api.getAuthFunc(ctx)
	if err != nil {
		return results, err
	}
	// TODO (wallyworld) - add state.State API for more efficient machines query
	machines, err := api.st.AllMachines()
	if err != nil {
		return results, err
	}
	for _, machine := range machines {
		if !canAccessFunc(machine.Tag()) {
			continue
		}
		_, err = api.getInstanceID(ctx, machine.Tag().Id())
		if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
			return results, err
		}
		if err == nil {
			// Machine may have been provisioned but machiner hasn't set the
			// status to Started yet.
			continue
		}
		machineName := coremachine.Name(machine.Tag().Id())
		statusInfo, err := api.statusService.GetInstanceStatus(ctx, machineName)
		if err != nil {
			continue
		}

		var result params.StatusResult
		result.Status = statusInfo.Status.String()
		result.Info = statusInfo.Message
		result.Data = statusInfo.Data
		if statusInfo.Status != status.Error && statusInfo.Status != status.ProvisioningError {
			continue
		}
		// Transient errors are marked as such in the status data.
		if transient, ok := result.Data["transient"].(bool); !ok || !transient {
			continue
		}
		result.Id = machine.Id()
		result.Life = life.Value(machine.Life().String())
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// AvailabilityZone returns a provider-specific availability zone for each given machine entity
func (api *ProvisionerAPI) AvailabilityZone(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		}
		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(tag.Id()))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(fmt.Errorf("%w: %w", err, errors.NotFound))
			continue
		}
		hc, err := api.machineService.GetHardwareCharacteristics(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFound)
			continue
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if hc.AvailabilityZone != nil {
			result.Results[i].Result = *hc.AvailabilityZone
		} else {
			result.Results[i].Result = ""
		}
	}
	return result, nil
}

// KeepInstance returns the keep-instance value for each given machine entity.
func (api *ProvisionerAPI) KeepInstance(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{

		Results: make([]params.BoolResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		keep, err := api.machineService.ShouldKeepInstance(ctx, coremachine.Name(tag.Id()))
		result.Results[i].Result = keep
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// DistributionGroup returns, for each given machine entity,
// a slice of instance.Ids that belong to the same distribution
// group as that machine. This information may be used to
// distribute instances for high availability.
func (api *ProvisionerAPI) DistributionGroup(ctx context.Context, args params.Entities) (params.DistributionGroupResults, error) {
	result := params.DistributionGroupResults{
		Results: make([]params.DistributionGroupResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineName := coremachine.Name(tag.Id())
		isController, err := api.machineService.IsMachineController(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("machine %q", machineName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// If the machine is a controller, return controller instances.
		// Otherwise, return instances with services in common with the machine
		// being provisioned.
		if isController {
			result.Results[i].Result, err = api.controllerInstances(ctx)
		} else {
			result.Results[i].Result, err = api.commonServiceInstances(ctx, machineName)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// controllerInstances returns all environ manager instances.
func (api *ProvisionerAPI) controllerInstances(ctx context.Context) ([]instance.Id, error) {
	controllerIds, err := api.st.ControllerIds()
	if err != nil {
		return nil, err
	}
	instances := make([]instance.Id, 0, len(controllerIds))
	for _, id := range controllerIds {
		instanceId, err := api.getInstanceID(ctx, id)
		if errors.Is(err, machineerrors.NotProvisioned) {
			continue
		} else if err != nil {
			return nil, err
		}
		instances = append(instances, instanceId)
	}
	return instances, nil
}

// commonServiceInstances returns instances with
// services in common with the specified machine.
func (api *ProvisionerAPI) commonServiceInstances(ctx context.Context, machineName coremachine.Name) ([]instance.Id, error) {
	unitNames, err := api.applicationService.GetUnitNamesOnMachine(ctx, machineName)
	if errors.Is(err, applicationerrors.MachineNotFound) {
		return nil, errors.NotFoundf("machine %q", machineName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	instanceIdSet := make(set.Strings)
	for _, unitName := range unitNames {
		if _, isPrincipal, err := api.applicationService.GetUnitPrincipal(ctx, unitName); err != nil {
			return nil, errors.Trace(err)
		} else if !isPrincipal {
			continue
		}

		appName := unitName.Application()
		machineIDs, err := state.ApplicationMachines(api.st, appName)
		if err != nil {
			return nil, err
		}
		for _, machineID := range machineIDs {
			instanceId, err := api.getInstanceID(ctx, machineID)
			if errors.Is(err, machineerrors.NotProvisioned) {
				continue
			}
			if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
				return nil, err
			}
			instanceIdSet.Add(instanceId.String())
		}
	}
	instanceIds := make([]instance.Id, instanceIdSet.Size())
	// Sort values to simplify testing.
	for i, instanceId := range instanceIdSet.SortedValues() {
		instanceIds[i] = instance.Id(instanceId)
	}
	return instanceIds, nil
}

// DistributionGroupByMachineId returns, for each given machine entity,
// a slice of machine.Ids that belong to the same distribution
// group as that machine. This information may be used to
// distribute instances for high availability.
func (api *ProvisionerAPI) DistributionGroupByMachineId(ctx context.Context, args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineName := coremachine.Name(tag.Id())
		isController, err := api.machineService.IsMachineController(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("machine %q", machineName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// If the machine is a controller, return controller instances.
		// Otherwise, return instances with services in common with the machine
		// being provisioned.
		if isController {
			result.Results[i].Result, err = controllerMachineIds(api.st, machineName)
		} else {
			result.Results[i].Result, err = api.commonApplicationMachineId(ctx, api.st, machineName)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// controllerMachineIds returns a slice of all other environ manager machine.Ids.
func controllerMachineIds(st *state.State, mName coremachine.Name) ([]string, error) {
	ids, err := st.ControllerIds()
	if err != nil {
		return nil, err
	}
	result := set.NewStrings(ids...)
	result.Remove(mName.String())
	return result.SortedValues(), nil
}

// commonApplicationMachineId returns a slice of machine.Ids with
// applications in common with the specified machine.
func (api *ProvisionerAPI) commonApplicationMachineId(ctx context.Context, st *state.State, mName coremachine.Name) ([]string, error) {
	applications, err := api.machineService.GetMachinePrincipalApplications(ctx, mName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.NotFoundf("machine %q", mName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	union := set.NewStrings()
	for _, app := range applications {
		machines, err := state.ApplicationMachines(st, app)
		if err != nil {
			return nil, err
		}
		union = union.Union(set.NewStrings(machines...))
	}
	union.Remove(mName.String())
	return union.SortedValues(), nil
}

// Constraints returns the constraints for each given machine entity.
func (api *ProvisionerAPI) Constraints(ctx context.Context, args params.Entities) (params.ConstraintsResults, error) {
	result := params.ConstraintsResults{
		Results: make([]params.ConstraintsResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machine, err := api.getMachine(canAccess, tag)
		if err == nil {
			var cons constraints.Value
			cons, err = machine.Constraints()
			if err == nil {
				result.Results[i].Constraints = cons
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// FindTools returns a List containing all tools matching the given parameters.
func (api *ProvisionerAPI) FindTools(ctx context.Context, args params.FindToolsParams) (params.FindToolsResult, error) {
	list, err := api.toolsFinder.FindAgents(ctx, common.FindAgentsParams{
		Number:      args.Number,
		Arch:        args.Arch,
		OSType:      args.OSType,
		AgentStream: args.AgentStream,
	})
	return params.FindToolsResult{
		List:  list,
		Error: apiservererrors.ServerError(err),
	}, nil
}

// SetInstanceInfo sets the provider specific machine id, nonce,
// metadata and network info for each given machine. Once set, the
// instance id cannot be changed.
func (api *ProvisionerAPI) SetInstanceInfo(ctx context.Context, args params.InstancesInfo) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Machines)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	setInstanceInfo := func(arg params.InstanceInfo) error {
		tag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return apiservererrors.ErrPerm
		}
		machine, err := api.getMachine(canAccess, tag)
		if err != nil {
			return err
		}
		volumes, err := storagecommon.VolumesToState(arg.Volumes)
		if err != nil {
			return err
		}
		volumeAttachments, err := storagecommon.VolumeAttachmentInfosToState(arg.VolumeAttachments)
		if err != nil {
			return err
		}

		ifaces := params.InterfaceInfoFromNetworkConfig(arg.NetworkConfig)
		devicesArgs, devicesAddrs := commonnetwork.NetworkInterfacesToStateArgs(ifaces)

		// TODO(nvinuesa): Remove double writing after finishing
		// implementing instance data on dqlite.
		err = machine.SetInstanceInfo(
			arg.InstanceId, arg.DisplayName, arg.Nonce, arg.Characteristics,
			devicesArgs, devicesAddrs,
			volumes, volumeAttachments, arg.CharmProfiles,
		)
		if err != nil {
			return errors.Annotatef(err, "cannot record provisioning info for %q", arg.InstanceId)
		}

		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(machine.Id()))
		if err != nil {
			return errors.Annotatef(err, "retrieving machineUUID for machine %q", machine.Id())
		}
		if err := api.machineService.SetMachineCloudInstance(
			ctx,
			machineUUID,
			arg.InstanceId,
			arg.DisplayName,
			arg.Nonce,
			arg.Characteristics,
		); err != nil {
			return errors.Annotatef(err, "setting machine cloud instance for machine uuid %q", machineUUID)
		}
		err = api.machineService.SetAppliedLXDProfileNames(ctx, machineUUID, arg.CharmProfiles)
		if errors.Is(err, machineerrors.NotProvisioned) {
			return errors.NotProvisionedf("machine %q", tag.Id())
		} else if err != nil {
			return errors.Annotatef(err, "setting lxd profiles for machine uuid %q", machineUUID)
		}

		return nil
	}
	for i, arg := range args.Machines {
		err := setInstanceInfo(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// WatchMachineErrorRetry returns a NotifyWatcher that notifies when
// the provisioner should retry provisioning machines with transient errors.
func (api *ProvisionerAPI) WatchMachineErrorRetry(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	if !api.authorizer.AuthController() {
		return result, apiservererrors.ErrPerm
	}
	watch := newWatchMachineErrorRetry()
	// Consume any initial event and forward it to the result.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}

// ReleaseContainerAddresses finds addresses allocated to a container and marks
// them as Dead, to be released and removed. It accepts container tags as
// arguments.
func (api *ProvisionerAPI) ReleaseContainerAddresses(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		api.logger.Errorf(ctx, "failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	// Loop over the passed container tags.
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			api.logger.Warningf(ctx, "failed to parse machine tag %q: %v", entity.Tag, err)
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		// The auth function (canAccess) checks that the machine is a
		// top level machine (we filter those out next) or that the
		// machine has the host as a parent.
		guest, err := api.getMachine(canAccess, tag)
		if err != nil {
			api.logger.Warningf(ctx, "failed to get machine %q: %v", tag, err)
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		} else if !guest.IsContainer() {
			err = errors.Errorf("cannot mark addresses for removal for %q: not a container", tag)
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// TODO(dimitern): Release those via the provider once we have
		// Environ.ReleaseContainerAddresses. See LP bug http://pad.lv/1585878
		err = guest.RemoveAllAddresses()
		if err != nil {
			api.logger.Warningf(ctx, "failed to remove container %q addresses: %v", tag, err)
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}

	return result, nil
}

// PrepareContainerInterfaceInfo allocates an address and returns information to
// configure networking for a container. It accepts container tags as arguments.
func (api *ProvisionerAPI) PrepareContainerInterfaceInfo(ctx context.Context, args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	return api.prepareOrGetContainerInterfaceInfo(ctx, args, false)
}

// TODO (manadart 2020-07-23): This method is not used and can be removed when
// next this facade version is bumped.
// We then don't need the parameterised prepareOrGet...

// GetContainerInterfaceInfo returns information to configure networking for a
// container. It accepts container tags as arguments.
func (api *ProvisionerAPI) GetContainerInterfaceInfo(ctx context.Context, args params.Entities) (
	params.MachineNetworkConfigResults,
	error,
) {
	return api.prepareOrGetContainerInterfaceInfo(ctx, args, true)
}

// perContainerHandler is the interface we need to trigger processing on
// every container passed in as a list of things to process.
type perContainerHandler interface {
	// ProcessOneContainer is called once we've assured ourselves that all of
	// the access permissions are correct and the container is ready to be
	// processed.
	// env is the Environment you are working, idx is the index for this
	// container (used for deciding where to store results), host is the
	// machine that is hosting the container.
	// Any errors that are returned from ProcessOneContainer will be turned
	// into ServerError and handed to SetError
	ProcessOneContainer(
		ctx context.Context,
		env environs.Environ,
		policy BridgePolicy, idx int,
		host Machine, hostIsManual bool,
		guest Machine,
		hostInstanceID, guestInstanceID instance.Id,
		allSubnets network.SubnetInfos,
	) error

	// SetError will be called whenever there is a problem with the a given
	// request. Generally this just does result.Results[i].Error = error
	// but the Result type is opaque so we can't do it ourselves.
	SetError(resultIndex int, err error)

	// ConfigType indicates the type of config the handler is getting for
	// for error messaging.
	ConfigType() string
}

func (api *ProvisionerAPI) processEachContainer(ctx context.Context, args params.Entities, handler perContainerHandler) error {
	env, hostMachine, canAccess, err := api.prepareContainerAccessEnvironment(ctx)
	if err != nil {
		// Overall error
		return errors.Trace(err)
	}

	hostInstanceID, err := api.getInstanceID(ctx, hostMachine.Id())
	if errors.Is(err, machineerrors.NotProvisioned) {
		return errors.NotProvisionedf("cannot prepare container %s config: host machine %q",
			handler.ConfigType(), hostMachine)
	}
	if err != nil {
		return errors.Trace(err)
	}

	hostIsManual, err := api.machineService.IsMachineManuallyProvisioned(ctx, coremachine.Name(hostMachine.Id()))
	if err != nil {
		return errors.Trace(err)
	}

	containerNetworkingMethod, err := api.agentProvisionerService.ContainerNetworkingMethod(ctx)
	if err != nil {
		return fmt.Errorf("cannot get container networking method: %w", err)
	}

	policy, err := containerizer.NewBridgePolicy(ctx, api.networkService, containerNetworkingMethod)
	if err != nil {
		return errors.Trace(err)
	}

	allSubnets, err := api.networkService.GetAllSubnets(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			handler.SetError(i, err)
			continue
		}

		// The auth function (canAccess) checks that the machine is a
		// top level machine (we filter those out next) or that the
		// machine has the host as a parent.
		guest, err := api.getMachine(canAccess, machineTag)
		if err != nil {
			handler.SetError(i, err)
			continue
		} else if !guest.IsContainer() {
			err = errors.Errorf("cannot prepare %s config for %q: not a container", handler.ConfigType(), machineTag)
			handler.SetError(i, err)
			continue
		}
		guestInstanceID, err := api.getInstanceID(ctx, hostMachine.Id())
		if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
			return errors.Trace(err)
		}
		if err := handler.ProcessOneContainer(
			ctx,
			env, policy, i,
			NewMachine(hostMachine),
			hostIsManual,
			NewMachine(guest),
			hostInstanceID,
			guestInstanceID,
			allSubnets,
		); err != nil {
			handler.SetError(i, err)
			continue
		}
	}
	return nil
}

type prepareOrGetHandler struct {
	result   params.MachineNetworkConfigResults
	maintain bool
	logger   logger.Logger
}

// SetError implements perContainerHandler.SetError
func (h *prepareOrGetHandler) SetError(idx int, err error) {
	h.result.Results[idx].Error = apiservererrors.ServerError(err)
}

// ConfigType implements perContainerHandler.ConfigType
func (h *prepareOrGetHandler) ConfigType() string {
	return "network"
}

// ProcessOneContainer implements perContainerHandler.ProcessOneContainer
func (h *prepareOrGetHandler) ProcessOneContainer(
	ctx context.Context,
	env environs.Environ, policy BridgePolicy,
	idx int,
	host Machine, hostIsManual bool,
	guest Machine,
	hostInstanceID, guestInstanceID instance.Id, _ network.SubnetInfos,
) error {
	if h.maintain {
		if guestInstanceID != "" {
			// Since we want to configure and create NICs on the
			// container before it starts, it must also be not
			// provisioned yet.
			return errors.Errorf("container %q already provisioned as %q", guest.Id(), guestInstanceID)
		}
	}

	// We do not ask the provider to allocate addresses for manually provisioned
	// machines as we do not expect such machines to be recognised (LP:1796106).
	var askProviderForAddress bool
	if !hostIsManual {
		askProviderForAddress = environs.SupportsContainerAddresses(ctx, env)
	}

	preparedInfo, err := policy.PopulateContainerLinkLayerDevices(host, guest, askProviderForAddress)
	if err != nil {
		return errors.Trace(err)
	}

	allocatedInfo := preparedInfo
	if askProviderForAddress {
		// supportContainerAddresses already checks that we can cast to an environ.Networking
		networking := env.(environs.Networking)
		allocatedInfo, err = networking.AllocateContainerAddresses(
			ctx, hostInstanceID, guest.MachineTag(), preparedInfo)
		if err != nil {
			return errors.Trace(err)
		}
		h.logger.Debugf(ctx, "got allocated info from provider: %+v", allocatedInfo)
	} else {
		h.logger.Debugf(ctx, "using dhcp allocated addresses")
	}

	allocatedConfig := params.NetworkConfigFromInterfaceInfo(allocatedInfo)
	h.logger.Debugf(ctx, "allocated network config: %+v", allocatedConfig)
	h.result.Results[idx].Config = allocatedConfig
	return nil
}

func (api *ProvisionerAPI) prepareOrGetContainerInterfaceInfo(
	ctx context.Context,
	args params.Entities, maintain bool,
) (params.MachineNetworkConfigResults, error) {
	c := &prepareOrGetHandler{
		result: params.MachineNetworkConfigResults{
			Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
		},
		maintain: maintain,
		logger:   api.logger,
	}

	if err := api.processEachContainer(ctx, args, c); err != nil {
		return c.result, errors.Trace(err)
	}
	return c.result, nil
}

// prepareContainerAccessEnvironment retrieves the environment, host machine, and access
// for working with containers.
func (api *ProvisionerAPI) prepareContainerAccessEnvironment(ctx context.Context) (environs.Environ, *state.Machine, common.AuthFunc, error) {
	env, err := environs.GetEnviron(ctx, api.configGetter, environs.NoopCredentialInvalidator(), environs.New)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "cannot authenticate request")
	}
	hostAuthTag := api.authorizer.GetAuthTag()
	if hostAuthTag == nil {
		return nil, nil, nil, errors.Errorf("authenticated entity tag is nil")
	}
	hostTag, err := names.ParseMachineTag(hostAuthTag.String())
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	host, err := api.getMachine(canAccess, hostTag)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	return env, host, canAccess, nil
}

type hostChangesHandler struct {
	result params.HostNetworkChangeResults
}

// ProcessOneContainer implements perContainerHandler.ProcessOneContainer
func (h *hostChangesHandler) ProcessOneContainer(
	_ context.Context,
	_ environs.Environ, policy BridgePolicy,
	idx int, host Machine, hostIsManual bool,
	guest Machine, _, _ instance.Id, allSubnets network.SubnetInfos,
) error {
	bridges, err := policy.FindMissingBridgesForContainer(host, guest, allSubnets)
	if err != nil {
		return err
	}

	for _, bridgeInfo := range bridges {
		h.result.Results[idx].NewBridges = append(
			h.result.Results[idx].NewBridges,
			params.DeviceBridgeInfo{
				HostDeviceName: bridgeInfo.DeviceName,
				BridgeName:     bridgeInfo.BridgeName,
				MACAddress:     bridgeInfo.MACAddress,
			})
	}
	return nil
}

// Implements perContainerHandler.SetError
func (h *hostChangesHandler) SetError(idx int, err error) {
	h.result.Results[idx].Error = apiservererrors.ServerError(err)
}

// Implements perContainerHandler.ConfigType
func (h *hostChangesHandler) ConfigType() string {
	return "network"
}

// HostChangesForContainers returns the set of changes that need to be done
// to the host machine to prepare it for the containers to be created.
// Pass in a list of the containers that you want the changes for.
func (api *ProvisionerAPI) HostChangesForContainers(ctx context.Context, args params.Entities) (params.HostNetworkChangeResults, error) {
	c := &hostChangesHandler{
		result: params.HostNetworkChangeResults{
			Results: make([]params.HostNetworkChange, len(args.Entities)),
		},
	}
	if err := api.processEachContainer(ctx, args, c); err != nil {
		return c.result, errors.Trace(err)
	}
	return c.result, nil
}

type containerProfileHandler struct {
	applicationService ApplicationService
	result             params.ContainerProfileResults
	modelName          string
	logger             logger.Logger
}

// ProcessOneContainer implements perContainerHandler.ProcessOneContainer
func (h *containerProfileHandler) ProcessOneContainer(
	ctx context.Context,
	_ environs.Environ, _ BridgePolicy, idx int,
	_ Machine, _ bool,
	guest Machine, _, _ instance.Id, _ network.SubnetInfos,
) error {
	guestName := coremachine.Name(guest.Id())
	unitNames, err := h.applicationService.GetUnitNamesOnMachine(ctx, guestName)
	if errors.Is(err, applicationerrors.MachineNotFound) {
		err = errors.NotFoundf("machine %q", guestName)
		h.SetError(idx, err)
		return errors.Trace(err)
	} else if err != nil {
		h.SetError(idx, err)
		return errors.Trace(err)
	}

	var resPro []*params.ContainerLXDProfile
	for _, unitName := range unitNames {
		appName := unitName.Application()
		locator, err := h.applicationService.GetCharmLocatorByApplicationName(ctx, appName)
		if err != nil {
			h.SetError(idx, err)
			return errors.Trace(err)
		}

		profile, revision, err := h.applicationService.GetCharmLXDProfile(ctx, locator)
		if err != nil {
			h.SetError(idx, err)
			return errors.Trace(err)
		}

		if profile.Empty() {
			h.logger.Tracef(ctx, "no profile to return for %q", unitName)
			continue
		}
		resPro = append(resPro, &params.ContainerLXDProfile{
			Profile: params.CharmLXDProfile{
				Config:      profile.Config,
				Description: profile.Description,
				Devices:     profile.Devices,
			},
			Name: lxdprofile.Name(h.modelName, appName, revision),
		})
	}

	h.result.Results[idx].LXDProfiles = resPro
	return nil
}

// Implements perContainerHandler.SetError
func (h *containerProfileHandler) SetError(idx int, err error) {
	h.result.Results[idx].Error = apiservererrors.ServerError(err)
}

// Implements perContainerHandler.ConfigType
func (h *containerProfileHandler) ConfigType() string {
	return "LXD profile"
}

// GetContainerProfileInfo returns information to configure a lxd profile(s) for a
// container based on the charms deployed to the container. It accepts container
// tags as arguments. Unlike machineLXDProfileNames which has the environ
// write the lxd profiles and returns the names of profiles already written.
func (api *ProvisionerAPI) GetContainerProfileInfo(ctx context.Context, args params.Entities) (params.ContainerProfileResults, error) {
	c := &containerProfileHandler{
		applicationService: api.applicationService,
		result: params.ContainerProfileResults{
			Results: make([]params.ContainerProfileResult, len(args.Entities)),
		},
		logger: api.logger,
	}

	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return c.result, errors.Trace(err)
	}
	c.modelName = modelInfo.Name

	if err := api.processEachContainer(ctx, args, c); err != nil {
		return c.result, errors.Trace(err)
	}
	return c.result, nil
}

// Status returns the status of the specified machine.
func (api *ProvisionerAPI) Status(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	results := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return results, err
	}
	for i, entity := range args.Entities {
		mTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(mTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := coremachine.Name(mTag.Id())

		statusInfo, err := api.statusService.GetMachineStatus(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i] = params.StatusResult{
			Status: statusInfo.Status.String(),
			Info:   statusInfo.Message,
			Data:   statusInfo.Data,
			Since:  statusInfo.Since,
		}
	}
	return results, nil
}

// SetStatus sets the status of the specified machine.
func (api *ProvisionerAPI) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canModify, err := api.getAuthFunc(ctx)
	if err != nil {
		return results, err
	}
	now := api.clock.Now()
	for i, entity := range args.Entities {
		machineTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canModify(machineTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := coremachine.Name(machineTag.Id())

		err = api.statusService.SetMachineStatus(ctx, machineName, status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
			Data:    entity.Data,
			Since:   &now,
		})
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// InstanceStatus returns the instance status for each given entity.
// Only machine tags are accepted.
func (api *ProvisionerAPI) InstanceStatus(ctx context.Context, args params.Entities) (params.StatusResults, error) {
	result := params.StatusResults{
		Results: make([]params.StatusResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		api.logger.Errorf(ctx, "failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		mTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(mTag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineName := coremachine.Name(mTag.Id())

		statusInfo, err := api.statusService.GetInstanceStatus(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i] = params.StatusResult{
			Status: statusInfo.Status.String(),
			Info:   statusInfo.Message,
			Data:   statusInfo.Data,
			Since:  statusInfo.Since,
		}
	}
	return result, nil
}

func (api *ProvisionerAPI) setOneInstanceStatus(ctx context.Context, canAccess common.AuthFunc, arg params.EntityStatusArgs) error {
	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return apiservererrors.ErrPerm
	}
	if !canAccess(mTag) {
		return apiservererrors.ErrPerm
	}

	machineName := coremachine.Name(mTag.Id())
	since := api.clock.Now()
	s := status.StatusInfo{
		Status:  status.Status(arg.Status),
		Message: arg.Info,
		Data:    arg.Data,
		Since:   &since,
	}

	err = api.statusService.SetInstanceStatus(ctx, machineName, s)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.NotFoundf("machine %q", machineName)
	} else if err != nil {
		return errors.Trace(err)
	}

	if arg.Status == status.ProvisioningError.String() || arg.Status == status.Error.String() {
		s.Status = status.Error
		err := api.statusService.SetMachineStatus(ctx, machineName, s)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", machineName)
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetInstanceStatus updates the instance status for each given
// entity. Only machine tags are accepted.
func (api *ProvisionerAPI) SetInstanceStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		err = api.setOneInstanceStatus(ctx, canAccess, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// SetModificationStatus updates the instance whilst changes are occurring. This
// is different from SetStatus and SetInstanceStatus, by the fact this holds
// information about the ongoing changes that are happening to instances.
// Consider LXD Profile updates that can modify a instance, but may not cause
// the instance to be placed into a error state. This modification status
// serves the purpose of highlighting that to the operator.
// Only machine tags are accepted.
//
// Deprecated: this facade was used for LXD profiles, which have been removed.
// Drop this facade on the next facade version bump.
func (api *ProvisionerAPI) SetModificationStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}, nil
}

// MarkMachinesForRemoval indicates that the specified machines are
// ready to have any provider-level resources cleaned up and then be
// removed.
func (api *ProvisionerAPI) MarkMachinesForRemoval(ctx context.Context, machines params.Entities) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(machines.Entities))
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		api.logger.Errorf(ctx, "failed to get an authorisation function: %v", err)
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, machine := range machines.Entities {
		results[i].Error = apiservererrors.ServerError(api.markOneMachineForRemoval(machine.Tag, canAccess))
	}
	return params.ErrorResults{Results: results}, nil
}

func (api *ProvisionerAPI) markOneMachineForRemoval(machineTag string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := api.getMachine(canAccess, mTag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.MarkForRemoval()
}

func (api *ProvisionerAPI) SetHostMachineNetworkConfig(ctx context.Context, args params.SetMachineNetworkConfig) error {
	return api.SetObservedNetworkConfig(ctx, args)
}

// CACert returns the certificate used to validate the state connection.
func (api *ProvisionerAPI) CACert(ctx context.Context) (params.BytesResult, error) {
	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.BytesResult{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()
	return params.BytesResult{Result: []byte(caCert)}, nil
}

// SetCharmProfiles records the given slice of charm profile names.
func (api *ProvisionerAPI) SetCharmProfiles(ctx context.Context, args params.SetProfileArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		api.logger.Errorf(ctx, "failed to get an authorisation function: %v", err)
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		err := api.setOneMachineCharmProfiles(ctx, a.Entity.Tag, a.Profiles, canAccess)
		if errors.Is(err, machineerrors.NotProvisioned) {
			results[i].Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %q", a.Entity.Tag))
		} else {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return params.ErrorResults{Results: results}, nil
}

func (api *ProvisionerAPI) setOneMachineCharmProfiles(ctx context.Context, machineTag string, profiles []string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess(mTag) {
		return apiservererrors.ErrPerm
	}
	machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(mTag.Id()))
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(api.machineService.SetAppliedLXDProfileNames(ctx, machineUUID, profiles))
}

// ModelUUID returns the model UUID that the current connection is for.
func (api *ProvisionerAPI) ModelUUID(ctx context.Context) params.StringResult {
	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.StringResult{Error: apiservererrors.ServerError(err)}
	}
	return params.StringResult{Result: string(modelInfo.UUID)}
}

// Remove removes every given machine from state.
// It will fail if the machine is not present.
func (api *ProvisionerAPI) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := api.getAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machine, err := api.getMachine(canModify, tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Only remove machines that are not Alive.
		if life := machine.Life(); life == state.Alive {
			err := fmt.Errorf("cannot remove machine %q: still alive", tag.String())
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := machine.Remove(); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}
