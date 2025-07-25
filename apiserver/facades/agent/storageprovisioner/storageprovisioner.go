// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/container"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// StorageProvisionerAPIv4 provides the StorageProvisioner API v4 facade.
type StorageProvisionerAPIv4 struct {
	*common.LifeGetter
	*common.InstanceIdGetter

	watcherRegistry facade.WatcherRegistry

	sb                         StorageBackend
	blockDeviceService         BlockDeviceService
	resources                  facade.Resources
	authorizer                 facade.Authorizer
	registry                   storage.ProviderRegistry
	storagePoolGetter          StoragePoolGetter
	storageStatusService       StorageStatusService
	storageProvisioningService StorageProvisioningService
	modelConfigService         ModelConfigService
	machineService             MachineService
	applicationService         ApplicationService
	getScopeAuthFunc           common.GetAuthFunc
	getStorageEntityAuthFunc   common.GetAuthFunc
	getMachineAuthFunc         common.GetAuthFunc
	getBlockDevicesAuthFunc    common.GetAuthFunc
	getAttachmentAuthFunc      func(context.Context) (func(names.Tag, names.Tag) bool, error)
	logger                     logger.Logger
	clock                      clock.Clock

	controllerUUID string
	modelUUID      model.UUID
}

// NewStorageProvisionerAPIv4 creates a new server-side StorageProvisioner v3 facade.
func NewStorageProvisionerAPIv4(
	ctx context.Context,
	watcherRegistry facade.WatcherRegistry,
	clock clock.Clock,
	sb StorageBackend,
	blockDeviceService BlockDeviceService,
	modelConfigService ModelConfigService,
	machineService MachineService,
	resources facade.Resources,
	applicationService ApplicationService,
	authorizer facade.Authorizer,
	registry storage.ProviderRegistry,
	storagePoolGetter StoragePoolGetter,
	storageStatusService StorageStatusService,
	storageProvisioningService StorageProvisioningService,
	logger logger.Logger,
	modelUUID model.UUID,
	controllerUUID string,
) (*StorageProvisionerAPIv4, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	canAccessStorageMachine := func(tag names.Tag, allowController bool) bool {
		authEntityTag := authorizer.GetAuthTag()
		if tag == authEntityTag {
			// Machine agents can access volumes
			// scoped to their own machine.
			return true
		}
		parentId := container.ParentId(tag.Id())
		if parentId == "" {
			return allowController && authorizer.AuthController()
		}
		// All containers with the authenticated
		// machine as a parent are accessible by it.
		return names.NewMachineTag(parentId) == authEntityTag
	}
	getScopeAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			switch tag := tag.(type) {
			case names.ModelTag:
				// Controllers can access all volumes
				// and file systems scoped to the environment.
				isModelManager := authorizer.AuthController()
				return isModelManager && tag == names.NewModelTag(modelUUID.String())
			case names.MachineTag:
				return canAccessStorageMachine(tag, false)
			case names.ApplicationTag:
				return authorizer.AuthController()
			default:
				return false
			}
		}, nil
	}
	canAccessStorageEntity := func(tag names.Tag, allowMachines bool) bool {
		switch tag := tag.(type) {
		case names.VolumeTag:
			machineTag, ok := names.VolumeMachine(tag)
			if ok {
				return canAccessStorageMachine(machineTag, false)
			}
			return authorizer.AuthController()
		case names.FilesystemTag:
			machineTag, ok := names.FilesystemMachine(tag)
			if ok {
				return canAccessStorageMachine(machineTag, false)
			}
			_, ok = names.FilesystemUnit(tag)
			if ok {
				return authorizer.AuthController()
			}
			_, err := storageProvisioningService.GetFilesystem(ctx, tag.Id())
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				return authorizer.AuthController()
			} else if err != nil {
				return false
			}
			// TODO: implement volume auth in Dqlite.
			// volumeTag, err := f.Volume()
			// if err == nil {
			// 	// The filesystem has a backing volume. If the
			// 	// authenticated agent has access to any of the
			// 	// machines that the volume is attached to, then
			// 	// it may access the filesystem too.
			// 	volumeAttachments, err := sb.VolumeAttachments(volumeTag)
			// 	if err != nil && !errors.Is(err, errors.NotFound) {
			// 		return false
			// 	}
			// 	for _, a := range volumeAttachments {
			// 		if canAccessStorageMachine(a.Host(), false) {
			// 			return true
			// 		}
			// 	}
			// } else if !errors.Is(err, errors.NotFound) && err != state.ErrNoBackingVolume {
			// 	return false
			// }
			return authorizer.AuthController()
		case names.MachineTag:
			return allowMachines && canAccessStorageMachine(tag, true)
		case names.ApplicationTag:
			return authorizer.AuthController()
		default:
			return false
		}
	}
	getStorageEntityAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return canAccessStorageEntity(tag, false)
		}, nil
	}
	getLifeAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return canAccessStorageEntity(tag, true)
		}, nil
	}
	getAttachmentAuthFunc := func(context.Context) (func(names.Tag, names.Tag) bool, error) {
		// getAttachmentAuthFunc returns a function that validates
		// access by the authenticated user to an attachment.
		return func(hostTag names.Tag, attachmentTag names.Tag) bool {
			if hostTag.Kind() == names.UnitTagKind {
				return authorizer.AuthController()
			}

			// Machine agents can access their own machine, and
			// machines contained. Controllers can access
			// top-level machines.
			machineAccessOk := canAccessStorageMachine(hostTag, true)

			if !machineAccessOk {
				return false
			}

			// Controllers can access model-scoped
			// volumes and volumes scoped to their own machines.
			// Other machine agents can access volumes regardless
			// of their scope.
			if !authorizer.AuthController() {
				return true
			}
			var machineScope names.MachineTag
			var hasMachineScope bool
			switch attachmentTag := attachmentTag.(type) {
			case names.VolumeTag:
				machineScope, hasMachineScope = names.VolumeMachine(attachmentTag)
			case names.FilesystemTag:
				machineScope, hasMachineScope = names.FilesystemMachine(attachmentTag)
			}
			return !hasMachineScope || machineScope == authorizer.GetAuthTag()
		}, nil
	}
	getMachineAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(tag, true)
			}
			return false
		}, nil
	}
	getBlockDevicesAuthFunc := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(tag, false)
			}
			return false
		}, nil
	}
	return &StorageProvisionerAPIv4{
		LifeGetter:       common.NewLifeGetter(applicationService, machineService, getLifeAuthFunc, logger),
		InstanceIdGetter: common.NewInstanceIdGetter(machineService, getMachineAuthFunc),

		watcherRegistry: watcherRegistry,

		sb:                         sb,
		resources:                  resources,
		authorizer:                 authorizer,
		registry:                   registry,
		storagePoolGetter:          storagePoolGetter,
		storageStatusService:       storageStatusService,
		storageProvisioningService: storageProvisioningService,
		modelConfigService:         modelConfigService,
		machineService:             machineService,
		applicationService:         applicationService,
		getScopeAuthFunc:           getScopeAuthFunc,
		getStorageEntityAuthFunc:   getStorageEntityAuthFunc,
		getAttachmentAuthFunc:      getAttachmentAuthFunc,
		getMachineAuthFunc:         getMachineAuthFunc,
		getBlockDevicesAuthFunc:    getBlockDevicesAuthFunc,
		blockDeviceService:         blockDeviceService,
		logger:                     logger,
		clock:                      clock,

		controllerUUID: controllerUUID,
		modelUUID:      modelUUID,
	}, nil
}

