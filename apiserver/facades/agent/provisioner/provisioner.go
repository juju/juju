// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	commonnetwork "github.com/juju/juju/apiserver/common/network"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	corecontainer "github.com/juju/juju/core/container"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/ssh"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// ProvisionerAPI provides access to the Provisioner API facade.
type ProvisionerAPI struct {
	*common.ControllerConfigAPI
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser
	*commonmodel.ModelConfigWatcher
	*commonmodel.ModelMachinesWatcher
	*common.InstanceIdGetter
	*common.ToolsGetter

	networkService            NetworkService
	controllerConfigService   ControllerConfigService
	cloudImageMetadataService CloudImageMetadataService
	agentProvisionerService   AgentProvisionerService
	keyUpdaterService         KeyUpdaterService
	modelConfigService        ModelConfigService
	modelInfoService          ModelInfoService
	machineService            MachineService
	statusService             StatusService
	applicationService        ApplicationService
	removalService            RemovalService
	authorizer                facade.Authorizer
	storageProviderRegistry   storage.ProviderRegistry
	storagePoolGetter         StoragePoolGetter
	configGetter              environs.EnvironConfigGetter
	getAuthFunc               common.GetAuthFunc
	getCanModify              common.GetAuthFunc
	toolsFinder               common.ToolsFinder
	watcherRegistry           facade.WatcherRegistry
	logger                    logger.Logger
	clock                     clock.Clock

	// Hold on to the controller UUID, as we'll reuse it for a lot of
	// calls.
	controllerUUID string
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
	domainServices := ctx.DomainServices()

	agentBinaryService := domainServices.AgentBinary()
	agentPasswordService := domainServices.AgentPassword()
	agentProvisionerService := domainServices.AgentProvisioner()
	agentService := domainServices.Agent()
	applicationService := domainServices.Application()
	cloudImageMetadataService := domainServices.CloudImageMetadata()
	cloudService := domainServices.Cloud()
	controllerNodeService := domainServices.ControllerNode()
	controllerConfigService := domainServices.ControllerConfig()
	credentialService := domainServices.Credential()
	externalControllerService := domainServices.ExternalController()
	keyUpdaterService := domainServices.KeyUpdater()
	machineService := domainServices.Machine()
	modelConfigService := domainServices.Config()
	modelInfoService := domainServices.ModelInfo()
	modelService := domainServices.Model()
	networkService := domainServices.Network()
	removalService := domainServices.Removal()
	statusService := domainServices.Status()
	storageService := domainServices.Storage()

	storageRegisty, err := storageService.GetStorageRegistry(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	configGetter := environConfigGetter{
		modelInfoService:   modelInfoService,
		cloudService:       cloudService,
		credentialService:  credentialService,
		modelConfigService: modelConfigService,
	}

	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	isCaasModel := modelInfo.Type == coremodel.CAAS

	controllerNodeServices := domainServices.ControllerNode()
	urlGetter := common.NewToolsURLGetter(string(modelInfo.UUID), controllerNodeServices)

	watcherRegistry := ctx.WatcherRegistry()
	modelConfigWatcher := commonmodel.NewModelConfigWatcher(modelConfigService, watcherRegistry)

	api := &ProvisionerAPI{
		PasswordChanger:      common.NewPasswordChanger(agentPasswordService, getAuthFunc),
		LifeGetter:           common.NewLifeGetter(applicationService, machineService, getAuthFunc, ctx.Logger()),
		APIAddresser:         common.NewAPIAddresser(controllerNodeService, watcherRegistry),
		ModelConfigWatcher:   modelConfigWatcher,
		ModelMachinesWatcher: commonmodel.NewModelMachinesWatcher(machineService, watcherRegistry, authorizer),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			controllerConfigService,
			controllerNodeServices,
			externalControllerService,
			modelService,
		),
		networkService:            networkService,
		controllerConfigService:   controllerConfigService,
		agentProvisionerService:   agentProvisionerService,
		cloudImageMetadataService: cloudImageMetadataService,
		keyUpdaterService:         keyUpdaterService,
		modelConfigService:        modelConfigService,
		modelInfoService:          modelInfoService,
		machineService:            machineService,
		statusService:             statusService,
		applicationService:        applicationService,
		removalService:            removalService,
		authorizer:                authorizer,
		configGetter:              configGetter,
		storageProviderRegistry:   storageRegisty,
		storagePoolGetter:         storageService,
		getAuthFunc:               getAuthFunc,
		getCanModify:              getCanModify,
		controllerUUID:            ctx.ControllerUUID(),
		watcherRegistry:           watcherRegistry,
		logger:                    ctx.Logger().Child("provisioner"),
		clock:                     ctx.Clock(),
	}
	if isCaasModel {
		return api, nil
	}

	api.InstanceIdGetter = common.NewInstanceIdGetter(machineService, getAuthFunc)

	api.toolsFinder = common.NewToolsFinder(
		urlGetter,
		ctx.ControllerObjectStore(),
		agentBinaryService,
	)
	api.ToolsGetter = common.NewToolsGetter(agentService, urlGetter, api.toolsFinder, getAuthOwner)
	return api, nil
}

