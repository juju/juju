// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/os/ostype"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment"
	statuserrors "github.com/juju/juju/domain/status/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	internalerrors "github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/resource"
	resourcecharmhub "github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type APIGroup struct {
	*common.PasswordChanger
	*common.AgentEntityWatcher
	*API

	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI
	lifeCanRead     common.GetAuthFunc
}

type NewResourceOpenerFunc func(ctx context.Context, appName string) (coreresource.Opener, error)

type API struct {
	auth            facade.Authorizer
	resources       facade.Resources
	watcherRegistry facade.WatcherRegistry

	store                   objectstore.ObjectStore
	state                   CAASApplicationProvisionerState
	newResourceOpener       NewResourceOpenerFunc
	storage                 StorageBackend
	storagePoolGetter       StoragePoolGetter
	applicationService      ApplicationService
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
	statusService           StatusService
	removalService          RemovalService
	leadershipRevoker       leadership.Revoker
	clock                   clock.Clock
	logger                  corelogger.Logger
}

// NewStateCAASApplicationProvisionerAPI provides the signature required for facade registration.
func NewStateCAASApplicationProvisionerAPI(stdCtx context.Context, ctx facade.ModelContext) (*APIGroup, error) {
	authorizer := ctx.Auth()

	st := ctx.State()
	domainServices := ctx.DomainServices()

	agentPasswordService := domainServices.AgentPassword()
	controllerConfigService := domainServices.ControllerConfig()
	modelConfigService := domainServices.Config()

	storageService := domainServices.Storage()
	applicationService := domainServices.Application()
	resourceService := domainServices.Resource()

	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelTag := names.NewModelTag(ctx.ModelUUID().String())

	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID := ctx.ModelUUID().String()

	newResourceOpener := func(ctx context.Context, appName string) (coreresource.Opener, error) {
		args := resource.ResourceOpenerArgs{
			ModelUUID:            modelUUID,
			ResourceService:      resourceService,
			ApplicationService:   applicationService,
			CharmhubClientGetter: resourcecharmhub.NewCharmHubOpener(modelConfigService),
		}
		return resource.NewResourceOpenerForApplication(ctx, args, appName)
	}

	services := Services{
		ApplicationService:      applicationService,
		ControllerConfigService: controllerConfigService,
		ControllerNodeService:   domainServices.ControllerNode(),
		ModelConfigService:      modelConfigService,
		ModelInfoService:        domainServices.ModelInfo(),
		StatusService:           domainServices.Status(),
		RemovalService:          domainServices.Removal(),
	}

	leadershipRevoker, err := ctx.LeadershipRevoker()
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}

	api, err := NewCAASApplicationProvisionerAPI(
		stateShim{State: st},
		ctx.Resources(),
		newResourceOpener,
		authorizer,
		sb,
		storageService,
		services,
		leadershipRevoker,
		ctx.ObjectStore(),
		ctx.Clock(),
		ctx.Logger().Child("caasapplicationprovisioner"),
		ctx.WatcherRegistry(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	lifeCanRead := common.AuthAny(
		common.AuthFuncForTagKind(names.ApplicationTagKind),
		common.AuthFuncForTagKind(names.UnitTagKind),
	)

	apiGroup := &APIGroup{
		PasswordChanger:    common.NewPasswordChanger(agentPasswordService, st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, ctx.WatcherRegistry(), common.AuthFuncForTagKind(names.ApplicationTagKind)),
		charmInfoAPI:       commonCharmsAPI,
		appCharmInfoAPI:    appCharmInfoAPI,
		lifeCanRead:        lifeCanRead,
		API:                api,
	}

	return apiGroup, nil
}

// CharmInfo returns information about the requested charm.
func (a *APIGroup) CharmInfo(ctx context.Context, args params.CharmURL) (params.Charm, error) {
	return a.charmInfoAPI.CharmInfo(ctx, args)
}

// ApplicationCharmInfo returns information about an application's charm.
func (a *APIGroup) ApplicationCharmInfo(ctx context.Context, args params.Entity) (params.Charm, error) {
	return a.appCharmInfoAPI.ApplicationCharmInfo(ctx, args)
}

// NewCAASApplicationProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASApplicationProvisionerAPI(
	st CAASApplicationProvisionerState,
	resources facade.Resources,
	newResourceOpener NewResourceOpenerFunc,
	authorizer facade.Authorizer,
	sb StorageBackend,
	storagePoolGetter StoragePoolGetter,
	services Services,
	leadershipRevoker leadership.Revoker,
	store objectstore.ObjectStore,
	clock clock.Clock,
	logger corelogger.Logger,
	watcherRegistry facade.WatcherRegistry,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		auth:                    authorizer,
		resources:               resources,
		watcherRegistry:         watcherRegistry,
		newResourceOpener:       newResourceOpener,
		state:                   st,
		storage:                 sb,
		store:                   store,
		storagePoolGetter:       storagePoolGetter,
		controllerConfigService: services.ControllerConfigService,
		controllerNodeService:   services.ControllerNodeService,
		modelConfigService:      services.ModelConfigService,
		modelInfoService:        services.ModelInfoService,
		applicationService:      services.ApplicationService,
		statusService:           services.StatusService,
		removalService:          services.RemovalService,
		leadershipRevoker:       leadershipRevoker,
		clock:                   clock,
		logger:                  logger,
	}, nil
}