// EnsureDead ensures that the specified entities are dead.
//
// Deprecated: This facade endpoint has not been in use since before 3.6, and
// should be removed on the next facade bump.
func (s *StorageProvisionerAPIv4) EnsureDead(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	return results, nil
}

// WatchBlockDevices watches for changes to the specified machines' block devices.
func (s *StorageProvisionerAPIv4) WatchBlockDevices(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	canAccess, err := s.getBlockDevicesAuthFunc(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity, watcherRegistry facade.WatcherRegistry) (string, error) {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return "", err
		}
		if !canAccess(machineTag) {
			return "", apiservererrors.ErrPerm
		}
		w, err := s.blockDeviceService.WatchBlockDevices(ctx, machineTag.Id())
		if err != nil {
			return "", err
		}
		watcherId, _, err := internal.EnsureRegisterWatcher(ctx, watcherRegistry, w)
		return watcherId, err
	}
	for i, arg := range args.Entities {
		var result params.NotifyWatchResult
		id, err := one(arg, s.watcherRegistry)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.NotifyWatcherId = id
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchMachines watches for changes to the specified machines.
func (s *StorageProvisionerAPIv4) WatchMachines(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i := range args.Entities {
		w := corewatcher.TODO[struct{}]()
		id, _, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].NotifyWatcherId = id
	}
	return results, nil
}

// WatchVolumes watches for changes to volumes scoped to the
// entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchVolumes(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	return s.watchStorageEntities(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedVolumes,
		s.storageProvisioningService.WatchMachineProvisionedVolumes,
	)
}

// WatchFilesystems watches for changes to filesystems scoped
// to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchFilesystems(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	return s.watchStorageEntities(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedFilesystems,
		s.storageProvisioningService.WatchMachineProvisionedFilesystems,
	)
}

func (s *StorageProvisionerAPIv4) watchStorageEntities(
	ctx context.Context,
	args params.Entities,
	watchModelStorage func(context.Context) (corewatcher.StringsWatcher, error),
	watchMachineStorage func(context.Context, machine.UUID) (corewatcher.StringsWatcher, error),
) (params.StringsWatchResults, error) {
	canAccess, err := s.getScopeAuthFunc(ctx)
	if err != nil {
		return params.StringsWatchResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (_ string, _ []string, err error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, apiservererrors.ErrPerm
		}

		var w corewatcher.StringsWatcher
		switch tag := tag.(type) {
		case names.MachineTag:
			var machineUUID machine.UUID
			machineUUID, err = s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.NotFoundf("machine %q", tag.Id())
			}
			if err != nil {
				return "", nil, internalerrors.Capture(err)
			}
			w, err = watchMachineStorage(ctx, machineUUID)
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.NotFoundf("machine %q", tag.Id())
			}
		case names.ModelTag:
			w, err = watchModelStorage(ctx)
		default:
			return "", nil, errors.NotSupportedf("watching storage for %v", tag)
		}
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		return id, changes, nil
	}
	for i, arg := range args.Entities {
		var result params.StringsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.StringsWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchVolumeAttachments watches for changes to volume attachments scoped to
// the entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchVolumeAttachments(ctx context.Context, args params.Entities) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedVolumeAttachments,
		s.storageProvisioningService.WatchMachineProvisionedVolumeAttachments,
		func(ctx context.Context, volumeAttachmentUUIDs ...string) ([]corewatcher.MachineStorageID, error) {
			if len(volumeAttachmentUUIDs) == 0 {
				return nil, nil
			}
			attachmentIDs, err := s.storageProvisioningService.GetVolumeAttachmentIDs(ctx, volumeAttachmentUUIDs)
			if err != nil {
				return nil, internalerrors.Capture(err)
			}
			if len(attachmentIDs) == 0 {
				return nil, nil
			}
			out := make([]corewatcher.MachineStorageID, 0, len(attachmentIDs))
			for _, id := range attachmentIDs {
				if id.MachineName == nil && id.UnitName == nil {
					// This should never happen.
					continue
				}
				if !names.IsValidVolume(id.VolumeID) {
					// This should never happen.
					s.logger.Errorf(ctx, "invalid volume tag ID %q", id.VolumeID)
					continue
				}
				machineStorageId := corewatcher.MachineStorageID{
					AttachmentTag: names.NewVolumeTag(id.VolumeID).String(),
				}
				if id.MachineName != nil {
					machineStorageId.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
				} else if id.UnitName != nil {
					if !names.IsValidUnit(id.UnitName.String()) {
						// This should never happen.
						s.logger.Errorf(ctx,
							"invalid unit name %q for volume ID %v",
							id.UnitName.String(), id.VolumeID,
						)
						continue
					}
					machineStorageId.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
				}
				out = append(out, machineStorageId)
			}
			return out, nil
		},
	)
}

// WatchFilesystemAttachments watches for changes to filesystem attachments
// scoped to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchFilesystemAttachments(ctx context.Context, args params.Entities) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedFilesystemAttachments,
		s.storageProvisioningService.WatchMachineProvisionedFilesystemAttachments,
		func(ctx context.Context, filesystemAttachmentUUIDs ...string) ([]corewatcher.MachineStorageID, error) {
			if len(filesystemAttachmentUUIDs) == 0 {
				return nil, nil
			}
			attachmentIDs, err := s.storageProvisioningService.GetFilesystemAttachmentIDs(ctx, filesystemAttachmentUUIDs)
			if err != nil {
				return nil, internalerrors.Capture(err)
			}
			if len(attachmentIDs) == 0 {
				return nil, nil
			}
			out := make([]corewatcher.MachineStorageID, 0, len(attachmentIDs))
			for _, id := range attachmentIDs {
				if id.MachineName == nil && id.UnitName == nil {
					// This should never happen.
					continue
				}
				if !names.IsValidFilesystem(id.FilesystemID) {
					// This should never happen.
					s.logger.Errorf(ctx, "invalid filesystem tag ID %q", id.FilesystemID)
					continue
				}
				machineStorageId := corewatcher.MachineStorageID{
					AttachmentTag: names.NewFilesystemTag(id.FilesystemID).String(),
				}
				if id.MachineName != nil {
					machineStorageId.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
				} else if id.UnitName != nil {
					if !names.IsValidUnit(id.UnitName.String()) {
						// This should never happen.
						s.logger.Errorf(ctx,
							"invalid unit name %q for filesystem ID %q",
							id.UnitName.String(), id.FilesystemID,
						)
						continue
					}
					machineStorageId.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
				}
				out = append(out, machineStorageId)
			}
			return out, nil
		},
	)
}

