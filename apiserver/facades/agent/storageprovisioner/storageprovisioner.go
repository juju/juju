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
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
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
	removalService             RemovalService
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
	removalService RemovalService,
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

	canAccessStorageMachine := func(ctx context.Context, tag names.MachineTag) bool {
		authEntityTag := authorizer.GetAuthTag()
		if tag == authEntityTag {
			// Machine agents can access volumes
			// scoped to their own machine.
			return true
		}
		parentTag := tag.Parent()
		if parentTag == nil {
			return authorizer.AuthController()
		}
		// All containers with the authenticated
		// machine as a parent are accessible by it.
		return parentTag == authEntityTag
	}
	getScopeAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			switch tag := tag.(type) {
			case names.ModelTag:
				// Controllers can access all volumes
				// and file systems scoped to the environment.
				isModelManager := authorizer.AuthController()
				return isModelManager && tag == names.NewModelTag(modelUUID.String())
			case names.MachineTag:
				return canAccessStorageMachine(ctx, tag)
			default:
				return false
			}
		}, nil
	}
	canAccessStorageEntity := func(ctx context.Context, tag names.Tag) bool {
		switch tag := tag.(type) {
		case names.VolumeTag:
			exists, err := storageProvisioningService.CheckVolumeForIDExists(
				ctx, tag.Id(),
			)
			if err != nil {
				logger.Errorf(ctx, "volume auth failed: %q", err)
				return false
			}
			return exists || authorizer.AuthController()
		case names.FilesystemTag:
			exists, err := storageProvisioningService.CheckFilesystemForIDExists(
				ctx, tag.Id(),
			)
			if err != nil {
				logger.Errorf(ctx, "filesystem auth failed: %q", err)
				return false
			}
			return exists || authorizer.AuthController()
		default:
			return false
		}
	}
	getStorageEntityAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return canAccessStorageEntity(ctx, tag)
		}, nil
	}
	getLifeAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			switch tag := tag.(type) {
			case names.MachineTag:
				return canAccessStorageMachine(ctx, tag)
			default:
				return canAccessStorageEntity(ctx, tag)
			}
		}, nil
	}
	getAttachmentAuthFunc := func(ctx context.Context) (func(names.Tag, names.Tag) bool, error) {
		// getAttachmentAuthFunc returns a function that validates
		// access by the authenticated user to an attachment.
		return func(hostTag names.Tag, attachmentTag names.Tag) bool {
			var machineTag names.MachineTag
			switch tag := hostTag.(type) {
			case names.MachineTag:
				machineTag = tag
			case names.UnitTag:
				return authorizer.AuthController()
			default:
				return false
			}
			if !canAccessStorageMachine(ctx, machineTag) {
				return false
			}
			return canAccessStorageEntity(ctx, attachmentTag)
		}, nil
	}
	getMachineAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(ctx, tag)
			}
			return false
		}, nil
	}
	getBlockDevicesAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag, ok := tag.(names.MachineTag); ok {
				return canAccessStorageMachine(ctx, tag)
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
		removalService:             removalService,
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
		machineUUID, err := s.getMachineUUID(ctx, machineTag)
		if err != nil {
			return "", err
		}
		w, err := s.blockDeviceService.WatchBlockDevicesForMachine(
			ctx, machineUUID)
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
	canAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil {
		return results, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	one := func(arg params.Entity) (string, error) {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return "", err
		}
		if !canAccess(machineTag) {
			return "", apiservererrors.ErrPerm
		}
		machineUUID, err := s.getMachineUUID(ctx, machineTag)
		if err != nil {
			return "", err
		}
		w, err := s.machineService.WatchMachineCloudInstances(ctx, machineUUID)
		if err != nil {
			return "", err
		}
		id, _, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
		return id, err
	}
	for i, arg := range args.Entities {
		id, err := one(arg)
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
func (s *StorageProvisionerAPIv4) WatchVolumes(
	ctx context.Context, args params.Entities,
) (params.StringsWatchResults, error) {
	return s.watchStorageEntities(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedVolumes,
		s.storageProvisioningService.WatchMachineProvisionedVolumes,
	)
}

// WatchFilesystems watches for changes to filesystems scoped
// to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchFilesystems(
	ctx context.Context, args params.Entities,
) (params.StringsWatchResults, error) {
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
			machineUUID, err = s.getMachineUUID(ctx, tag)
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
func (s *StorageProvisionerAPIv4) WatchVolumeAttachments(
	ctx context.Context, args params.Entities,
) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedVolumeAttachments,
		s.storageProvisioningService.WatchMachineProvisionedVolumeAttachments,
		s.watchVolumeAttachmentsMapper,
	)
}

// watchVolumeAttachmentsMapper is the mapper function for the mapping of volume
// attachment UUIDs to machine/unit tags and filesystem tag. It is used by the
// WatchVolumeAttachments facade method.
func (s *StorageProvisionerAPIv4) watchVolumeAttachmentsMapper(
	ctx context.Context, volumeAttachmentUUIDs ...string,
) ([]corewatcher.MachineStorageID, error) {
	if len(volumeAttachmentUUIDs) == 0 {
		return nil, nil
	}
	attachmentIDs, err := s.storageProvisioningService.GetVolumeAttachmentIDs(
		ctx, volumeAttachmentUUIDs)
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
			machineStorageId.MachineTag = names.NewMachineTag(
				id.MachineName.String()).String()
			out = append(out, machineStorageId)
			continue
		}
		if !names.IsValidUnit(id.UnitName.String()) {
			// This should never happen.
			s.logger.Errorf(ctx,
				"invalid unit name %q for volume ID %v",
				id.UnitName.String(), id.VolumeID,
			)
			continue
		}
		machineStorageId.MachineTag = names.NewUnitTag(
			id.UnitName.String()).String()
		out = append(out, machineStorageId)
	}
	return out, nil
}