// ProvisionerAPIV11 provides v10 of the provisioner facade.
// It relies on agent-set origin when calling SetHostMachineNetworkConfig.
type ProvisionerAPIV11 struct {
	*ProvisionerAPI
}

// getInstanceID returns the instance ID for the given machine.
func (api *ProvisionerAPI) getInstanceID(ctx context.Context, machineName coremachine.Name) (instance.Id, error) {
	machineUUID, err := api.machineService.GetMachineUUID(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", apiservererrors.ServerError(errors.NotFoundf("machine %q", machineName))
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

	// If we're watching all the machine containers, ensure that the container
	// type is supported. It should be noted, that as of today, we only support
	// LXD.
	if arg.ContainerType != "" {
		if _, err := instance.ParseContainerType(arg.ContainerType); err != nil {
			return nothing, apiservererrors.ServerError(
				errors.NotSupportedf("container type %q is not supported", arg.ContainerType),
			)
		}
	}

	watcher, err := api.machineService.WatchMachineContainerLife(ctx, coremachine.Name(tag.Id()))
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nothing, apiservererrors.ServerError(errors.NotFoundf("machine %q", tag.Id()))
	} else if err != nil {
		return nothing, apiservererrors.ServerError(err)
	}

	watcherID, changes, err := internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watcher)
	if err != nil {
		return nothing, apiservererrors.ServerError(errors.Annotatef(err, "registering watcher for machine %q", tag.Id()))
	}
	return params.StringsWatchResult{
		StringsWatcherId: watcherID,
		Changes:          changes,
	}, nil
}

