// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"time"

	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.caasapplicationprovisioner")

type APIGroup struct {
	*common.PasswordChanger
	*common.LifeGetter
	*common.AgentEntityWatcher
	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI
	*API
}

type NewResourceOpenerFunc func(appName string) (resource.Opener, error)

type API struct {
	auth      facade.Authorizer
	resources facade.Resources

	ctrlSt             CAASApplicationControllerState
	state              CAASApplicationProvisionerState
	newResourceOpener  NewResourceOpenerFunc
	storage            StorageBackend
	storagePoolManager poolmanager.PoolManager
	registry           storage.ProviderRegistry
	clock              clock.Clock
}

// NewStateCAASApplicationProvisionerAPI provides the signature required for facade registration.
func NewStateCAASApplicationProvisionerAPI(ctx facade.Context) (*APIGroup, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	st := ctx.State()
	sb, err := state.NewStorageBackend(ctx.State())
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	commonState := &charmscommon.StateShim{st}
	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(commonState, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}

	newResourceOpener := func(appName string) (resource.Opener, error) {
		return resourceadapters.NewResourceOpenerForApplication(resourceadapters.NewResourceOpenerState(st), appName)
	}

	api, err := NewCAASApplicationProvisionerAPI(
		stateShim{ctx.StatePool().SystemState()},
		stateShim{st},
		resources,
		newResourceOpener,
		authorizer,
		sb,
		pm,
		registry,
		clock.WallClock,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiGroup := &APIGroup{
		PasswordChanger:    common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		LifeGetter:         common.NewLifeGetter(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		charmInfoAPI:       commonCharmsAPI,
		appCharmInfoAPI:    appCharmInfoAPI,
		API:                api,
	}

	return apiGroup, nil
}

// CharmInfo returns information about the requested charm.
func (a *APIGroup) CharmInfo(args params.CharmURL) (params.Charm, error) {
	return a.charmInfoAPI.CharmInfo(args)
}

// ApplicationCharmInfo returns information about an application's charm.
func (a *APIGroup) ApplicationCharmInfo(args params.Entity) (params.Charm, error) {
	return a.appCharmInfoAPI.ApplicationCharmInfo(args)
}

// NewCAASApplicationProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASApplicationProvisionerAPI(
	ctrlSt CAASApplicationControllerState,
	st CAASApplicationProvisionerState,
	resources facade.Resources,
	newResourceOpener NewResourceOpenerFunc,
	authorizer facade.Authorizer,
	sb StorageBackend,
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
	clock clock.Clock,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		auth:               authorizer,
		resources:          resources,
		newResourceOpener:  newResourceOpener,
		ctrlSt:             ctrlSt,
		state:              st,
		storage:            sb,
		storagePoolManager: storagePoolManager,
		registry:           registry,
		clock:              clock,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch applications
// deployed to this model.
func (a *API) WatchApplications() (params.StringsWatchResult, error) {
	watch := a.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// ProvisioningInfo returns the info needed to provision a caas application.
func (a *API) ProvisioningInfo(args params.Entities) (params.CAASApplicationProvisioningInfoResults, error) {
	var result params.CAASApplicationProvisioningInfoResults
	result.Results = make([]params.CAASApplicationProvisioningInfo, len(args.Entities))
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		info, err := a.provisioningInfo(appName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = *info
	}
	return result, nil
}

func (a *API) provisioningInfo(appName names.ApplicationTag) (*params.CAASApplicationProvisioningInfo, error) {
	app, err := a.state.Application(appName.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := a.ctrlSt.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := a.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	filesystemParams, err := a.applicationFilesystemParams(app, cfg, modelConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	devices, err := a.devicesParams(app)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cons, err := app.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	mergedCons, err := a.state.ResolveConstraints(cons)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resourceTags := tags.ResourceTags(
		names.NewModelTag(modelConfig.UUID()),
		names.NewControllerTag(cfg.ControllerUUID()),
		modelConfig,
	)

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return nil, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in model config %q", modelConfig.Name()),
		)
	}
	imagePath, err := podcfg.GetJujuOCIImagePath(cfg, vers.ToPatch(), version.OfficialBuild)
	if err != nil {
		return nil, errors.Annotatef(err, "getting juju oci image path")
	}
	imageRepo := cfg.CAASImageRepo()
	imageInfo := params.DockerImageInfo{
		Username:      imageRepo.Username,
		Password:      imageRepo.Password,
		Email:         imageRepo.Email,
		Repository:    imageRepo.Repository,
		Auth:          imageRepo.Auth.Content(),
		IdentityToken: imageRepo.IdentityToken.Content(),
		RegistryToken: imageRepo.RegistryToken.Content(),
	}

	apiHostPorts, err := a.ctrlSt.APIHostPortsForAgents()
	if err != nil {
		return nil, errors.Annotatef(err, "getting api addresses")
	}
	addrs := []string(nil)
	for _, hostPorts := range apiHostPorts {
		ordered := hostPorts.HostPorts().PrioritizedForScope(network.ScopeMatchCloudLocal)
		for _, addr := range ordered {
			if addr != "" {
				addrs = append(addrs, addr)
			}
		}
	}
	caCert, _ := cfg.CACert()
	charmURL, _ := app.CharmURL()
	appConfig, err := app.ApplicationConfig()
	if err != nil {
		return nil, errors.Annotatef(err, "getting application config")
	}
	return &params.CAASApplicationProvisioningInfo{
		ImagePath:            imagePath,
		Version:              vers,
		APIAddresses:         addrs,
		CACert:               caCert,
		Tags:                 resourceTags,
		Filesystems:          filesystemParams,
		Devices:              devices,
		Constraints:          mergedCons,
		Series:               app.Series(),
		ImageRepo:            imageInfo,
		CharmModifiedVersion: app.CharmModifiedVersion(),
		CharmURL:             charmURL.String(),
		Trust:                appConfig.GetBool(application.TrustConfigOptionName, false),
		Scale:                app.GetScale(),
	}, nil
}

// SetOperatorStatus sets the status of each given entity.
func (a *API) SetOperatorStatus(args params.SetStatus) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		info := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
		}
		results.Results[i].Error = apiservererrors.ServerError(a.setStatus(tag, info))
	}
	return results, nil
}