// Remove removes every given unit from state, calling EnsureDead
// first, then Remove.
func (a *API) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := common.AuthFuncForTagKind(names.UnitTagKind)(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitUUID, err := a.applicationService.GetUnitUUID(ctx, coreunit.Name(tag.Id()))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := a.removalService.MarkUnitAsDead(ctx, unitUUID); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		_, err = a.removalService.RemoveUnit(ctx, unitUUID, false, 0)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", tag.Id()))
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// WatchProvisioningInfo provides a watcher for changes that affect the
// information returned by ProvisioningInfo. This is useful for ensuring the
// latest application stated is ensured.
func (a *API) WatchProvisioningInfo(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	var result params.NotifyWatchResults
	result.Results = make([]params.NotifyWatchResult, len(args.Entities))
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		res, err := a.watchProvisioningInfo(ctx, appName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].NotifyWatcherId = res.NotifyWatcherId
	}
	return result, nil
}

func (a *API) watchProvisioningInfo(ctx context.Context, appName names.ApplicationTag) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}

	appWatcher, err := a.applicationService.WatchApplication(ctx, appName.Id())
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerConfigWatcher, err := a.controllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	controllerConfigNotifyWatcher, err := corewatcher.Normalise(controllerConfigWatcher)
	if err != nil {
		return result, errors.Trace(err)
	}
	controllerAPIHostPortsWatcher, err := a.controllerNodeService.WatchControllerAPIAddresses(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfigWatcher, err := a.modelConfigService.Watch()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfigNotifyWatcher, err := corewatcher.Normalise(modelConfigWatcher)
	if err != nil {
		return result, errors.Trace(err)
	}

	// TODO: Either this needs to be a watcher in the application domain that covers
	// all the required values in ProvisioningInfo, or we need to refactor the worker
	// to watch what it needs. Currently we are missing when the charm is available,
	// which leads to the caasapplicationprovisioner worker not progressing with the
	// provisioning of k8s resources.
	multiWatcher, err := eventsource.NewMultiNotifyWatcher(ctx,
		appWatcher,
		controllerConfigNotifyWatcher,
		controllerAPIHostPortsWatcher,
		modelConfigNotifyWatcher,
	)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, a.watcherRegistry, multiWatcher)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

// ProvisioningInfo returns the info needed to provision a caas application.
func (a *API) ProvisioningInfo(ctx context.Context, args params.Entities) (params.CAASApplicationProvisioningInfoResults, error) {
	var result params.CAASApplicationProvisioningInfoResults
	result.Results = make([]params.CAASApplicationProvisioningInfo, len(args.Entities))
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		info, err := a.provisioningInfo(ctx, appName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = *info
	}
	return result, nil
}

