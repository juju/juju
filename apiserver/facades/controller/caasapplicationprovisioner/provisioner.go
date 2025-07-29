// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/os/ostype"
	coreresource "github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type APIGroup struct {
	*API

	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI
	lifeCanRead     common.GetAuthFunc
}

type NewResourceOpenerFunc func(ctx context.Context, appName string) (coreresource.Opener, error)

type API struct {
	auth            facade.Authorizer
	watcherRegistry facade.WatcherRegistry

	applicationService      ApplicationService
	controllerConfigService ControllerConfigService
	controllerNodeService   ControllerNodeService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
	statusService           StatusService
	removalService          RemovalService
	getCanWatch             common.GetAuthFunc
	clock                   clock.Clock
	logger                  corelogger.Logger
}

// NewStateCAASApplicationProvisionerAPI provides the signature required for facade registration.
func NewStateCAASApplicationProvisionerAPI(stdCtx context.Context, ctx facade.ModelContext) (*APIGroup, error) {
	authorizer := ctx.Auth()

	domainServices := ctx.DomainServices()

	controllerConfigService := domainServices.ControllerConfig()
	modelConfigService := domainServices.Config()

	applicationService := domainServices.Application()

	modelTag := names.NewModelTag(ctx.ModelUUID().String())

	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(modelTag, applicationService, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
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

	api, err := NewCAASApplicationProvisionerAPI(
		authorizer,
		services,
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
		charmInfoAPI:    commonCharmsAPI,
		appCharmInfoAPI: appCharmInfoAPI,
		lifeCanRead:     lifeCanRead,
		API:             api,
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
	authorizer facade.Authorizer,
	services Services,
	clock clock.Clock,
	logger corelogger.Logger,
	watcherRegistry facade.WatcherRegistry,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	getCanWatch := common.AuthFuncForTagKind(names.ApplicationTagKind)

	return &API{
		auth:                    authorizer,
		watcherRegistry:         watcherRegistry,
		controllerConfigService: services.ControllerConfigService,
		controllerNodeService:   services.ControllerNodeService,
		modelConfigService:      services.ModelConfigService,
		modelInfoService:        services.ModelInfoService,
		applicationService:      services.ApplicationService,
		statusService:           services.StatusService,
		removalService:          services.RemovalService,
		getCanWatch:             getCanWatch,
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
	mergedCons, err := a.modelInfoService.ResolveConstraints(ctx, cons)
	if err != nil && !errors.Is(err, modelerrors.ConstraintsNotFound) {
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
	addrs, err := a.controllerNodeService.GetAllAPIAddressesForAgents(ctx)
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
		Devices:              devices,
		Constraints:          mergedCons,
		Base:                 base,
		ImageRepo:            params.NewDockerImageInfo(docker.ConvertToResourceImageDetails(imageRepoDetails), imagePath),
		CharmModifiedVersion: charmModifiedVersion,
		Trust:                trustSetting,
		Scale:                scale,
	}, nil
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
	unitUUID, err := a.applicationService.GetUnitUUID(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return params.DestroyUnitResult{}, nil
	} else if err != nil {
		return params.DestroyUnitResult{}, internalerrors.Errorf("getting unit %q UUID: %w", unitName, err)
	}

	maxWait := time.Duration(0)
	if args.MaxWait != nil {
		maxWait = *args.MaxWait
	}
	_, err = a.removalService.RemoveUnit(ctx, unitUUID, args.Force, maxWait)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return params.DestroyUnitResult{}, nil
	} else if err != nil {
		return params.DestroyUnitResult{}, internalerrors.Errorf("destroying unit %q: %w", unitName, err)
	}

	return params.DestroyUnitResult{}, nil
}

// Watch starts an NotifyWatcher for the given application.
func (a *API) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}

	canWatch, err := a.getCanWatch(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canWatch(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		}

		watcherID, err := a.watchEntity(ctx, tag.Id())
		result.Results[i].NotifyWatcherId = watcherID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (a *API) watchEntity(ctx context.Context, appName string) (string, error) {
	watcher, err := a.applicationService.WatchApplication(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return "", errors.NotFoundf("application %q", appName)
	} else if err != nil {
		return "", errors.Trace(err)
	}

	id, _, err := internal.EnsureRegisterWatcher(ctx, a.watcherRegistry, watcher)
	if err != nil {
		return "", errors.Annotatef(err, "registering watcher for application %q", appName)
	}
	return id, nil
}

func encodeOSType(t deployment.OSType) (string, error) {
	switch t {
	case deployment.Ubuntu:
		return strings.ToLower(ostype.Ubuntu.String()), nil
	default:
		return "", internalerrors.Errorf("unsupported OS type %v", t)
	}
}
