// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/storage"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// StorageAPI provides access to the Storage API facade.
type StorageAPI struct {
	blockDeviceService         BlockDeviceService
	applicationService         ApplicationService
	storageProvisioningService StorageProvisioningService
	watcherRegistry            facade.WatcherRegistry
	accessUnit                 common.GetAuthFunc
}

// newStorageAPI creates a new server-side Storage API facade.
func newStorageAPI(
	blockDeviceService BlockDeviceService,
	applicationService ApplicationService,
	storageProvisioningService StorageProvisioningService,
	watcherRegistry facade.WatcherRegistry,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {

	return &StorageAPI{
		blockDeviceService:         blockDeviceService,
		applicationService:         applicationService,
		storageProvisioningService: storageProvisioningService,
		watcherRegistry:            watcherRegistry,
		accessUnit:                 accessUnit,
	}, nil
}

func (s *StorageAPI) getUnitUUID(
	ctx context.Context, tag names.UnitTag,
) (coreunit.UUID, error) {
	unitUUID, err := s.applicationService.GetUnitUUID(ctx, coreunit.Name(tag.Id()))
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return "", internalerrors.Errorf(
			"invalid unit name for %q", tag.Id(),
		).Add(errors.NotValid)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return "", internalerrors.Errorf(
			"unit %q not found", tag.Id(),
		).Add(errors.NotFound)
	case err != nil:
		return "", internalerrors.Errorf("getting unit UUID for %q: %w", tag.Id(), err)
	}
	return unitUUID, nil
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
	one := func(entity string) ([]params.StorageAttachmentId, error) {
		unitTag, err := names.ParseUnitTag(entity)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		if !canAccess(unitTag) {
			return nil, apiservererrors.ErrPerm
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		sIDs, err := s.storageProvisioningService.GetStorageAttachmentIDsForUnit(ctx, unitUUID)
		switch {
		case errors.Is(err, coreerrors.NotValid):
			return nil, internalerrors.Errorf(
				"invalid unit uuid for %q", unitTag.Id(),
			).Add(errors.NotValid)
		case errors.Is(err, applicationerrors.UnitNotFound):
			return nil, internalerrors.Errorf(
				"unit %q not found", unitTag.Id(),
			).Add(errors.NotFound)
		case err != nil:
			return nil, internalerrors.Errorf(
				"getting storage IDs for unit %q: %w", unitTag.Id(), err,
			)
		}

		storageAttachmentIds := make([]params.StorageAttachmentId, 0, len(sIDs))
		for _, sID := range sIDs {
			if !names.IsValidStorage(sID) {
				// This should never happen. But to avoid a panic, we
				// return an error if we encounter an invalid storage ID.
				return nil, internalerrors.Errorf(
					"invalid storage ID %q for unit %q", sID, unitTag.Id(),
				).Add(errors.NotValid)
			}
			storageAttachmentIds = append(storageAttachmentIds, params.StorageAttachmentId{
				UnitTag:    unitTag.String(),
				StorageTag: names.NewStorageTag(sID).String(),
			})
		}
		return storageAttachmentIds, nil
	}
	for i, entity := range args.Entities {
		storageAttachmentIds, err := one(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = params.StorageAttachmentIds{
			Ids: storageAttachmentIds,
		}
	}
	return result, nil
}

// DestroyUnitStorageAttachments marks each storage attachment of the
// specified units as Dying.
func (s *StorageAPI) DestroyUnitStorageAttachments(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
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
	one := func(arg params.StorageAttachmentId) (params.StorageAttachment, error) {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return params.StorageAttachment{}, internalerrors.Capture(err)
		}
		if !canAccess(unitTag) {
			return params.StorageAttachment{}, apiservererrors.ErrPerm
		}

		storageTag, err := names.ParseStorageTag(arg.StorageTag)
		if err != nil {
			return params.StorageAttachment{}, internalerrors.Capture(err)
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return params.StorageAttachment{}, internalerrors.Capture(err)
		}

		storageAttachmentUUID, err := s.storageProvisioningService.GetStorageAttachmentUUIDForUnit(
			ctx, storageTag.Id(), unitUUID,
		)
		switch {
		case errors.Is(err, storageprovisioningerrors.StorageInstanceNotFound):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"storage instance not found for %q %q",
				storageTag.Id(), unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageAttachmentNotFound):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"storage attachment not found for %q %q",
				storageTag.Id(), unitTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return params.StorageAttachment{}, internalerrors.Errorf(
				"getting storage attachment uuid for %q unit %q: %w",
				storageTag.Id(), arg.UnitTag, err,
			)
		}

		attachmentInfo, err := s.storageProvisioningService.GetUnitStorageAttachmentInfo(
			ctx, storageAttachmentUUID,
		)
		switch {
		case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"filesystem for storage attachment %q unit %q not found",
				arg.StorageTag, unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"filesystem attachment for storage attachment %q unit %q not found",
				arg.StorageTag, unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"volume attachment for storage attachment %q unit %q not found",
				arg.StorageTag, unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageAttachmentNotProvisioned):
			return params.StorageAttachment{}, internalerrors.Errorf(
				"storage attachment %q for unit %q not fully provisioned",
				arg.StorageTag, unitTag.Id(),
			).Add(coreerrors.NotProvisioned)
		case err != nil:
			return params.StorageAttachment{}, internalerrors.Errorf(
				"getting storage attachment info for storage %q unit %q: %w",
				arg.StorageTag, unitTag.Id(), err,
			)
		}

		sa := params.StorageAttachment{
			StorageTag: storageTag.String(),
			UnitTag:    unitTag.String(),
			Location:   attachmentInfo.Location,
		}
		sa.Life, err = attachmentInfo.Life.Value()
		if err != nil {
			return params.StorageAttachment{}, internalerrors.Errorf(
				"invalid life %q for storage attachment %q unit %q: %w",
				attachmentInfo.Life, arg.StorageTag, unitTag.Id(), err,
			)
		}
		if attachmentInfo.Owner != nil {
			sa.OwnerTag = names.NewUnitTag(attachmentInfo.Owner.String()).String()
		}
		switch attachmentInfo.Kind {
		case storage.StorageKindBlock:
			sa.Kind = params.StorageKindBlock
		case storage.StorageKindFilesystem:
			sa.Kind = params.StorageKindFilesystem
		default:
			sa.Kind = params.StorageKindUnknown
		}
		return sa, nil
	}
	for i, arg := range args.Ids {
		sa, err := one(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = sa
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
	one := func(arg params.StorageAttachmentId) (corelife.Value, error) {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}
		if !canAccess(unitTag) {
			return "", apiservererrors.ErrPerm
		}

		storageTag, err := names.ParseStorageTag(arg.StorageTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}
		storageID, err := corestorage.ParseID(storageTag.Id())
		if errors.Is(err, corestorage.InvalidStorageID) {
			return "", internalerrors.Errorf(
				"invalid storage ID %q for unit %q", storageTag.Id(), unitTag.Id(),
			).Add(errors.NotValid)
		} else if err != nil {
			return "", internalerrors.Errorf(
				"parsing storage ID %q for unit %q: %w", storageTag.Id(), unitTag.Id(), err,
			)
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}

		life, err := s.storageProvisioningService.GetStorageAttachmentLife(ctx, unitUUID, storageID.String())
		switch {
		case errors.Is(err, coreerrors.NotValid):
			return "", internalerrors.Errorf(
				"invalid unit UUID %q for %q", unitUUID, unitTag.Id(),
			).Add(errors.NotValid)
		case errors.Is(err, applicationerrors.UnitNotFound):
			return "", internalerrors.Errorf(
				"unit %q not found", unitTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageInstanceNotFound):
			return "", internalerrors.Errorf(
				"storage instance %q not found for unit %q", storageID, unitTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageAttachmentNotFound):
			return "", internalerrors.Errorf(
				"storage attachment %q for unit %q not found", arg.StorageTag, unitTag.Id(),
			).Add(errors.NotFound)
		case err != nil:
			return "", internalerrors.Errorf(
				"getting storage attachment life for storage %q unit %q: %w",
				arg.StorageTag, unitTag.Id(), err,
			)
		}
		return life.Value()
	}
	for i, arg := range args.Ids {
		life, err := one(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Life = life
	}
	return result, nil
}

// WatchUnitStorageAttachments creates watchers for a collection of units,
// each of which can be used to watch for lifecycle changes to the corresponding
// unit's storage attachments.
func (s *StorageAPI) WatchUnitStorageAttachments(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.StringsWatchResults{}, err
	}

	one := func(tag string) (watcher.StringsWatcher, error) {
		unitTag, err := names.ParseUnitTag(tag)
		if err != nil {
			return nil, internalerrors.Errorf("parsing unit tag %q: %w", tag, err)
		}
		if !canAccess(unitTag) {
			return nil, apiservererrors.ErrPerm
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}

		w, err := s.storageProvisioningService.WatchStorageAttachmentsForUnit(ctx, unitUUID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		return w, nil
	}

	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		w, err := one(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if results.Results[i].StringsWatcherId, results.Results[i].Changes, err = internal.EnsureRegisterWatcher(
			ctx, s.watcherRegistry, w,
		); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// WatchStorageAttachments creates watchers for a collection of storage
// attachments, each of which can be used to watch changes to storage
// attachment info.
func (s *StorageAPI) WatchStorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.NotifyWatchResults, error) {
	canAccess, err := s.accessUnit(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, err
	}

	one := func(id params.StorageAttachmentId) (watcher.NotifyWatcher, error) {
		unitTag, err := names.ParseUnitTag(id.UnitTag)
		if err != nil {
			return nil, internalerrors.Errorf("parsing unit tag %q: %w", id.UnitTag, err)
		}
		if !canAccess(unitTag) {
			return nil, apiservererrors.ErrPerm
		}
		storageTag, err := names.ParseStorageTag(id.StorageTag)
		if err != nil {
			return nil, internalerrors.Errorf("parsing storage tag %q: %w", id.StorageTag, err)
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		storageAttachmentUUID, err := s.storageProvisioningService.GetStorageAttachmentUUIDForUnit(ctx, storageTag.Id(), unitUUID)
		switch {
		case errors.Is(err, applicationerrors.UnitNotFound):
			return nil, internalerrors.Errorf(
				"unit %q not found", unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageInstanceNotFound):
			return nil, internalerrors.Errorf(
				"storage instance not found for %q %q", storageTag.Id(), unitTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.StorageAttachmentNotFound):
			return nil, internalerrors.Errorf(
				"storage attachment not found for %q %q", storageTag.Id(), unitTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return nil, internalerrors.Errorf(
				"getting storage attachment uuid for %q unit %q: %w",
				storageTag.Id(), id.UnitTag, err,
			)
		}
		w, err := s.storageProvisioningService.WatchStorageAttachment(ctx, storageAttachmentUUID)
		if err != nil {
			return nil, internalerrors.Errorf(
				"watching storage attachment for %q unit %q: %w",
				storageTag.Id(), id.UnitTag, err,
			)
		}
		return w, nil
	}

	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Ids)),
	}
	for i, id := range args.Ids {
		w, err := one(id)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(
			ctx, s.watcherRegistry, w,
		)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// RemoveStorageAttachments removes the specified storage
// attachments from state.
func (s *StorageAPI) RemoveStorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	return results, nil
}
