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
	unitName, err := coreunit.NewName(tag.Id())
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", internalerrors.Errorf(
			"invalid unit name %q", tag.Id(),
		).Add(errors.NotValid)
	} else if err != nil {
		return "", internalerrors.Capture(err)
	}

	unitUUID, err := s.applicationService.GetUnitUUID(ctx, unitName)
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return "", internalerrors.Errorf(
			"invalid unit name %q", unitName,
		).Add(errors.NotValid)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return "", internalerrors.Errorf(
			"unit %q not found", unitName,
		).Add(errors.NotFound)
	case err != nil:
		return "", internalerrors.Errorf("getting unit %q UUID: %w", unitName, err)
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
		sIDs, err := s.storageProvisioningService.GetStorageIDsForUnit(
			ctx, unitUUID,
		)
		switch {
		case errors.Is(err, coreunit.InvalidUnitName):
			return nil, internalerrors.Errorf(
				"invalid unit name %q", unitTag.Id(),
			).Add(errors.NotValid)
		case errors.Is(err, applicationerrors.UnitNotFound):
			return nil, internalerrors.Errorf(
				"unit %q not found", unitTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, corestorage.InvalidStorageID):
			return nil, internalerrors.Errorf(
				"invalid storage ID for unit %q", unitTag.Id(),
			).Add(errors.NotValid)
		case err != nil:
			return nil, internalerrors.Errorf(
				"getting storage IDs for unit %q: %w", unitTag.Id(), err,
			)
		}

		var storageAttachmentIds []params.StorageAttachmentId
		for _, sID := range sIDs {
			if !names.IsValidStorage(sID.String()) {
				return nil, internalerrors.Errorf(
					"invalid storage ID %q for unit %q", sID, unitTag.Id(),
				).Add(errors.NotValid)
			}

			storageAttachmentIds = append(storageAttachmentIds, params.StorageAttachmentId{
				UnitTag:    unitTag.String(),
				StorageTag: names.NewStorageTag(sID.String()).String(),
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
	result := params.StorageAttachmentResults{
		Results: make([]params.StorageAttachmentResult, len(args.Ids)),
	}
	return result, nil
}

// StorageAttachmentLife returns the lifecycle state of the storage attachments
// with the specified tags.
func (s *StorageAPI) StorageAttachmentLife(ctx context.Context, args params.StorageAttachmentIds) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Ids)),
	}
	one := func(arg params.StorageAttachmentId) (corelife.Value, error) {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}

		unitUUID, err := s.getUnitUUID(ctx, unitTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}

		storageTag, err := names.ParseStorageTag(arg.StorageTag)
		if err != nil {
			return "", internalerrors.Capture(err)
		}
		storageID, err := corestorage.ParseID(storageTag.Id())
		if errors.Is(err, corestorage.InvalidStorageID) {
			return "", internalerrors.Errorf(
				"invalid storage ID %q for unit %q", arg.StorageTag, unitTag.Id(),
			).Add(errors.NotValid)
		} else if err != nil {
			return "", internalerrors.Capture(err)
		}

		life, err := s.storageProvisioningService.GetAttachmentLife(ctx, unitUUID, storageID)
		switch {
		case errors.Is(err, coreerrors.NotValid):
			return "", internalerrors.Errorf(
				"invalid unit UUID %q for %q", unitUUID, unitTag.Id(),
			).Add(errors.NotValid)
		case errors.Is(err, corestorage.InvalidStorageID):
			return "", internalerrors.Errorf(
				"invalid storage ID %q for unit %q", storageID, unitTag.Id(),
			).Add(errors.NotValid)
		case errors.Is(err, applicationerrors.UnitNotFound):
			return "", internalerrors.Errorf(
				"unit %q not found", unitTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, storageprovisioningerrors.AttachmentNotFound):
			return "", internalerrors.Errorf(
				"attachment %q for unit %q not found", arg.StorageTag, unitTag.Id(),
			).Add(errors.NotFound)
		case err != nil:
			return "", internalerrors.Errorf(
				"getting attachment life for storage %q unit %q: %w", arg.StorageTag, unitTag.Id(), err,
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
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i := range args.Entities {
		var err error
		results.Results[i].StringsWatcherId, results.Results[i].Changes, err = internal.EnsureRegisterWatcher(
			ctx,
			s.watcherRegistry,
			watcher.TODO[[]string](),
		)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// WatchStorageAttachments creates watchers for a collection of storage
// attachments, each of which can be used to watch changes to storage
// attachment info.
func (s *StorageAPI) WatchStorageAttachments(ctx context.Context, args params.StorageAttachmentIds) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Ids)),
	}
	for i := range args.Ids {
		var err error
		results.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(
			ctx,
			s.watcherRegistry,
			watcher.TODO[struct{}](),
		)
		results.Results[i].Error = apiservererrors.ServerError(err)
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
