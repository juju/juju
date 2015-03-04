// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// StorageAPI provides access to the Storage API facade.
type StorageAPI struct {
	st         storageStateInterface
	resources  *common.Resources
	accessUnit common.GetAuthFunc
}

// newStorageAPI creates a new server-side Storage API facade.
func newStorageAPI(
	st storageStateInterface,
	resources *common.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {

	return &StorageAPI{
		st:         st,
		resources:  resources,
		accessUnit: accessUnit,
	}, nil
}

// UnitStorageAttachments returns the storage attachments for a collection of units.
func (s *StorageAPI) UnitStorageAttachments(args params.Entities) (params.StorageAttachmentsResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.StorageAttachmentsResults{}, err
	}
	result := params.StorageAttachmentsResults{
		Results: make([]params.StorageAttachmentsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		storageAttachments, err := s.getOneUnitStorageAttachments(canAccess, entity.Tag)
		if err == nil {
			result.Results[i].Result = storageAttachments
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneUnitStorageAttachments(canAccess common.AuthFunc, unitTag string) ([]params.StorageAttachment, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil || !canAccess(tag) {
		return nil, common.ErrPerm
	}
	stateStorageAttachments, err := s.st.StorageAttachments(tag)
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
	} else if err != nil {
		return nil, err
	}
	var result []params.StorageAttachment
	for _, stateStorageAttachment := range stateStorageAttachments {
		storageAttachment, err := s.fromStateStorageAttachment(stateStorageAttachment)
		if errors.IsNotProvisioned(err) {
			// don't return unprovisioned storage attachments
			continue
		} else if err != nil {
			return nil, err
		}
		result = append(result, storageAttachment)
	}
	return result, nil
}

func (s *StorageAPI) fromStateStorageAttachment(stateStorageAttachment state.StorageAttachment) (params.StorageAttachment, error) {
	info, err := common.StorageAttachmentInfo(s.st, stateStorageAttachment)
	if err != nil {
		return params.StorageAttachment{}, err
	}
	stateStorageInstance, err := s.st.StorageInstance(stateStorageAttachment.StorageInstance())
	if err != nil {
		return params.StorageAttachment{}, err
	}
	return params.StorageAttachment{
		stateStorageAttachment.StorageInstance().String(),
		stateStorageInstance.Owner().String(),
		stateStorageAttachment.Unit().String(),
		params.StorageKind(stateStorageInstance.Kind()),
		info.Location,
		params.Life(stateStorageAttachment.Life().String()),
	}, nil
}

// StorageAttachments returns the storage attachments with the specified tags.
func (s *StorageAPI) StorageAttachments(args params.StorageAttachmentIds) (params.StorageAttachmentResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.StorageAttachmentResults{}, err
	}
	result := params.StorageAttachmentResults{
		Results: make([]params.StorageAttachmentResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		storageAttachment, err := s.getOneStorageAttachment(canAccess, id)
		if err == nil {
			result.Results[i].Result = storageAttachment
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneStorageAttachment(canAccess common.AuthFunc, id params.StorageAttachmentId) (params.StorageAttachment, error) {
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil || !canAccess(unitTag) {
		return params.StorageAttachment{}, common.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return params.StorageAttachment{}, err
	}
	stateStorageAttachment, err := s.st.StorageAttachment(storageTag, unitTag)
	if errors.IsNotFound(err) {
		return params.StorageAttachment{}, err
	}
	return s.fromStateStorageAttachment(stateStorageAttachment)
}

// WatchUnitStorageAttachments creates watchers for a collection of units,
// each of which can be used to watch for lifecycle changes to the corresponding
// unit's storage attachments.
func (s *StorageAPI) WatchUnitStorageAttachments(args params.Entities) (params.StringsWatchResults, error) {
	canAccess, err := s.accessUnit()
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
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (s *StorageAPI) watchOneUnitStorageAttachments(tag string, canAccess func(names.Tag) bool) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unitTag, err := names.ParseUnitTag(tag)
	if err != nil || !canAccess(unitTag) {
		return nothing, common.ErrPerm
	}
	watch := s.st.WatchStorageAttachments(unitTag)
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: s.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchStorageAttachmentInfos creates watchers for a collection of storage
// attachments, each of which can be used to watch changes to storage
// attachment info.
func (s *StorageAPI) WatchStorageAttachmentInfos(args params.StorageAttachmentIds) (params.NotifyWatchResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		result, err := s.watchOneStorageAttachment(id, canAccess)
		if err == nil {
			results.Results[i] = result
		}
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

func (s *StorageAPI) watchOneStorageAttachment(id params.StorageAttachmentId, canAccess func(names.Tag) bool) (params.NotifyWatchResult, error) {
	// Watching a storage attachment is implemented as watching the
	// underlying volume or filesystem attachment. The only thing
	// we don't necessarily see in doing this is the lifecycle state
	// changes, but these may be observed by using the
	// WatchUnitStorageAttachments watcher.
	nothing := params.NotifyWatchResult{}
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil || !canAccess(unitTag) {
		return nothing, common.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return nothing, err
	}
	watch, err := common.WatchStorageAttachmentInfo(s.st, storageTag, unitTag)
	if err != nil {
		return nothing, errors.Trace(err)
	}
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: s.resources.Register(watch),
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}