// WatchVolumeAttachmentPlans watches for changes to volume attachments for a machine for the purpose of allowing
// that machine to run any initialization needed, for that volume to actually appear as a block device (ie: iSCSI)
func (s *StorageProvisionerAPIv4) WatchVolumeAttachmentPlans(ctx context.Context, args params.Entities) (params.MachineStorageIdsWatchResults, error) {
	canAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil {
		return params.MachineStorageIdsWatchResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.MachineStorageIdsWatchResults{
		Results: make([]params.MachineStorageIdsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (_ string, _ []corewatcher.MachineStorageID, err error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, apiservererrors.ErrPerm
		}
		tag, ok := tag.(names.MachineTag)
		if !ok {
			return "", nil, apiservererrors.ErrPerm
		}

		defer func() {
			if errors.Is(err, machineerrors.MachineNotFound) {
				err = errors.NotFoundf("machine %q", tag.Id())
			}
		}()

		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return "", nil, errors.NotFoundf("machine %q", tag.Id())
		}
		if err != nil {
			return "", nil, internalerrors.Capture(err)
		}
		sourceWatcher, err := s.storageProvisioningService.WatchVolumeAttachmentPlans(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return "", nil, errors.NotFoundf("machine %q", tag.Id())
		}
		if err != nil {
			return "", nil, internalerrors.Capture(err)
		}
		w, err := newStringSourcedWatcher(
			sourceWatcher,
			func(_ context.Context, volumeIDs ...string) ([]corewatcher.MachineStorageID, error) {
				if len(volumeIDs) == 0 {
					return nil, nil
				}
				out := make([]corewatcher.MachineStorageID, len(volumeIDs))
				for i, volumeID := range volumeIDs {
					if !names.IsValidVolume(volumeID) {
						// This should never happen.
						s.logger.Errorf(ctx, "invalid volume tag ID %q", volumeID)
						continue
					}
					out[i] = corewatcher.MachineStorageID{
						MachineTag:    tag.String(),
						AttachmentTag: names.NewVolumeTag(volumeID).String(),
					}
				}
				return out, nil
			},
		)
		if err != nil {
			return "", nil, internalerrors.Capture(err)
		}
		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, internalerrors.Capture(err)
		}
		return id, changes, nil
	}
	for i, arg := range args.Entities {
		var result params.MachineStorageIdsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.MachineStorageIdsWatcherId = id
			result.Changes = machineStorageIDsToParams(changes)
		}
		results.Results[i] = result
	}
	return results, nil
}

func machineStorageIDsToParams(ids []corewatcher.MachineStorageID) []params.MachineStorageId {
	out := make([]params.MachineStorageId, len(ids))
	for i, id := range ids {
		out[i] = params.MachineStorageId{
			MachineTag:    id.MachineTag,
			AttachmentTag: id.AttachmentTag,
		}
	}
	return out
}

func (s *StorageProvisionerAPIv4) RemoveVolumeAttachmentPlan(ctx context.Context, args params.MachineStorageIds) (params.ErrorResults, error) {
	canAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}

	one := func(arg params.MachineStorageId) error {
		volumeAttachmentPlan, err := s.oneVolumeAttachmentPlan(arg, canAccess)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				return apiservererrors.ErrPerm
			}
			return apiservererrors.ServerError(err)
		}
		if volumeAttachmentPlan.Life() != state.Dying {
			return apiservererrors.ErrPerm
		}
		return s.sb.RemoveVolumeAttachmentPlan(
			volumeAttachmentPlan.Machine(),
			volumeAttachmentPlan.Volume(),
			false)
	}
	for i, arg := range args.Ids {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) watchAttachments(
	ctx context.Context,
	args params.Entities,
	watchModelAttachments func(context.Context) (corewatcher.StringsWatcher, error),
	watchMachineAttachments func(context.Context, machine.UUID) (corewatcher.StringsWatcher, error),
	parseAttachmentIds func(context.Context, ...string) ([]corewatcher.MachineStorageID, error),
) (params.MachineStorageIdsWatchResults, error) {
	canAccess, err := s.getScopeAuthFunc(ctx)
	if err != nil {
		return params.MachineStorageIdsWatchResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.MachineStorageIdsWatchResults{
		Results: make([]params.MachineStorageIdsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (_ string, _ []corewatcher.MachineStorageID, err error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return "", nil, apiservererrors.ErrPerm
		}

		var sourceWatcher corewatcher.StringsWatcher
		switch tag := tag.(type) {
		case names.MachineTag:
			var machineUUID machine.UUID
			machineUUID, err = s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.NotFoundf("machine %q", tag.Id())
			}
			if err != nil {
				return "", nil, internalerrors.Capture(err)
			}
			sourceWatcher, err = watchMachineAttachments(ctx, machineUUID)
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.NotFoundf("machine %q", tag.Id())
			}
		case names.ModelTag:
			sourceWatcher, err = watchModelAttachments(ctx)
		default:
			return "", nil, errors.NotSupportedf("watching attachments for %v", tag)
		}
		if err != nil {
			return "", nil, internalerrors.Errorf("watching attachments for %v: %v", tag, err)
		}

		w, err := newStringSourcedWatcher(sourceWatcher, parseAttachmentIds)
		if err != nil {
			return "", nil, internalerrors.Errorf("creating attachment watcher for %v: %v", tag, err)
		}

		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, internalerrors.Errorf("registering attachment watcher for %v: %v", tag, err)
		}
		return id, changes, nil
	}
	for i, arg := range args.Entities {
		var result params.MachineStorageIdsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.MachineStorageIdsWatcherId = id
			result.Changes = machineStorageIDsToParams(changes)
		}
		results.Results[i] = result
	}
	return results, nil
}

// Volumes returns details of volumes with the specified tags.
func (s *StorageProvisionerAPIv4) Volumes(ctx context.Context, args params.Entities) (params.VolumeResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.VolumeResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.VolumeResults{
		Results: make([]params.VolumeResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.Volume, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.Volume{}, apiservererrors.ErrPerm
		}
		volume, err := s.sb.Volume(tag)
		if errors.Is(err, errors.NotFound) {
			return params.Volume{}, apiservererrors.ErrPerm
		} else if err != nil {
			return params.Volume{}, err
		}
		return storagecommon.VolumeFromState(volume)
	}
	for i, arg := range args.Entities {
		var result params.VolumeResult
		volume, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volume
		}
		results.Results[i] = result
	}
	return results, nil
}

