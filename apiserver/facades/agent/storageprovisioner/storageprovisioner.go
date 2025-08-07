// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/container"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// StorageProvisionerAPIv4 provides the StorageProvisioner API v4 facade.
type StorageProvisionerAPIv4 struct {
	*common.InstanceIdGetter

	watcherRegistry facade.WatcherRegistry

	blockDeviceService         BlockDeviceService
	authorizer                 facade.Authorizer
	registry                   storage.ProviderRegistry
	storagePoolGetter          StoragePoolGetter
	storageStatusService       StorageStatusService
	storageProvisioningService StorageProvisioningService
	machineService             MachineService
	applicationService         ApplicationService
	getScopeAuthFunc           common.GetAuthFunc
	getStorageEntityAuthFunc   common.GetAuthFunc
	getLifeAuthFunc            common.GetAuthFunc
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
	blockDeviceService BlockDeviceService,
	machineService MachineService,
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

			exists, err := storageProvisioningService.CheckFilesystemForIDExists(
				ctx, tag.Id(),
			)
			if err != nil {
				return false
			}
			if !exists {
				return authorizer.AuthController()
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
		InstanceIdGetter: common.NewInstanceIdGetter(machineService, getMachineAuthFunc),

		watcherRegistry: watcherRegistry,

		authorizer:                 authorizer,
		registry:                   registry,
		storagePoolGetter:          storagePoolGetter,
		storageStatusService:       storageStatusService,
		storageProvisioningService: storageProvisioningService,
		machineService:             machineService,
		applicationService:         applicationService,
		getScopeAuthFunc:           getScopeAuthFunc,
		getStorageEntityAuthFunc:   getStorageEntityAuthFunc,
		getLifeAuthFunc:            getLifeAuthFunc,
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

// Life returns the life of the entities passed in.
// The entities are expected to be either filesystems or volumes tags.
func (s *StorageProvisionerAPIv4) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	canAccess, err := s.getLifeAuthFunc(ctx)
	if err != nil {
		return params.LifeResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		var life domainlife.Life
		switch tag := tag.(type) {
		case names.VolumeTag:
			life, err = s.lifeForVolume(ctx, tag)
		case names.FilesystemTag:
			life, err = s.lifeForFilesystem(ctx, tag)
		default:
			err = errors.Errorf(
				"invalid tag %q, expected volume or filesystem", tag,
			).Add(coreerrors.NotValid)
		}
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if results.Results[i].Life, err = life.Value(); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) lifeForFilesystem(
	ctx context.Context, tag names.FilesystemTag,
) (domainlife.Life, error) {
	filesystemUUID, err := s.storageProvisioningService.GetFilesystemUUIDForID(ctx, tag.Id())
	switch {
	case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
		return -1, errors.Errorf(
			"filesystem not found for id %q", tag.Id(),
		).Add(coreerrors.NotFound)
	case err != nil:
		return -1, errors.Errorf("getting filesystem UUID for id %q: %v", tag.Id(), err)
	}

	life, err := s.storageProvisioningService.GetFilesystemLife(ctx, filesystemUUID)
	switch {
	case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
		return -1, errors.Errorf(
			"filesystem not found for id %q", tag.Id(),
		).Add(coreerrors.NotFound)
	case err != nil:
		return -1, errors.Errorf("getting filesystem life for id %q: %v", tag.Id(), err)
	}
	return life, nil
}

func (s *StorageProvisionerAPIv4) lifeForVolume(
	ctx context.Context, tag names.VolumeTag,
) (domainlife.Life, error) {
	volumeUUID, err := s.storageProvisioningService.GetVolumeUUIDForID(ctx, tag.Id())
	switch {
	case errors.Is(err, storageprovisioningerrors.VolumeNotFound):
		return -1, errors.Errorf(
			"volume not found for id %q", tag.Id(),
		).Add(coreerrors.NotFound)
	case err != nil:
		return -1, errors.Errorf("getting volume UUID for id %q: %v", tag.Id(), err)
	}

	life, err := s.storageProvisioningService.GetVolumeLife(ctx, volumeUUID)
	switch {
	case errors.Is(err, storageprovisioningerrors.VolumeNotFound):
		return -1, errors.Errorf(
			"volume not found for id %q", tag.Id(),
		).Add(coreerrors.NotFound)
	case err != nil:
		return -1, errors.Errorf("getting volume UUID for id %q: %v", tag.Id(), err)
	}

	return life, nil
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
				return "", nil, errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
			if err != nil {
				return "", nil, errors.Capture(err)
			}
			w, err = watchMachineStorage(ctx, machineUUID)
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		case names.ModelTag:
			w, err = watchModelStorage(ctx)
		default:
			return "", nil, errors.Errorf(
				"watching storage for %v", tag,
			).Add(coreerrors.NotSupported)
		}
		if err != nil {
			return "", nil, errors.Capture(err)
		}

		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, errors.Capture(err)
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
				return nil, errors.Capture(err)
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
				return nil, errors.Capture(err)
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
				err = errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		}()

		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return "", nil, errors.Errorf(
				"machine %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		}
		if err != nil {
			return "", nil, errors.Capture(err)
		}
		sourceWatcher, err := s.storageProvisioningService.WatchVolumeAttachmentPlans(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return "", nil, errors.Errorf(
				"machine %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		}
		if err != nil {
			return "", nil, errors.Capture(err)
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
			return "", nil, errors.Capture(err)
		}
		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, errors.Capture(err)
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
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
	}
	// TODO: implement this method using the storageProvisioningService.
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
				return "", nil, errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
			if err != nil {
				return "", nil, errors.Capture(err)
			}
			sourceWatcher, err = watchMachineAttachments(ctx, machineUUID)
			if errors.Is(err, machineerrors.MachineNotFound) {
				return "", nil, errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		case names.ModelTag:
			sourceWatcher, err = watchModelAttachments(ctx)
		default:
			return "", nil, errors.Errorf(
				"watching attachments for %v", tag,
			).Add(coreerrors.NotSupported)
		}
		if err != nil {
			return "", nil, errors.Errorf("watching attachments for %v: %v", tag, err)
		}

		w, err := newStringSourcedWatcher(sourceWatcher, parseAttachmentIds)
		if err != nil {
			return "", nil, errors.Errorf("creating attachment watcher for %v: %v", tag, err)
		}

		id, changes, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, errors.Errorf("registering attachment watcher for %v: %v", tag, err)
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
	results := params.VolumeResults{
		Results: make([]params.VolumeResult, len(args.Entities)),
	}
	// TODO: implement this method using the storageProvisioningService.
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
		fs, err := s.storageProvisioningService.GetFilesystemForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			return params.Filesystem{}, errors.Errorf(
				"filesystem %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		}
		if err != nil {
			return params.Filesystem{}, errors.Errorf(
				"getting filesystem %q: %v", tag.Id(), err,
			)
		}
		if fs.SizeMiB == 0 {
			// TODO: We think that a filesystem with size 0 is not provisioned.
			// The size is set when the storage provisioner worker calls SetFilesystemInfo.
			// This is a temporary workaround for checking the provision state of the filesystem.
			// Ideally, we should have a consistent way to check provisioning status
			// for all storage entities.
			return params.Filesystem{}, errors.Errorf(
				"filesystem %q is not provisioned", tag.Id(),
			).Add(coreerrors.NotProvisioned)
		}
		result := params.Filesystem{
			FilesystemTag: tag.String(),
			Info: params.FilesystemInfo{
				ProviderId: fs.ProviderID,
				Size:       fs.SizeMiB,
			},
		}
		if fs.BackingVolume == nil {
			// Filesystem is not backed by a volume.
			return result, nil
		}
		if !names.IsValidVolume(fs.BackingVolume.VolumeID) {
			return params.Filesystem{}, errors.Errorf(
				"invalid volume ID %q for filesystem %q", fs.BackingVolume.VolumeID, tag.Id(),
			).Add(coreerrors.NotValid)
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
	// TODO: implement this method using the storageProvisioningService.
	results := params.VolumeAttachmentPlanResults{
		Results: make([]params.VolumeAttachmentPlanResult, len(args.Ids)),
	}
	return results, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachments(ctx context.Context, args params.MachineStorageIds) (params.VolumeAttachmentResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.VolumeAttachmentResults{
		Results: make([]params.VolumeAttachmentResult, len(args.Ids)),
	}
	return results, nil
}

// VolumeBlockDevices returns details of the block devices corresponding to the
// volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeBlockDevices(ctx context.Context, args params.MachineStorageIds) (params.BlockDeviceResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.BlockDeviceResults{
		Results: make([]params.BlockDeviceResult, len(args.Ids)),
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
			return result, errors.Errorf(
				"parsing host tag %q: %w", arg.MachineTag, err,
			)
		}

		filesystemTag, err := names.ParseFilesystemTag(arg.AttachmentTag)
		if err != nil {
			return result, errors.Errorf(
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
				return result, errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			} else if err != nil {
				return result, errors.Capture(err)
			}
			fsAttachment, err = s.storageProvisioningService.GetFilesystemAttachmentForMachine(
				ctx, filesystemTag.Id(), machineUUID,
			)
		case names.UnitTag:
			var unitUUID coreunit.UUID
			unitUUID, err = s.getUnitUUID(ctx, tag)
			if err != nil {
				return result, errors.Capture(err)
			}
			fsAttachment, err = s.storageProvisioningService.GetFilesystemAttachmentForUnit(
				ctx, filesystemTag.Id(), unitUUID,
			)
		default:
			return result, errors.Errorf(
				"filesystem attachment host tag %q", tag,
			).Add(coreerrors.NotValid)
		}

		switch {
		case errors.Is(err, machineerrors.MachineNotFound):
			return result, errors.Errorf(
				"machine %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound):
			return result, errors.Errorf(
				"filesystem attachment %q on %q not found", filesystemTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
			return result, errors.Errorf(
				"filesystem %q not found for attachment on %q", filesystemTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return result, errors.Errorf(
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
			return result, errors.Errorf(
				"filesystem attachment %q on %q is not provisioned", filesystemTag.Id(), hostTag.String(),
			).Add(coreerrors.NotProvisioned)
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
	// TODO: implement this method using the storageProvisioningService.
	results := params.VolumeParamsResults{
		Results: make([]params.VolumeParamsResult, len(args.Entities)),
	}
	return results, nil
}

// RemoveVolumeParams returns the parameters for destroying
// or releasing the volumes with the specified tags.
func (s *StorageProvisionerAPIv4) RemoveVolumeParams(ctx context.Context, args params.Entities) (params.RemoveVolumeParamsResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.RemoveVolumeParamsResults{
		Results: make([]params.RemoveVolumeParamsResult, len(args.Entities)),
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

	results := params.FilesystemParamsResults{
		Results: make([]params.FilesystemParamsResult, 0, len(args.Entities)),
	}

	one := func(arg params.Entity) (params.FilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.FilesystemParams{}, apiservererrors.ErrPerm
		}

		uuid, err := s.storageProvisioningService.GetFilesystemUUIDForID(
			ctx, tag.Id(),
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}

		fsParams, err := s.storageProvisioningService.GetFilesystemParams(
			ctx, uuid,
		)
		if err != nil {
			return params.FilesystemParams{}, err
		}

		rval := params.FilesystemParams{
			// VolumeTag and Attachment params have never been set.
			Attributes:    make(map[string]any, len(fsParams.Attributes)),
			FilesystemTag: tag.String(),
			Provider:      fsParams.Provider,
			Size:          fsParams.SizeMiB,
			Tags:          fsParams.Tags,
		}

		for k, v := range fsParams.Attributes {
			rval.Attributes[k] = v
		}

		return rval, nil
	}

	for _, arg := range args.Entities {
		var result params.FilesystemParamsResult
		filesystemParams, err := one(arg)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			err = errors.Errorf(
				"filesystem %q not found", arg.Tag,
			).Add(coreerrors.NotFound)
		}

		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemParams
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// RemoveFilesystemParams returns the parameters for destroying or
// releasing the filesystems with the specified tags.
func (s *StorageProvisionerAPIv4) RemoveFilesystemParams(ctx context.Context, args params.Entities) (params.RemoveFilesystemParamsResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.RemoveFilesystemParamsResults{
		Results: make([]params.RemoveFilesystemParamsResult, len(args.Entities)),
	}
	return results, nil
}

// VolumeAttachmentParams returns the parameters for creating the volume
// attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachmentParams(
	ctx context.Context,
	args params.MachineStorageIds,
) (params.VolumeAttachmentParamsResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.VolumeAttachmentParamsResults{
		Results: make([]params.VolumeAttachmentParamsResult, len(args.Ids)),
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
		Results: make([]params.FilesystemAttachmentParamsResult, 0, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.FilesystemAttachmentParams, error) {
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}
		if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
			return params.FilesystemAttachmentParams{}, errors.Errorf(
				"filesystem attachment host tag %q not valid", hostTag,
			).Add(coreerrors.NotValid)
		}
		filesystemTag, err := names.ParseFilesystemTag(arg.AttachmentTag)
		if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}
		if !canAccess(hostTag, filesystemTag) {
			return params.FilesystemAttachmentParams{}, apiservererrors.ErrPerm
		}

		attachmentUUID, err := s.getFilesystemAttachmentUUID(
			ctx, filesystemTag, hostTag,
		)
		if err != nil {
			return params.FilesystemAttachmentParams{}, err
		}

		fsParams, err := s.storageProvisioningService.GetFilesystemAttachmentParams(
			ctx, attachmentUUID,
		)
		if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
			err = errors.Errorf(
				"filesystem attachment for filesystem %q and host %q not found",
				filesystemTag, hostTag,
			).Add(coreerrors.NotFound)
		}

		return params.FilesystemAttachmentParams{
			FilesystemTag: filesystemTag.String(),
			MachineTag:    hostTag.String(),
			InstanceId:    fsParams.MachineInstanceID,
			Provider:      fsParams.Provider,
			ProviderId:    fsParams.ProviderID,
			MountPoint:    fsParams.MountPoint,
			ReadOnly:      fsParams.ReadOnly,
		}, nil
	}
	for _, arg := range args.Ids {
		var result params.FilesystemAttachmentParamsResult
		filesystemAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = filesystemAttachment
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
func (s *StorageProvisionerAPIv4) SetVolumeInfo(ctx context.Context, args params.Volumes) (params.ErrorResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Volumes)),
	}
	one := func(vol params.Volume) error {
		volumeTag, err := names.ParseVolumeTag(vol.VolumeTag)
		if err != nil {
			return errors.Errorf(
				"parsing volume tag %q: %w", vol.VolumeTag, err,
			)
		}
		if !canAccess(volumeTag) {
			return apiservererrors.ErrPerm
		}
		if vol.Info.Pool != "" {
			return errors.New("pool field must not be set")
		}
		info := storageprovisioning.VolumeProvisionedInfo{
			ProviderID: vol.Info.ProviderId,
			SizeMiB:    vol.Info.SizeMiB,
			HardwareID: vol.Info.HardwareId,
			WWN:        vol.Info.WWN,
			Persistent: vol.Info.Persistent,
		}
		err = s.storageProvisioningService.SetVolumeProvisionedInfo(
			ctx, volumeTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			return errors.Errorf(
				"volume %q not found", volumeTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}
	for i, vol := range args.Volumes {
		err := one(vol)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
func (s *StorageProvisionerAPIv4) SetFilesystemInfo(ctx context.Context, args params.Filesystems) (params.ErrorResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Filesystems)),
	}
	one := func(fs params.Filesystem) error {
		filesystemTag, err := names.ParseFilesystemTag(fs.FilesystemTag)
		if err != nil {
			return errors.Errorf(
				"parsing filesystem tag %q: %w", fs.FilesystemTag, err,
			)
		}
		if !canAccess(filesystemTag) {
			return apiservererrors.ErrPerm
		}
		if fs.Info.Pool != "" {
			return errors.New("pool field must not be set")
		}
		if fs.VolumeTag != "" {
			// TODO(storage): once volumes are implemented, we need to check that
			// the volume referenced here is provisioned, attached and owned by
			// the same storage instance. This could be pushed into the
			// storageprovisioning service, but that would require it to
			// understand the provisioned status of a volume.
			return errors.New("volume backed filesystem not implemented").
				Add(coreerrors.NotImplemented)
		}
		info := storageprovisioning.FilesystemProvisionedInfo{
			ProviderID: fs.Info.ProviderId,
			SizeMiB:    fs.Info.Size,
		}
		err = s.storageProvisioningService.SetFilesystemProvisionedInfo(
			ctx, filesystemTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			return errors.Errorf(
				"filesystem %q not found", filesystemTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}
	for i, fs := range args.Filesystems {
		err := one(fs)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) CreateVolumeAttachmentPlans(ctx context.Context, args params.VolumeAttachmentPlans) (params.ErrorResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachmentPlans)),
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
	one := func(vp params.VolumeAttachmentPlan) error {
		machineTag, err := names.ParseMachineTag(vp.MachineTag)
		if err != nil {
			return errors.Errorf(
				"parsing volume tag %q: %w", vp.VolumeTag, err,
			)
		}
		volumeTag, err := names.ParseVolumeTag(vp.VolumeTag)
		if err != nil {
			return errors.Errorf(
				"parsing volume tag %q: %w", vp.VolumeTag, err,
			)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		if vp.Life != "" {
			return errors.New("life field must not be set")
		}
		if vp.PlanInfo.DeviceType != "" {
			return errors.New("device type field must not be set")
		}
		if len(vp.PlanInfo.DeviceAttributes) != 0 {
			return errors.New("device attributes field must not be set")
		}
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		blockDeviceInfo := blockdevice.BlockDevice{
			DeviceName:     vp.BlockDevice.DeviceName,
			DeviceLinks:    vp.BlockDevice.DeviceLinks,
			Label:          vp.BlockDevice.Label,
			UUID:           vp.BlockDevice.UUID,
			HardwareId:     vp.BlockDevice.HardwareId,
			WWN:            vp.BlockDevice.WWN,
			BusAddress:     vp.BlockDevice.BusAddress,
			SizeMiB:        vp.BlockDevice.Size,
			FilesystemType: vp.BlockDevice.FilesystemType,
			InUse:          vp.BlockDevice.InUse,
			MountPoint:     vp.BlockDevice.MountPoint,
			SerialId:       vp.BlockDevice.SerialId,
		}
		err = s.storageProvisioningService.SetVolumeAttachmentPlanProvisionedBlockDevice(
			ctx, volumeTag.Id(), machineUUID, blockDeviceInfo)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			return errors.Errorf(
				"volume attachment plan for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}
	for i, vp := range args.VolumeAttachmentPlans {
		err := one(vp)
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
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachments)),
	}
	one := func(va params.VolumeAttachment) error {
		machineTag, err := names.ParseMachineTag(va.MachineTag)
		if err != nil {
			return errors.Errorf(
				"parsing volume tag %q: %w", va.VolumeTag, err,
			)
		}
		volumeTag, err := names.ParseVolumeTag(va.VolumeTag)
		if err != nil {
			return errors.Errorf(
				"parsing volume tag %q: %w", va.VolumeTag, err,
			)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		volumeAttachmentUUID, err := s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDMachine(
			ctx, volumeTag.Id(), machineUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return errors.Errorf(
				"volume attachment for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		info := storageprovisioning.VolumeAttachmentProvisionedInfo{
			ReadOnly:              va.Info.ReadOnly,
			BlockDeviceName:       va.Info.DeviceName,
			BlockDeviceLink:       va.Info.DeviceLink,
			BlockDeviceBusAddress: va.Info.BusAddress,
		}
		err = s.storageProvisioningService.SetVolumeAttachmentProvisionedInfo(
			ctx, volumeAttachmentUUID, info)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return errors.Errorf(
				"volume attachment for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		if va.Info.PlanInfo != nil {
			info := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
				DeviceType:       string(va.Info.PlanInfo.DeviceType),
				DeviceAttributes: va.Info.PlanInfo.DeviceAttributes,
			}
			err = s.storageProvisioningService.SetVolumeAttachmentPlanProvisionedInfo(
				ctx, volumeTag.Id(), machineUUID, info)
			if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
				return errors.Errorf(
					"volume attachment plan for machine %q and volume %q not found",
					machineTag.Id(), volumeTag.Id(),
				).Add(coreerrors.NotFound)
			} else if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	}
	for i, va := range args.VolumeAttachments {
		err := one(va)
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
		return params.ErrorResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.FilesystemAttachments)),
	}
	one := func(fa params.FilesystemAttachment) error {
		hostTag, err := names.ParseTag(fa.MachineTag)
		if err != nil {
			return errors.Errorf(
				"parsing host tag %q: %w", fa.MachineTag, err,
			)
		}
		filesystemTag, err := names.ParseFilesystemTag(fa.FilesystemTag)
		if err != nil {
			return errors.Errorf(
				"parsing filesystem tag %q: %w", fa.FilesystemTag, err,
			)
		}
		if !canAccess(hostTag, filesystemTag) {
			return apiservererrors.ErrPerm
		}

		info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
			MountPoint: fa.Info.MountPoint,
			ReadOnly:   fa.Info.ReadOnly,
		}
		switch tag := hostTag.(type) {
		case names.MachineTag:
			var machineUUID machine.UUID
			machineUUID, err = s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				return errors.Errorf(
					"machine %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			} else if err != nil {
				return errors.Capture(err)
			}
			err = s.storageProvisioningService.SetFilesystemAttachmentProvisionedInfoForMachine(
				ctx, filesystemTag.Id(), machineUUID, info)
		case names.UnitTag:
			unitName := coreunit.Name(tag.Id())
			var unitUUID coreunit.UUID
			unitUUID, err = s.applicationService.GetUnitUUID(ctx, unitName)
			if errors.Is(err, coreunit.InvalidUnitName) {
				return errors.Errorf(
					"invalid unit name %q", unitName,
				).Add(coreerrors.NotValid)
			} else if errors.Is(err, applicationerrors.UnitNotFound) {
				return errors.Errorf(
					"unit %q not found", unitName,
				).Add(coreerrors.NotFound)
			} else if err != nil {
				return errors.Errorf("getting unit %q UUID: %w", unitName, err)
			}
			err = s.storageProvisioningService.SetFilesystemAttachmentProvisionedInfoForUnit(
				ctx, filesystemTag.Id(), unitUUID, info)
		default:
			return errors.Errorf(
				"filesystem attachment host tag %q not found", tag,
			).Add(coreerrors.NotValid)
		}
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			return errors.Errorf(
				"filesystem %q not found", filesystemTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}
	for i, fa := range args.FilesystemAttachments {
		err := one(fa)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) getUnitUUID(
	ctx context.Context, tag names.UnitTag,
) (coreunit.UUID, error) {
	unitName, err := coreunit.NewName(tag.Id())
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf(
			"invalid unit name %q", tag.Id(),
		).Add(coreerrors.NotValid)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := s.applicationService.GetUnitUUID(ctx, unitName)
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf(
			"invalid unit name %q", unitName,
		).Add(coreerrors.NotValid)
	} else if errors.Is(err, applicationerrors.UnitNotFound) {
		return "", errors.Errorf(
			"unit %q not found", unitName,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf("getting unit %q UUID: %w", unitName, err)
	}
	return unitUUID, nil
}

func (s *StorageProvisionerAPIv4) filesystemAttachmentLife(
	ctx context.Context, fsTag names.FilesystemTag, hostTag names.Tag,
) (life.Value, error) {
	fsAttachmentUUID, err := s.getFilesystemAttachmentUUID(ctx, fsTag, hostTag)
	if err != nil {
		return "", errors.Capture(err)
	}

	fsLife, err := s.storageProvisioningService.GetFilesystemAttachmentLife(
		ctx, fsAttachmentUUID,
	)
	if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
		return "", errors.Errorf(
			"filesystem attachment %q on %q not found", fsTag.Id(), hostTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting filesystem attachment life for %q on %q: %w",
			fsTag.Id(), hostTag.Id(), err,
		)
	}
	return fsLife.Value()
}

func (s *StorageProvisionerAPIv4) getFilesystemAttachmentUUID(
	ctx context.Context, fsTag names.FilesystemTag, hostTag names.Tag,
) (storageprovisioning.FilesystemAttachmentUUID, error) {
	errHandler := func(err error) error {
		switch {
		case errors.Is(err, applicationerrors.UnitNotFound):
			return errors.Errorf(
				"unit %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, machineerrors.MachineNotFound):
			return errors.Errorf(
				"machine %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound):
			return errors.Errorf(
				"filesystem attachment %q on %q not found", fsTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
			return errors.Errorf(
				"filesystem %q not found for attachment on %q", fsTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return errors.Errorf(
				"getting filesystem attachment UUID for %q on %q: %w",
				fsTag.Id(), hostTag.Id(), err,
			)
		}
		return nil
	}

	var rval storageprovisioning.FilesystemAttachmentUUID
	switch tag := hostTag.(type) {
	case names.MachineTag:
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if err != nil {
			return "", errHandler(err)
		}

		rval, err = s.storageProvisioningService.GetFilesystemAttachmentUUIDForFilesystemIDMachine(
			ctx, fsTag.Id(), machineUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	case names.UnitTag:
		unitUUID, err := s.getUnitUUID(ctx, tag)
		if err != nil {
			return "", errors.Capture(err)
		}
		rval, err = s.storageProvisioningService.GetFilesystemAttachmentUUIDForFilesystemIDUnit(
			ctx, fsTag.Id(), unitUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	default:
		return "", errors.Errorf(
			"filesystem attachment host tag %q is not a valid", hostTag.String(),
		).Add(coreerrors.NotValid)
	}

	return rval, nil
}

func (s *StorageProvisionerAPIv4) getVolumeAttachmentUUID(
	ctx context.Context, volTag names.VolumeTag, hostTag names.Tag,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	errHandler := func(err error) error {
		switch {
		case errors.Is(err, machineerrors.MachineNotFound):
			return errors.Errorf(
				"machine %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound):
			return errors.Errorf(
				"volume attachment %q on %q not found", volTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.VolumeNotFound):
			return errors.Errorf(
				"volume %q not found for attachment on %q", volTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return errors.Errorf(
				"getting volume attachment uuid for %q on %q: %w",
				volTag.Id(), hostTag.Id(), err,
			)
		}
		return nil
	}

	var rval storageprovisioning.VolumeAttachmentUUID
	switch tag := hostTag.(type) {
	case names.MachineTag:
		machineUUID, err := s.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if err != nil {
			return "", errHandler(err)
		}

		rval, err = s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDMachine(
			ctx, volTag.Id(), machineUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	case names.UnitTag:
		unitUUID, err := s.getUnitUUID(ctx, tag)
		if err != nil {
			return "", errors.Capture(err)
		}
		rval, err = s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDUnit(
			ctx, volTag.Id(), unitUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	default:
		return "", errors.Errorf(
			"volume attachment host tag %q is not a valid", hostTag.String(),
		).Add(coreerrors.NotValid)
	}

	return rval, nil
}

func (s *StorageProvisionerAPIv4) volumeAttachmentLife(
	ctx context.Context, volTag names.VolumeTag, hostTag names.Tag,
) (life.Value, error) {
	uuid, err := s.getVolumeAttachmentUUID(ctx, volTag, hostTag)
	if err != nil {
		return "", errors.Capture(err)
	}

	volLife, err := s.storageProvisioningService.GetVolumeAttachmentLife(
		ctx, uuid,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return "", errors.Errorf(
			"volume attachment %q on %q not found", volTag.Id(), hostTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting volume attachment life for %q on %q: %w",
			volTag.Id(), hostTag.Id(), err,
		)
	}
	return volLife.Value()
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
	oneLife := func(arg params.MachineStorageId) (life.Value, error) {
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return "", err
		}

		tag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return "", err
		}
		if !canAccess(hostTag, tag) {
			return "", apiservererrors.ErrPerm
		}
		switch tag := tag.(type) {
		case names.VolumeTag:
			return s.volumeAttachmentLife(ctx, tag, hostTag)
		case names.FilesystemTag:
			return s.filesystemAttachmentLife(ctx, tag, hostTag)
		default:
			return "", errors.Errorf(
				"attachment tag %q is not a valid", tag.String(),
			).Add(coreerrors.NotValid)
		}
	}
	for i, arg := range args.Ids {
		life, err := oneLife(arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Life = life
	}
	return results, nil
}

// Remove removes volumes and filesystems from state.
func (s *StorageProvisionerAPIv4) Remove(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	return results, nil
}

// RemoveAttachment removes the specified machine storage attachments
// from state.
func (s *StorageProvisionerAPIv4) RemoveAttachment(ctx context.Context, args params.MachineStorageIds) (params.ErrorResults, error) {
	// TODO: implement this method using the storageProvisioningService.
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Ids)),
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
				statusErr = errors.Errorf(
					"filesystem %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		case names.VolumeTag:
			statusErr = s.storageStatusService.SetVolumeStatus(ctx, tag.Id(), sInfo)
			if errors.Is(statusErr, storageerrors.VolumeNotFound) {
				statusErr = errors.Errorf(
					"volume %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		default:
			statusErr = apiservererrors.ErrPerm
		}
		result.Results[i].Error = apiservererrors.ServerError(statusErr)
	}
	return result, nil
}
