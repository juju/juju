// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

var logger = loggo.GetLogger("juju.apiserver.controller.caasunitprovisioner")

type Facade struct {
	*common.LifeGetter
	entityWatcher   *common.AgentEntityWatcher
	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI

	resources          facade.Resources
	state              CAASUnitProvisionerState
	storage            StorageBackend
	storagePoolManager poolmanager.PoolManager
	registry           storage.ProviderRegistry
	devices            DeviceBackend
	clock              clock.Clock
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASUnitProvisionerState,
	sb StorageBackend,
	db DeviceBackend,
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	charmInfoAPI *charmscommon.CharmInfoAPI,
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI,
	clock clock.Clock,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	accessApplication := common.AuthFuncForTagKind(names.ApplicationTagKind)
	return &Facade{
		entityWatcher:   common.NewAgentEntityWatcher(st, resources, accessApplication),
		charmInfoAPI:    charmInfoAPI,
		appCharmInfoAPI: appCharmInfoAPI,
		LifeGetter: common.NewLifeGetter(
			st, common.AuthAny(
				common.AuthFuncForTagKind(names.ApplicationTagKind),
				common.AuthFuncForTagKind(names.UnitTagKind),
			),
		),
		resources:          resources,
		state:              st,
		storage:            sb,
		devices:            db,
		storagePoolManager: storagePoolManager,
		registry:           registry,
		clock:              clock,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch applications
// deployed to this model.
func (f *Facade) WatchApplications() (params.StringsWatchResult, error) {
	watch := f.state.WatchApplications()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: f.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// CharmInfo returns information about the requested charm.
func (f *Facade) CharmInfo(args params.CharmURL) (params.Charm, error) {
	return f.charmInfoAPI.CharmInfo(args)
}

// ApplicationCharmInfo returns information about an application's charm.
func (f *Facade) ApplicationCharmInfo(args params.Entity) (params.Charm, error) {
	return f.appCharmInfoAPI.ApplicationCharmInfo(args)
}

// Watch starts a NotifyWatcher for each entity given.
func (f *Facade) Watch(args params.Entities) (params.NotifyWatchResults, error) {
	return f.entityWatcher.Watch(args)
}

// WatchApplicationsScale starts a NotifyWatcher to watch changes
// to the applications' scale.
func (f *Facade) WatchApplicationsScale(args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchApplicationScale(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchApplicationScale(tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	w := app.WatchScale()
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// WatchPodSpec starts a NotifyWatcher to watch changes to the
// pod spec for specified units in this model.
func (f *Facade) WatchPodSpec(args params.Entities) (params.NotifyWatchResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchPodSpec(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchPodSpec(model Model, tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	w, err := model.WatchPodSpec(tag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// ApplicationsScale returns the scaling info for specified applications in this model.
func (f *Facade) ApplicationsScale(args params.Entities) (params.IntResults, error) {
	results := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		scale, err := f.applicationScale(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = scale
	}
	logger.Debugf("application scale result: %#v", results)
	return results, nil
}

func (f *Facade) applicationScale(tagString string) (int, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return 0, errors.Trace(err)
	}
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return 0, errors.Trace(err)
	}
	return app.GetScale(), nil
}

// ApplicationsTrust returns the trust status for specified applications in this model.
func (f *Facade) ApplicationsTrust(args params.Entities) (params.BoolResults, error) {
	results := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		trust, err := f.applicationTrust(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = trust
	}
	logger.Debugf("application trust result: %#v", results)
	return results, nil
}

func (f *Facade) applicationTrust(tagString string) (bool, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return false, errors.Trace(err)
	}
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return false, errors.Trace(err)
	}
	cfg, err := app.ApplicationConfig()
	if err != nil {
		return false, errors.Trace(err)
	}
	return cfg.GetBool(application.TrustConfigOptionName, false), nil
}

// WatchApplicationsTrustHash starts a StringsWatcher to watch changes
// to the applications' trust status.
func (f *Facade) WatchApplicationsTrustHash(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, err := f.watchApplicationTrustHash(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
	}
	return results, nil
}

func (f *Facade) watchApplicationTrustHash(tagString string) (string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	// This is currently implemented by just watching the
	// app config settings which is where the trust value
	// is stored. A similar pattern is used for model config
	// watchers pending better filtering on watchers.
	w := app.WatchConfigSettingsHash()
	if _, ok := <-w.Changes(); ok {
		return f.resources.Register(w), nil
	}
	return "", watcher.EnsureErr(w)
}

// DeploymentMode returns the deployment mode of the given applications' charms.
func (f *Facade) DeploymentMode(args params.Entities) (params.StringResults, error) {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		mode, err := f.applicationDeploymentMode(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = mode
	}
	return results, nil
}

func (f *Facade) applicationDeploymentMode(tagString string) (string, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", errors.Trace(err)
	}
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	ch, _, err := app.Charm()
	if err != nil {
		return "", errors.Trace(err)
	}
	var mode charm.DeploymentMode
	if d := ch.Meta().Deployment; d != nil {
		mode = d.DeploymentMode
	}
	if mode == "" {
		mode = charm.ModeWorkload
	}
	return string(mode), nil
}

// ProvisioningInfo returns the provisioning info for specified applications in this model.
func (f *Facade) ProvisioningInfo(args params.Entities) (params.KubernetesProvisioningInfoResults, error) {
	model, err := f.state.Model()
	if err != nil {
		return params.KubernetesProvisioningInfoResults{}, errors.Trace(err)
	}
	results := params.KubernetesProvisioningInfoResults{
		Results: make([]params.KubernetesProvisioningInfoResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		info, err := f.provisioningInfo(model, arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = info
	}
	return results, nil
}

func (f *Facade) provisioningInfo(model Model, tagString string) (*params.KubernetesProvisioningInfo, error) {
	appTag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// First the pod spec.
	podSpec, err := model.PodSpec(appTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rawSpec, err := model.RawK8sSpec(appTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if podSpec != "" && rawSpec != "" {
		// This should never happen.
		return nil, errors.New("both k8s spec and raw k8s spec were set")
	}

	// Now get any required storage. We need to provision storage
	// at the same time as the pod as it can't be attached later.

	// All units are currently homogeneous so we just
	// need to get info for the first alive unit.
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerCfg, err := f.state.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return nil, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in model config %q", modelConfig.Name()),
		)
	}
	registryPath, err := podcfg.GetJujuOCIImagePath(controllerCfg, vers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(controllerCfg.CAASImageRepo())
	if err != nil {
		return nil, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	imageRepo := params.NewDockerImageInfo(imageRepoDetails, registryPath)
	logger.Tracef("imageRepo %v", imageRepo)
	filesystemParams, err := f.applicationFilesystemParams(app, controllerCfg, modelConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	devices, err := f.devicesParams(app)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cons, err := app.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	mergedCons, err := f.state.ResolveConstraints(cons)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourceTags := tags.ResourceTags(
		names.NewModelTag(modelConfig.UUID()),
		names.NewControllerTag(controllerCfg.ControllerUUID()),
		modelConfig,
	)

	ch, _, err := app.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := &params.KubernetesProvisioningInfo{
		PodSpec:              podSpec,
		RawK8sSpec:           rawSpec,
		Filesystems:          filesystemParams,
		Devices:              devices,
		Constraints:          mergedCons,
		Tags:                 resourceTags,
		CharmModifiedVersion: app.CharmModifiedVersion(),
		ImageRepo:            imageRepo,
		StorageID:            app.StorageID(),
	}
	deployInfo := ch.Meta().Deployment
	if deployInfo != nil {
		info.DeploymentInfo = &params.KubernetesDeploymentInfo{
			DeploymentType: string(deployInfo.DeploymentType),
			ServiceType:    string(deployInfo.ServiceType),
		}
	}
	return info, nil
}

func filesystemParams(
	app Application,
	cons state.StorageConstraints,
	storageName string,
	controllerUUID string,
	modelConfig *config.Config,
	poolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (*params.KubernetesFilesystemParams, error) {

	filesystemTags, err := storagecommon.StorageTags(nil, modelConfig.UUID(), controllerUUID, modelConfig)
	if err != nil {
		return nil, errors.Annotate(err, "computing storage tags")
	}
	filesystemTags[tags.JujuStorageOwner] = app.Name()

	storageClassName, _ := modelConfig.AllAttrs()[k8sconstants.WorkloadStorageKey].(string)
	if cons.Pool == "" && storageClassName == "" {
		return nil, errors.Errorf("storage pool for %q must be specified since there's no model default storage class", storageName)
	}
	fsParams, err := caasoperatorprovisioner.CharmStorageParams(controllerUUID, storageClassName, modelConfig, cons.Pool, poolManager, registry)
	if err != nil {
		return nil, errors.Maskf(err, "getting filesystem storage parameters")
	}

	fsParams.Size = cons.Size
	fsParams.StorageName = storageName
	fsParams.Tags = filesystemTags
	return fsParams, nil
}

// applicationFilesystemParams retrieves FilesystemParams for the filesystems
// that should be provisioned with, and attached to, pods of the application.
func (f *Facade) applicationFilesystemParams(
	app Application,
	controllerConfig controller.Config,
	modelConfig *config.Config,
) ([]params.KubernetesFilesystemParams, error) {
	storageConstraints, err := app.StorageConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ch, _, err := app.Charm()
	if err != nil {
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
			app, cons, name,
			controllerConfig.ControllerUUID(),
			modelConfig,
			f.storagePoolManager, f.registry,
		)
		if err != nil {
			return nil, errors.Annotatef(err, "getting filesystem %q parameters", name)
		}
		for i := 0; i < int(cons.Count); i++ {
			charmStorage := ch.Meta().Storage[name]
			id := fmt.Sprintf("%s/%v", name, i)
			tag := names.NewStorageTag(id)
			location, err := state.FilesystemMountPoint(charmStorage, tag, "ubuntu")
			if err != nil {
				return nil, errors.Trace(err)
			}
			filesystemAttachmentParams := params.KubernetesFilesystemAttachmentParams{
				Provider:   fsParams.Provider,
				MountPoint: location,
				ReadOnly:   charmStorage.ReadOnly,
			}
			fsParams.Attachment = &filesystemAttachmentParams
			allFilesystemParams = append(allFilesystemParams, *fsParams)
		}
	}
	return allFilesystemParams, nil
}

func (f *Facade) devicesParams(app Application) ([]params.KubernetesDeviceParams, error) {
	devices, err := app.DeviceConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("getting device constraints from state: %#v", devices)
	var devicesParams []params.KubernetesDeviceParams
	for _, d := range devices {
		devicesParams = append(devicesParams, params.KubernetesDeviceParams{
			Type:       params.DeviceType(d.Type),
			Count:      d.Count,
			Attributes: d.Attributes,
		})
	}
	return devicesParams, nil
}

// ApplicationsConfig returns the config for the specified applications.
func (f *Facade) ApplicationsConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := f.getApplicationConfig(arg.Tag)
		results.Results[i].Config = result
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (f *Facade) getApplicationConfig(tagString string) (map[string]interface{}, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return app.ApplicationConfig()
}

// UpdateApplicationsUnits updates the Juju data model to reflect the given
// units of the specified application.
func (f *Facade) UpdateApplicationsUnits(args params.UpdateApplicationUnitArgs) (params.UpdateApplicationUnitResults, error) {
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
		app, err := f.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		appStatus := appUpdate.Status
		if appStatus.Status != "" && appStatus.Status != status.Unknown {
			now := f.clock.Now()
			err = app.SetOperatorStatus(status.StatusInfo{
				Status:  appStatus.Status,
				Message: appStatus.Info,
				Data:    appStatus.Data,
				Since:   &now,
			})
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
		appUnitInfo, err := f.updateUnitsFromCloud(app, appUpdate.Scale, appUpdate.Generation, appUpdate.Units)
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

// updateStatus constructs the agent and cloud container status values.
func (f *Facade) updateStatus(params params.ApplicationUnitParams) (
	agentStatus *status.StatusInfo,
	cloudContainerStatus *status.StatusInfo,
) {
	var containerStatus status.Status
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
		containerStatus = status.Waiting
	case status.Running:
		// A pod has finished starting so the workload is now active.
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		containerStatus = status.Running
	case status.Error:
		agentStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: params.Info,
			Data:    params.Data,
		}
		containerStatus = status.Error
	case status.Blocked:
		containerStatus = status.Blocked
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
	}
	cloudContainerStatus = &status.StatusInfo{
		Status:  containerStatus,
		Message: params.Info,
		Data:    params.Data,
	}
	return agentStatus, cloudContainerStatus
}

// updateUnitsFromCloud takes a slice of unit information provided by an external
// source (typically a cloud update event) and merges that with the existing unit
// data model in state. The passed in units are the complete set for the cloud, so
// any existing units in state with provider ids which aren't in the set will be removed.
func (f *Facade) updateUnitsFromCloud(app Application, scale *int,
	generation *int64, unitUpdates []params.ApplicationUnitParams) ([]params.ApplicationUnitInfo, error) {
	logger.Debugf("unit updates: %#v", unitUpdates)
	if scale != nil {
		logger.Debugf("application scale: %v", *scale)
		if *scale > 0 && len(unitUpdates) == 0 {
			// no ops for empty units because we can not determine if it's stateful or not in this case.
			logger.Debugf("ignoring empty k8s event for %q", app.Tag().String())
			return nil, nil
		}
	}
	// Set up the initial data structures.
	existingStateUnits, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stateUnitsById := make(map[string]stateUnit)
	cloudPodsById := make(map[string]params.ApplicationUnitParams)

	// Record all unit provider ids known to exist in the cloud.
	for _, u := range unitUpdates {
		cloudPodsById[u.ProviderId] = u
	}

	stateUnitExistsInCloud := func(providerId string) bool {
		if providerId == "" {
			return false
		}
		_, ok := cloudPodsById[providerId]
		return ok
	}

	unitInfo := &updateStateUnitParams{
		stateUnitsInCloud: make(map[string]Unit),
		deletedRemoved:    true,
	}
	var (
		// aliveStateIds holds the provider ids of alive units in state.
		aliveStateIds = set.NewStrings()

		// extraStateIds holds the provider ids of units in state which
		// no longer exist in the cloud.
		extraStateIds = set.NewStrings()
	)

	// Loop over any existing state units and record those which do not yet have
	// provider ids, and those which have been removed or updated.
	for _, u := range existingStateUnits {
		var providerId string
		info, err := u.ContainerInfo()
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			providerId = info.ProviderId()
		}

		unitAlive := u.Life() == state.Alive
		if !unitAlive {
			continue
		}
		if providerId == "" {
			logger.Debugf("unit %q is not associated with any pod", u.Name())
			unitInfo.unassociatedUnits = append(unitInfo.unassociatedUnits, u)
			continue
		}

		stateUnitsById[providerId] = stateUnit{Unit: u}
		stateUnitInCloud := stateUnitExistsInCloud(providerId)
		aliveStateIds.Add(providerId)
		if stateUnitInCloud {
			logger.Debugf("unit %q (%v) has changed in the cloud", u.Name(), providerId)
			unitInfo.stateUnitsInCloud[u.UnitTag().String()] = u
		} else {
			logger.Debugf("unit %q (%v) has removed in the cloud", u.Name(), providerId)
			extraStateIds.Add(providerId)
		}
	}

	// Do it in sorted order so it's deterministic for tests.
	var ids []string
	for id := range cloudPodsById {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Sort extra ids also to guarantee order.
	var extraIds []string
	for id := range extraStateIds {
		extraIds = append(extraIds, id)
	}
	sort.Strings(extraIds)
	unassociatedUnitCount := len(unitInfo.unassociatedUnits)
	extraUnitsInStateCount := 0
	if scale != nil {
		extraUnitsInStateCount = len(stateUnitsById) + unassociatedUnitCount - *scale
	}

	for _, id := range ids {
		u := cloudPodsById[id]
		unitInfo.deletedRemoved = !u.Stateful
		if aliveStateIds.Contains(id) {
			u.UnitTag = stateUnitsById[id].UnitTag().String()
			unitInfo.existingCloudPods = append(unitInfo.existingCloudPods, u)
			continue
		}

		// First attempt to add any new cloud pod not yet represented in state
		// to a unit which does not yet have a provider id.
		if unassociatedUnitCount > 0 {
			unassociatedUnitCount--
			unitInfo.addedCloudPods = append(unitInfo.addedCloudPods, u)
			continue
		}

		// A new pod was added to the cloud but does not yet have a unit in state.
		unitInfo.addedCloudPods = append(unitInfo.addedCloudPods, u)
	}

	// If there are any extra provider ids left over after allocating all the cloud pods,
	// then consider those state units as terminated.
	logger.Debugf("alive state ids %v", aliveStateIds.Values())
	logger.Debugf("extra state ids %v", extraStateIds.Values())
	logger.Debugf("extra units in state: %v", extraUnitsInStateCount)
	for _, providerId := range extraIds {
		u := stateUnitsById[providerId]
		logger.Debugf("unit %q (%v) has been removed from the cloud", u.Name(), providerId)
		// If the unit in state is surplus to the application scale, remove it from state also.
		// We retain units in state that are not surplus to cloud requirements as they will
		// be regenerated by the cloud and we want to keep a stable unit name.
		u.delete = unitInfo.deletedRemoved && scale != nil
		if !u.delete && extraUnitsInStateCount > 0 {
			logger.Debugf("deleting %v because it exceeds the scale of %v", u.Name(), scale)
			u.delete = true
			extraUnitsInStateCount--
		}
		unitInfo.removedUnits = append(unitInfo.removedUnits, u)
	}

	if err := f.updateStateUnits(app, unitInfo); err != nil {
		return nil, errors.Trace(err)
	}

	var providerIds []string
	for _, u := range unitUpdates {
		providerIds = append(providerIds, u.ProviderId)
	}
	m, err := f.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	containers, err := m.Containers(providerIds...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var appUnitInfo []params.ApplicationUnitInfo
	for _, c := range containers {
		appUnitInfo = append(appUnitInfo, params.ApplicationUnitInfo{
			ProviderId: c.ProviderId(),
			UnitTag:    names.NewUnitTag(c.Unit()).String(),
		})
	}

	if scale == nil {
		return appUnitInfo, nil
	}
	// Update the scale last now that the state
	// model accurately reflects the cluster pods.
	currentScale := app.GetScale()
	var gen int64
	if generation != nil {
		gen = *generation
	}
	if currentScale != *scale {
		return appUnitInfo, app.SetScale(*scale, gen, false)
	}
	return appUnitInfo, nil
}

type stateUnit struct {
	Unit
	delete bool
}

type updateStateUnitParams struct {
	stateUnitsInCloud map[string]Unit
	addedCloudPods    []params.ApplicationUnitParams
	existingCloudPods []params.ApplicationUnitParams
	removedUnits      []stateUnit
	unassociatedUnits []Unit
	deletedRemoved    bool
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

func (f *Facade) updateStateUnits(app Application, unitInfo *updateStateUnitParams) error {

	if app.Life() != state.Alive {
		// We ignore any updates for dying applications.
		logger.Debugf("ignoring unit updates for dying application: %v", app.Name())
		return nil
	}

	logger.Tracef("added cloud units: %+v", unitInfo.addedCloudPods)
	logger.Tracef("existing cloud units: %+v", unitInfo.existingCloudPods)
	logger.Tracef("removed units: %+v", unitInfo.removedUnits)
	logger.Tracef("unassociated units: %+v", unitInfo.unassociatedUnits)

	// Now we have the added, removed, updated units all sorted,
	// generate the state update operations.
	var unitUpdate state.UpdateUnitsOperation

	filesystemUpdates := make(map[string]filesystemInfo)
	filesystemStatus := make(map[string]status.StatusInfo)
	volumeUpdates := make(map[string]volumeInfo)
	volumeStatus := make(map[string]status.StatusInfo)

	for _, u := range unitInfo.removedUnits {
		// If a unit is removed from the cloud, all filesystems are considered detached.
		unitStorage, err := f.storage.UnitStorageAttachments(u.UnitTag())
		if err != nil {
			return errors.Trace(err)
		}
		for _, sa := range unitStorage {
			fs, err := f.storage.StorageInstanceFilesystem(sa.StorageInstance())
			if err != nil {
				return errors.Trace(err)
			}
			filesystemStatus[fs.FilesystemTag().String()] = status.StatusInfo{Status: status.Detached}
		}

		if u.delete {
			unitUpdate.Deletes = append(unitUpdate.Deletes, u.DestroyOperation())
		}
		// We'll set the status as Terminated. This will either be transient, as will
		// occur when a pod is restarted external to Juju, or permanent if the pod has
		// been deleted external to Juju. In the latter case, juju remove-unit will be
		// need to clean things up on the Juju side.
		cloudContainerStatus := &status.StatusInfo{
			Status:  status.Terminated,
			Message: "unit stopped by the cloud",
		}
		agentStatus := &status.StatusInfo{
			Status: status.Idle,
		}
		updateProps := state.UnitUpdateProperties{
			CloudContainerStatus: cloudContainerStatus,
			AgentStatus:          agentStatus,
		}
		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(updateProps))
	}

	processUnitParams := func(unitParams params.ApplicationUnitParams) *state.UnitUpdateProperties {
		agentStatus, cloudContainerStatus := f.updateStatus(unitParams)
		return &state.UnitUpdateProperties{
			ProviderId:           &unitParams.ProviderId,
			Address:              &unitParams.Address,
			Ports:                &unitParams.Ports,
			AgentStatus:          agentStatus,
			CloudContainerStatus: cloudContainerStatus,
		}
	}

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
			logger.Debugf("updating storage %v for %v", storageName, unitTag)
			if len(infos) == 0 {
				continue
			}

			unitStorage, err := f.storage.UnitStorageAttachments(unitTag)
			if err != nil {
				return errors.Trace(err)
			}

			// Loop over all the storage for the unit and skip storage not
			// relevant for storageName.
			// TODO(caas) - Add storage bankend API to get all unit storage instances for a named storage.
			for _, sa := range unitStorage {
				si, err := f.storage.StorageInstance(sa.StorageInstance())
				if errors.IsNotFound(err) {
					logger.Warningf("ignoring non-existent storage instance %v for unit %v", sa.StorageInstance(), unitTag.Id())
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if si.StorageName() != storageName {
					continue
				}
				fs, err := f.storage.StorageInstanceFilesystem(sa.StorageInstance())
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
					vol, err := f.storage.StorageInstanceVolume(sa.StorageInstance())
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

	var unitParamsWithFilesystemInfo []params.ApplicationUnitParams

	for _, unitParams := range unitInfo.existingCloudPods {
		u, ok := unitInfo.stateUnitsInCloud[unitParams.UnitTag]
		if !ok {
			logger.Warningf("unexpected unit parameters %+v not in state", unitParams)
			continue
		}
		updateProps := processUnitParams(unitParams)
		if len(unitParams.FilesystemInfo) > 0 {
			unitParamsWithFilesystemInfo = append(unitParamsWithFilesystemInfo, unitParams)
		}
		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(*updateProps))
	}

	// For newly added units in the cloud, either update state units which
	// exist but which do not yet have provider ids (recording the provider
	// id as well), or add a brand new unit.
	idx := 0
	for _, unitParams := range unitInfo.addedCloudPods {
		if idx < len(unitInfo.unassociatedUnits) {
			u := unitInfo.unassociatedUnits[idx]
			updateProps := processUnitParams(unitParams)
			unitUpdate.Updates = append(unitUpdate.Updates,
				u.UpdateOperation(*updateProps))
			idx++
			if len(unitParams.FilesystemInfo) > 0 {
				unitParamsWithFilesystemInfo = append(unitParamsWithFilesystemInfo, unitParams)
			}
			continue
		}

		// Process units added directly in the cloud instead of via Juju.
		updateProps := processUnitParams(unitParams)
		if len(unitParams.FilesystemInfo) > 0 {
			unitParamsWithFilesystemInfo = append(unitParamsWithFilesystemInfo, unitParams)
		}
		unitUpdate.Adds = append(unitUpdate.Adds,
			app.AddOperation(*updateProps))
	}
	err := app.UpdateUnits(&unitUpdate)
	// We ignore any updates for dying applications.
	if stateerrors.IsNotAlive(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Now update filesystem info - attachment data and status.
	// For units added to the cloud directly, we first need to lookup the
	// newly created unit tag from Juju using the cloud provider ids.
	var providerIds []string
	for _, unitParams := range unitParamsWithFilesystemInfo {
		if unitParams.UnitTag == "" {
			providerIds = append(providerIds, unitParams.ProviderId)
		}
	}
	m, err := f.state.Model()
	if err != nil {
		return errors.Trace(err)
	}
	var providerIdToUnit = make(map[string]names.UnitTag)
	containers, err := m.Containers(providerIds...)
	if err != nil {
		return errors.Trace(err)
	}
	for _, c := range containers {
		providerIdToUnit[c.ProviderId()] = names.NewUnitTag(c.Unit())
	}

	processedFilesystemIds := set.NewStrings()
	for _, unitParams := range unitParamsWithFilesystemInfo {
		var (
			unitTag names.UnitTag
			ok      bool
		)
		// For units added to the cloud directly, we first need to lookup the
		// newly created unit tag from Juju using the cloud provider ids.
		if unitParams.UnitTag == "" {
			unitTag, ok = providerIdToUnit[unitParams.ProviderId]
			if !ok {
				logger.Warningf("cannot update filesystem data for unknown pod %q", unitParams.ProviderId)
				continue
			}
		} else {
			unitTag, _ = names.ParseUnitTag(unitParams.UnitTag)
		}
		if err := processFilesystemParams(processedFilesystemIds, unitTag, unitParams); err != nil {
			return errors.Annotatef(err, "processing filesystem info for unit %q", unitTag.Id())
		}
	}

	// If pods are recreated on the Kubernetes side, new units are created on the Juju
	// side and so any previously attached filesystems become orphaned and need to
	// be cleaned up.
	appName := app.Name()
	if err := f.cleanupOrphanedFilesystems(processedFilesystemIds); err != nil {
		return errors.Annotatef(err, "deleting orphaned filesystems for %v", appName)
	}

	// First do the volume updates as volumes need to be attached before the filesystem updates.
	if err := f.updateVolumeInfo(volumeUpdates, volumeStatus); err != nil {
		return errors.Annotatef(err, "updating volume information for %v", appName)
	}

	err = f.updateFilesystemInfo(filesystemUpdates, filesystemStatus)
	return errors.Annotatef(err, "updating filesystem information for %v", appName)
}

func (f *Facade) cleanupOrphanedFilesystems(processedFilesystemIds set.Strings) error {
	// TODO(caas) - record unit id on the filesystem so we can query by unit
	allFilesystems, err := f.storage.AllFilesystems()
	if err != nil {
		return errors.Trace(err)
	}
	for _, fs := range allFilesystems {
		fsInfo, err := fs.Info()
		if errors.IsNotProvisioned(err) {
			continue
		}
		if err != nil {
			return errors.Trace(err)
		}
		if !processedFilesystemIds.Contains(fsInfo.FilesystemId) {
			continue
		}

		storageTag, err := fs.Storage()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err != nil {
			continue
		}

		si, err := f.storage.StorageInstance(storageTag)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err != nil {
			continue
		}
		_, ok := si.Owner()
		if ok {
			continue
		}

		logger.Debugf("found orphaned filesystem %v", fs.FilesystemTag())
		// TODO (anastasiamac 2019-04-04) We can now force storage removal
		// but for now, while we have not an arg passed in, just hardcode.
		err = f.storage.DestroyStorageInstance(storageTag, false, false, time.Duration(0))
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		err = f.storage.DestroyFilesystem(fs.FilesystemTag(), false)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (f *Facade) updateVolumeInfo(volumeUpdates map[string]volumeInfo, volumeStatus map[string]status.StatusInfo) error {
	// Do it in sorted order so it's deterministic for tests.
	var volTags []string
	for tag := range volumeUpdates {
		volTags = append(volTags, tag)
	}
	sort.Strings(volTags)

	logger.Debugf("updating volume data: %+v", volumeUpdates)
	for _, tagString := range volTags {
		volTag, _ := names.ParseVolumeTag(tagString)
		volData := volumeUpdates[tagString]

		vol, err := f.storage.Volume(volTag)
		if err != nil {
			return errors.Trace(err)
		}
		// If we have already recorded the provisioning info,
		// it's an error to try and do it again.
		_, err = vol.Info()
		if err != nil && !errors.IsNotProvisioned(err) {
			return errors.Trace(err)
		}
		if err != nil {
			// Provisioning info not set yet.
			err = f.storage.SetVolumeInfo(volTag, state.VolumeInfo{
				Size:       volData.size,
				VolumeId:   volData.volumeId,
				Persistent: volData.persistent,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}

		err = f.storage.SetVolumeAttachmentInfo(volData.unitTag, volTag, state.VolumeAttachmentInfo{
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

	logger.Debugf("updating volume status: %+v", volumeStatus)
	for _, tagString := range volTags {
		volTag, _ := names.ParseVolumeTag(tagString)
		volStatus := volumeStatus[tagString]
		vol, err := f.storage.Volume(volTag)
		if err != nil {
			return errors.Trace(err)
		}
		now := f.clock.Now()
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

func (f *Facade) updateFilesystemInfo(filesystemUpdates map[string]filesystemInfo, filesystemStatus map[string]status.StatusInfo) error {
	// Do it in sorted order so it's deterministic for tests.
	var fsTags []string
	for tag := range filesystemUpdates {
		fsTags = append(fsTags, tag)
	}
	sort.Strings(fsTags)

	logger.Debugf("updating filesystem data: %+v", filesystemUpdates)
	for _, tagString := range fsTags {
		fsTag, _ := names.ParseFilesystemTag(tagString)
		fsData := filesystemUpdates[tagString]

		fs, err := f.storage.Filesystem(fsTag)
		if err != nil {
			return errors.Trace(err)
		}
		// If we have already recorded the provisioning info,
		// it's an error to try and do it again.
		_, err = fs.Info()
		if err != nil && !errors.IsNotProvisioned(err) {
			return errors.Trace(err)
		}
		if err != nil {
			// Provisioning info not set yet.
			err = f.storage.SetFilesystemInfo(fsTag, state.FilesystemInfo{
				Size:         fsData.size,
				FilesystemId: fsData.filesystemId,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}

		err = f.storage.SetFilesystemAttachmentInfo(fsData.unitTag, fsTag, state.FilesystemAttachmentInfo{
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

	logger.Debugf("updating filesystem status: %+v", filesystemStatus)
	for _, tagString := range fsTags {
		fsTag, _ := names.ParseFilesystemTag(tagString)
		fsStatus := filesystemStatus[tagString]
		fs, err := f.storage.Filesystem(fsTag)
		if err != nil {
			return errors.Trace(err)
		}
		now := f.clock.Now()
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

// ClearApplicationsResources clears the flags which indicate
// applications still have resources in the cluster.
func (f *Facade) ClearApplicationsResources(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	for i, entity := range args.Entities {
		appTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := f.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = app.ClearResources()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// UpdateApplicationsService updates the Juju data model to reflect the given
// service details of the specified application.
func (f *Facade) UpdateApplicationsService(args params.UpdateApplicationServiceArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
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
		app, err := f.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		sAddrs, err := params.ToProviderAddresses(appUpdate.Addresses...).ToSpaceAddresses(f.state)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := app.UpdateCloudService(appUpdate.ProviderId, sAddrs); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
		if appUpdate.Scale != nil {
			var generation int64
			if appUpdate.Generation != nil {
				generation = *appUpdate.Generation
			}
			if err := app.SetScale(*appUpdate.Scale, generation, false); err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
			}
		}
	}
	return result, nil
}

// SetOperatorStatus updates the operator status for each given application.
func (f *Facade) SetOperatorStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		appTag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := f.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		now := f.clock.Now()
		s := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		if err := app.SetOperatorStatus(s); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}