func (a *API) setStatus(tag names.ApplicationTag, info status.StatusInfo) error {
	app, err := a.state.Application(tag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	return app.SetOperatorStatus(info)
}

// Units returns all the units for each application specified.
func (a *API) Units(args params.Entities) (params.CAASUnitsResults, error) {
	results := params.CAASUnitsResults{
		Results: make([]params.CAASUnitsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := a.state.Application(appName.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		units, err := app.AllUnits()
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result := params.CAASUnitsResult{
			Units: make([]params.CAASUnitInfo, len(units)),
		}
		for uIdx, unit := range units {
			unitStatus, err := unit.Status()
			if err != nil {
				result.Error = apiservererrors.ServerError(err)
				break
			}
			result.Units[uIdx] = params.CAASUnitInfo{
				Tag: unit.Tag().String(),
				UnitStatus: &params.UnitStatus{
					AgentStatus:    statusInfoToDetailedStatus(unitStatus),
					WorkloadStatus: statusInfoToDetailedStatus(unitStatus),
				},
			}
		}
		results.Results[i] = result
	}
	return results, nil
}

func statusInfoToDetailedStatus(in status.StatusInfo) params.DetailedStatus {
	return params.DetailedStatus{
		Status: in.Status.String(),
		Info:   in.Message,
		Since:  in.Since,
		Data:   in.Data,
	}
}

// CAASApplicationGarbageCollect cleans up units that have gone away permanently.
// Only observed units will be deleted as new units could have surfaced between
// the capturing of kuberentes pod state/application state and this call.
func (a *API) CAASApplicationGarbageCollect(args params.CAASApplicationGarbageCollectArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := a.garbageCollectOneApplication(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

func (a *API) garbageCollectOneApplication(args params.CAASApplicationGarbageCollectArg) error {
	appName, err := names.ParseApplicationTag(args.Application.Tag)
	if err != nil {
		return errors.Trace(err)
	}
	observedUnitTags := set.NewStrings()
	for _, v := range args.ObservedUnits.Entities {
		tag, err := names.ParseUnitTag(v.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		observedUnitTags.Add(tag.String())
	}
	app, err := a.state.Application(appName.Id())
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(sidecar): support more than statefulset
	/*ch, _, err := app.Charm()
	if err != nil {
		return errors.Trace(err)
	}*/
	deploymentType := caas.DeploymentStateful

	model, err := a.state.Model()
	if err != nil {
		return errors.Trace(err)
	}
	containers, err := model.Containers(args.ActivePodNames...)
	if err != nil {
		return errors.Trace(err)
	}
	foundUnits := set.NewStrings()
	for _, v := range containers {
		foundUnits.Add(names.NewUnitTag(v.Unit()).String())
	}

	units, err := app.AllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	filesystemStatus := make(map[string]status.StatusInfo)
	op := state.UpdateUnitsOperation{}
	for _, unit := range units {
		tag := unit.Tag()
		if !args.Force {
			if !observedUnitTags.Contains(tag.String()) {
				// Was not known yet when pulling kubernetes state.
				logger.Debugf("skipping unit %q because it was not known to the worker", tag.String())
				continue
			}
			if foundUnits.Contains(tag.String()) {
				// Not ready to be deleted.
				logger.Debugf("skipping unit %q because the pod still exists", tag.String())
				continue
			}
			switch deploymentType {
			case caas.DeploymentStateful:
				ordinal := tag.(names.UnitTag).Number()
				if ordinal < args.DesiredReplicas {
					// Don't delete units that will reappear.
					logger.Debugf("skipping unit %q because its still needed", tag.String())
					continue
				}
			case caas.DeploymentStateless, caas.DeploymentDaemon:
				ci, err := unit.ContainerInfo()
				if errors.IsNotFound(err) {
					logger.Debugf("skipping unit %q because it hasn't been assigned a pod", tag.String())
					continue
				} else if err != nil {
					return errors.Trace(err)
				}
				if ci.ProviderId() == "" {
					logger.Debugf("skipping unit %q because it hasn't been assigned a pod", tag.String())
					continue
				}
			default:
				return errors.Errorf("unknown deployment type %q", deploymentType)
			}
		}

		logger.Debugf("deleting unit %q", tag.String())

		// If a unit is removed from the cloud, all filesystems are considered detached.
		unitStorage, err := a.storage.UnitStorageAttachments(tag.(names.UnitTag))
		if err != nil {
			return errors.Trace(err)
		}
		for _, sa := range unitStorage {
			fs, err := a.storage.StorageInstanceFilesystem(sa.StorageInstance())
			if err != nil {
				return errors.Trace(err)
			}
			filesystemStatus[fs.FilesystemTag().String()] = status.StatusInfo{Status: status.Detached}
		}
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
		op.Updates = append(op.Updates, unit.UpdateOperation(updateProps))
		destroyOp := unit.DestroyOperation()
		destroyOp.Force = true
		op.Deletes = append(op.Deletes, destroyOp)
	}
	if len(op.Deletes) == 0 {
		return nil
	}

	if err := a.updateFilesystemInfo(nil, filesystemStatus); err != nil {
		return errors.Annotatef(err, "updating filesystem information for %v", appName)
	}

	return app.UpdateUnits(&op)
}

// CharmStorageParams returns filesystem parameters needed
// to provision storage used for a charm operator or workload.
func CharmStorageParams(
	controllerUUID string,
	storageClassName string,
	modelCfg *config.Config,
	poolName string,
	poolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (*params.KubernetesFilesystemParams, error) {
	// The defaults here are for operator storage.
	// Workload storage will override these elsewhere.
	const size uint64 = 1024
	tags := tags.ResourceTags(
		names.NewModelTag(modelCfg.UUID()),
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

	providerType, attrs, err := poolStorageProvider(poolManager, registry, maybePoolName)
	if err != nil && (!errors.IsNotFound(err) || poolName != "") {
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

func poolStorageProvider(poolManager poolmanager.PoolManager, registry storage.ProviderRegistry, poolName string) (storage.ProviderType, map[string]interface{}, error) {
	pool, err := poolManager.Get(poolName)
	if errors.IsNotFound(err) {
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
	providerType := pool.Provider()
	return providerType, pool.Attrs(), nil
}

func (a *API) applicationFilesystemParams(
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
			a.storagePoolManager, a.registry,
		)
		if err != nil {
			return nil, errors.Annotatef(err, "getting filesystem %q parameters", name)
		}
		for i := 0; i < int(cons.Count); i++ {
			charmStorage := ch.Meta().Storage[name]
			id := fmt.Sprintf("%s/%v", name, i)
			tag := names.NewStorageTag(id)
			location, err := state.FilesystemMountPoint(charmStorage, tag, "kubernetes")
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
	fsParams, err := CharmStorageParams(controllerUUID, storageClassName, modelConfig, cons.Pool, poolManager, registry)
	if err != nil {
		return nil, errors.Maskf(err, "getting filesystem storage parameters")
	}

	fsParams.Size = cons.Size
	fsParams.StorageName = storageName
	fsParams.Tags = filesystemTags
	return fsParams, nil
}

func (a *API) devicesParams(app Application) ([]params.KubernetesDeviceParams, error) {
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

// ApplicationOCIResources returns the OCI image resources for an application.
func (a *API) ApplicationOCIResources(args params.Entities) (params.CAASApplicationOCIResourceResults, error) {
	res := params.CAASApplicationOCIResourceResults{
		Results: make([]params.CAASApplicationOCIResourceResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		appTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		ch, _, err := app.Charm()
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		resources, err := a.newResourceOpener(app.Name())
		if err != nil {
			res.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		imageResources := params.CAASApplicationOCIResources{
			Images: make(map[string]params.DockerImageInfo),
		}
		for _, v := range ch.Meta().Resources {
			if v.Type != charmresource.TypeContainerImage {
				continue
			}
			reader, err := resources.OpenResource(v.Name)
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
		}
		if res.Results[i].Error != nil {
			continue
		}
		res.Results[i].Result = &imageResources
	}
	return res, nil
}

func readDockerImageResource(reader io.Reader) (params.DockerImageInfo, error) {
	var details resources.DockerImageDetails
	contents, err := ioutil.ReadAll(reader)
	if err != nil {
		return params.DockerImageInfo{}, errors.Trace(err)
	}
	if err := json.Unmarshal(contents, &details); err != nil {
		if err := yaml.Unmarshal(contents, &details); err != nil {
			return params.DockerImageInfo{}, errors.Annotate(err, "file neither valid json or yaml")
		}
	}
	if err := resources.ValidateDockerRegistryPath(details.RegistryPath); err != nil {
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
func (a *API) UpdateApplicationsUnits(args params.UpdateApplicationUnitArgs) (params.UpdateApplicationUnitResults, error) {
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
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if app.Life() != state.Alive {
			// We ignore any updates for dying applications.
			logger.Debugf("ignoring unit updates for dying application: %v", app.Name())
			continue
		}
		appStatus := appUpdate.Status
		if appStatus.Status != "" && appStatus.Status != status.Unknown {
			now := a.clock.Now()
			err = app.SetStatus(status.StatusInfo{
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
		appUnitInfo, err := a.updateUnitsFromCloud(app, appUpdate.Units)
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

func (a *API) updateUnitsFromCloud(app Application, unitUpdates []params.ApplicationUnitParams) ([]params.ApplicationUnitInfo, error) {
	logger.Debugf("unit updates: %#v", unitUpdates)

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
	units, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitByTag := make(map[string]Unit)
	for _, v := range units {
		unitByTag[v.Tag().String()] = v
	}
	unitByProviderID := make(map[string]Unit)
	for _, v := range containers {
		tag := names.NewUnitTag(v.Unit())
		unit, ok := unitByTag[tag.String()]
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
			logger.Debugf("updating storage %v for %v", storageName, unitTag)
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

	unitUpdate := state.UpdateUnitsOperation{}
	processedFilesystemIds := set.NewStrings()
	for _, unitParams := range unitUpdates {
		unit, ok := unitByProviderID[unitParams.ProviderId]
		if !ok {
			logger.Warningf("ignoring non-existent unit with provider id %q", unitParams.ProviderId)
			continue
		}

		updateProps := processUnitParams(unitParams)
		unitUpdate.Updates = append(unitUpdate.Updates, unit.UpdateOperation(*updateProps))

		if len(unitParams.FilesystemInfo) > 0 {
			err := processFilesystemParams(processedFilesystemIds, unit.Tag().(names.UnitTag), unitParams)
			if err != nil {
				return nil, errors.Annotatef(err, "processing filesystems for unit %q", unit.Tag())
			}
		}
	}

	err = app.UpdateUnits(&unitUpdate)
	// We ignore any updates for dying applications.
	if stateerrors.IsNotAlive(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// If pods are recreated on the Kubernetes side, new units are created on the Juju
	// side and so any previously attached filesystems become orphaned and need to
	// be cleaned up.
	appName := app.Name()
	if err := a.cleanupOrphanedFilesystems(processedFilesystemIds); err != nil {
		return nil, errors.Annotatef(err, "deleting orphaned filesystems for %v", appName)
	}

	// First do the volume updates as volumes need to be attached before the filesystem updates.
	if err := a.updateVolumeInfo(volumeUpdates, volumeStatus); err != nil {
		return nil, errors.Annotatef(err, "updating volume information for %v", appName)
	}

	if err := a.updateFilesystemInfo(filesystemUpdates, filesystemStatus); err != nil {
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

func (a *API) cleanupOrphanedFilesystems(processedFilesystemIds set.Strings) error {
	// TODO(caas) - record unit id on the filesystem so we can query by unit
	allFilesystems, err := a.storage.AllFilesystems()
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

		si, err := a.storage.StorageInstance(storageTag)
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
		err = a.storage.DestroyStorageInstance(storageTag, false, false, time.Duration(0))
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		err = a.storage.DestroyFilesystem(fs.FilesystemTag(), false)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (a *API) updateVolumeInfo(volumeUpdates map[string]volumeInfo, volumeStatus map[string]status.StatusInfo) error {
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

		vol, err := a.storage.Volume(volTag)
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

	logger.Debugf("updating volume status: %+v", volumeStatus)
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

func (a *API) updateFilesystemInfo(filesystemUpdates map[string]filesystemInfo, filesystemStatus map[string]status.StatusInfo) error {
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

		fs, err := a.storage.Filesystem(fsTag)
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

	logger.Debugf("updating filesystem status: %+v", filesystemStatus)
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

func processUnitParams(unitParams params.ApplicationUnitParams) *state.UnitUpdateProperties {
	agentStatus, cloudContainerStatus := updateStatus(unitParams)
	return &state.UnitUpdateProperties{
		ProviderId:           &unitParams.ProviderId,
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
