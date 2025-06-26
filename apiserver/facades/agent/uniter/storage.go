// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// StorageAPI provides access to the Storage API facade.
type StorageAPI struct {
	storage            storageAccess
	blockDeviceService blockDeviceService
	applicationService ApplicationService
	resources          facade.Resources
	accessUnit         common.GetAuthFunc
}

// newStorageAPI creates a new server-side Storage API facade.
func newStorageAPI(
	storage storageAccess,
	blockDeviceService blockDeviceService,
	applicationService ApplicationService,
	resources facade.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {

	return &StorageAPI{
		storage:            storage,
		blockDeviceService: blockDeviceService,
		applicationService: applicationService,
		resources:          resources,
		accessUnit:         accessUnit,
	}, nil
}

// UnitStorageAttachments returns the IDs of storage attachments for a collection of units.
func (s *StorageAPI) UnitStorageAttachments(ctx context.Context, args params.Entities) (params.StorageAttachmentIdsResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.StorageAttachmentIdsResults{}, err
	}
	result := params.StorageAttachmentIdsResults{
		Results: make([]params.StorageAttachmentIdsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		storageAttachmentIds, err := s.getOneUnitStorageAttachmentIds(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Result = params.StorageAttachmentIds{
				Ids: storageAttachmentIds,
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneUnitStorageAttachmentIds(canAccess common.AuthFunc, unitTag string) ([]params.StorageAttachmentId, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil || !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	stateStorageAttachments, err := s.storage.UnitStorageAttachments(tag)
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, err
	}
	result := make([]params.StorageAttachmentId, len(stateStorageAttachments))
	for i, stateStorageAttachment := range stateStorageAttachments {
		result[i] = params.StorageAttachmentId{
			UnitTag:    unitTag,
			StorageTag: stateStorageAttachment.StorageInstance().String(),
		}
	}
	return result, nil
}

// DestroyUnitStorageAttachments marks each storage attachment of the
// specified units as Dying.
func (s *StorageAPI) DestroyUnitStorageAttachments(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	one := func(tag string) error {
		unitTag, err := names.ParseUnitTag(tag)
		if err != nil {
			return err
		}
		if !canAccess(unitTag) {
			return apiservererrors.ErrPerm
		}
		return s.storage.DestroyUnitStorageAttachments(unitTag)
	}
	for i, entity := range args.Entities {
		err := one(entity.Tag)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// StorageAttachments returns the storage attachments with the specified tags.
func (s *StorageAPI) StorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.StorageAttachmentResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.StorageAttachmentResults{}, err
	}
	result := params.StorageAttachmentResults{
		Results: make([]params.StorageAttachmentResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		storageAttachment, err := s.getOneStorageAttachment(ctx, canAccess, id)
		if err == nil {
			result.Results[i].Result = storageAttachment
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// StorageAttachmentLife returns the lifecycle state of the storage attachments
// with the specified tags.
func (s *StorageAPI) StorageAttachmentLife(ctx context.Context, args params.StorageAttachmentIds) (params.LifeResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.LifeResults{}, err
	}
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		stateStorageAttachment, err := s.getOneStateStorageAttachment(canAccess, id)
		if err == nil {
			life := stateStorageAttachment.Life()
			result.Results[i].Life = life.Value()
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneStorageAttachment(ctx context.Context, canAccess common.AuthFunc, id params.StorageAttachmentId) (params.StorageAttachment, error) {
	stateStorageAttachment, err := s.getOneStateStorageAttachment(canAccess, id)
	if err != nil {
		return params.StorageAttachment{}, err
	}
	return s.fromStateStorageAttachment(ctx, stateStorageAttachment)
}

func (s *StorageAPI) getOneStateStorageAttachment(canAccess common.AuthFunc, id params.StorageAttachmentId) (state.StorageAttachment, error) {
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(unitTag) {
		return nil, apiservererrors.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return nil, err
	}
	return s.storage.StorageAttachment(storageTag, unitTag)
}

func (s *StorageAPI) fromStateStorageAttachment(ctx context.Context, stateStorageAttachment state.StorageAttachment) (params.StorageAttachment, error) {
	// hostTag is the tag either of the unit (this is the case in K8s), or the
	// machine if the unit is assigned.
	var hostTag names.Tag
	hostTag = stateStorageAttachment.Unit()
	// If the unit is not assigned, we keep the hostTag as initially defined
	// (i.e. the unit tag).
	machineName, err := s.applicationService.GetUnitMachineName(ctx, unit.Name(hostTag.Id()))
	if err != nil && !errors.Is(err, applicationerrors.UnitMachineNotAssigned) {
		return params.StorageAttachment{}, err
	} else if err == nil {
		hostTag = names.NewMachineTag(machineName.String())
	}

	info, err := storagecommon.StorageAttachmentInfo(ctx,
		s.storage, s.storage.VolumeAccess(), s.storage.FilesystemAccess(), s.blockDeviceService, stateStorageAttachment, hostTag)
	if err != nil {
		return params.StorageAttachment{}, err
	}
	stateStorageInstance, err := s.storage.StorageInstance(stateStorageAttachment.StorageInstance())
	if err != nil {
		return params.StorageAttachment{}, err
	}
	var ownerTag string
	if owner, ok := stateStorageInstance.Owner(); ok {
		ownerTag = owner.String()
	}
	return params.StorageAttachment{
		StorageTag: stateStorageAttachment.StorageInstance().String(),
		OwnerTag:   ownerTag,
		UnitTag:    stateStorageAttachment.Unit().String(),
		Kind:       params.StorageKind(stateStorageInstance.Kind()),
		Location:   info.Location,
		Life:       life.Value(stateStorageAttachment.Life().String()),
	}, nil
}

// WatchUnitStorageAttachments creates watchers for a collection of units,
// each of which can be used to watch for lifecycle changes to the corresponding
// unit's storage attachments.
func (s *StorageAPI) WatchUnitStorageAttachments(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result, err := s.watchOneUnitStorageAttachments(entity.Tag, canAccess)
		if err == nil {
			results.Results[i] = result
		}
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageAPI) watchOneUnitStorageAttachments(tag string, canAccess func(names.Tag) bool) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unitTag, err := names.ParseUnitTag(tag)
	if err != nil || !canAccess(unitTag) {
		return nothing, apiservererrors.ErrPerm
	}
	watch := s.storage.WatchStorageAttachments(unitTag)
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: s.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchStorageAttachments creates watchers for a collection of storage
// attachments, each of which can be used to watch changes to storage
// attachment info.
func (s *StorageAPI) WatchStorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.NotifyWatchResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		result, err := s.watchOneStorageAttachment(ctx, id, canAccess)
		if err == nil {
			results.Results[i] = result
		}
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageAPI) watchOneStorageAttachment(ctx context.Context, id params.StorageAttachmentId, canAccess func(names.Tag) bool) (params.NotifyWatchResult, error) {
	// Watching a storage attachment is implemented as watching the
	// underlying volume or filesystem attachment. The only thing
	// we don't necessarily see in doing this is the lifecycle state
	// changes, but these may be observed by using the
	// WatchUnitStorageAttachments watcher.
	nothing := params.NotifyWatchResult{}
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil || !canAccess(unitTag) {
		return nothing, apiservererrors.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return nothing, err
	}

	// hostTag is the tag either of the unit (this is the case in K8s), or the
	// machine if the unit is assigned.
	var hostTag names.Tag
	hostTag = unitTag
	// If the unit is not assigned, we keep the hostTag as initially defined
	// (i.e. the unit tag).
	machineName, err := s.applicationService.GetUnitMachineName(ctx, unit.Name(hostTag.Id()))
	if err != nil && !errors.Is(err, applicationerrors.UnitMachineNotAssigned) {
		return params.NotifyWatchResult{}, err
	} else if err == nil {
		hostTag = names.NewMachineTag(machineName.String())
	}

	watch, err := watchStorageAttachment(ctx,
		s.storage, s.storage.VolumeAccess(), s.storage.FilesystemAccess(), s.blockDeviceService, storageTag, hostTag, unitTag)
	if err != nil {
		return nothing, errors.Trace(err)
	}

	if _, err := internal.FirstResult[struct{}](ctx, watch); err != nil {
		return nothing, errors.Trace(err)
	}

	return params.NotifyWatchResult{
		NotifyWatcherId: s.resources.Register(watch),
	}, nil
}

// RemoveStorageAttachments removes the specified storage
// attachments from state.
func (s *StorageAPI) RemoveStorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		err := s.removeOneStorageAttachment(id, canAccess)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

func (s *StorageAPI) removeOneStorageAttachment(id params.StorageAttachmentId, canAccess func(names.Tag) bool) error {
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil {
		return err
	}
	if !canAccess(unitTag) {
		return apiservererrors.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return err
	}
	// TODO (anastasiamac 2019-04-04) We can now force storage removal
	// but for now, while we have not an arg passed in, just hardcode.
	err = s.storage.RemoveStorageAttachment(storageTag, unitTag, false)
	if errors.Is(err, errors.NotFound) {
		err = nil
	}
	return err
}

// addStorageToOneUnitOperation returns a ModelOperation for adding storage to
// the specified unit.
func (s *StorageAPI) addStorageToOneUnitOperation(unitTag names.UnitTag, addParams params.StorageAddParams, curCons map[string]state.StorageConstraints) (state.ModelOperation, error) {
	validCons, err := validConstraints(addParams, curCons)
	if err != nil {
		return nil, errors.Annotatef(err, "adding storage %v for %v", addParams.StorageName, addParams.UnitTag)
	}

	modelOp, err := s.storage.AddStorageForUnitOperation(unitTag, addParams.StorageName, validCons)
	if err != nil {
		return nil, errors.Annotatef(err, "adding storage %v for %v", addParams.StorageName, addParams.UnitTag)
	}

	return modelOp, nil
}

func validConstraints(
	p params.StorageAddParams,
	cons map[string]state.StorageConstraints,
) (state.StorageConstraints, error) {
	emptyCons := state.StorageConstraints{}

	result, ok := cons[p.StorageName]
	if !ok {
		return emptyCons, errors.NotFoundf("storage %q", p.StorageName)
	}

	onlyCount := params.StorageDirectives{Count: p.Directives.Count}
	if p.Directives != onlyCount {
		return emptyCons, errors.New("only count can be specified")
	}

	if p.Directives.Count == nil || *p.Directives.Count == 0 {
		return emptyCons, errors.New("count must be specified")
	}

	result.Count = *p.Directives.Count
	return result, nil
}

// watchStorageAttachment returns a state.NotifyWatcher that reacts to changes
// to the VolumeAttachmentInfo or FilesystemAttachmentInfo corresponding to the
// tags specified.
func watchStorageAttachment(
	ctx context.Context,
	st storageInterface,
	stVolume storageVolumeInterface,
	stFile storageFilesystemInterface,
	blockDeviceService blockDeviceService,
	storageTag names.StorageTag,
	hostTag names.Tag,
	unitTag names.UnitTag,
) (eventsource.Watcher[struct{}], error) {
	storageInstance, err := st.StorageInstance(storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	var watchers []eventsource.Watcher[struct{}]
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		if stVolume == nil {
			return nil, errors.NotImplementedf("BlockStorage instance")
		}
		volume, err := stVolume.StorageInstanceVolume(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage volume")
		}
		// We need to watch both the volume attachment, and the
		// machine's block devices. A volume attachment's block
		// device could change (most likely, become present).
		watchers = []eventsource.Watcher[struct{}]{
			stVolume.WatchVolumeAttachment(hostTag, volume.VolumeTag()),
		}

		// TODO(caas) - we currently only support block devices on machines.
		if hostTag.Kind() == names.MachineTagKind {
			// TODO(axw) 2015-09-30 #1501203
			// We should filter the events to only those relevant
			// to the volume attachment. This means we would need
			// to either start th block device watcher after we
			// have provisioned the volume attachment (cleaner?),
			// or have the filter ignore changes until the volume
			// attachment is provisioned.
			blockDeviceWatcher, err := blockDeviceService.WatchBlockDevices(ctx, hostTag.(names.MachineTag).Id())
			if err != nil {
				return nil, errors.Annotate(err, "getting block device watcher")
			}
			watchers = append(watchers, &watcherAdaptor{blockDeviceWatcher})
		}
	case state.StorageKindFilesystem:
		if stFile == nil {
			return nil, errors.NotImplementedf("FilesystemStorage instance")
		}
		filesystem, err := stFile.StorageInstanceFilesystem(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage filesystem")
		}
		watchers = []eventsource.Watcher[struct{}]{
			stFile.WatchFilesystemAttachment(hostTag, filesystem.FilesystemTag()),
		}
	default:
		return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
	}
	watchers = append(watchers, st.WatchStorageAttachment(storageTag, unitTag))
	return eventsource.NewMultiNotifyWatcher(ctx, watchers...)
}

// watcherAdaptor adapts a core watcher to a state watcher.
// TODO(wallyworld) - this will be obsolete once storage is implemented in dqlite.
type watcherAdaptor struct {
	corewatcher.NotifyWatcher
}

func (w *watcherAdaptor) Changes() <-chan struct{} {
	return w.NotifyWatcher.Changes()
}

func (w *watcherAdaptor) Stop() error {
	w.NotifyWatcher.Kill()
	return nil
}

func (w *watcherAdaptor) Err() error {
	return w.NotifyWatcher.Wait()
}