func (a *API) provisioningInfo(ctx context.Context, appTag names.ApplicationTag) (*params.CAASApplicationProvisioningInfo, error) {
	// TODO: Either this needs to be implemented in the application domain or the worker
	// needs to be refactored to fetch each value individually.
	appName := appTag.Id()
	// TODO(storage): We must keep this legacy Application here because of
	// `StorageConstraints()`. Once that's migrated to dqlite this should be
	// removed in favor of a domain service method.
	app, err := a.state.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	locator, err := a.applicationService.GetCharmLocatorByApplicationName(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", appName)
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm for application %s", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	isAvailable, err := a.applicationService.IsCharmAvailable(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm %s", locator)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if !isAvailable {
		// TODO: WatchProvisioningInfo needs to fire when this changes.
		return nil, errors.NotProvisionedf("charm for application %q", appName)
	}

	cfg, err := a.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelConfig, err := a.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelInfo, err := a.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	filesystemParams, err := a.applicationFilesystemParams(ctx, app, appName, locator, cfg, modelConfig, modelInfo.UUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	devices, err := a.devicesParams(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	appID, err := a.applicationService.GetApplicationIDByName(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	cons, err := a.applicationService.GetApplicationConstraints(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %s", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(model): This call should be removed once the model constraints are
	// removed from the state.
	mergedCons, err := a.state.ResolveConstraints(cons)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourceTags := tags.ResourceTags(
		names.NewModelTag(modelInfo.UUID.String()),
		names.NewControllerTag(cfg.ControllerUUID()),
		modelConfig,
	)

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return nil, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in model config %q", modelConfig.Name()),
		)
	}
	imagePath, err := podcfg.GetJujuOCIImagePath(cfg, vers)
	if err != nil {
		return nil, errors.Annotatef(err, "getting juju oci image path")
	}
	addrs, err := a.controllerNodeService.GetAllAPIAddressesForAgentsInPreferredOrder(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting api addresses")
	}
	caCert, _ := cfg.CACert()

	scale, err := a.applicationService.GetApplicationScale(ctx, appName)
	if err != nil {
		return nil, errors.Annotatef(err, "getting application scale")
	}
	origin, err := a.applicationService.GetApplicationCharmOrigin(ctx, appName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	osType, err := encodeOSType(origin.Platform.OSType)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	base := params.Base{
		Name:    osType,
		Channel: origin.Platform.Channel,
	}
	imageRepoDetails, err := docker.NewImageRepoDetails(cfg.CAASImageRepo())
	if err != nil {
		return nil, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	charmURL, err := charmscommon.CharmURLFromLocator(locator.Name, locator)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	charmModifiedVersion, err := a.applicationService.GetCharmModifiedVersion(ctx, appID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	trustSetting, err := a.applicationService.GetApplicationTrustSetting(ctx, appName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return &params.CAASApplicationProvisioningInfo{
		Version:              vers,
		APIAddresses:         addrs,
		CACert:               caCert,
		Tags:                 resourceTags,
		Filesystems:          filesystemParams,
		Devices:              devices,
		Constraints:          mergedCons,
		Base:                 base,
		ImageRepo:            params.NewDockerImageInfo(docker.ConvertToResourceImageDetails(imageRepoDetails), imagePath),
		CharmModifiedVersion: charmModifiedVersion,
		CharmURL:             charmURL,
		Trust:                trustSetting,
		Scale:                scale,
	}, nil
}

// CharmStorageParams returns filesystem parameters needed
// to provision storage used for a charm operator or workload.
func CharmStorageParams(
	ctx context.Context,
	controllerUUID string,
	storageClassName string,
	modelCfg *config.Config,
	modelUUID model.UUID,
	poolName string,
	storagePoolGetter StoragePoolGetter,
) (*params.KubernetesFilesystemParams, error) {
	// The defaults here are for operator storage.
	// Workload storage will override these elsewhere.
	const size uint64 = 1024
	tags := tags.ResourceTags(
		names.NewModelTag(modelUUID.String()),
		names.NewControllerTag(controllerUUID),
		modelCfg,
	)

	result := &params.KubernetesFilesystemParams{
		StorageName: "charm",
		Size:        size,
		Provider:    string(k8sconstants.StorageProviderType),
		Tags:        tags,
		Attributes:  make(map[string]interface{}),
	}

	// The storage key value from the model config might correspond
	// to a storage pool, unless there's been a specific storage pool
	// requested.
	// First, blank out the fallback pool name used in previous
	// versions of Juju.
	if poolName == string(k8sconstants.StorageProviderType) {
		poolName = ""
	}
	maybePoolName := poolName
	if maybePoolName == "" {
		maybePoolName = storageClassName
	}

	registry, err := storagePoolGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	providerType, attrs, err := poolStorageProvider(ctx, storagePoolGetter, registry, maybePoolName)
	if err != nil && (!errors.Is(err, storageerrors.PoolNotFoundError) || poolName != "") {
		return nil, errors.Trace(err)
	}
	if err == nil {
		result.Provider = string(providerType)
		if len(attrs) > 0 {
			result.Attributes = attrs
		}
	}
	if _, ok := result.Attributes[k8sconstants.StorageClass]; !ok && result.Provider == string(k8sconstants.StorageProviderType) {
		result.Attributes[k8sconstants.StorageClass] = storageClassName
	}
	return result, nil
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
	GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error)
}

func poolStorageProvider(ctx context.Context, storagePoolGetter StoragePoolGetter, registry storage.ProviderRegistry, poolName string) (storage.ProviderType, map[string]interface{}, error) {
	pool, err := storagePoolGetter.GetStoragePoolByName(ctx, poolName)
	if errors.Is(err, storageerrors.PoolNotFoundError) {
		// If there's no pool called poolName, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolName)
		provider, err1 := registry.StorageProvider(providerType)
		if err1 != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return "", nil, errors.Trace(err)
		}
		if !provider.Supports(storage.StorageKindFilesystem) {
			return "", nil, errors.NotValidf("storage provider %q", providerType)
		}
		return providerType, nil, nil
	} else if err != nil {
		return "", nil, errors.Trace(err)
	}
	var attr map[string]any
	if len(pool.Attrs) > 0 {
		attr = make(map[string]any, len(pool.Attrs))
		for k, v := range pool.Attrs {
			attr[k] = v
		}
	}
	return storage.ProviderType(pool.Provider), attr, nil
}

func (a *API) applicationFilesystemParams(
	ctx context.Context,
	app Application,
	appName string,
	locator applicationcharm.CharmLocator,
	controllerConfig controller.Config,
	modelConfig *config.Config,
	modelUUID model.UUID,
) ([]params.KubernetesFilesystemParams, error) {
	storageConstraints, err := app.StorageConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmStorage, err := a.applicationService.GetCharmMetadataStorage(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return nil, errors.NotFoundf("charm %s", locator)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	var allFilesystemParams []params.KubernetesFilesystemParams
	// To always guarantee the same order, sort by names.
	var sNames []string
	for name := range storageConstraints {
		sNames = append(sNames, name)
	}
	sort.Strings(sNames)
	for _, name := range sNames {
		cons := storageConstraints[name]
		fsParams, err := filesystemParams(
			ctx,
			appName, cons, name,
			controllerConfig.ControllerUUID(),
			modelConfig,
			modelUUID,
			a.storagePoolGetter,
		)
		if err != nil {
			return nil, errors.Annotatef(err, "getting filesystem %q parameters", name)
		}
		for i := 0; i < int(cons.Count); i++ {
			storage := charmStorage[name]
			id := fmt.Sprintf("%s/%v", name, i)
			tag := names.NewStorageTag(id)
			location, err := state.FilesystemMountPoint(storage, tag, "ubuntu")
			if err != nil {
				return nil, errors.Trace(err)
			}
			filesystemAttachmentParams := params.KubernetesFilesystemAttachmentParams{
				Provider:   fsParams.Provider,
				MountPoint: location,
				ReadOnly:   storage.ReadOnly,
			}
			fsParams.Attachment = &filesystemAttachmentParams
			allFilesystemParams = append(allFilesystemParams, *fsParams)
		}
	}
	return allFilesystemParams, nil
}

func filesystemParams(
	ctx context.Context,
	appName string,
	cons state.StorageConstraints,
	storageName string,
	controllerUUID string,
	modelConfig *config.Config,
	modelUUID model.UUID,
	storagePoolGetter StoragePoolGetter,
) (*params.KubernetesFilesystemParams, error) {
	filesystemTags, err := storagecommon.StorageTags(nil, modelUUID.String(), controllerUUID, modelConfig)
	if err != nil {
		return nil, errors.Annotate(err, "computing storage tags")
	}
	filesystemTags[tags.JujuStorageOwner] = appName

	storageClassName, _ := modelConfig.AllAttrs()[k8sconstants.WorkloadStorageKey].(string)
	if cons.Pool == "" && storageClassName == "" {
		return nil, errors.Errorf("storage pool for %q must be specified since there's no model default storage class", storageName)
	}
	fsParams, err := CharmStorageParams(ctx, controllerUUID, storageClassName, modelConfig, modelUUID, cons.Pool, storagePoolGetter)
	if err != nil {
		return nil, errors.Maskf(err, "getting filesystem storage parameters")
	}

	fsParams.Size = cons.Size
	fsParams.StorageName = storageName
	fsParams.Tags = filesystemTags
	return fsParams, nil
}

func (a *API) devicesParams(ctx context.Context, appName string) ([]params.KubernetesDeviceParams, error) {
	devices, err := a.applicationService.GetDeviceConstraints(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	a.logger.Debugf(ctx, "getting device constraints from state: %#v", devices)
	var devicesParams []params.KubernetesDeviceParams
	for _, d := range devices {
		devicesParams = append(devicesParams, params.KubernetesDeviceParams{
			Type:       params.DeviceType(d.Type),
			Count:      int64(d.Count),
			Attributes: d.Attributes,
		})
	}
	return devicesParams, nil
}

// ApplicationOCIResources returns the OCI image resources for an application.
func (a *API) ApplicationOCIResources(ctx context.Context, args params.Entities) (params.CAASApplicationOCIResourceResults, error) {
	res := params.CAASApplicationOCIResourceResults{
		Results: make([]params.CAASApplicationOCIResourceResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		appTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		appName := appTag.Id()
		locator, err := a.applicationService.GetCharmLocatorByApplicationName(ctx, appName)
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			err = errors.NotFoundf("application %s", appName)
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		} else if errors.Is(err, applicationerrors.CharmNotFound) {
			err = errors.NotFoundf("charm for application %s", appName)
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		} else if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		charmResources, err := a.applicationService.GetCharmMetadataResources(ctx, locator)
		if errors.Is(err, applicationerrors.CharmNotFound) {
			err = errors.NotFoundf("charm %s", locator)
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		} else if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		resourceClient, err := a.newResourceOpener(ctx, appName)
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		imageResources := params.CAASApplicationOCIResources{
			Images: make(map[string]params.DockerImageInfo),
		}
		for _, v := range charmResources {
			if v.Type != charmresource.TypeContainerImage {
				continue
			}
			reader, err := resourceClient.OpenResource(ctx, v.Name)
			if err != nil {
				res.Results[i].Error = apiservererrors.ServerError(err)
				break
			}
			rsc, err := readDockerImageResource(reader)
			_ = reader.Close()
			if err != nil {
				res.Results[i].Error = apiservererrors.ServerError(err)
				break
			}
			imageResources.Images[v.Name] = rsc
			err = resourceClient.SetResourceUsed(ctx, v.Name)
			if err != nil {
				a.logger.Errorf(ctx, "setting resource %s of application %s as in use: %w", v.Name, appName, err)
				res.Results[i].Error = apiservererrors.ServerError(err)
				break
			}
		}
		if res.Results[i].Error != nil {
			continue
		}
		res.Results[i].Result = &imageResources
	}
	return res, nil
}

func readDockerImageResource(reader io.Reader) (params.DockerImageInfo, error) {
	var details docker.DockerImageDetails
	contents, err := io.ReadAll(reader)
	if err != nil {
		return params.DockerImageInfo{}, errors.Trace(err)
	}
	if err := json.Unmarshal(contents, &details); err != nil {
		if err := yaml.Unmarshal(contents, &details); err != nil {
			return params.DockerImageInfo{}, errors.Annotate(err, "file neither valid json or yaml")
		}
	}
	if err := docker.ValidateDockerRegistryPath(details.RegistryPath); err != nil {
		return params.DockerImageInfo{}, err
	}
	return params.DockerImageInfo{
		RegistryPath: details.RegistryPath,
		Username:     details.Username,
		Password:     details.Password,
	}, nil
}

// UpdateApplicationsUnits updates the Juju data model to reflect the given
// units of the specified application.
func (a *API) UpdateApplicationsUnits(ctx context.Context, args params.UpdateApplicationUnitArgs) (params.UpdateApplicationUnitResults, error) {
	result := params.UpdateApplicationUnitResults{
		Results: make([]params.UpdateApplicationUnitResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		appLife, err := a.applicationService.GetApplicationLifeByName(ctx, appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if appLife != life.Alive {
			// We ignore any updates for dying applications.
			a.logger.Debugf(ctx, "ignoring unit updates for dying application: %v", appTag.Id())
			continue
		}

		appStatus := appUpdate.Status
		if appStatus.Status != "" && appStatus.Status != status.Unknown {
			err = a.statusService.SetApplicationStatus(ctx, appTag.Id(), status.StatusInfo{
				Status:  appStatus.Status,
				Message: appStatus.Info,
				Data:    appStatus.Data,
				Since:   ptr(a.clock.Now()),
			})
			if errors.Is(err, statuserrors.ApplicationNotFound) {
				result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("application %q", appTag.Id()))
				continue
			} else if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
		appUnitInfo, err := a.updateUnitsFromCloud(ctx, appTag.Id(), appUpdate.Units)
		if err != nil {
			// Mask any not found errors as the worker (caller) treats them specially
			// and they are not relevant here.
			result.Results[i].Error = apiservererrors.ServerError(errors.Mask(err))
		}

		// Errors from SetScale will also include unit info.
		if appUnitInfo != nil {
			result.Results[i].Info = &params.UpdateApplicationUnitsInfo{
				Units: appUnitInfo,
			}
		}
	}
	return result, nil
}

type filesystemInfo struct {
	unitTag      names.UnitTag
	providerId   string
	mountPoint   string
	readOnly     bool
	size         uint64
	filesystemId string
}

type volumeInfo struct {
	unitTag    names.UnitTag
	providerId string
	readOnly   bool
	persistent bool
	size       uint64
	volumeId   string
}

func (a *API) updateUnitsFromCloud(ctx context.Context, appName string, unitUpdates []params.ApplicationUnitParams) ([]params.ApplicationUnitInfo, error) {
	a.logger.Debugf(ctx, "unit updates: %#v", unitUpdates)

	m, err := a.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var providerIds []string
	for _, u := range unitUpdates {
		providerIds = append(providerIds, u.ProviderId)
	}
	containers, err := m.Containers(providerIds...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := a.applicationService.GetUnitNamesForApplication(ctx, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitByTag := make(map[string]coreunit.Name)
	for _, v := range units {
		unitByTag[v.String()] = v
	}
	unitByProviderID := make(map[string]coreunit.Name)
	for _, v := range containers {
		tag := names.NewUnitTag(v.Unit())
		unit, ok := unitByTag[tag.Id()]
		if !ok {
			return nil, errors.NotFoundf("unit %q with provider id %q", tag, v.ProviderId())
		}
		unitByProviderID[v.ProviderId()] = unit
	}

	filesystemUpdates := make(map[string]filesystemInfo)
	filesystemStatus := make(map[string]status.StatusInfo)
	volumeUpdates := make(map[string]volumeInfo)
	volumeStatus := make(map[string]status.StatusInfo)

	processFilesystemParams := func(processedFilesystemIds set.Strings, unitTag names.UnitTag, unitParams params.ApplicationUnitParams) error {
		// Once a unit is available in the cluster, we consider
		// its filesystem(s) to be attached since the unit is
		// not considered ready until this happens.
		filesystemInfoByName := make(map[string][]params.KubernetesFilesystemInfo)
		for _, fsInfo := range unitParams.FilesystemInfo {
			infos := filesystemInfoByName[fsInfo.StorageName]
			infos = append(infos, fsInfo)
			filesystemInfoByName[fsInfo.StorageName] = infos
		}

		for storageName, infos := range filesystemInfoByName {
			a.logger.Debugf(ctx, "updating storage %v for %v", storageName, unitTag)
			if len(infos) == 0 {
				continue
			}

			unitStorage, err := a.storage.UnitStorageAttachments(unitTag)
			if err != nil {
				return errors.Trace(err)
			}

			// Loop over all the storage for the unit and skip storage not
			// relevant for storageName.
			// TODO(caas) - Add storage bankend API to get all unit storage instances for a named storage.
			for _, sa := range unitStorage {
				si, err := a.storage.StorageInstance(sa.StorageInstance())
				if errors.Is(err, errors.NotFound) {
					//a.logger.Warningf(ctx, "ignoring non-existent storage instance %v for unit %v", sa.StorageInstance(), unitTag.Id())
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if si.StorageName() != storageName {
					continue
				}
				fs, err := a.storage.StorageInstanceFilesystem(sa.StorageInstance())
				if err != nil {
					return errors.Trace(err)
				}
				fsInfo := infos[0]
				processedFilesystemIds.Add(fsInfo.FilesystemId)

				// k8s reports provisioned info even when the volume is not ready.
				// Only update state when volume is created so Juju doesn't think
				// the volume is active when it's not.
				if fsInfo.Status != status.Pending.String() {
					filesystemUpdates[fs.FilesystemTag().String()] = filesystemInfo{
						unitTag:      unitTag,
						providerId:   unitParams.ProviderId,
						mountPoint:   fsInfo.MountPoint,
						readOnly:     fsInfo.ReadOnly,
						size:         fsInfo.Size,
						filesystemId: fsInfo.FilesystemId,
					}
				}
				filesystemStatus[fs.FilesystemTag().String()] = status.StatusInfo{
					Status:  status.Status(fsInfo.Status),
					Message: fsInfo.Info,
					Data:    fsInfo.Data,
				}

				// If the filesystem has a backing volume, get that info also.
				if _, err := fs.Volume(); err == nil {
					vol, err := a.storage.StorageInstanceVolume(sa.StorageInstance())
					if err != nil {
						return errors.Trace(err)
					}
					if fsInfo.Volume.Status != status.Pending.String() {
						volumeUpdates[vol.VolumeTag().String()] = volumeInfo{
							unitTag:    unitTag,
							providerId: unitParams.ProviderId,
							size:       fsInfo.Volume.Size,
							volumeId:   fsInfo.Volume.VolumeId,
							persistent: fsInfo.Volume.Persistent,
							readOnly:   fsInfo.ReadOnly,
						}
					}
					volumeStatus[vol.VolumeTag().String()] = status.StatusInfo{
						Status:  status.Status(fsInfo.Volume.Status),
						Message: fsInfo.Volume.Info,
						Data:    fsInfo.Volume.Data,
					}
				}

				infos = infos[1:]
				if len(infos) == 0 {
					break
				}
			}
		}
		return nil
	}

	unitUpdateParams := make(map[coreunit.Name]applicationservice.UpdateCAASUnitParams, len(unitUpdates))
	processedFilesystemIds := set.NewStrings()
	for _, unitParams := range unitUpdates {
		unitName, ok := unitByProviderID[unitParams.ProviderId]
		if !ok {
			//a.logger.Warningf(ctx, "ignoring non-existent unit with provider id %q", unitParams.ProviderId)
			continue
		}

		updateParams := processUnitParams(unitParams)
		unitUpdateParams[unitName] = updateParams

		unitTag := names.NewUnitTag(unitName.String())

		if len(unitParams.FilesystemInfo) > 0 {
			err := processFilesystemParams(processedFilesystemIds, unitTag, unitParams)
			if err != nil {
				return nil, errors.Annotatef(err, "processing filesystems for unit %q", unitTag)
			}
		}
	}
	for unitName := range unitUpdateParams {
		err = a.applicationService.UpdateCAASUnit(ctx, unitName, unitUpdateParams[unitName])
		// We ignore any updates for dying applications.
		if errors.Is(err, applicationerrors.ApplicationNotAlive) {
			return nil, nil
		} else if err != nil {
			return nil, errors.Annotatef(err, "updating unit %q", unitName)
		}
	}

	// If pods are recreated on the Kubernetes side, new units are created on the Juju
	// side and so any previously attached filesystems become orphaned and need to
	// be cleaned up.
	if err := a.cleanupOrphanedFilesystems(ctx, processedFilesystemIds); err != nil {
		return nil, errors.Annotatef(err, "deleting orphaned filesystems for %v", appName)
	}

	// First do the volume updates as volumes need to be attached before the filesystem updates.
	if err := a.updateVolumeInfo(ctx, volumeUpdates, volumeStatus); err != nil {
		return nil, errors.Annotatef(err, "updating volume information for %v", appName)
	}

	if err := a.updateFilesystemInfo(ctx, filesystemUpdates, filesystemStatus); err != nil {
		return nil, errors.Annotatef(err, "updating filesystem information for %v", appName)
	}

	var appUnitInfo []params.ApplicationUnitInfo
	for _, c := range containers {
		appUnitInfo = append(appUnitInfo, params.ApplicationUnitInfo{
			ProviderId: c.ProviderId(),
			UnitTag:    names.NewUnitTag(c.Unit()).String(),
		})
	}
	return appUnitInfo, nil
}

func (a *API) cleanupOrphanedFilesystems(ctx context.Context, processedFilesystemIds set.Strings) error {
	// TODO(caas) - record unit id on the filesystem so we can query by unit
	allFilesystems, err := a.storage.AllFilesystems()
	if err != nil {
		return errors.Trace(err)
	}
	for _, fs := range allFilesystems {
		fsInfo, err := fs.Info()
		if errors.Is(err, errors.NotProvisioned) {
			continue
		}
		if err != nil {
			return errors.Trace(err)
		}
		if !processedFilesystemIds.Contains(fsInfo.FilesystemId) {
			continue
		}

		storageTag, err := fs.Storage()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if err != nil {
			continue
		}

		si, err := a.storage.StorageInstance(storageTag)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if err != nil {
			continue
		}
		_, ok := si.Owner()
		if ok {
			continue
		}

		a.logger.Debugf(ctx, "found orphaned filesystem %v", fs.FilesystemTag())
		// TODO (anastasiamac 2019-04-04) We can now force storage removal
		// but for now, while we have not an arg passed in, just hardcode.
		err = a.storage.DestroyStorageInstance(storageTag, false, false, time.Duration(0))
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		err = a.storage.DestroyFilesystem(fs.FilesystemTag(), false)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *API) updateVolumeInfo(ctx context.Context, volumeUpdates map[string]volumeInfo, volumeStatus map[string]status.StatusInfo) error {
	// Do it in sorted order so it's deterministic for tests.
	var volTags []string
	for tag := range volumeUpdates {
		volTags = append(volTags, tag)
	}
	sort.Strings(volTags)

	a.logger.Debugf(ctx, "updating volume data: %+v", volumeUpdates)
	for _, tagString := range volTags {
		volTag, _ := names.ParseVolumeTag(tagString)
		volData := volumeUpdates[tagString]

		vol, err := a.storage.Volume(volTag)
		if err != nil {
			return errors.Trace(err)
		}
		// If we have already recorded the provisioning info,
		// it's an error to try and do it again.
		_, err = vol.Info()
		if err != nil && !errors.Is(err, errors.NotProvisioned) {
			return errors.Trace(err)
		}
		if err != nil {
			// Provisioning info not set yet.
			err = a.storage.SetVolumeInfo(volTag, state.VolumeInfo{
				Size:       volData.size,
				VolumeId:   volData.volumeId,
				Persistent: volData.persistent,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}
		err = a.storage.SetVolumeAttachmentInfo(volData.unitTag, volTag, state.VolumeAttachmentInfo{
			ReadOnly: volData.readOnly,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Do it in sorted order so it's deterministic for tests.
	volTags = []string{}
	for tag := range volumeStatus {
		volTags = append(volTags, tag)
	}
	sort.Strings(volTags)

	a.logger.Debugf(ctx, "updating volume status: %+v", volumeStatus)
	for _, tagString := range volTags {
		volTag, _ := names.ParseVolumeTag(tagString)
		volStatus := volumeStatus[tagString]
		vol, err := a.storage.Volume(volTag)
		if err != nil {
			return errors.Trace(err)
		}
		now := a.clock.Now()
		err = vol.SetStatus(status.StatusInfo{
			Status:  volStatus.Status,
			Message: volStatus.Message,
			Data:    volStatus.Data,
			Since:   &now,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (a *API) updateFilesystemInfo(ctx context.Context, filesystemUpdates map[string]filesystemInfo, filesystemStatus map[string]status.StatusInfo) error {
	// Do it in sorted order so it's deterministic for tests.
	var fsTags []string
	for tag := range filesystemUpdates {
		fsTags = append(fsTags, tag)
	}
	sort.Strings(fsTags)

	a.logger.Debugf(ctx, "updating filesystem data: %+v", filesystemUpdates)
	for _, tagString := range fsTags {
		fsTag, _ := names.ParseFilesystemTag(tagString)
		fsData := filesystemUpdates[tagString]

		fs, err := a.storage.Filesystem(fsTag)
		if err != nil {
			return errors.Trace(err)
		}
		// If we have already recorded the provisioning info,
		// it's an error to try and do it again.
		_, err = fs.Info()
		if err != nil && !errors.Is(err, errors.NotProvisioned) {
			return errors.Trace(err)
		}
		if err != nil {
			// Provisioning info not set yet.
			err = a.storage.SetFilesystemInfo(fsTag, state.FilesystemInfo{
				Size:         fsData.size,
				FilesystemId: fsData.filesystemId,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}

		err = a.storage.SetFilesystemAttachmentInfo(fsData.unitTag, fsTag, state.FilesystemAttachmentInfo{
			MountPoint: fsData.mountPoint,
			ReadOnly:   fsData.readOnly,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Do it in sorted order so it's deterministic for tests.
	fsTags = []string{}
	for tag := range filesystemStatus {
		fsTags = append(fsTags, tag)
	}
	sort.Strings(fsTags)

	a.logger.Debugf(ctx, "updating filesystem status: %+v", filesystemStatus)
	for _, tagString := range fsTags {
		fsTag, _ := names.ParseFilesystemTag(tagString)
		fsStatus := filesystemStatus[tagString]
		fs, err := a.storage.Filesystem(fsTag)
		if err != nil {
			return errors.Trace(err)
		}
		now := a.clock.Now()
		err = fs.SetStatus(status.StatusInfo{
			Status:  fsStatus.Status,
			Message: fsStatus.Message,
			Data:    fsStatus.Data,
			Since:   &now,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func processUnitParams(unitParams params.ApplicationUnitParams) applicationservice.UpdateCAASUnitParams {
	agentStatus, cloudContainerStatus := updateStatus(unitParams)
	return applicationservice.UpdateCAASUnitParams{
		ProviderID:           &unitParams.ProviderId,
		Address:              &unitParams.Address,
		Ports:                &unitParams.Ports,
		AgentStatus:          agentStatus,
		CloudContainerStatus: cloudContainerStatus,
	}
}

// updateStatus constructs the agent and cloud container status values.
func updateStatus(params params.ApplicationUnitParams) (
	agentStatus *status.StatusInfo,
	cloudContainerStatus *status.StatusInfo,
) {
	switch status.Status(params.Status) {
	case status.Unknown:
		// The container runtime can spam us with unimportant
		// status updates, so ignore any irrelevant ones.
		return nil, nil
	case status.Allocating:
		// The container runtime has decided to restart the pod.
		agentStatus = &status.StatusInfo{
			Status:  status.Allocating,
			Message: params.Info,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Waiting,
			Message: params.Info,
			Data:    params.Data,
		}
	case status.Running:
		// A pod has finished starting so the workload is now active.
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Running,
			Message: params.Info,
			Data:    params.Data,
		}
	case status.Error:
		agentStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: params.Info,
			Data:    params.Data,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: params.Info,
			Data:    params.Data,
		}
	case status.Blocked:
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		cloudContainerStatus = &status.StatusInfo{
			Status:  status.Blocked,
			Message: params.Info,
			Data:    params.Data,
		}
	}
	return agentStatus, cloudContainerStatus
}

// ClearApplicationsResources clears the flags which indicate
// applications still have resources in the cluster.
func (a *API) ClearApplicationsResources(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	for i := range args.Entities {
		result.Results[i].Error = apiservererrors.ServerError(errors.NotImplementedf("ClearResources"))
	}
	return result, nil
}

// DestroyUnits is responsible for scaling down a set of units on the this
// Application.
func (a *API) DestroyUnits(ctx context.Context, args params.DestroyUnitsParams) (params.DestroyUnitResults, error) {
	results := make([]params.DestroyUnitResult, 0, len(args.Units))

	for _, unit := range args.Units {
		res, err := a.destroyUnit(ctx, unit)
		if err != nil {
			res = params.DestroyUnitResult{
				Error: apiservererrors.ServerError(err),
			}
		}
		results = append(results, res)
	}

	return params.DestroyUnitResults{
		Results: results,
	}, nil
}

func (a *API) destroyUnit(ctx context.Context, args params.DestroyUnitParams) (params.DestroyUnitResult, error) {
	unitTag, err := names.ParseUnitTag(args.UnitTag)
	if err != nil {
		return params.DestroyUnitResult{}, internalerrors.Errorf("parsing unit tag: %w", err)
	}
	unitName, err := coreunit.NewName(unitTag.Id())
	if err != nil {
		return params.DestroyUnitResult{}, internalerrors.Errorf("parsing unit name: %w", err)
	}

	err = a.applicationService.DestroyUnit(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return params.DestroyUnitResult{}, nil
	} else if err != nil {
		return params.DestroyUnitResult{}, internalerrors.Errorf("destroying unit %q: %w", unitName, err)
	}

	return params.DestroyUnitResult{}, nil
}

func ptr[T any](v T) *T {
	return &v
}

func encodeOSType(t deployment.OSType) (string, error) {
	switch t {
	case deployment.Ubuntu:
		return strings.ToLower(ostype.Ubuntu.String()), nil
	default:
		return "", internalerrors.Errorf("unsupported OS type %v", t)
	}
}
