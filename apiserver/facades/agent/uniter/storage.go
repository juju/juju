// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// StorageAPI provides access to the Storage API facade.
type StorageAPI struct {
	blockDeviceService blockDeviceService
	applicationService ApplicationService
	watcherRegistry    facade.WatcherRegistry
	accessUnit         common.GetAuthFunc
}

// newStorageAPI creates a new server-side Storage API facade.
func newStorageAPI(
	blockDeviceService blockDeviceService,
	applicationService ApplicationService,
	watcherRegistry facade.WatcherRegistry,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {

	return &StorageAPI{
		blockDeviceService: blockDeviceService,
		applicationService: applicationService,
		watcherRegistry:    watcherRegistry,
		accessUnit:         accessUnit,
	}, nil
}

// UnitStorageAttachments returns the IDs of storage attachments for a collection of units.
func (s *StorageAPI) UnitStorageAttachments(ctx context.Context, args params.Entities) (params.StorageAttachmentIdsResults, error) {
	result := params.StorageAttachmentIdsResults{
		Results: make([]params.StorageAttachmentIdsResult, len(args.Entities)),
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
