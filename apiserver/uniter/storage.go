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

// UnitStorageAttachments returns the IDs of storage attachments for a collection of units.
func (s *StorageAPI) UnitStorageAttachments(args params.Entities) (params.StorageAttachmentIdsResults, error) {
	canAccess, err := s.accessUnit()
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
				storageAttachmentIds,
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneUnitStorageAttachmentIds(canAccess common.AuthFunc, unitTag string) ([]params.StorageAttachmentId, error) {
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil || !canAccess(tag) {
		return nil, common.ErrPerm
	}
	stateStorageAttachments, err := s.st.UnitStorageAttachments(tag)
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
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
func (s *StorageAPI) DestroyUnitStorageAttachments(args params.Entities) (params.ErrorResults, error) {
	canAccess, err := s.accessUnit()
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
			return common.ErrPerm
		}
		return s.st.DestroyUnitStorageAttachments(unitTag)
	}
	for i, entity := range args.Entities {
		err := one(entity.Tag)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
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

// StorageAttachmentLife returns the lifecycle state of the storage attachments
// with the specified tags.
func (s *StorageAPI) StorageAttachmentLife(args params.StorageAttachmentIds) (params.LifeResults, error) {
	canAccess, err := s.accessUnit()
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
			result.Results[i].Life = params.Life(life.String())
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (s *StorageAPI) getOneStorageAttachment(canAccess common.AuthFunc, id params.StorageAttachmentId) (params.StorageAttachment, error) {
	stateStorageAttachment, err := s.getOneStateStorageAttachment(canAccess, id)
	if err != nil {
		return params.StorageAttachment{}, err
	}
	return s.fromStateStorageAttachment(stateStorageAttachment)
}

func (s *StorageAPI) getOneStateStorageAttachment(canAccess common.AuthFunc, id params.StorageAttachmentId) (state.StorageAttachment, error) {
	unitTag, err := names.ParseUnitTag(id.UnitTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(unitTag) {
		return nil, common.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return nil, err
	}
	return s.st.StorageAttachment(storageTag, unitTag)
}

func (s *StorageAPI) fromStateStorageAttachment(stateStorageAttachment state.StorageAttachment) (params.StorageAttachment, error) {
	machineTag, err := s.st.UnitAssignedMachine(stateStorageAttachment.Unit())
	if err != nil {
		return params.StorageAttachment{}, err
	}
	info, err := common.StorageAttachmentInfo(s.st, stateStorageAttachment, machineTag)
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

// WatchStorageAttachments creates watchers for a collection of storage
// attachments, each of which can be used to watch changes to storage
// attachment info.
func (s *StorageAPI) WatchStorageAttachments(args params.StorageAttachmentIds) (params.NotifyWatchResults, error) {
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
	machineTag, err := s.st.UnitAssignedMachine(unitTag)
	if err != nil {
		return nothing, err
	}
	watch, err := common.WatchStorageAttachment(s.st, storageTag, machineTag, unitTag)
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

// RemoveStorageAttachments removes the specified storage
// attachments from state.
func (s *StorageAPI) RemoveStorageAttachments(args params.StorageAttachmentIds) (params.ErrorResults, error) {
	canAccess, err := s.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		err := s.removeOneStorageAttachment(id, canAccess)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
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
		return common.ErrPerm
	}
	storageTag, err := names.ParseStorageTag(id.StorageTag)
	if err != nil {
		return err
	}
	return s.st.RemoveStorageAttachment(storageTag, unitTag)
}

// AddUnitStorage validates and creates additional storage instances for units.
// Failures on an individual storage instance do not block remaining
// instances from being processed.
func (a *StorageAPI) AddUnitStorage(
	args params.StoragesAddParams,
) (params.ErrorResults, error) {
	canAccess, err := a.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	if len(args.Storages) == 0 {
		return params.ErrorResults{}, nil
	}

	serverErr := func(err error) params.ErrorResult {
		return params.ErrorResult{common.ServerError(err)}
	}

	storageErr := func(err error, s, u string) params.ErrorResult {
		return serverErr(errors.Annotatef(err, "adding storage %v for %v", s, u))
	}

	result := make([]params.ErrorResult, len(args.Storages))
	for i, one := range args.Storages {
		u, err := accessUnitTag(one.UnitTag, canAccess)
		if err != nil {
			result[i] = serverErr(err)
			continue
		}

		cons, err := a.st.UnitStorageConstraints(u)
		if err != nil {
			result[i] = serverErr(err)
			continue
		}

		oneCons, err := validConstraints(one, cons)
		if err != nil {
			result[i] = storageErr(err, one.StorageName, one.UnitTag)
			continue
		}

		err = a.st.AddStorageForUnit(u, one.StorageName, oneCons)
		if err != nil {
			result[i] = storageErr(err, one.StorageName, one.UnitTag)
		}
	}
	return params.ErrorResults{Results: result}, nil
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

	onlyCount := params.StorageConstraints{Count: p.Constraints.Count}
	if p.Constraints != onlyCount {
		return emptyCons, errors.New("only count can be specified")
	}

	if p.Constraints.Count == nil || *p.Constraints.Count == 0 {
		return emptyCons, errors.New("count must be specified")
	}

	result.Count = *p.Constraints.Count
	return result, nil
}

func accessUnitTag(tag string, canAccess func(names.Tag) bool) (names.UnitTag, error) {
	u, err := names.ParseUnitTag(tag)
	if err != nil {
		return names.UnitTag{}, errors.Annotatef(err, "parsing unit tag %v", tag)
	}
	if !canAccess(u) {
		return names.UnitTag{}, common.ErrPerm
	}
	return u, nil
}