// Filesystems returns details of filesystems with the specified tags.
func (s *StorageProvisionerAPIv4) Filesystems(ctx context.Context, args params.Entities) (params.FilesystemResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.FilesystemResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	one := func(arg params.Entity) (params.Filesystem, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.Filesystem{}, apiservererrors.ErrPerm
		}
		fs, err := s.storageProvisioningService.GetFilesystem(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			return params.Filesystem{}, internalerrors.Errorf(
				"filesystem %q not found", tag.Id(),
			).Add(errors.NotFound)
		}
		if err != nil {
			return params.Filesystem{}, internalerrors.Errorf(
				"getting filesystem %q: %v", tag.Id(), err,
			)
		}
		if fs.Size == 0 {
			// TODO: We think that a filesystem with size 0 is not provisioned.
			// The size is set when the storage provisioner worker calls SetFilesystemInfo.
			// This is a temporary workaround for checking the provision state of the filesystem.
			// Ideally, we should have a consistent way to check provisioning status
			// for all storage entities.
			return params.Filesystem{}, internalerrors.Errorf(
				"filesystem %q is not provisioned", tag.Id(),
			).Add(errors.NotProvisioned)
		}
		result := params.Filesystem{
			FilesystemTag: tag.String(),
			Info: params.FilesystemInfo{
				FilesystemId: fs.FilesystemID,
				Size:         fs.Size,
			},
		}
		if fs.BackingVolume == nil {
			// Filesystem is not backed by a volume.
			return result, nil
		}
		if !names.IsValidVolume(fs.BackingVolume.VolumeID) {
			return params.Filesystem{}, internalerrors.Errorf(
				"invalid volume ID %q for filesystem %q", fs.BackingVolume.VolumeID, tag.Id(),
			).Add(errors.NotValid)
		}
		result.VolumeTag = names.NewVolumeTag(fs.BackingVolume.VolumeID).String()
		return result, nil
	}

	results := params.FilesystemResults{
		Results: make([]params.FilesystemResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		var result params.FilesystemResult
		filesystem, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystem
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeAttachmentPlans returns details of volume attachment plans with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachmentPlans(ctx context.Context, args params.MachineStorageIds) (params.VolumeAttachmentPlanResults, error) {
	// NOTE(gsamfira): Containers will probably not be a concern for this at the moment
	// revisit this if containers should be treated
	canAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil {
		return params.VolumeAttachmentPlanResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.VolumeAttachmentPlanResults{
		Results: make([]params.VolumeAttachmentPlanResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.VolumeAttachmentPlan, error) {
		volumeAttachmentPlan, err := s.oneVolumeAttachmentPlan(arg, canAccess)
		if err != nil {
			return params.VolumeAttachmentPlan{}, err
		}
		return storagecommon.VolumeAttachmentPlanFromState(volumeAttachmentPlan)
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentPlanResult
		volumeAttachmentPlan, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachmentPlan
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachments(ctx context.Context, args params.MachineStorageIds) (params.VolumeAttachmentResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.VolumeAttachmentResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.VolumeAttachmentResults{
		Results: make([]params.VolumeAttachmentResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.VolumeAttachment, error) {
		volumeAttachment, err := s.oneVolumeAttachment(arg, canAccess)
		if err != nil {
			return params.VolumeAttachment{}, err
		}
		return storagecommon.VolumeAttachmentFromState(volumeAttachment)
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeBlockDevices returns details of the block devices corresponding to the
// volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeBlockDevices(ctx context.Context, args params.MachineStorageIds) (params.BlockDeviceResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.BlockDeviceResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.BlockDeviceResults{
		Results: make([]params.BlockDeviceResult, len(args.Ids)),
	}
	for i, arg := range args.Ids {
		var result params.BlockDeviceResult
		blockDevice, err := s.oneVolumeBlockDevice(ctx, arg, canAccess)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = blockDevice
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemAttachments returns details of filesystem attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) FilesystemAttachments(ctx context.Context, args params.MachineStorageIds) (params.FilesystemAttachmentResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.FilesystemAttachmentResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.FilesystemAttachmentResults{
		Results: make([]params.FilesystemAttachmentResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (result params.FilesystemAttachment, _ error) {
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return result, internalerrors.Errorf(
				"parsing host tag %q: %w", arg.MachineTag, err,
			)
		}

		filesystemTag, err := names.ParseFilesystemTag(arg.AttachmentTag)
		if err != nil {
			return result, internalerrors.Errorf(
				"parsing filesystem tag %q: %w", arg.AttachmentTag, err,
			)
		}
		if !canAccess(hostTag, filesystemTag) {
			return result, apiservererrors.ErrPerm
		}

		var fsAttachment storageprovisioning.FilesystemAttachment
		switch tag := hostTag.(type) {
		case names.MachineTag:
			var machineUUID machine.UUID
			machineUUID, err = s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return result, internalerrors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(errors.NotFound)
			} else if err != nil {
				return result, internalerrors.Capture(err)
			}
			fsAttachment, err = s.storageProvisioningService.GetFilesystemAttachmentForMachine(
				ctx, machineUUID, filesystemTag.Id(),
			)
		case names.UnitTag:
			var unitName coreunit.Name
			unitName, err = coreunit.NewName(tag.Id())
			if errors.Is(err, coreunit.InvalidUnitName) {
				return result, internalerrors.Errorf(
					"invalid unit name %q", tag.Id(),
				).Add(errors.NotValid)
			} else if err != nil {
				return result, internalerrors.Capture(err)
			}

			var unitUUID coreunit.UUID
			unitUUID, err = s.applicationService.GetUnitUUID(ctx, unitName)
			if errors.Is(err, coreunit.InvalidUnitName) {
				return result, internalerrors.Errorf(
					"invalid unit name %q", unitName,
				).Add(errors.NotValid)
			} else if errors.Is(err, applicationerrors.UnitNotFound) {
				return result, internalerrors.Errorf(
					"unit %q not found", unitName,
				).Add(errors.NotFound)
			} else if err != nil {
				return result, internalerrors.Errorf("getting unit %q UUID: %w", unitName, err)
			}
			fsAttachment, err = s.storageProvisioningService.GetFilesystemAttachmentForUnit(
				ctx, unitUUID, filesystemTag.Id(),
			)
		default:
			return result, errors.NotValidf("filesystem attachment host tag %q", tag)
		}

		switch {
		case errors.Is(err, machineerrors.MachineNotFound):
			return result, internalerrors.Errorf(
				"machine %q not found", hostTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound):
			return result, internalerrors.Errorf(
				"filesystem attachment %q on %q not found", filesystemTag.Id(), hostTag.Id(),
			).Add(errors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
			return result, internalerrors.Errorf(
				"filesystem %q not found for attachment on %q", filesystemTag.Id(), hostTag.Id(),
			).Add(errors.NotFound)
		case err != nil:
			return result, internalerrors.Errorf(
				"getting filesystem attachment for %q on %q: %w",
				filesystemTag.Id(), hostTag.Id(), err,
			)
		}
		if fsAttachment.MountPoint == "" {
			// TODO: We think that filesystem attachments is not provisioned if
			// the mount point is empty. The mount point is set when the the
			// storage provisioner worker calls the SetFilesystemAttachmentInfo method.
			// This is a temporary workaround for checking the provisioned state of
			// filesystem attachments. Ideally, we should have a consistent way to check
			// provisioning status for all storage entities.
			return result, internalerrors.Errorf(
				"filesystem attachment %q on %q is not provisioned", filesystemTag.Id(), hostTag.String(),
			).Add(errors.NotProvisioned)
		}
		return params.FilesystemAttachment{
			FilesystemTag: filesystemTag.String(),
			MachineTag:    hostTag.String(),
			Info: params.FilesystemAttachmentInfo{
				MountPoint: fsAttachment.MountPoint,
				ReadOnly:   fsAttachment.ReadOnly,
			},
		}, nil
	}
	for i, arg := range args.Ids {
		var result params.FilesystemAttachmentResult
		filesystemAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeParams returns the parameters for creating or destroying
// the volumes with the specified tags.
func (s *StorageProvisionerAPIv4) VolumeParams(ctx context.Context, args params.Entities) (params.VolumeParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.VolumeParamsResults{}, err
	}
	modelCfg, err := s.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return params.VolumeParamsResults{}, err
	}

	results := params.VolumeParamsResults{
		Results: make([]params.VolumeParamsResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.VolumeParams, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.VolumeParams{}, apiservererrors.ErrPerm
		}
		volume, err := s.sb.Volume(tag)
		if errors.Is(err, errors.NotFound) {
			return params.VolumeParams{}, apiservererrors.ErrPerm
		} else if err != nil {
			return params.VolumeParams{}, err
		}
		volumeAttachments, err := s.sb.VolumeAttachments(tag)
		if err != nil {
			return params.VolumeParams{}, err
		}
		storageInstance, err := storagecommon.MaybeAssignedStorageInstance(
			volume.StorageInstance,
			s.sb.StorageInstance,
		)
		if err != nil {
			return params.VolumeParams{}, err
		}
		volumeParams, err := storagecommon.VolumeParams(
			ctx, volume, storageInstance, s.modelUUID.String(), s.controllerUUID,
			modelCfg, s.storagePoolGetter, s.registry,
		)
		if err != nil {
			return params.VolumeParams{}, err
		}
		if len(volumeAttachments) == 1 {
			// There is exactly one attachment to be made, so make
			// it immediately. Otherwise we will defer attachments
			// until later.
			volumeAttachment := volumeAttachments[0]
			volumeAttachmentParams, ok := volumeAttachment.Params()
			if !ok {
				return params.VolumeParams{}, errors.Errorf(
					"volume %q is already attached to %q",
					volumeAttachment.Volume().Id(),
					names.ReadableString(volumeAttachment.Host()),
				)
			}
			// Volumes can be attached to units (caas models) or machines.
			// We only care about instance id for machine attachments.
			var instanceId instance.Id
			if machineTag, ok := volumeAttachment.Host().(names.MachineTag); ok {
				machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
				if errors.Is(err, machineerrors.MachineNotFound) {
					return params.VolumeParams{}, errors.NotFoundf("machine %q", tag.Id())
				}
				if err != nil {
					return params.VolumeParams{}, err
				}
				instanceId, err = s.machineService.GetInstanceID(ctx, machineUUID)
				if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
					return params.VolumeParams{}, err
				}
			}
			volumeParams.Attachment = &params.VolumeAttachmentParams{
				VolumeTag:  tag.String(),
				MachineTag: volumeAttachment.Host().String(),
				VolumeId:   "",
				InstanceId: instanceId.String(),
				Provider:   volumeParams.Provider,
				ReadOnly:   volumeAttachmentParams.ReadOnly,
			}
		}
		return volumeParams, nil
	}
	for i, arg := range args.Entities {
		var result params.VolumeParamsResult
		volumeParams, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// RemoveVolumeParams returns the parameters for destroying
// or releasing the volumes with the specified tags.
func (s *StorageProvisionerAPIv4) RemoveVolumeParams(ctx context.Context, args params.Entities) (params.RemoveVolumeParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.RemoveVolumeParamsResults{}, err
	}
	results := params.RemoveVolumeParamsResults{
		Results: make([]params.RemoveVolumeParamsResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.RemoveVolumeParams, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.RemoveVolumeParams{}, apiservererrors.ErrPerm
		}
		volume, err := s.sb.Volume(tag)
		if errors.Is(err, errors.NotFound) {
			return params.RemoveVolumeParams{}, apiservererrors.ErrPerm
		} else if err != nil {
			return params.RemoveVolumeParams{}, err
		}
		if life := volume.Life(); life != state.Dead {
			return params.RemoveVolumeParams{}, errors.Errorf(
				"%s is not dead (%s)",
				names.ReadableString(tag), life,
			)
		}
		volumeInfo, err := volume.Info()
		if err != nil {
			return params.RemoveVolumeParams{}, err
		}
		provider, _, err := storagecommon.StoragePoolConfig(
			ctx, volumeInfo.Pool, s.storagePoolGetter, s.registry,
		)
		if err != nil {
			return params.RemoveVolumeParams{}, err
		}
		return params.RemoveVolumeParams{
			Provider: string(provider),
			VolumeId: volumeInfo.VolumeId,
			Destroy:  !volume.Releasing(),
		}, nil
	}
	for i, arg := range args.Entities {
		var result params.RemoveVolumeParamsResult
		volumeParams, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemParams returns the parameters for creating the filesystems
// with the specified tags.
func (s *StorageProvisionerAPIv4) FilesystemParams(ctx context.Context, args params.Entities) (params.FilesystemParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.FilesystemParamsResults{}, err
	}
	modelConfig, err := s.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return params.FilesystemParamsResults{}, err
	}
	results := params.FilesystemParamsResults{
		Results: make([]params.FilesystemParamsResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.FilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.FilesystemParams{}, apiservererrors.ErrPerm
		}
		filesystem, err := s.sb.Filesystem(tag)
		if errors.Is(err, errors.NotFound) {
			return params.FilesystemParams{}, apiservererrors.ErrPerm
		} else if err != nil {
			return params.FilesystemParams{}, err
		}
		storageInstance, err := storagecommon.MaybeAssignedStorageInstance(
			filesystem.Storage,
			s.sb.StorageInstance,
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}
		filesystemParams, err := storagecommon.FilesystemParams(
			ctx, filesystem, storageInstance, s.modelUUID.String(), s.controllerUUID,
			modelConfig, s.storagePoolGetter, s.registry,
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}
		return filesystemParams, nil
	}
	for i, arg := range args.Entities {
		var result params.FilesystemParamsResult
		filesystemParams, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// RemoveFilesystemParams returns the parameters for destroying or
// releasing the filesystems with the specified tags.
func (s *StorageProvisionerAPIv4) RemoveFilesystemParams(ctx context.Context, args params.Entities) (params.RemoveFilesystemParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.RemoveFilesystemParamsResults{}, err
	}
	results := params.RemoveFilesystemParamsResults{
		Results: make([]params.RemoveFilesystemParamsResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.RemoveFilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.RemoveFilesystemParams{}, apiservererrors.ErrPerm
		}
		filesystem, err := s.sb.Filesystem(tag)
		if errors.Is(err, errors.NotFound) {
			return params.RemoveFilesystemParams{}, apiservererrors.ErrPerm
		} else if err != nil {
			return params.RemoveFilesystemParams{}, err
		}
		if life := filesystem.Life(); life != state.Dead {
			return params.RemoveFilesystemParams{}, errors.Errorf(
				"%s is not dead (%s)",
				names.ReadableString(tag), life,
			)
		}
		filesystemInfo, err := filesystem.Info()
		if err != nil {
			return params.RemoveFilesystemParams{}, err
		}
		provider, _, err := storagecommon.StoragePoolConfig(
			ctx, filesystemInfo.Pool, s.storagePoolGetter, s.registry,
		)
		if err != nil {
			return params.RemoveFilesystemParams{}, err
		}
		return params.RemoveFilesystemParams{
			Provider:     string(provider),
			FilesystemId: filesystemInfo.FilesystemId,
			Destroy:      !filesystem.Releasing(),
		}, nil
	}
	for i, arg := range args.Entities {
		var result params.RemoveFilesystemParamsResult
		filesystemParams, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemParams
		}
		results.Results[i] = result
	}
	return results, nil
}

// VolumeAttachmentParams returns the parameters for creating the volume
// attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachmentParams(
	ctx context.Context,
	args params.MachineStorageIds,
) (params.VolumeAttachmentParamsResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.VolumeAttachmentParamsResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.VolumeAttachmentParamsResults{
		Results: make([]params.VolumeAttachmentParamsResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.VolumeAttachmentParams, error) {
		volumeAttachment, err := s.oneVolumeAttachment(arg, canAccess)
		if err != nil {
			return params.VolumeAttachmentParams{}, err
		}
		// Volumes can be attached to units (caas models) or machines.
		// We only care about instance id for machine attachments.
		var instanceId instance.Id
		if machineTag, ok := volumeAttachment.Host().(names.MachineTag); ok {
			machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return params.VolumeAttachmentParams{}, errors.NotFoundf("machine %q", machineTag.Id())
			}
			if err != nil {
				return params.VolumeAttachmentParams{}, err
			}
			instanceId, err = s.machineService.GetInstanceID(ctx, machineUUID)
			if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
				return params.VolumeAttachmentParams{}, err
			}
		}
		volume, err := s.sb.Volume(volumeAttachment.Volume())
		if err != nil {
			return params.VolumeAttachmentParams{}, err
		}
		var volumeId string
		var pool string
		if volumeParams, ok := volume.Params(); ok {
			pool = volumeParams.Pool
		} else {
			volumeInfo, err := volume.Info()
			if err != nil {
				return params.VolumeAttachmentParams{}, err
			}
			volumeId = volumeInfo.VolumeId
			pool = volumeInfo.Pool
		}
		providerType, _, err := storagecommon.StoragePoolConfig(ctx, pool, s.storagePoolGetter, s.registry)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Trace(err)
		}
		var readOnly bool
		if volumeAttachmentParams, ok := volumeAttachment.Params(); ok {
			readOnly = volumeAttachmentParams.ReadOnly
		} else {
			// Attachment parameters may be requested even if the
			// attachment exists; i.e. for reattachment.
			volumeAttachmentInfo, err := volumeAttachment.Info()
			if err != nil {
				return params.VolumeAttachmentParams{}, errors.Trace(err)
			}
			readOnly = volumeAttachmentInfo.ReadOnly
		}
		return params.VolumeAttachmentParams{
			VolumeTag:  volumeAttachment.Volume().String(),
			MachineTag: volumeAttachment.Host().String(),
			VolumeId:   volumeId,
			InstanceId: instanceId.String(),
			Provider:   string(providerType),
			ReadOnly:   readOnly,
		}, nil
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentParamsResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

// FilesystemAttachmentParams returns the parameters for creating the filesystem
// attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) FilesystemAttachmentParams(
	ctx context.Context,
	args params.MachineStorageIds,
) (params.FilesystemAttachmentParamsResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.FilesystemAttachmentParamsResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.FilesystemAttachmentParamsResults{
		Results: make([]params.FilesystemAttachmentParamsResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.FilesystemAttachmentParams, error) {
		filesystemAttachment, err := s.oneFilesystemAttachment(arg, canAccess)
		if err != nil {
			return params.FilesystemAttachmentParams{}, errors.Trace(err)
		}
		hostTag := filesystemAttachment.Host()
		// Filesystems can be attached to units (caas models) or machines.
		// We only care about instance id for machine attachments.
		var instanceId instance.Id
		if machineTag, ok := filesystemAttachment.Host().(names.MachineTag); ok {
			machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return params.FilesystemAttachmentParams{}, errors.NotFoundf("machine %q", machineTag.Id())
			}
			if err != nil {
				return params.FilesystemAttachmentParams{}, err
			}
			instanceId, err = s.machineService.GetInstanceID(ctx, machineUUID)
			if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
				return params.FilesystemAttachmentParams{}, errors.Trace(err)
			}
		}
		filesystem, err := s.sb.Filesystem(filesystemAttachment.Filesystem())
		if err != nil {
			return params.FilesystemAttachmentParams{}, errors.Trace(err)
		}
		var filesystemId string
		var pool string
		if filesystemParams, ok := filesystem.Params(); ok {
			pool = filesystemParams.Pool
		} else {
			filesystemInfo, err := filesystem.Info()
			if err != nil {
				return params.FilesystemAttachmentParams{}, errors.Trace(err)
			}
			filesystemId = filesystemInfo.FilesystemId
			pool = filesystemInfo.Pool
		}
		providerType, _, err := storagecommon.StoragePoolConfig(ctx, pool, s.storagePoolGetter, s.registry)
		if err != nil {
			return params.FilesystemAttachmentParams{}, errors.Trace(err)
		}
		var location string
		var readOnly bool
		if filesystemAttachmentParams, ok := filesystemAttachment.Params(); ok {
			location = filesystemAttachmentParams.Location
			readOnly = filesystemAttachmentParams.ReadOnly
		} else {
			// Attachment parameters may be requested even if the
			// attachment exists; i.e. for reattachment.
			filesystemAttachmentInfo, err := filesystemAttachment.Info()
			if err != nil {
				return params.FilesystemAttachmentParams{}, errors.Trace(err)
			}
			location = filesystemAttachmentInfo.MountPoint
			readOnly = filesystemAttachmentInfo.ReadOnly
		}
		return params.FilesystemAttachmentParams{
			FilesystemTag: filesystemAttachment.Filesystem().String(),
			MachineTag:    hostTag.String(),
			FilesystemId:  filesystemId,
			InstanceId:    instanceId.String(),
			Provider:      string(providerType),
			// TODO(axw) dealias MountPoint. We now have
			// Path, MountPoint and Location in different
			// parts of the codebase.
			MountPoint: location,
			ReadOnly:   readOnly,
		}, nil
	}
	for i, arg := range args.Ids {
		var result params.FilesystemAttachmentParamsResult
		filesystemAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemAttachment
		}
		results.Results[i] = result
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) oneVolumeAttachmentPlan(
	id params.MachineStorageId, canAccess common.AuthFunc,
) (state.VolumeAttachmentPlan, error) {
	machineTag, err := names.ParseMachineTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(machineTag) {
		return nil, apiservererrors.ErrPerm
	}
	volumeAttachmentPlan, err := s.sb.VolumeAttachmentPlan(machineTag, volumeTag)
	if err != nil {
		return nil, err
	}
	return volumeAttachmentPlan, nil
}

func (s *StorageProvisionerAPIv4) oneVolumeAttachment(
	id params.MachineStorageId, canAccess func(names.Tag, names.Tag) bool,
) (state.VolumeAttachment, error) {
	hostTag, err := names.ParseTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
		return nil, errors.NotValidf("volume attachment host tag %q", hostTag)
	}
	volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(hostTag, volumeTag) {
		return nil, apiservererrors.ErrPerm
	}
	volumeAttachment, err := s.sb.VolumeAttachment(hostTag, volumeTag)
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, err
	}
	return volumeAttachment, nil
}

func (s *StorageProvisionerAPIv4) oneVolumeBlockDevice(
	ctx context.Context,
	id params.MachineStorageId, canAccess func(names.Tag, names.Tag) bool,
) (params.BlockDevice, error) {
	volumeAttachment, err := s.oneVolumeAttachment(id, canAccess)
	if err != nil {
		return params.BlockDevice{}, err
	}
	volume, err := s.sb.Volume(volumeAttachment.Volume())
	if err != nil {
		return params.BlockDevice{}, err
	}
	volumeInfo, err := volume.Info()
	if err != nil {
		return params.BlockDevice{}, err
	}
	volumeAttachmentInfo, err := volumeAttachment.Info()
	if err != nil {
		return params.BlockDevice{}, err
	}
	planCanAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return params.BlockDevice{}, err
	}
	var blockDeviceInfo state.BlockDeviceInfo

	volumeAttachmentPlan, err := s.oneVolumeAttachmentPlan(id, planCanAccess)

	if err != nil {
		// Volume attachment plans are optional. We should not err out
		// if one is missing, and simply return an empty state.BlockDeviceInfo{}
		if !errors.Is(err, errors.NotFound) {
			return params.BlockDevice{}, err
		}
		blockDeviceInfo = state.BlockDeviceInfo{}
	} else {
		blockDeviceInfo, err = volumeAttachmentPlan.BlockDeviceInfo()

		if err != nil {
			// Volume attachment plans are optional. We should not err out
			// if one is missing, and simply return an empty state.BlockDeviceInfo{}
			if !errors.Is(err, errors.NotFound) {
				return params.BlockDevice{}, err
			}
			blockDeviceInfo = state.BlockDeviceInfo{}
		}
	}
	blockDevices, err := s.blockDeviceService.BlockDevices(ctx, volumeAttachment.Host().(names.MachineTag).Id())
	if err != nil {
		return params.BlockDevice{}, err
	}
	bd, ok := storagecommon.MatchingFilesystemBlockDevice(
		ctx,
		blockDevices,
		volumeInfo,
		volumeAttachmentInfo,
		storagecommon.VolumeAttachmentPlanBlockInfoFromState(blockDeviceInfo),
	)
	if !ok {
		return params.BlockDevice{}, errors.NotFoundf(
			"block device for volume %v on %v",
			volumeAttachment.Volume().Id(),
			names.ReadableString(volumeAttachment.Host()),
		)
	}
	return params.BlockDevice{
		DeviceName:     bd.DeviceName,
		DeviceLinks:    bd.DeviceLinks,
		Label:          bd.Label,
		UUID:           bd.UUID,
		HardwareId:     bd.HardwareId,
		WWN:            bd.WWN,
		BusAddress:     bd.BusAddress,
		Size:           bd.SizeMiB,
		FilesystemType: bd.FilesystemType,
		InUse:          bd.InUse,
		MountPoint:     bd.MountPoint,
		SerialId:       bd.SerialId,
	}, nil
}

func (s *StorageProvisionerAPIv4) oneFilesystemAttachment(
	id params.MachineStorageId, canAccess func(names.Tag, names.Tag) bool,
) (state.FilesystemAttachment, error) {
	hostTag, err := names.ParseTag(id.MachineTag)
	if err != nil {
		return nil, err
	}
	if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
		return nil, errors.NotValidf("filesystem attachment host tag %q", hostTag)
	}
	filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(hostTag, filesystemTag) {
		return nil, apiservererrors.ErrPerm
	}
	filesystemAttachment, err := s.sb.FilesystemAttachment(hostTag, filesystemTag)
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, err
	}
	return filesystemAttachment, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
func (s *StorageProvisionerAPIv4) SetVolumeInfo(ctx context.Context, args params.Volumes) (params.ErrorResults, error) {
	canAccessVolume, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Volumes)),
	}
	one := func(arg params.Volume) error {
		volumeTag, volumeInfo, err := storagecommon.VolumeToState(arg)
		if err != nil {
			return errors.Trace(err)
		} else if !canAccessVolume(volumeTag) {
			return apiservererrors.ErrPerm
		}
		err = s.sb.SetVolumeInfo(volumeTag, volumeInfo)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.Volumes {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
func (s *StorageProvisionerAPIv4) SetFilesystemInfo(ctx context.Context, args params.Filesystems) (params.ErrorResults, error) {
	canAccessFilesystem, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Filesystems)),
	}
	one := func(arg params.Filesystem) error {
		filesystemTag, filesystemInfo, err := storagecommon.FilesystemToState(arg)
		if err != nil {
			return errors.Trace(err)
		} else if !canAccessFilesystem(filesystemTag) {
			return apiservererrors.ErrPerm
		}
		err = s.sb.SetFilesystemInfo(filesystemTag, filesystemInfo)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.Filesystems {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) CreateVolumeAttachmentPlans(ctx context.Context, args params.VolumeAttachmentPlans) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachmentPlans)),
	}
	one := func(arg params.VolumeAttachmentPlan) error {
		machineTag, volumeTag, planInfo, _, err := storagecommon.VolumeAttachmentPlanToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		// Check that the machine is provisioned.
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", machineTag.Id())
		}
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := s.machineService.GetInstanceID(ctx, machineUUID); err != nil {
			return errors.Trace(err)
		}
		err = s.sb.CreateVolumeAttachmentPlan(machineTag, volumeTag, planInfo)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	for i, plan := range args.VolumeAttachmentPlans {
		err := one(plan)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) SetVolumeAttachmentPlanBlockInfo(ctx context.Context, args params.VolumeAttachmentPlans) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachmentPlans)),
	}
	one := func(arg params.VolumeAttachmentPlan) error {
		machineTag, volumeTag, _, blockInfo, err := storagecommon.VolumeAttachmentPlanToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		// Check that the machine is provisioned.
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", machineTag.Id())
		}
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := s.machineService.GetInstanceID(ctx, machineUUID); err != nil {
			return errors.Trace(err)
		}
		err = s.sb.SetVolumeAttachmentPlanBlockInfo(machineTag, volumeTag, blockInfo)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	for i, plan := range args.VolumeAttachmentPlans {
		err := one(plan)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// SetVolumeAttachmentInfo records the details of newly provisioned volume
// attachments.
func (s *StorageProvisionerAPIv4) SetVolumeAttachmentInfo(
	ctx context.Context,
	args params.VolumeAttachments,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachments)),
	}
	one := func(arg params.VolumeAttachment) error {
		machineTag, volumeTag, volumeAttachmentInfo, err := storagecommon.VolumeAttachmentToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		// Check that the machine is provisioned.
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", machineTag.Id())
		}
		if err != nil {
			return errors.Trace(err)
		}
		_, err = s.machineService.GetInstanceID(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			return apiservererrors.ServerError(errors.NotProvisionedf("machine %s", machineTag.Id()))
		}
		if err != nil {
			return errors.Trace(err)
		}
		err = s.sb.SetVolumeAttachmentInfo(machineTag, volumeTag, volumeAttachmentInfo)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.VolumeAttachments {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// SetFilesystemAttachmentInfo records the details of newly provisioned filesystem
// attachments.
func (s *StorageProvisionerAPIv4) SetFilesystemAttachmentInfo(
	ctx context.Context,
	args params.FilesystemAttachments,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.FilesystemAttachments)),
	}
	one := func(arg params.FilesystemAttachment) error {
		machineTag, filesystemTag, filesystemAttachmentInfo, err := storagecommon.FilesystemAttachmentToState(arg)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(machineTag, filesystemTag) {
			return apiservererrors.ErrPerm
		}
		// Check that the machine is provisioned before setting attachment info.
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.NotFoundf("machine %q", machineTag.Id())
		}
		if err != nil {
			return errors.Trace(err)
		}
		_, err = s.machineService.GetInstanceID(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			return apiservererrors.ServerError(errors.NotProvisionedf("machine %s", machineTag.Id()))
		}
		if err != nil {
			return errors.Trace(err)
		}
		err = s.sb.SetFilesystemAttachmentInfo(machineTag, filesystemTag, filesystemAttachmentInfo)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		}
		return errors.Trace(err)
	}
	for i, arg := range args.FilesystemAttachments {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// AttachmentLife returns the lifecycle state of each specified machine
// storage attachment.
func (s *StorageProvisionerAPIv4) AttachmentLife(ctx context.Context, args params.MachineStorageIds) (params.LifeResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.LifeResults{}, err
	}
	results := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (life.Value, error) {
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return "", err
		}
		if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
			return "", errors.NotValidf("attachment host tag %q", hostTag)
		}
		attachmentTag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return "", err
		}
		if !canAccess(hostTag, attachmentTag) {
			return "", apiservererrors.ErrPerm
		}
		var lifer state.Lifer
		switch attachmentTag := attachmentTag.(type) {
		case names.VolumeTag:
			lifer, err = s.sb.VolumeAttachment(hostTag, attachmentTag)
		case names.FilesystemTag:
			lifer, err = s.sb.FilesystemAttachment(hostTag, attachmentTag)
		}
		if err != nil {
			return "", errors.Trace(err)
		}
		return life.Value(lifer.Life().String()), nil
	}
	for i, arg := range args.Ids {
		life, err := one(arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			results.Results[i].Life = life
		}
	}
	return results, nil
}