func (api *ProvisionerAPI) EnsureDead(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return results, nil
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.removalService.MarkMachineAsDead(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasContainers) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeMachineHasContainers, "machine %q hosts containers", tag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasUnits) {
			results.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeHasAssignedUnits, "machine %q hosts units", tag.Id())
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
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
	// TODO(jack-w-shaw): Push this into the service layer.
	machineNames, err := api.machineService.AllMachineNames(ctx)
	if err != nil {
		return results, errors.Annotate(err, "getting all machine names")
	}
	for _, machineName := range machineNames {
		machineTag := names.NewMachineTag(machineName.String())
		if !canAccessFunc(machineTag) {
			continue
		}
		_, err = api.getInstanceID(ctx, machineName)
		if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
			return results, err
		}
		if err == nil {
			// Machine may have been provisioned but machiner hasn't set the
			// status to Started yet.
			continue
		}
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
		machineLife, err := api.machineService.GetMachineLife(ctx, machineName)
		if err != nil {
			return results, err
		}
		result.Id = machineName.String()
		result.Life = machineLife
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

		result.Results[i].Result, err = api.commonServiceInstances(ctx, machineName)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
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
		machineNames, err := api.applicationService.GetMachinesForApplication(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return nil, errors.NotFoundf("application %q", appName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		for _, machineName := range machineNames {
			instanceId, err := api.getInstanceID(ctx, machineName)
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
		result.Results[i].Result, err = api.commonApplicationMachineId(ctx, machineName)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// commonApplicationMachineId returns a slice of machine.Ids with
// applications in common with the specified machine.
func (api *ProvisionerAPI) commonApplicationMachineId(ctx context.Context, mName coremachine.Name) ([]string, error) {
	applications, err := api.machineService.GetMachinePrincipalApplications(ctx, mName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.NotFoundf("machine %q", mName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	union := set.NewStrings()
	for _, app := range applications {
		machines, err := api.applicationService.GetMachinesForApplication(ctx, app)
		if err != nil {
			return nil, err
		}
		for _, machine := range machines {
			union.Add(machine.String())
		}
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
		if err != nil || !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		cons, err := api.machineService.GetMachineConstraints(ctx, coremachine.Name(tag.Id()))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Constraints = cons
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
		if err != nil || !canAccess(tag) {
			return apiservererrors.ErrPerm
		}

		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(tag.Id()))
		if err != nil {
			return errors.Annotatef(err, "retrieving machineUUID for machine %q", tag.Id())
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
	var err error
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watch)
	return result, err
}

// ReleaseContainerAddresses finds addresses allocated to a container and marks
// them as Dead, to be released and removed. It accepts container tags as
// arguments.
func (api *ProvisionerAPI) ReleaseContainerAddresses(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	return result, nil
}

// PrepareContainerInterfaceInfo allocates an address and returns information to
// configure networking for a container. It accepts container tags as arguments.
func (api *ProvisionerAPI) PrepareContainerInterfaceInfo(
	ctx context.Context, args params.Entities,
) (params.MachineNetworkConfigResults, error) {
	result := params.MachineNetworkConfigResults{}

	hostUUID, hostName, err := api.getAuthedCallerMachine(ctx)
	if err != nil {
		return result, err
	}

	hostInstanceID, err := api.machineService.GetInstanceID(ctx, hostUUID)
	if errors.Is(err, machineerrors.NotProvisioned) {
		return result, apiservererrors.ServerError(errors.NotProvisionedf("host machine %q", hostName))
	}

	results := make([]params.MachineNetworkConfigResult, len(args.Entities))
	for i, entity := range args.Entities {
		gTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		guestUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(gTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = apiservererrors.ServerError(errors.NotFoundf("machine %q", hostName))
			continue
		}

		devsForGuest, err := api.networkService.DevicesForGuest(ctx, hostUUID, guestUUID)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// TODO (manadart 2025-07-23): I so, so do not want to use
		//  InterfaceInfos anymore, but changing it would flow deep into the
		//  MAAS provider, which is not going to be undertaken under the Dqlite
		//  rewrite. Ideally we would use NetInterface from the network domain
		//  everywhere. Anyone who reads this and wants to be a hero...
		//  If we *did* change the provider to use NetInterface, the correct
		//  thing to do would be to push the logic for
		//  AllocateContainerAddresses into DevicesForGuest - choreography for
		//  these concerns is business logic and should not be in the facade.
		preparedInfo := toInterfaceInfos(devsForGuest)

		allocatedInfo, err := api.networkService.AllocateContainerAddresses(
			ctx, hostInstanceID, gTag.Id(), preparedInfo)
		if errors.Is(err, networkerrors.ContainerAddressesNotSupported) {
			api.logger.Debugf(ctx, "using DHCP allocated addresses")
			allocatedInfo = preparedInfo
		} else if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		} else {
			api.logger.Debugf(ctx, "got allocated info from provider: %+v", allocatedInfo)
		}

		allocatedConfig := params.NetworkConfigFromInterfaceInfo(allocatedInfo)
		api.logger.Debugf(ctx, "allocated network config: %+v", allocatedConfig)
		results[i].Config = allocatedConfig
	}

	result.Results = results
	return result, nil
}

// perContainerHandler is the interface we need to trigger processing on
// every container passed in as a list of things to process.
type perContainerHandler interface {
	// ProcessOneContainer is called once we've assured ourselves that all of
	// the access permissions are correct and the container is ready to be
	// processed.
	// idx is the index for this, hostInstanceID is the machine that is hosting
	// the container.
	// Any errors that are returned from ProcessOneContainer will be turned
	// into ServerError and handed to SetError
	ProcessOneContainer(
		ctx context.Context,
		idx int,
		guestMachineName coremachine.Name,
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
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot authenticate request")
	}
	hostAuthTag := api.authorizer.GetAuthTag()
	if hostAuthTag == nil {
		return errors.Errorf("authenticated entity tag is nil")
	}

	for i, entity := range args.Entities {
		guestTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			handler.SetError(i, err)
			continue
		}
		if !canAccess(guestTag) {
			handler.SetError(i, apiservererrors.ErrPerm)
			continue
		}

		guestMachineName := coremachine.Name(guestTag.Id())
		if !guestMachineName.IsContainer() {
			err = errors.Errorf("cannot prepare %s config for %q: not a container", handler.ConfigType(), guestTag)
			handler.SetError(i, err)
			continue
		}
		if err := handler.ProcessOneContainer(
			ctx,
			i,
			coremachine.Name(guestTag.Id()),
		); err != nil {
			handler.SetError(i, err)
			continue
		}
	}
	return nil
}

func toInterfaceInfos(netInterfaces []domainnetwork.NetInterface) network.InterfaceInfos {
	res := make(network.InterfaceInfos, len(netInterfaces))
	for i, netInterface := range netInterfaces {
		var mtu int
		if netInterface.MTU != nil {
			mtu = int(*netInterface.MTU)
		}
		var mac string
		if netInterface.MACAddress != nil {
			mac = *netInterface.MACAddress
		}

		var (
			addrs         network.ProviderAddresses
			nicConfigType network.AddressConfigType
		)

		// There is a single address populated for each interface.
		// The *device* config type is populated from the address.
		// Note that we populate the *CIDR* from the address value.
		if len(netInterface.Addrs) > 0 {
			a := netInterface.Addrs[0]
			addrs = network.ProviderAddresses{{MachineAddress: network.MachineAddress{
				ConfigType: a.ConfigType,
				CIDR:       a.AddressValue,
			}}}
			nicConfigType = a.ConfigType
		}

		res[i] = network.InterfaceInfo{
			MACAddress:          mac,
			ConfigType:          nicConfigType,
			VLANTag:             int(netInterface.VLANTag),
			InterfaceName:       netInterface.Name,
			ParentInterfaceName: netInterface.ParentDeviceName,
			InterfaceType:       netInterface.Type,
			Disabled:            !netInterface.IsEnabled,
			NoAutoStart:         !netInterface.IsAutoStart,
			Addresses:           addrs,
			DNSServers:          netInterface.DNSAddresses,
			MTU:                 mtu,
		}
	}
	return res
}

// HostChangesForContainers returns the set of changes that need to be done
// to the host machine to prepare it for the containers to be created.
// Pass in a list of the containers that you want the changes for.
func (api *ProvisionerAPI) HostChangesForContainers(
	ctx context.Context, args params.Entities,
) (params.HostNetworkChangeResults, error) {
	result := params.HostNetworkChangeResults{}

	hostUUID, _, err := api.getAuthedCallerMachine(ctx)
	if err != nil {
		return result, err
	}

	results := make([]params.HostNetworkChange, len(args.Entities))
	for i, entity := range args.Entities {
		mTag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineName := coremachine.Name(mTag.Id())

		guestUUID, err := api.machineService.GetMachineUUID(ctx, machineName)
		if err != nil {
			if errors.Is(err, machineerrors.MachineNotFound) {
				results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", machineName)
			} else {
				results[i].Error = apiservererrors.ServerError(err)
			}
			continue
		}

		changes, err := api.networkService.DevicesToBridge(ctx, hostUUID, guestUUID)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results[i] = params.HostNetworkChange{
			NewBridges: transform.Slice(changes, func(in domainnetwork.DeviceToBridge) params.DeviceBridgeInfo {
				return params.DeviceBridgeInfo{
					HostDeviceName: in.DeviceName,
					BridgeName:     in.BridgeName,
					MACAddress:     in.MACAddress,
				}
			}),
		}
	}

	result.Results = results
	return result, nil
}

func (api *ProvisionerAPI) getAuthedCallerMachine(ctx context.Context) (coremachine.UUID, coremachine.Name, error) {
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	hostAuthTag := api.authorizer.GetAuthTag()
	if hostAuthTag == nil {
		return "", "", apiservererrors.ErrPerm
	}

	if !canAccess(hostAuthTag) {
		return "", "", apiservererrors.ErrPerm
	}

	hostTag, err := names.ParseMachineTag(hostAuthTag.String())
	if err != nil {
		return "", "", err
	}
	hostName := coremachine.Name(hostTag.Id())

	hostUUID, err := api.machineService.GetMachineUUID(ctx, hostName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", "", apiservererrors.ErrPerm
	}

	return hostUUID, hostName, err
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
	idx int,
	guestMachineName coremachine.Name,
) error {
	unitNames, err := h.applicationService.GetUnitNamesOnMachine(ctx, guestMachineName)
	if errors.Is(err, applicationerrors.MachineNotFound) {
		err = errors.NotFoundf("machine %q", guestMachineName)
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
		mTag, err := names.ParseMachineTag(machine.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(mTag) {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(mTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", mTag.Id())
			continue
		} else if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.removalService.MarkInstanceAsDead(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", mTag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasContainers) {
			results[i].Error = apiservererrors.ParamsErrorf(params.CodeMachineHasContainers, "machine %q hosts containers", mTag.Id())
			continue
		} else if errors.Is(err, removalerrors.MachineHasUnits) {
			results[i].Error = apiservererrors.ParamsErrorf(params.CodeHasAssignedUnits, "machine %q hosts units", mTag.Id())
			continue
		} else if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return params.ErrorResults{Results: results}, nil
}

func (api *ProvisionerAPI) SetHostMachineNetworkConfig(ctx context.Context, args params.SetMachineNetworkConfig) error {
	canModify, err := api.getCanModify(ctx)
	if err != nil {
		return err
	}

	mTag, err := names.ParseMachineTag(args.Tag)
	if err != nil {
		return apiservererrors.ErrPerm
	}

	if !canModify(mTag) {
		return apiservererrors.ErrPerm
	}

	mUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(mTag.Id()))
	if err != nil {
		return apiservererrors.ServerError(err)
	}

	nics, err := commonnetwork.ParamsNetworkConfigToDomain(ctx, args.Config, network.OriginMachine)
	if err != nil {
		return apiservererrors.ServerError(err)
	}

	if err := api.networkService.SetMachineNetConfig(ctx, mUUID, nics); err != nil {
		return apiservererrors.ServerError(err)
	}
	return nil
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
		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		machineName := coremachine.Name(tag.Id())
		machineUUID, err := api.machineService.GetMachineUUID(ctx, machineName)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.removalService.DeleteMachine(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			result.Results[i].Error = apiservererrors.ParamsErrorf(params.CodeNotFound, "machine %q not found", tag.Id())
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

type environConfigGetter struct {
	modelInfoService   ModelInfoService
	cloudService       CloudService
	credentialService  CredentialService
	modelConfigService ModelConfigService
}

// ControllerUUID returns the universally unique identifier of the controller.
func (g environConfigGetter) ControllerUUID(ctx context.Context) (string, error) {
	modelInfo, err := g.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return modelInfo.ControllerUUID.String(), nil
}

// ModelConfig implements environs.EnvironConfigGetter.
func (g environConfigGetter) ModelConfig(ctx context.Context) (*config.Config, error) {
	return g.modelConfigService.ModelConfig(ctx)
}

// CloudSpec implements environs.EnvironConfigGetter.
func (g environConfigGetter) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return CloudSpecForModel(ctx, g.modelInfoService, g.cloudService, g.credentialService)
}

// CloudSpecForModel returns a CloudSpec for the specified model.
func CloudSpecForModel(
	ctx context.Context,
	modelInfoService ModelInfoService,
	cloudService CloudService,
	credentialService CredentialService,
) (environscloudspec.CloudSpec, error) {
	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	cld, err := cloudService.Cloud(ctx, modelInfo.Cloud)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	regionName := modelInfo.CloudRegion
	credentialKey := credential.Key{
		Cloud: modelInfo.Cloud,
		Owner: coremodel.ControllerModelOwnerUsername,
		Name:  modelInfo.CredentialName,
	}
	cred, err := credentialService.CloudCredential(ctx, credentialKey)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}
	return environscloudspec.MakeCloudSpec(*cld, regionName, &cred)
}