// WatchFilesystemAttachments watches for changes to filesystem attachments
// scoped to the entity with the tag passed to NewState.
func (s *StorageProvisionerAPIv4) WatchFilesystemAttachments(
	ctx context.Context, args params.Entities,
) (params.MachineStorageIdsWatchResults, error) {
	return s.watchAttachments(
		ctx, args,
		s.storageProvisioningService.WatchModelProvisionedFilesystemAttachments,
		s.storageProvisioningService.WatchMachineProvisionedFilesystemAttachments,
		s.watchFilesystemAttachmentsMapper,
	)
}

// watchFilesystemAttachmentsMapper is the mapper function for the mapping of
// filesystem attachment UUIDs to machine/unit tags and filesystem tag. It is
// used by the WatchFilesystemAttachments facade method.
func (s *StorageProvisionerAPIv4) watchFilesystemAttachmentsMapper(
	ctx context.Context, filesystemAttachmentUUIDs ...string,
) ([]corewatcher.MachineStorageID, error) {
	if len(filesystemAttachmentUUIDs) == 0 {
		return nil, nil
	}
	attachmentIDs, err := s.storageProvisioningService.GetFilesystemAttachmentIDs(
		ctx, filesystemAttachmentUUIDs)
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
			s.logger.Errorf(
				ctx, "invalid filesystem tag ID %q", id.FilesystemID)
			continue
		}
		machineStorageId := corewatcher.MachineStorageID{
			AttachmentTag: names.NewFilesystemTag(
				id.FilesystemID).String(),
		}
		if id.MachineName != nil {
			machineStorageId.MachineTag = names.NewMachineTag(
				id.MachineName.String()).String()
			out = append(out, machineStorageId)
			continue
		}
		if !names.IsValidUnit(id.UnitName.String()) {
			// This should never happen.
			s.logger.Errorf(
				ctx, "invalid unit name %q for filesystem ID %q",
				id.UnitName.String(), id.FilesystemID,
			)
			continue
		}
		machineStorageId.MachineTag = names.NewUnitTag(
			id.UnitName.String()).String()
		out = append(out, machineStorageId)
	}
	return out, nil
}