// Remove removes volumes and filesystems from state.
func (s *StorageProvisionerAPIv4) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	one := func(arg params.Entity) error {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(tag) {
			return apiservererrors.ErrPerm
		}
		switch tag := tag.(type) {
		case names.FilesystemTag:
			return s.sb.RemoveFilesystem(tag)
		case names.VolumeTag:
			return s.sb.RemoveVolume(tag)
		default:
			// should have been picked up by canAccess
			s.logger.Debugf(ctx, "unexpected %v tag", tag.Kind())
			return apiservererrors.ErrPerm
		}
	}
	for i, arg := range args.Entities {
		err := one(arg)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// RemoveAttachment removes the specified machine storage attachments
// from state.
func (s *StorageProvisionerAPIv4) RemoveAttachment(ctx context.Context, args params.MachineStorageIds) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	removeAttachment := func(arg params.MachineStorageId) error {
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return err
		}
		if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
			return errors.NotValidf("attachment host tag %q", hostTag)
		}
		attachmentTag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return err
		}
		if !canAccess(hostTag, attachmentTag) {
			return apiservererrors.ErrPerm
		}
		switch attachmentTag := attachmentTag.(type) {
		case names.VolumeTag:
			return s.sb.RemoveVolumeAttachment(hostTag, attachmentTag, false)
		case names.FilesystemTag:
			return s.sb.RemoveFilesystemAttachment(hostTag, attachmentTag, false)
		default:
			return apiservererrors.ErrPerm
		}
	}
	for i, arg := range args.Ids {
		if err := removeAttachment(arg); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// SetStatus sets the status of each given storage artefact.
func (s *StorageProvisionerAPIv4) SetStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	now := s.clock.Now()
	for i, arg := range args.Entities {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		sInfo := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		var statusErr error
		switch tag := tag.(type) {
		case names.FilesystemTag:
			statusErr = s.storageStatusService.SetFilesystemStatus(ctx, tag.Id(), sInfo)
			if errors.Is(statusErr, storageerrors.FilesystemNotFound) {
				statusErr = errors.NotFoundf("filesystem %q", tag.Id())
			}
		case names.VolumeTag:
			statusErr = s.storageStatusService.SetVolumeStatus(ctx, tag.Id(), sInfo)
			if errors.Is(statusErr, storageerrors.VolumeNotFound) {
				statusErr = errors.NotFoundf("volume %q", tag.Id())
			}
		default:
			statusErr = apiservererrors.ErrPerm
		}
		result.Results[i].Error = apiservererrors.ServerError(statusErr)
	}
	return result, nil
}
