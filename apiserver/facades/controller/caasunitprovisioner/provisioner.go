// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

var logger = loggo.GetLogger("juju.apiserver.controller.caasunitprovisioner")

type Facade struct {
	*common.LifeGetter
	resources               facade.Resources
	state                   CAASUnitProvisionerState
	storage                 StorageBackend
	storageProviderRegistry storage.ProviderRegistry
	storagePoolManager      poolmanager.PoolManager
	clock                   clock.Clock
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	sb, err := state.NewStorageBackend(ctx.State())
	if err != nil {
		return nil, errors.Trace(err)
	}

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(ctx.State())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(ctx.State()), registry)

	return NewFacade(
		resources,
		authorizer,
		stateShim{ctx.State()},
		sb,
		registry,
		pm,
		clock.WallClock,
	)
}

// NewFacade returns a new CAAS unit provisioner Facade facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASUnitProvisionerState,
	sb StorageBackend,
	storageProviderRegistry storage.ProviderRegistry,
	storagePoolManager poolmanager.PoolManager,
	clock clock.Clock,
) (*Facade, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &Facade{
		LifeGetter: common.NewLifeGetter(
			st, common.AuthAny(
				common.AuthFuncForTagKind(names.ApplicationTagKind),
				common.AuthFuncForTagKind(names.UnitTagKind),
			),
		),
		resources:               resources,
		state:                   st,
		storage:                 sb,
		storageProviderRegistry: storageProviderRegistry,
		storagePoolManager:      storagePoolManager,
		clock:                   clock,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
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

// WatchUnits starts a StringsWatcher to watch changes to the
// lifecycle states of units for the specified applications in
// this model.
func (f *Facade) WatchUnits(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		id, changes, err := f.watchUnits(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = id
		results.Results[i].Changes = changes
	}
	return results, nil
}

func (f *Facade) watchUnits(tagString string) (string, []string, error) {
	tag, err := names.ParseApplicationTag(tagString)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	app, err := f.state.Application(tag.Id())
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	w := app.WatchUnits()
	if changes, ok := <-w.Changes(); ok {
		return f.resources.Register(w), changes, nil
	}
	return "", nil, watcher.EnsureErr(w)
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
			results.Results[i].Error = common.ServerError(err)
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
			results.Results[i].Error = common.ServerError(err)
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

	// Now get any required storage. We need to provision storage
	// at the same time as the pod as it can't be attached later.

	// All units are currently homogeneous so we just
	// need to get info for the first unit.
	app, err := f.state.Application(appTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Should never happen, but just in case.
	if len(units) == 0 {
		return nil, errors.Errorf("cannot provision application %q with no units", appTag.Id())
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	filesystemParams, err := f.applicationFilesystemParams(modelConfig, units[0].UnitTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The juju-storage-owner tag is set to the unit. We use it as a label on the CAAS volume.
	// Since we used an arbitrary unit to get the info, reset the tag to the application.
	for _, fsp := range filesystemParams {
		fsp.Tags[tags.JujuStorageOwner] = appTag.Id()
	}

	return &params.KubernetesProvisioningInfo{
		PodSpec:     podSpec,
		Filesystems: filesystemParams,
	}, nil
}

func filesystemParams(
	f state.Filesystem,
	storageInstance state.StorageInstance,
	modelUUID, controllerUUID string,
	modelConfig *config.Config,
	poolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (params.KubernetesFilesystemParams, error) {

	var pool string
	var size uint64
	if stateFilesystemParams, ok := f.Params(); ok {
		pool = stateFilesystemParams.Pool
		size = stateFilesystemParams.Size
	} else {
		filesystemInfo, err := f.Info()
		if err != nil {
			return params.KubernetesFilesystemParams{}, errors.Trace(err)
		}
		pool = filesystemInfo.Pool
		size = filesystemInfo.Size
	}

	filesystemTags, err := storagecommon.StorageTags(storageInstance, modelUUID, controllerUUID, modelConfig)
	if err != nil {
		return params.KubernetesFilesystemParams{}, errors.Annotate(err, "computing storage tags")
	}

	providerType, cfg, err := storagecommon.StoragePoolConfig(pool, poolManager, registry)
	if err != nil {
		return params.KubernetesFilesystemParams{}, errors.Trace(err)
	}
	result := params.KubernetesFilesystemParams{
		Provider:    string(providerType),
		Attributes:  cfg.Attrs(),
		Tags:        filesystemTags,
		Size:        size,
		StorageName: storageInstance.StorageName(),
	}
	return result, nil
}

// applicationFilesystemParams retrieves FilesystemParams for the filesystems
// that should be provisioned with, and attached to, pods of the application.
func (f *Facade) applicationFilesystemParams(
	modelConfig *config.Config,
	unitTag names.UnitTag,
) ([]params.KubernetesFilesystemParams, error) {
	attachments, err := f.storage.UnitStorageAttachments(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(attachments) == 0 {
		return nil, nil
	}

	controllerCfg, err := f.state.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allFilesystemParams := make([]params.KubernetesFilesystemParams, 0, len(attachments))
	for _, attachment := range attachments {
		si, err := f.storage.StorageInstance(attachment.StorageInstance())
		if err != nil {
			return nil, errors.Trace(err)
		}
		fs, err := f.storage.StorageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		filesystemParams, err := filesystemParams(
			fs, si, modelConfig.UUID(), controllerCfg.ControllerUUID(),
			modelConfig, f.storagePoolManager, f.storageProviderRegistry,
		)
		if err != nil {
			return nil, errors.Annotatef(err, "getting filesystem %q parameters", fs.Tag().Id())
		}
		filesystemAttachment, err := f.storage.FilesystemAttachment(unitTag, fs.FilesystemTag())
		if err != nil {
			return nil, errors.Annotatef(err, "getting filesystem %q attachment info", fs.Tag().Id())
		}
		var location string
		var readOnly bool
		if filesystemAttachmentParams, ok := filesystemAttachment.Params(); ok {
			location = filesystemAttachmentParams.Location
			readOnly = filesystemAttachmentParams.ReadOnly
		} else {
			// All units are the same so even if the attachment exists
			// for the unit used to gather info, we still need to read
			// the relevant attachment params for the application as a whole.
			filesystemAttachmentInfo, err := filesystemAttachment.Info()
			if err != nil {
				return nil, errors.Trace(err)
			}
			location = filesystemAttachmentInfo.MountPoint
			readOnly = filesystemAttachmentInfo.ReadOnly
		}
		filesystemAttachmentParams := params.KubernetesFilesystemAttachmentParams{
			Provider:   filesystemParams.Provider,
			MountPoint: location,
			ReadOnly:   readOnly,
		}
		filesystemParams.Attachment = &filesystemAttachmentParams
		allFilesystemParams = append(allFilesystemParams, filesystemParams)
	}
	return allFilesystemParams, nil
}

// ApplicationsConfig returns the config for the specified applications.
func (f *Facade) ApplicationsConfig(args params.Entities) (params.ApplicationGetConfigResults, error) {
	results := params.ApplicationGetConfigResults{
		Results: make([]params.ConfigResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		result, err := f.getApplicationConfig(arg.Tag)
		results.Results[i].Config = result
		results.Results[i].Error = common.ServerError(err)
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
func (a *Facade) UpdateApplicationsUnits(args params.UpdateApplicationUnitArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = a.updateUnitsFromCloud(app, appUpdate.Units)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// updateStatus constructs the unit and agent status values based on the pod status.
func (a *Facade) updateStatus(params params.ApplicationUnitParams) (
	agentStatus *status.StatusInfo,
	unitStatus *status.StatusInfo,
	_ error,
) {
	switch status.Status(params.Status) {
	case status.Unknown:
		// The container runtime can spam us with unimportant
		// status updates, so ignore any irrelevant ones.
		return nil, nil, nil
	case status.Allocating:
		// The container runtime has decided to restart the pod.
		agentStatus = &status.StatusInfo{
			Status: status.Allocating,
			//Message: params.Info,
		}
		unitStatus = &status.StatusInfo{
			Status:  status.Waiting,
			Message: status.MessageWaitForContainer,
		}
	case status.Running:
		// A pod has finished starting so the workload is now active.
		agentStatus = &status.StatusInfo{
			Status: status.Idle,
		}
		unitStatus = &status.StatusInfo{
			Status:  status.Active,
			Message: params.Info,
		}
	case status.Error:
		agentStatus = &status.StatusInfo{
			Status:  status.Error,
			Message: params.Info,
			Data:    params.Data,
		}
	}
	return agentStatus, unitStatus, nil
}

// updateUnitsFromCloud takes a slice of unit information provided by an external
// source (typically a cloud update event) and merges that with the existing unit
// data model in state. The passed in units are the complete set for the cloud, so
// any existing units in state with provider ids which aren't in the set will be removed.
func (a *Facade) updateUnitsFromCloud(app Application, unitUpdates []params.ApplicationUnitParams) error {
	logger.Debugf("unit updates: %#v", unitUpdates)
	// Set up the initial data structures.
	existingStateUnits, err := app.AllUnits()
	if err != nil {
		return errors.Trace(err)
	}

	stateUnitsById := make(map[string]Unit)
	cloudUnitsById := make(map[string]params.ApplicationUnitParams)

	// Record all unit provider ids known to exist in the cloud.
	for _, u := range unitUpdates {
		cloudUnitsById[u.ProviderId] = u
	}

	stateUnitExistsInCloud := func(providerId string) bool {
		if providerId == "" {
			return false
		}
		_, ok := cloudUnitsById[providerId]
		return ok
	}

	unitInfo := &updateStateUnitParams{
		stateUnitsInCloud: make(map[string]Unit),
		deletedRemoved:    false,
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
			return errors.Trace(err)
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
		stateUnitsById[providerId] = u
		stateUnitInCloud := stateUnitExistsInCloud(providerId)
		aliveStateIds.Add(providerId)

		if stateUnitInCloud {
			logger.Debugf("unit %q (%v) has changed in the cloud", u.Name(), providerId)
			unitInfo.stateUnitsInCloud[u.UnitTag().String()] = u
		} else {
			extraStateIds.Add(providerId)
		}
	}

	// Do it in sorted order so it's deterministic for tests.
	var ids []string
	for id := range cloudUnitsById {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Sort extra ids also to guarantee order.
	var extraIds []string
	for id := range extraStateIds {
		extraIds = append(extraIds, id)
	}
	sort.Strings(extraIds)
	extraIdIndex := 0
	unassociatedUnitCount := len(unitInfo.unassociatedUnits)

	for _, id := range ids {
		u := cloudUnitsById[id]
		if aliveStateIds.Contains(id) {
			u.UnitTag = stateUnitsById[id].UnitTag().String()
			unitInfo.existingCloudUnits = append(unitInfo.existingCloudUnits, u)
			continue
		}

		// First attempt to add any new cloud pod not yet represented in state
		// to a unit which does not yet have a provider id.
		if unassociatedUnitCount > 0 {
			unassociatedUnitCount -= 1
			unitInfo.addedCloudUnits = append(unitInfo.addedCloudUnits, u)
			continue
		}

		// If there are units in state which used to be be associated with a pod
		// but are now not, we give those state units a pod which is not
		// associated with any unit. The pod may have been added new due to a scale
		// out operation, or the pod's state unit was deleted but the cloud removed
		// a different pod in response and so we need to re-associated this still
		// alive pod with the orphaned unit.
		if !extraStateIds.IsEmpty() {
			extraId := extraIds[extraIdIndex]
			extraIdIndex += 1
			extraStateIds.Remove(extraId)
			u.ProviderId = id
			unit := stateUnitsById[extraId]
			u.UnitTag = unit.UnitTag().String()
			unitInfo.existingCloudUnits = append(unitInfo.existingCloudUnits, u)
			unitInfo.stateUnitsInCloud[u.UnitTag] = unit
			continue
		}

		// A new pod was added to the cloud but does not yet have a unit in state.
		unitInfo.addedCloudUnits = append(unitInfo.addedCloudUnits, u)
	}

	// If there are any extra provider ids left over after allocating all the cloud pods,
	// then consider those state units as terminated.
	for _, providerId := range extraStateIds.Values() {
		u := stateUnitsById[providerId]
		logger.Debugf("unit %q (%v) has been removed from the cloud", u.Name(), providerId)
		unitInfo.removedUnits = append(unitInfo.removedUnits, u)
	}

	return a.updateStateUnits(app, unitInfo)
}

type updateStateUnitParams struct {
	stateUnitsInCloud  map[string]Unit
	addedCloudUnits    []params.ApplicationUnitParams
	existingCloudUnits []params.ApplicationUnitParams
	removedUnits       []Unit
	unassociatedUnits  []Unit
	deletedRemoved     bool
}

type filesystemInfo struct {
	unitTag      names.UnitTag
	providerId   string
	mountPoint   string
	readOnly     bool
	size         uint64
	filesystemId string
}

func (a *Facade) updateStateUnits(app Application, unitInfo *updateStateUnitParams) error {

	if app.Life() != state.Alive {
		// We ignore any updates for dying applications.
		logger.Debugf("ignoring unit updates for dying application: %v", app.Name())
		return nil
	}

	logger.Tracef("added cloud units: %+v", unitInfo.addedCloudUnits)
	logger.Tracef("existing cloud units: %+v", unitInfo.existingCloudUnits)
	logger.Tracef("removed units: %+v", unitInfo.removedUnits)
	logger.Tracef("unassociated units: %+v", unitInfo.unassociatedUnits)

	// Now we have the added, removed, updated units all sorted,
	// generate the state update operations.
	var unitUpdate state.UpdateUnitsOperation

	filesystemUpdates := make(map[string]filesystemInfo)
	filesystemStatus := make(map[string]status.StatusInfo)

	for _, u := range unitInfo.removedUnits {
		// If a unit is removed from the cloud, all filesystems are considered detached.
		unitStorage, err := a.storage.UnitStorageAttachments(u.UnitTag())
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

		if unitInfo.deletedRemoved {
			unitUpdate.Deletes = append(unitUpdate.Deletes, u.DestroyOperation())
			continue
		}
		// We'll set the status as Terminated. This will either be transient, as will
		// occur when a pod is restarted external to Juju, or permanent if the pod has
		// been deleted external to Juju. In the latter case, juju remove-unit will be
		// need to clean things up on the Juju side.
		unitStatus := &status.StatusInfo{
			Status:  status.Terminated,
			Message: "unit stopped by the cloud",
		}
		agentStatus := &status.StatusInfo{
			Status: status.Idle,
		}
		// And we'll reset the provider id - the pod may be restarted and we'll
		// record the new id next time.
		resetId := ""
		updateProps := state.UnitUpdateProperties{
			ProviderId:  &resetId,
			UnitStatus:  unitStatus,
			AgentStatus: agentStatus,
		}
		unitUpdate.Updates = append(unitUpdate.Updates,
			u.UpdateOperation(updateProps))
	}

	processUnitParams := func(unitParams params.ApplicationUnitParams) (*state.UnitUpdateProperties, error) {
		agentStatus, unitStatus, err := a.updateStatus(unitParams)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &state.UnitUpdateProperties{
			ProviderId:  &unitParams.ProviderId,
			Address:     &unitParams.Address,
			Ports:       &unitParams.Ports,
			AgentStatus: agentStatus,
			UnitStatus:  unitStatus,
		}, nil
	}

	processFilesystemParams := func(unitTag names.UnitTag, unitParams params.ApplicationUnitParams) error {
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

			// Loop over all the storage for the unit ans skip storage not
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
				filesystemUpdates[fs.FilesystemTag().String()] = filesystemInfo{
					unitTag:      unitTag,
					providerId:   unitParams.ProviderId,
					mountPoint:   fsInfo.MountPoint,
					readOnly:     fsInfo.ReadOnly,
					size:         fsInfo.Size,
					filesystemId: fsInfo.FilesystemId,
				}
				filesystemStatus[fs.FilesystemTag().String()] = status.StatusInfo{Status: status.Attached}
				infos = infos[1:]
				if len(infos) == 0 {
					break
				}
			}
		}
		return nil
	}

	var unitParamsWithFilesystemInfo []params.ApplicationUnitParams

	for _, unitParams := range unitInfo.existingCloudUnits {
		u, ok := unitInfo.stateUnitsInCloud[unitParams.UnitTag]
		if !ok {
			logger.Warningf("unexpected unit parameters %+v not in state", unitParams)
			continue
		}
		updateProps, err := processUnitParams(unitParams)
		if err != nil {
			return errors.Trace(err)
		}
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
	for _, unitParams := range unitInfo.addedCloudUnits {
		if idx < len(unitInfo.unassociatedUnits) {
			u := unitInfo.unassociatedUnits[idx]
			updateProps, err := processUnitParams(unitParams)
			if err != nil {
				return errors.Trace(err)
			}
			unitUpdate.Updates = append(unitUpdate.Updates,
				u.UpdateOperation(*updateProps))
			idx += 1
			if len(unitParams.FilesystemInfo) > 0 {
				unitParamsWithFilesystemInfo = append(unitParamsWithFilesystemInfo, unitParams)
			}
			continue
		}

		// TODO(caas) - attempting 2 way sync has unintended consequences on some deployments
		// For now disable.
		//// Process units added directly in the cloud instead of via Juju.
		//updateProps, err := processUnitParams(unitParams)
		//if err != nil {
		//	return errors.Trace(err)
		//}
		//if len(unitParams.FilesystemInfo) > 0 {
		//	unitParamsWithFilesystemInfo = append(unitParamsWithFilesystemInfo, unitParams)
		//}
		//unitUpdate.Adds = append(unitUpdate.Adds,
		//	app.AddOperation(*updateProps))
	}
	err := app.UpdateUnits(&unitUpdate)
	// We ignore any updates for dying applications.
	if state.IsNotAlive(err) {
		return nil
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
	m, err := a.state.Model()
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
		if err := processFilesystemParams(unitTag, unitParams); err != nil {
			return errors.Annotatef(err, "processing filesystem info for unit %q", unitTag.Id())
		}
	}

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
		if err == nil {
			// Provisioning info has already been set.
			continue
		}
		err = a.storage.SetFilesystemInfo(fsTag, state.FilesystemInfo{
			Size:         fsData.size,
			FilesystemId: fsData.filesystemId,
		})
		if err != nil {
			return errors.Trace(err)
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
		fs.SetStatus(status.StatusInfo{
			Status: fsStatus.Status,
			Since:  &now,
		})
	}

	return err
}

// UpdateApplicationsService updates the Juju data model to reflect the given
// service details of the specified application.
func (a *Facade) UpdateApplicationsService(args params.UpdateApplicationServiceArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if len(args.Args) == 0 {
		return result, nil
	}
	for i, appUpdate := range args.Args {
		appTag, err := names.ParseApplicationTag(appUpdate.ApplicationTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := a.state.Application(appTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := app.UpdateCloudService(appUpdate.ProviderId, params.NetworkAddresses(appUpdate.Addresses...)); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}