// WatchVolumeAttachmentPlans watches for changes to volume attachments for a
// machine for the purpose of allowing that machine to run any initialization
// needed, for that volume to actually appear as a block device (ie: iSCSI)
func (s *StorageProvisionerAPIv4) WatchVolumeAttachmentPlans(
	ctx context.Context, args params.Entities,
) (params.MachineStorageIdsWatchResults, error) {
	canAccess, err := s.getMachineAuthFunc(ctx)
	if err != nil {
		return params.MachineStorageIdsWatchResults{}, apiservererrors.ServerError(
			apiservererrors.ErrPerm)
	}
	results := params.MachineStorageIdsWatchResults{
		Results: make([]params.MachineStorageIdsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []corewatcher.MachineStorageID, error) {
		machineTag, err := names.ParseMachineTag(arg.Tag)
		if err != nil {
			return "", nil, errors.New(
				"machine tag invalid",
			).Add(coreerrors.NotValid)
		}
		if !canAccess(machineTag) {
			return "", nil, apiservererrors.ErrPerm
		}
		return s.watchVolumeAttachmentPlans(ctx, machineTag)
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

// watchVolumeAttachmentPlans performs operations required for the corresponding
// facade method WatchVolumeAttachmentPlans.
func (s *StorageProvisionerAPIv4) watchVolumeAttachmentPlans(
	ctx context.Context, machineTag names.MachineTag,
) (string, []corewatcher.MachineStorageID, error) {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return "", nil, errors.Capture(err)
	}
	sourceWatcher, err := s.storageProvisioningService.WatchVolumeAttachmentPlans(
		ctx, machineUUID)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", nil, errors.Errorf(
			"machine %q not found", machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", nil, errors.Capture(err)
	}
	mapper := func(
		ctx context.Context, volumeIDs ...string,
	) ([]corewatcher.MachineStorageID, error) {
		if len(volumeIDs) == 0 {
			return nil, nil
		}
		out := make([]corewatcher.MachineStorageID, 0, len(volumeIDs))
		for _, volumeID := range volumeIDs {
			if !names.IsValidVolume(volumeID) {
				// This should never happen.
				s.logger.Errorf(ctx, "invalid volume tag ID %q", volumeID)
				continue
			}
			out = append(out, corewatcher.MachineStorageID{
				MachineTag:    machineTag.String(),
				AttachmentTag: names.NewVolumeTag(volumeID).String(),
			})
		}
		return out, nil
	}
	w, err := newStringSourcedWatcher(sourceWatcher, mapper)
	if err != nil {
		return "", nil, errors.Capture(err)
	}
	id, changes, err := internal.EnsureRegisterWatcher(
		ctx, s.watcherRegistry, w)
	if err != nil {
		return "", nil, errors.Capture(err)
	}
	return id, changes, nil
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
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 0, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) error {
		tag, err := names.ParseVolumeTag(arg.AttachmentTag)
		if err != nil {
			return errors.Errorf(
				"volume tag %q invalid", arg.AttachmentTag,
			).Add(coreerrors.NotValid)
		}
		machineTag, err := names.ParseMachineTag(arg.MachineTag)
		if err != nil {
			return errors.Errorf(
				"machine tag %q invalid", arg.MachineTag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(machineTag, tag) {
			return apiservererrors.ErrPerm
		}
		return s.removeVolumeAttachmentPlan(ctx, machineTag, tag)
	}
	for _, arg := range args.Ids {
		var result params.ErrorResult
		err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// removeVolumeAttachmentPlan handles the volume attachment plan removal
// operations for the corresponding facade method RemoveVolumeAttachmentPlan.
func (s *StorageProvisionerAPIv4) removeVolumeAttachmentPlan(
	ctx context.Context, machineTag names.MachineTag, tag names.VolumeTag,
) error {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return err
	}
	volumeAttachmentPlanUUID, err := s.getVolumeAttachmentPlanUUID(
		ctx, tag, machineUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return nil
	} else if err != nil {
		return err
	}
	err = s.removalService.MarkVolumeAttachmentPlanAsDead(
		ctx, volumeAttachmentPlanUUID)
	if errors.Is(err, removalerrors.EntityStillAlive) {
		return errors.Errorf(
			"volume %q attachment plan for machine %q is still alive",
			tag.Id(), machineTag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
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
			machineUUID, err = s.getMachineUUID(ctx, tag)
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
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.VolumeResults{}, apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	one := func(arg params.Entity) (params.Volume, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil {
			return params.Volume{}, errors.Errorf(
				"volume tag %q invalid", arg.Tag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(tag) {
			return params.Volume{}, apiservererrors.ErrPerm
		}
		vol, err := s.storageProvisioningService.GetVolumeByID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			return params.Volume{}, errors.Errorf(
				"volume %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		}
		if err != nil {
			return params.Volume{}, errors.Errorf(
				"getting volume %q: %v", tag.Id(), err,
			)
		}
		if vol.SizeMiB == 0 {
			// TODO: We think that a volume with size 0 is not provisioned.
			// The size is set when the storage provisioner worker calls SetVolumeInfo.
			// This is a temporary workaround for checking the provision state of the volume.
			// Ideally, we should have a consistent way to check provisioning status
			// for all storage entities.
			return params.Volume{}, errors.Errorf(
				"volume %q is not provisioned", tag.Id(),
			).Add(coreerrors.NotProvisioned)
		}
		result := params.Volume{
			VolumeTag: tag.String(),
			Info: params.VolumeInfo{
				ProviderId: vol.ProviderID,
				HardwareId: vol.HardwareID,
				WWN:        vol.WWN,
				Persistent: vol.Persistent,
				SizeMiB:    vol.SizeMiB,
			},
		}
		return result, nil
	}

	results := params.VolumeResults{
		Results: make([]params.VolumeResult, len(args.Entities)),
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
				SizeMiB:    fs.SizeMiB,
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

// getVolumeAttachmentPlanUUID is a utility function that translates a facade
// request that is for a volume attachment plan into the correct attachment
// plan uuid.
//
// This func allows all users of this value to share a common set of error
// handling logic and get to the desired value quicker. The error returned from
// this func has been converted to an error understood by this facade caller and
// requires no further translation.
func (s *StorageProvisionerAPIv4) getVolumeAttachmentPlanUUID(
	ctx context.Context,
	volumeTag names.VolumeTag,
	machineUUID machine.UUID,
) (storageprovisioning.VolumeAttachmentPlanUUID, error) {
	vapUUID, err := s.storageProvisioningService.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		ctx, volumeTag.Id(), machineUUID,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		return "", errors.Errorf(
			"volume attachment plan %q on machine %q not found",
			volumeTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q not found", volumeTag.Id(),
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineUUID,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting volume attachment plan %q on machine %q: %v",
			volumeTag.Id(), machineUUID, err,
		)
	}
	return vapUUID, nil
}

// VolumeAttachmentPlans returns details of volume attachment plans with the
// specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachmentPlans(
	ctx context.Context, args params.MachineStorageIds,
) (params.VolumeAttachmentPlanResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.VolumeAttachmentPlanResults{},
			apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	one := func(id params.MachineStorageId) (params.VolumeAttachmentPlan, error) {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			return params.VolumeAttachmentPlan{}, errors.New(
				"volume tag invalid",
			).Add(coreerrors.NotValid)
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			return params.VolumeAttachmentPlan{}, errors.New(
				"machine tag invalid",
			).Add(coreerrors.NotValid)
		}
		if !canAccess(machineTag, volumeTag) {
			return params.VolumeAttachmentPlan{}, apiservererrors.ErrPerm
		}
		return s.volumeAttachmentPlan(ctx, machineTag, volumeTag)
	}

	results := params.VolumeAttachmentPlanResults{
		Results: make([]params.VolumeAttachmentPlanResult, len(args.Ids)),
	}

	for i, arg := range args.Ids {
		var result params.VolumeAttachmentPlanResult
		vap, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = vap
		}
		results.Results[i] = result
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) volumeAttachmentPlan(
	ctx context.Context,
	machineTag names.MachineTag,
	volumeTag names.VolumeTag,
) (params.VolumeAttachmentPlan, error) {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return params.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	planUUID, err := s.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
	if err != nil {
		return params.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	vap, err := s.storageProvisioningService.GetVolumeAttachmentPlan(
		ctx, planUUID,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		return params.VolumeAttachmentPlan{}, errors.Errorf(
			"volume attachment plan %q on machine %q not found",
			volumeTag.Id(), machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.VolumeAttachmentPlan{}, errors.Errorf(
			"getting volume attachment plan %q on machine %q: %v",
			volumeTag.Id(), machineTag.Id(), err,
		)
	}

	life, err := vap.Life.Value()
	if err != nil {
		return params.VolumeAttachmentPlan{}, errors.Errorf(
			"getting volume attachment plan %q life: %w",
			planUUID, err,
		)
	}

	var deviceType storage.DeviceType
	switch vap.DeviceType {
	case storageprovisioning.PlanDeviceTypeISCSI:
		deviceType = storage.DeviceTypeISCSI
	case storageprovisioning.PlanDeviceTypeLocal:
		deviceType = storage.DeviceTypeLocal
	default:
		return params.VolumeAttachmentPlan{}, errors.Errorf(
			"unknown device type %q", vap.DeviceType,
		)
	}

	return params.VolumeAttachmentPlan{
		VolumeTag:  volumeTag.String(),
		MachineTag: machineTag.String(),
		Life:       life,
		PlanInfo: params.VolumeAttachmentPlanInfo{
			DeviceAttributes: vap.DeviceAttributes,
			DeviceType:       deviceType,
		},
	}, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeAttachments(
	ctx context.Context, args params.MachineStorageIds,
) (params.VolumeAttachmentResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.VolumeAttachmentResults{},
			apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	one := func(id params.MachineStorageId) (params.VolumeAttachment, error) {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			return params.VolumeAttachment{}, errors.New(
				"volume tag invalid",
			).Add(coreerrors.NotValid)
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			return params.VolumeAttachment{}, errors.New(
				"machine tag invalid",
			).Add(coreerrors.NotValid)
		}
		if !canAccess(machineTag, volumeTag) {
			return params.VolumeAttachment{}, apiservererrors.ErrPerm
		}
		return s.volumeAttachments(ctx, machineTag, volumeTag)
	}

	results := params.VolumeAttachmentResults{
		Results: make([]params.VolumeAttachmentResult, len(args.Ids)),
	}
	for i, arg := range args.Ids {
		var result params.VolumeAttachmentResult
		va, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = va
		}
		results.Results[i] = result
	}
	return results, nil
}

func (s *StorageProvisionerAPIv4) volumeAttachments(
	ctx context.Context, machineTag names.MachineTag, volumeTag names.VolumeTag,
) (params.VolumeAttachment, error) {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return params.VolumeAttachment{}, errors.Capture(err)
	}

	uuid, err := s.getVolumeAttachmentUUID(ctx, volumeTag, machineUUID)
	if err != nil {
		return params.VolumeAttachment{}, errors.Capture(err)
	}

	va, err := s.storageProvisioningService.GetVolumeAttachment(ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return params.VolumeAttachment{}, errors.Errorf(
			"volume attachment %q on machine %q not found",
			volumeTag.Id(), machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.VolumeAttachment{}, errors.Errorf(
			"getting volume attachment %q on machine %q: %v",
			volumeTag.Id(), machineTag.Id(), err,
		)
	}

	if len(va.BlockDeviceLinks) == 0 {
		// TODO: We think that a volume attachment with no device link is
		// not provisioned. The property is set when the storage provisioner
		// calls SetVolumeAttachmentInfo. This is a temporary workaround for
		// checking the provision state of an attachment.Ideally, we should
		// have a consistent way to check provisioning status for all
		// storage entities.
		return params.VolumeAttachment{}, errors.Errorf(
			"volume %q is not provisioned", volumeTag.Id(),
		).Add(coreerrors.NotProvisioned)
	}

	result := params.VolumeAttachment{
		VolumeTag:  volumeTag.String(),
		MachineTag: machineTag.String(),
		Info: params.VolumeAttachmentInfo{
			DeviceName: va.BlockDeviceName,
			DeviceLink: domainblockdevice.IDLink(va.BlockDeviceLinks),
			BusAddress: va.BlockDeviceBusAddress,
			ReadOnly:   va.ReadOnly,
			// PlanInfo is only used by a storage provisioner to set the
			// plan info, not to read it.
		},
	}
	return result, nil
}

// VolumeBlockDevices returns details of the block devices corresponding to the
// volume attachments with the specified IDs.
func (s *StorageProvisionerAPIv4) VolumeBlockDevices(ctx context.Context, args params.MachineStorageIds) (params.BlockDeviceResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.BlockDeviceResults{},
			apiservererrors.ServerError(apiservererrors.ErrPerm)
	}

	one := func(id params.MachineStorageId) (params.BlockDevice, error) {
		tag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			return params.BlockDevice{}, errors.Errorf(
				"volume tag %q invalid", id.AttachmentTag,
			).Add(coreerrors.NotValid)
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			return params.BlockDevice{}, errors.Errorf(
				"machine tag %q invalid", id.MachineTag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(tag) {
			return params.BlockDevice{}, apiservererrors.ErrPerm
		}

		machineUUID, err := s.getMachineUUID(ctx, machineTag)
		if err != nil {
			return params.BlockDevice{}, errors.Capture(err)
		}

		va, err := s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDMachine(
			ctx, tag.Id(), machineUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return params.BlockDevice{}, errors.Errorf(
				"volume attachment %q on machine %q not found",
				tag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			return params.BlockDevice{}, errors.Errorf(
				"volume %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		} else if errors.Is(err, machineerrors.MachineNotFound) {
			return params.BlockDevice{}, errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return params.BlockDevice{}, errors.Errorf(
				"getting volume attachment %q on machine %q: %v",
				tag.Id(), machineTag.Id(), err,
			)
		}

		bdUUID, err := s.storageProvisioningService.GetBlockDeviceForVolumeAttachment(
			ctx, va)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice) {
			return params.BlockDevice{}, errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				tag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned)
		} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return params.BlockDevice{}, errors.Errorf(
				"volume attachment %q on machine %q not found",
				tag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return params.BlockDevice{}, errors.Errorf(
				"getting volume attachment %q on machine %q: %v",
				tag.Id(), machineTag.Id(), err,
			)
		}

		bd, err := s.blockDeviceService.GetBlockDevice(ctx, bdUUID)
		if errors.Is(err, blockdeviceerrors.BlockDeviceNotFound) {
			return params.BlockDevice{}, errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				tag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned)
		} else if err != nil {
			return params.BlockDevice{}, errors.Errorf(
				"getting block device %q: %v", bdUUID, err,
			)
		}

		if len(bd.DeviceLinks) == 0 {
			// TODO: We think that a block device with no device links is
			// not provisioned. The property is set when the storage provisioner
			// calls SetVolumeAttachmentInfo. This is a temporary workaround for
			// checking the provision state of an attachment.Ideally, we should
			// have a consistent way to check provisioning status for all
			// storage entities.
			return params.BlockDevice{}, errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				tag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned)
		}

		result := params.BlockDevice{
			DeviceName:     bd.DeviceName,
			DeviceLinks:    bd.DeviceLinks,
			Label:          bd.FilesystemLabel,
			UUID:           bd.FilesystemUUID,
			HardwareId:     bd.HardwareId,
			WWN:            bd.WWN,
			BusAddress:     bd.BusAddress,
			SizeMiB:        bd.SizeMiB,
			FilesystemType: bd.FilesystemType,
			InUse:          bd.InUse,
			MountPoint:     bd.MountPoint,
			SerialId:       bd.SerialId,
		}
		return result, nil
	}

	results := params.BlockDeviceResults{
		Results: make([]params.BlockDeviceResult, len(args.Ids)),
	}
	for i, arg := range args.Ids {
		var result params.BlockDeviceResult
		bd, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = bd
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
			machineUUID, err = s.getMachineUUID(ctx, tag)
			if err != nil {
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
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.VolumeParamsResults{}, err
	}

	results := params.VolumeParamsResults{
		Results: make([]params.VolumeParamsResult, 0, len(args.Entities)),
	}

	var volModelTags map[string]string
	one := func(arg params.Entity) (params.VolumeParams, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.VolumeParams{}, apiservererrors.ErrPerm
		}

		if volModelTags == nil {
			volModelTags, err = s.storageProvisioningService.
				GetStorageResourceTagsForModel(ctx)
			if err != nil {
				return params.VolumeParams{}, errors.Errorf(
					"getting volume model tags: %w", err,
				)
			}
		}

		uuid, err := s.storageProvisioningService.GetVolumeUUIDForID(
			ctx, tag.Id(),
		)
		if err != nil {
			return params.VolumeParams{}, err
		}

		volParams, err := s.storageProvisioningService.GetVolumeParams(
			ctx, uuid,
		)
		if err != nil {
			return params.VolumeParams{}, err
		}

		rval := params.VolumeParams{
			Attributes: make(map[string]any, len(volParams.Attributes)),
			VolumeTag:  tag.String(),
			Provider:   volParams.Provider,
			SizeMiB:    volParams.SizeMiB,
			Tags:       volModelTags,
		}
		for k, v := range volParams.Attributes {
			rval.Attributes[k] = v
		}

		if volParams.VolumeAttachmentUUID == nil {
			return rval, nil
		}

		vaParams, err := s.storageProvisioningService.GetVolumeAttachmentParams(
			ctx, *volParams.VolumeAttachmentUUID,
		)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return rval, nil
		} else if err != nil {
			return params.VolumeParams{}, errors.Capture(err)
		}

		rval.Attachment = &params.VolumeAttachmentParams{
			VolumeTag:  tag.String(),
			InstanceId: vaParams.MachineInstanceID,
			Provider:   vaParams.Provider,
			ProviderId: vaParams.ProviderID,
			ReadOnly:   vaParams.ReadOnly,
		}
		if vaParams.Machine != nil {
			rval.Attachment.MachineTag = names.NewMachineTag(
				vaParams.Machine.String(),
			).String()
		}

		return rval, nil
	}

	for _, arg := range args.Entities {
		var result params.VolumeParamsResult
		volumeParams, err := one(arg)
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			err = apiservererrors.ErrPerm
		}

		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeParams
		}
		results.Results = append(results.Results, result)
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
		Results: make([]params.RemoveVolumeParamsResult, 0, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.RemoveVolumeParams, error) {
		tag, err := names.ParseVolumeTag(arg.Tag)
		if err != nil {
			return params.RemoveVolumeParams{}, errors.Errorf(
				"volume tag %q invalid", arg.Tag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(tag) {
			return params.RemoveVolumeParams{}, apiservererrors.ErrPerm
		}
		return s.removeVolumeParams(ctx, tag)
	}
	for _, arg := range args.Entities {
		var result params.RemoveVolumeParamsResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// removeVolumeParams performs operations required for the corresponding
// facade method RemoveVolumeParams.
func (s *StorageProvisionerAPIv4) removeVolumeParams(
	ctx context.Context, tag names.VolumeTag,
) (params.RemoveVolumeParams, error) {
	uuid, err := s.storageProvisioningService.GetVolumeUUIDForID(
		ctx, tag.Id(),
	)
	if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return params.RemoveVolumeParams{}, errors.Errorf(
			"volume %q not found", tag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.RemoveVolumeParams{}, err
	}

	rp, err := s.storageProvisioningService.GetVolumeRemovalParams(
		ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
		return params.RemoveVolumeParams{}, errors.Errorf(
			"volume %q is not yet dead", tag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return params.RemoveVolumeParams{}, errors.Errorf(
			"volume %q not found", tag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.RemoveVolumeParams{}, err
	}

	return params.RemoveVolumeParams{
		Provider:   rp.Provider,
		ProviderId: rp.ProviderID,
		Destroy:    rp.Obliterate,
	}, nil
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

	var fsModelTags map[string]string
	one := func(arg params.Entity) (params.FilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil || !canAccess(tag) {
			return params.FilesystemParams{}, apiservererrors.ErrPerm
		}

		if fsModelTags == nil {
			fsModelTags, err = s.storageProvisioningService.
				GetStorageResourceTagsForModel(ctx)
			if err != nil {
				return params.FilesystemParams{}, errors.Errorf(
					"getting filesystem model tags: %w", err,
				)
			}
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
			// Attachment params have never been set.
			Attributes:    make(map[string]any, len(fsParams.Attributes)),
			FilesystemTag: tag.String(),
			Provider:      fsParams.Provider,
			SizeMiB:       fsParams.SizeMiB,
			Tags:          fsModelTags,
		}
		for k, v := range fsParams.Attributes {
			rval.Attributes[k] = v
		}

		// If this fs is backed by a volume, pass that along.
		if fsParams.BackingVolume != nil {
			rval.VolumeTag = names.NewVolumeTag(
				fsParams.BackingVolume.VolumeID,
			).String()
		}

		return rval, nil
	}

	for _, arg := range args.Entities {
		var result params.FilesystemParamsResult
		filesystemParams, err := one(arg)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			err = apiservererrors.ErrPerm
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
func (s *StorageProvisionerAPIv4) RemoveFilesystemParams(
	ctx context.Context, args params.Entities,
) (params.RemoveFilesystemParamsResults, error) {
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.RemoveFilesystemParamsResults{}, err
	}
	results := params.RemoveFilesystemParamsResults{
		Results: make([]params.RemoveFilesystemParamsResult, 0, len(args.Entities)),
	}
	one := func(arg params.Entity) (params.RemoveFilesystemParams, error) {
		tag, err := names.ParseFilesystemTag(arg.Tag)
		if err != nil {
			return params.RemoveFilesystemParams{}, errors.Errorf(
				"filesystem tag %q invalid", arg.Tag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(tag) {
			return params.RemoveFilesystemParams{}, apiservererrors.ErrPerm
		}
		return s.removeFilesystemParams(ctx, tag)
	}
	for _, arg := range args.Entities {
		var result params.RemoveFilesystemParamsResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// removeFilesystemParams performs operations required for the corresponding
// facade method RemoveFilesystemParams.
func (s *StorageProvisionerAPIv4) removeFilesystemParams(
	ctx context.Context, tag names.FilesystemTag,
) (params.RemoveFilesystemParams, error) {
	uuid, err := s.storageProvisioningService.GetFilesystemUUIDForID(
		ctx, tag.Id(),
	)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return params.RemoveFilesystemParams{}, errors.Errorf(
			"filesystem %q not found", tag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.RemoveFilesystemParams{}, err
	}

	rp, err := s.storageProvisioningService.GetFilesystemRemovalParams(
		ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
		return params.RemoveFilesystemParams{}, errors.Errorf(
			"filesystem %q is not yet dead", tag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		return params.RemoveFilesystemParams{}, errors.Errorf(
			"filesystem %q not found", tag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return params.RemoveFilesystemParams{}, err
	}

	return params.RemoveFilesystemParams{
		Provider:   rp.Provider,
		ProviderId: rp.ProviderID,
		Destroy:    rp.Obliterate,
	}, nil
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
		Results: make([]params.VolumeAttachmentParamsResult, 0, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) (params.VolumeAttachmentParams, error) {
		machineTag, err := names.ParseMachineTag(arg.MachineTag)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Errorf(
				"parsing machine tag: %w", err,
			)
		}
		volumeTag, err := names.ParseVolumeTag(arg.AttachmentTag)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Errorf(
				"parsing volume tag: %w", err,
			)
		}
		if !canAccess(machineTag, volumeTag) {
			return params.VolumeAttachmentParams{}, apiservererrors.ErrPerm
		}

		machineUUID, err := s.getMachineUUID(ctx, machineTag)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Capture(err)
		}

		attachmentUUID, err := s.getVolumeAttachmentUUID(
			ctx, volumeTag, machineUUID,
		)
		if err != nil {
			return params.VolumeAttachmentParams{}, errors.Capture(err)
		}

		volParams, err := s.storageProvisioningService.GetVolumeAttachmentParams(
			ctx, attachmentUUID,
		)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			return params.VolumeAttachmentParams{}, errors.Errorf(
				"volume attachment for volume %q and host %q not found",
				volumeTag, machineTag,
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return params.VolumeAttachmentParams{}, errors.Capture(err)
		}

		return params.VolumeAttachmentParams{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			InstanceId: volParams.MachineInstanceID,
			Provider:   volParams.Provider,
			ProviderId: volParams.ProviderID,
			ReadOnly:   volParams.ReadOnly,
		}, nil
	}
	for _, arg := range args.Ids {
		var result params.VolumeAttachmentParamsResult
		volumeAttachment, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.Result = volumeAttachment
		}
		results.Results = append(results.Results, result)
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
		if err != nil {
			return params.FilesystemAttachmentParams{}, errors.Capture(err)
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
			s.logger.Warningf(ctx,
				"TODO(storage): check fs volume tag matches fs vol back")
		}
		info := storageprovisioning.FilesystemProvisionedInfo{
			ProviderID: fs.Info.ProviderId,
			SizeMiB:    fs.Info.SizeMiB,
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

func (s *StorageProvisionerAPIv4) CreateVolumeAttachmentPlans(
	ctx context.Context, args params.VolumeAttachmentPlans,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(
			apiservererrors.ErrPerm)
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachmentPlans)),
	}
	one := func(vp params.VolumeAttachmentPlan) error {
		machineTag, err := names.ParseMachineTag(vp.MachineTag)
		if err != nil {
			return errors.Errorf("parsing machine tag: %w", err)
		}
		volumeTag, err := names.ParseVolumeTag(vp.VolumeTag)
		if err != nil {
			return errors.Errorf("parsing volume tag: %w", err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		if vp.BlockDevice != nil {
			return errors.New("block device field must not be set")
		}
		return s.createVolumeAttachmentPlan(
			ctx, machineTag, volumeTag, vp.PlanInfo)
	}
	for i, vp := range args.VolumeAttachmentPlans {
		err := one(vp)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// createVolumeAttachmentPlan performs operations required for the corresponding
// facade method CreateVolumeAttachmentPlans.
func (s *StorageProvisionerAPIv4) createVolumeAttachmentPlan(
	ctx context.Context,
	machineTag names.MachineTag,
	volumeTag names.VolumeTag,
	planInfo params.VolumeAttachmentPlanInfo,
) error {
	var planDeviceType storageprovisioning.PlanDeviceType
	switch planInfo.DeviceType {
	case storage.DeviceTypeISCSI:
		planDeviceType = storageprovisioning.PlanDeviceTypeISCSI
	case storage.DeviceTypeLocal:
		planDeviceType = storageprovisioning.PlanDeviceTypeLocal
	default:
		return errors.Errorf(
			"plan device type %q not valid", planInfo.DeviceType,
		).Add(coreerrors.NotValid)
	}

	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return errors.Capture(err)
	}

	attachmentUUID, err := s.getVolumeAttachmentUUID(
		ctx, volumeTag, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	_, err = s.storageProvisioningService.CreateVolumeAttachmentPlan(
		ctx, attachmentUUID, planDeviceType, planInfo.DeviceAttributes,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return errors.Errorf(
			"volume attachment for machine %q and volume %q not found",
			machineTag.Id(), volumeTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (s *StorageProvisionerAPIv4) SetVolumeAttachmentPlanBlockInfo(
	ctx context.Context, args params.VolumeAttachmentPlans,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(
			apiservererrors.ErrPerm)
	}

	one := func(vp params.VolumeAttachmentPlan) error {
		machineTag, err := names.ParseMachineTag(vp.MachineTag)
		if err != nil {
			return errors.Errorf("parsing machine tag: %w", err)
		}
		volumeTag, err := names.ParseVolumeTag(vp.VolumeTag)
		if err != nil {
			return errors.Errorf("parsing volume tag: %w", err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		if vp.BlockDevice == nil {
			return nil
		}
		return s.setVolumeAttachmentPlanBlockInfo(
			ctx, machineTag, volumeTag, *vp.BlockDevice)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachmentPlans)),
	}

	for i, vp := range args.VolumeAttachmentPlans {
		err := one(vp)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}

	return results, nil
}

// setVolumeAttachmentPlanBlockInfo performs operations required for the
// corresponding facade method SetVolumeAttachmentPlanBlockInfo.
func (s *StorageProvisionerAPIv4) setVolumeAttachmentPlanBlockInfo(
	ctx context.Context,
	machineTag names.MachineTag,
	volumeTag names.VolumeTag,
	bd params.BlockDevice,
) error {
	blockDevice := blockdevice.BlockDevice{
		DeviceName:      bd.DeviceName,
		DeviceLinks:     bd.DeviceLinks,
		FilesystemLabel: bd.Label,
		FilesystemUUID:  bd.UUID,
		HardwareId:      bd.HardwareId,
		WWN:             bd.WWN,
		BusAddress:      bd.BusAddress,
		SizeMiB:         bd.SizeMiB,
		FilesystemType:  bd.FilesystemType,
		InUse:           bd.InUse,
		MountPoint:      bd.MountPoint,
		SerialId:        bd.SerialId,
	}
	if domainblockdevice.IsEmpty(blockDevice) {
		return nil
	}

	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return errors.Capture(err)
	}

	planUUID, err := s.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	blockDeviceUUID, err := s.blockDeviceService.MatchOrCreateBlockDevice(
		ctx, machineUUID, blockDevice)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return errors.Errorf(
			"machine %q not found", machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return errors.Capture(err)
	}

	err = s.storageProvisioningService.SetVolumeAttachmentPlanProvisionedBlockDevice(
		ctx, planUUID, blockDeviceUUID)
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

// SetVolumeAttachmentInfo records the details of newly provisioned volume
// attachments.
func (s *StorageProvisionerAPIv4) SetVolumeAttachmentInfo(
	ctx context.Context,
	args params.VolumeAttachments,
) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, apiservererrors.ServerError(
			apiservererrors.ErrPerm)
	}

	one := func(va params.VolumeAttachment) error {
		machineTag, err := names.ParseMachineTag(va.MachineTag)
		if err != nil {
			return errors.Errorf("parsing machine tag: %w", err)
		}
		volumeTag, err := names.ParseVolumeTag(va.VolumeTag)
		if err != nil {
			return errors.Errorf("parsing volume tag: %w", err)
		}
		if !canAccess(machineTag, volumeTag) {
			return apiservererrors.ErrPerm
		}
		return s.setVolumeAttachmentInfo(ctx, machineTag, volumeTag, va.Info)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.VolumeAttachments)),
	}

	for i, va := range args.VolumeAttachments {
		err := one(va)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}

	return results, nil
}

// setVolumeAttachmentInfo performs operations required for the corresponding
// facade method SetVolumeAttachmentInfo.
func (s *StorageProvisionerAPIv4) setVolumeAttachmentInfo(
	ctx context.Context, machineTag names.MachineTag, volumeTag names.VolumeTag,
	vai params.VolumeAttachmentInfo,
) error {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return errors.Capture(err)
	}

	volumeAttachmentUUID, err := s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDMachine(
		ctx, volumeTag.Id(), machineUUID)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return errors.Errorf(
			"volume attachment %q on %q not found",
			volumeTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return errors.Errorf(
			"volume %q not found for attachment on %q",
			volumeTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return errors.Capture(err)
	}

	info := storageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly: vai.ReadOnly,
	}
	if vai.DeviceName != "" ||
		vai.DeviceLink != "" ||
		vai.BusAddress != "" {
		device := blockdevice.BlockDevice{
			DeviceName: vai.DeviceName,
			BusAddress: vai.BusAddress,
		}
		if vai.DeviceLink != "" {
			device.DeviceLinks = []string{vai.DeviceLink}
		}
		blockDevUUID, err := s.blockDeviceService.MatchOrCreateBlockDevice(
			ctx, machineUUID, device)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		info.BlockDeviceUUID = &blockDevUUID
	}

	err = s.storageProvisioningService.SetVolumeAttachmentProvisionedInfo(
		ctx, volumeAttachmentUUID, info,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return errors.Errorf(
			"volume attachment for machine %q and volume %q not found",
			machineTag.Id(), volumeTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return errors.Capture(err)
	}

	if vai.PlanInfo == nil {
		return nil
	}

	planUUID, err := s.getVolumeAttachmentPlanUUID(
		ctx, volumeTag, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	planInfo := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
		DeviceAttributes: vai.PlanInfo.DeviceAttributes,
	}
	switch vai.PlanInfo.DeviceType {
	case storage.DeviceTypeLocal:
		planInfo.DeviceType = storageprovisioning.PlanDeviceTypeLocal
	case storage.DeviceTypeISCSI:
		planInfo.DeviceType = storageprovisioning.PlanDeviceTypeISCSI
	}
	err = s.storageProvisioningService.SetVolumeAttachmentPlanProvisionedInfo(
		ctx, planUUID, planInfo,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		return errors.Errorf(
			"volume attachment plan for machine %q and volume %q not found",
			machineTag.Id(), volumeTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return errors.Errorf("setting volume attachment plan info: %w", err)
	}
	return nil
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
			machineUUID, err = s.getMachineUUID(ctx, tag)
			if err != nil {
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
		machineUUID, err := s.getMachineUUID(ctx, tag)
		if err != nil {
			return "", errors.Capture(err)
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
	ctx context.Context, volTag names.VolumeTag, machineUUID machine.UUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	rval, err := s.storageProvisioningService.GetVolumeAttachmentUUIDForVolumeIDMachine(
		ctx, volTag.Id(), machineUUID,
	)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return "", errors.Errorf(
			"volume attachment %q on %q not found", volTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q not found for attachment on %q", volTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting volume attachment uuid for %q on %q: %w",
			volTag.Id(), machineUUID, err,
		)
	}
	return rval, nil
}

func (s *StorageProvisionerAPIv4) volumeAttachmentLife(
	ctx context.Context, volTag names.VolumeTag, hostTag names.Tag,
) (life.Value, error) {
	machineTag, ok := hostTag.(names.MachineTag)
	if !ok {
		return "", errors.New(
			"volume attachments only supported on machines",
		).Add(coreerrors.NotImplemented)
	}

	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := s.getVolumeAttachmentUUID(ctx, volTag, machineUUID)
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
	canAccess, err := s.getStorageEntityAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 0, len(args.Entities)),
	}
	one := func(arg params.Entity) error {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return errors.Errorf(
				"tag %q invalid", arg.Tag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(tag) {
			return apiservererrors.ErrPerm
		}
		switch t := tag.(type) {
		case names.VolumeTag:
			return s.removeVolume(ctx, t)
		case names.FilesystemTag:
			return s.removeFilesystem(ctx, t)
		}
		return errors.Errorf(
			"tag %q invalid", arg.Tag,
		).Add(coreerrors.NotValid)
	}
	for _, arg := range args.Entities {
		var result params.ErrorResult
		err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// removeVolume handles the volume removal operations for the corresponding
// facade method Remove.
func (s *StorageProvisionerAPIv4) removeVolume(
	ctx context.Context, tag names.VolumeTag,
) error {
	uuid, err := s.storageProvisioningService.GetVolumeUUIDForID(
		ctx, tag.Id(),
	)
	if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		// Already removed.
		return nil
	} else if err != nil {
		return err
	}

	err = s.removalService.RemoveDeadVolume(ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
		return errors.Errorf(
			"volume %q is not yet dead", tag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		// Already removed.
		return nil
	} else if err != nil {
		return err
	}

	return nil
}

// removeFilesystem handles the filesystem removal operations for the
// corresponding facade method Remove.
func (s *StorageProvisionerAPIv4) removeFilesystem(
	ctx context.Context, tag names.FilesystemTag,
) error {
	uuid, err := s.storageProvisioningService.GetFilesystemUUIDForID(
		ctx, tag.Id(),
	)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		// Already removed.
		return nil
	} else if err != nil {
		return err
	}

	err = s.removalService.RemoveDeadFilesystem(ctx, uuid)
	if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
		return errors.Errorf(
			"filesystem %q is not yet dead", tag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
		// Already removed.
		return nil
	} else if err != nil {
		return err
	}

	return nil
}

// RemoveAttachment removes the specified machine storage attachments
// from state.
func (s *StorageProvisionerAPIv4) RemoveAttachment(ctx context.Context, args params.MachineStorageIds) (params.ErrorResults, error) {
	canAccess, err := s.getAttachmentAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 0, len(args.Ids)),
	}
	one := func(arg params.MachineStorageId) error {
		tag, err := names.ParseTag(arg.AttachmentTag)
		if err != nil {
			return errors.Errorf(
				"tag %q invalid", arg.AttachmentTag,
			).Add(coreerrors.NotValid)
		}
		hostTag, err := names.ParseTag(arg.MachineTag)
		if err != nil {
			return errors.Errorf(
				"tag %q invalid", arg.MachineTag,
			).Add(coreerrors.NotValid)
		}
		if !canAccess(hostTag, tag) {
			return apiservererrors.ErrPerm
		}
		switch t := tag.(type) {
		case names.VolumeTag:
			if m, ok := hostTag.(names.MachineTag); ok {
				return s.removeVolumeAttachment(ctx, m, t)
			}
		case names.FilesystemTag:
			return s.removeFilesystemAttachment(ctx, hostTag, t)
		}
		return errors.Errorf(
			"tag %q on host %q invalid", arg.AttachmentTag, arg.MachineTag,
		).Add(coreerrors.NotValid)
	}
	for _, arg := range args.Ids {
		var result params.ErrorResult
		err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		}
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// removeVolumeAttachment handles the volume attachment removal operations
// for the corresponding facade method RemoveAttachment.
func (s *StorageProvisionerAPIv4) removeVolumeAttachment(
	ctx context.Context, machineTag names.MachineTag, tag names.VolumeTag,
) error {
	machineUUID, err := s.getMachineUUID(ctx, machineTag)
	if err != nil {
		return err
	}
	volumeAttachmentUUID, err := s.getVolumeAttachmentUUID(
		ctx, tag, machineUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return nil
	} else if err != nil {
		return err
	}
	err = s.removalService.MarkVolumeAttachmentAsDead(
		ctx, volumeAttachmentUUID)
	if errors.Is(err, removalerrors.EntityStillAlive) {
		return errors.Errorf(
			"volume %q attachment to machine %q is still alive",
			tag.Id(), machineTag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

// removeFilesystemAttachment handles the filesystem attachment removal operations
// for the corresponding facade method RemoveAttachment.
func (s *StorageProvisionerAPIv4) removeFilesystemAttachment(
	ctx context.Context, hostTag names.Tag, tag names.FilesystemTag,
) error {
	filesystemAttachmentUUID, err := s.getFilesystemAttachmentUUID(
		ctx, tag, hostTag)
	if errors.Is(err, coreerrors.NotFound) {
		return nil
	} else if err != nil {
		return err
	}
	err = s.removalService.MarkFilesystemAttachmentAsDead(
		ctx, filesystemAttachmentUUID)
	if errors.Is(err, removalerrors.EntityStillAlive) {
		return errors.Errorf(
			"filesystem %q attachment to %s %q is still alive",
			tag.Id(), hostTag.Kind(), hostTag.Id(),
		)
	} else if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
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

// getMachineUUID is a utility function that gets a machine uuid by tag.
//
// This func allows all users of this value to share a common set of error
// handling logic and get to the desired value quicker. The error returned from
// this func has been converted to an error understood by this facade caller and
// requires no further translation.
func (s *StorageProvisionerAPIv4) getMachineUUID(
	ctx context.Context, machineTag names.MachineTag,
) (machine.UUID, error) {
	machineUUID, err := s.machineService.GetMachineUUID(
		ctx, machine.Name(machineTag.Id()))
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return machineUUID, nil
}
