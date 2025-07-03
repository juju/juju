// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// filesystemsChanged is called when the lifecycle states of the filesystems
// with the provided IDs have been seen to have changed.
func filesystemsChanged(ctx *context, changes []string) error {
	ctx.config.Logger.Debugf("alvin filesystems changed: %+v", changes)
	tags := make([]names.Tag, len(changes))
	for i, change := range changes {
		tags[i] = names.NewFilesystemTag(change)
	}
	alive, dying, dead, err := storageEntityLife(ctx, tags)
	if err != nil {
		return errors.Trace(err)
	}

	ctx.config.Logger.Debugf("filesystems alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(alive)+len(dying)+len(dead) == 0 {
		return nil
	}

	// Get filesystem information for filesystems, so we can provision,
	// deprovision, attach and detach.
	filesystemTags := make([]names.FilesystemTag, 0, len(alive)+len(dying)+len(dead))
	for _, tag := range alive {
		filesystemTags = append(filesystemTags, tag.(names.FilesystemTag))
	}
	for _, tag := range dying {
		filesystemTags = append(filesystemTags, tag.(names.FilesystemTag))
	}
	for _, tag := range dead {
		filesystemTags = append(filesystemTags, tag.(names.FilesystemTag))
	}
	filesystemResults, err := ctx.config.Filesystems.Filesystems(filesystemTags)
	if err != nil {
		return errors.Annotatef(err, "getting filesystem information")
	}
	ctx.config.Logger.Debugf("alvin filesystemResults %+v", filesystemResults)

	aliveFilesystemTags := filesystemTags[:len(alive)]
	dyingFilesystemTags := filesystemTags[len(alive) : len(alive)+len(dying)]
	deadFilesystemTags := filesystemTags[len(alive)+len(dying):]
	aliveFilesystemResults := filesystemResults[:len(alive)]
	dyingFilesystemResults := filesystemResults[len(alive) : len(alive)+len(dying)]
	deadFilesystemResults := filesystemResults[len(alive)+len(dying):]

	if err := processDeadFilesystems(ctx, deadFilesystemTags, deadFilesystemResults); err != nil {
		return errors.Annotate(err, "deprovisioning filesystems")
	}
	if err := processDyingFilesystems(ctx, dyingFilesystemTags, dyingFilesystemResults); err != nil {
		return errors.Annotate(err, "processing dying filesystems")
	}
	if err := processAliveFilesystems(ctx, aliveFilesystemTags, aliveFilesystemResults); err != nil {
		return errors.Annotate(err, "provisioning filesystems")
	}
	return nil
}

// filesystemAttachmentsChanged is called when the lifecycle states of the filesystem
// attachments with the provided IDs have been seen to have changed.
func filesystemAttachmentsChanged(ctx *context, watcherIds []watcher.MachineStorageId) error {
	ctx.config.Logger.Debugf("alvin filesystemAttachmentsChanged changed: %+v", watcherIds)

	ids := copyMachineStorageIds(watcherIds)
	alive, dying, dead, gone, err := attachmentLife(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.config.Logger.Debugf("filesystem attachment alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(dead) != 0 {
		// We should not see dead filesystem attachments;
		// attachments go directly from Dying to removed.
		ctx.config.Logger.Warningf("unexpected dead filesystem attachments: %v", dead)
	}
	// Clean up any attachments which have been removed.
	for _, id := range gone {
		delete(ctx.filesystemAttachments, id)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get filesystem information for alive and dying filesystem attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	filesystemAttachmentResults, err := ctx.config.Filesystems.FilesystemAttachments(ids)
	if err != nil {
		return errors.Annotatef(err, "getting filesystem attachment information")
	}

	// Deprovision Dying filesystem attachments.
	dyingFilesystemAttachmentResults := filesystemAttachmentResults[len(alive):]
	if err := processDyingFilesystemAttachments(ctx, dying, dyingFilesystemAttachmentResults); err != nil {
		return errors.Annotate(err, "destroying filesystem attachments")
	}

	// Provision Alive filesystem attachments.
	aliveFilesystemAttachmentResults := filesystemAttachmentResults[:len(alive)]
	if err := processAliveFilesystemAttachments(ctx, alive, aliveFilesystemAttachmentResults); err != nil {
		return errors.Annotate(err, "creating filesystem attachments")
	}

	return nil
}

// processDyingFilesystems processes the FilesystemResults for Dying filesystems,
// removing them from provisioning-pending as necessary.
func processDyingFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	for _, tag := range tags {
		removePendingFilesystem(ctx, tag)
	}
	return nil
}

func updateFilesystem(ctx *context, info storage.Filesystem) {
	ctx.filesystems[info.Tag] = info
	for id, params := range ctx.incompleteFilesystemAttachmentParams {
		if params.FilesystemId == "" && id.AttachmentTag == info.Tag.String() {
			updatePendingFilesystemAttachment(ctx, id, params)
		}
	}
}

func updatePendingFilesystem(ctx *context, params storage.FilesystemParams) {
	ctx.config.Logger.Debugf("alvin updatePendingFilesystem called: %+v", params)
	if params.Volume != (names.VolumeTag{}) {
		// The filesystem is volume-backed: we must watch for
		// the corresponding block device. This will trigger a
		// one-time (for the volume) forced update of block
		// devices. If the block device is not immediately
		// available, then we rely on the watcher. The forced
		// update is necessary in case the block device was
		// added to state already, and we didn't observe it.
		if _, ok := ctx.volumeBlockDevices[params.Volume]; !ok {
			ctx.pendingVolumeBlockDevices.Add(params.Volume)
			ctx.incompleteFilesystemParams[params.Tag] = params
			return
		}
	}
	delete(ctx.incompleteFilesystemParams, params.Tag)
	scheduleOperations(ctx, &createFilesystemOp{args: params})
}

func removePendingFilesystem(ctx *context, tag names.FilesystemTag) {
	delete(ctx.incompleteFilesystemParams, tag)
	ctx.schedule.Remove(tag)
}

// updatePendingFilesystemAttachment adds the given filesystem attachment params to
// either the incomplete set or the schedule. If the params are incomplete
// due to a missing instance ID, updatePendingFilesystemAttachment will request
// that the machine be watched so its instance ID can be learned.
func updatePendingFilesystemAttachment(
	ctx *context,
	id params.MachineStorageId,
	params storage.FilesystemAttachmentParams,
) {
	var incomplete bool
	filesystem, ok := ctx.filesystems[params.Filesystem]
	ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment called: %+v", filesystem)

	if !ok {
		incomplete = true
	} else {
		params.FilesystemId = filesystem.FilesystemId
		if filesystem.Volume != (names.VolumeTag{}) {
			// The filesystem is volume-backed: if the filesystem
			// was created in another session, then the block device
			// may not have been seen yet. We must wait for the block
			// device watcher to trigger.
			ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete 1: %+v", incomplete)

			if _, ok := ctx.volumeBlockDevices[filesystem.Volume]; !ok {
				ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete 2: %+v", incomplete)

				incomplete = true
			}
		}
		ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete first: %+v", incomplete)

	}
	if params.InstanceId == "" {
		watchMachine(ctx, params.Machine.(names.MachineTag))
		incomplete = true
		ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete instanceID: %+v", incomplete)
	}
	if params.FilesystemId == "" {
		incomplete = true
		ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete FilesystemId: %+v", incomplete)

	}
	ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment incomplete: %+v", incomplete)

	if incomplete {
		ctx.incompleteFilesystemAttachmentParams[id] = params
		return
	}
	delete(ctx.incompleteFilesystemAttachmentParams, id)
	scheduleOperations(ctx, &attachFilesystemOp{args: params})
	ctx.config.Logger.Debugf("alvin updatePendingFilesystemAttachment scheduled: %+v", incomplete)
}

// removePendingFilesystemAttachment removes the specified pending filesystem
// attachment from the incomplete set and/or the schedule if it exists
// there.
func removePendingFilesystemAttachment(ctx *context, id params.MachineStorageId) {
	delete(ctx.incompleteFilesystemAttachmentParams, id)
	ctx.schedule.Remove(id)
}

// processDeadFilesystems processes the FilesystemResults for Dead filesystems,
// deprovisioning filesystems and removing from state as necessary.
func processDeadFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	for _, tag := range tags {
		removePendingFilesystem(ctx, tag)
	}
	var destroy []names.FilesystemTag
	var remove []names.Tag
	for i, result := range filesystemResults {
		tag := tags[i]
		if result.Error == nil {
			ctx.config.Logger.Debugf("filesystem %s is provisioned, queuing for deprovisioning", tag.Id())
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			updateFilesystem(ctx, filesystem)
			destroy = append(destroy, tag)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			ctx.config.Logger.Debugf("filesystem %s is not provisioned, queuing for removal", tag.Id())
			remove = append(remove, tag)
			continue
		}
		return errors.Annotatef(result.Error, "getting filesystem information for filesystem %s", tag.Id())
	}
	if len(destroy) > 0 {
		ops := make([]scheduleOp, len(destroy))
		for i, tag := range destroy {
			ops[i] = &removeFilesystemOp{tag: tag}
		}
		scheduleOperations(ctx, ops...)
	}
	if err := removeEntities(ctx, remove); err != nil {
		return errors.Annotate(err, "removing filesystems from state")
	}
	return nil
}

// processDyingFilesystemAttachments processes the FilesystemAttachmentResults for
// Dying filesystem attachments, detaching filesystems and updating state as necessary.
func processDyingFilesystemAttachments(
	ctx *context,
	ids []params.MachineStorageId,
	filesystemAttachmentResults []params.FilesystemAttachmentResult,
) error {
	for _, id := range ids {
		removePendingFilesystemAttachment(ctx, id)
	}
	detach := make([]params.MachineStorageId, 0, len(ids))
	remove := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range filesystemAttachmentResults {
		id := ids[i]
		if result.Error == nil {
			detach = append(detach, id)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			remove = append(remove, id)
			continue
		}
		return errors.Annotatef(result.Error, "getting information for filesystem attachment %v", id)
	}
	if len(detach) > 0 {
		attachmentParams, err := filesystemAttachmentParams(ctx, detach)
		if err != nil {
			return errors.Trace(err)
		}
		ops := make([]scheduleOp, len(attachmentParams))
		for i, p := range attachmentParams {
			ops[i] = &detachFilesystemOp{args: p}
		}
		scheduleOperations(ctx, ops...)
	}
	if err := removeAttachments(ctx, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
	}
	return nil
}

// processAliveFilesystems processes the FilesystemResults for Alive filesystems,
// provisioning filesystems and setting the info in state as necessary.
func processAliveFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	// Filter out the already-provisioned filesystems.
	pending := make([]names.FilesystemTag, 0, len(tags))
	for i, result := range filesystemResults {
		ctx.config.Logger.Debugf("alvin processAliveFileSystems result: %+v", result)

		tag := tags[i]
		if result.Error == nil {
			// Filesystem is already provisioned: skip.
			ctx.config.Logger.Debugf("filesystem %q is already provisioned, nothing to do", tag.Id())
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			updateFilesystem(ctx, filesystem)
			if !ctx.isApplicationKind() {
				if filesystem.Volume != (names.VolumeTag{}) {
					// Ensure that volume-backed filesystems' block
					// devices are present even after creating the
					// filesystem, so that attachments can be made.
					maybeAddPendingVolumeBlockDevice(ctx, filesystem.Volume)
				}
			}
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting filesystem information for filesystem %q", tag.Id(),
			)
		}
		// The filesystem has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, tag)
	}
	if len(pending) == 0 {
		return nil
	}
	ctx.config.Logger.Debugf("alvin processAliveFilesystems pending: %+v", pending)
	params, err := filesystemParams(ctx, pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	ctx.config.Logger.Debugf("alvin processAliveFilesystems filesystemParams: %+v", params)

	for _, params := range params {
		if ctx.isApplicationKind() {
			ctx.config.Logger.Debugf("not queuing filesystem for %v unit", ctx.config.Scope.Id())
			continue
		}
		updatePendingFilesystem(ctx, params)
	}
	return nil
}

func maybeAddPendingVolumeBlockDevice(ctx *context, v names.VolumeTag) {
	if _, ok := ctx.volumeBlockDevices[v]; !ok {
		ctx.pendingVolumeBlockDevices.Add(v)
	}
}

// processAliveFilesystemAttachments processes the FilesystemAttachmentResults
// for Alive filesystem attachments, attaching filesystems and setting the info
// in state as necessary.
func processAliveFilesystemAttachments(
	ctx *context,
	ids []params.MachineStorageId,
	filesystemAttachmentResults []params.FilesystemAttachmentResult,
) error {
	// Filter out the already-attached.
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range filesystemAttachmentResults {
		if result.Error == nil {
			// Filesystem attachment is already provisioned: if we
			// didn't (re)attach in this session, then we must do
			// so now.
			action := "nothing to do"
			if _, ok := ctx.filesystemAttachments[ids[i]]; !ok {
				// Not yet (re)attached in this session.
				pending = append(pending, ids[i])
				action = "will reattach"
			}
			ctx.config.Logger.Debugf(
				"%s is already attached to %s, %s",
				ids[i].AttachmentTag, ids[i].MachineTag, action,
			)
			removePendingFilesystemAttachment(ctx, ids[i])
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting information for attachment %v", ids[i],
			)
		}
		// The filesystem has not yet been attached, so
		// record its tag to enquire about parameters below.
		pending = append(pending, ids[i])
	}
	if len(pending) == 0 {
		return nil
	}
	params, err := filesystemAttachmentParams(ctx, pending)
	if err != nil {
		return errors.Trace(err)
	}
	for i, params := range params {
		if params.Machine != nil && params.Machine.Kind() != names.MachineTagKind {
			ctx.config.Logger.Debugf("not queuing filesystem attachment for non-machine %v", params.Machine)
			continue
		}
		updatePendingFilesystemAttachment(ctx, pending[i], params)
	}
	return nil
}

// filesystemAttachmentParams obtains the specified attachments' parameters.
func filesystemAttachmentParams(
	ctx *context, ids []params.MachineStorageId,
) ([]storage.FilesystemAttachmentParams, error) {
	paramsResults, err := ctx.config.Filesystems.FilesystemAttachmentParams(ids)
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem attachment params")
	}
	attachmentParams := make([]storage.FilesystemAttachmentParams, len(ids))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting filesystem attachment parameters")
		}
		params, err := filesystemAttachmentParamsFromParams(result.Result)
		if err != nil {
			return nil, errors.Annotate(err, "getting filesystem attachment parameters")
		}
		attachmentParams[i] = params
	}
	return attachmentParams, nil
}

// filesystemParams obtains the specified filesystems' parameters.
func filesystemParams(ctx *context, tags []names.FilesystemTag) ([]storage.FilesystemParams, error) {
	paramsResults, err := ctx.config.Filesystems.FilesystemParams(tags)
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem params")
	}
	allParams := make([]storage.FilesystemParams, len(tags))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting filesystem parameters")
		}
		params, err := filesystemParamsFromParams(result.Result)
		if err != nil {
			return nil, errors.Annotate(err, "getting filesystem parameters")
		}
		allParams[i] = params
	}
	return allParams, nil
}

// removeFilesystemParams obtains the specified filesystems' destruction parameters.
func removeFilesystemParams(ctx *context, tags []names.FilesystemTag) ([]params.RemoveFilesystemParams, error) {
	paramsResults, err := ctx.config.Filesystems.RemoveFilesystemParams(tags)
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem params")
	}
	allParams := make([]params.RemoveFilesystemParams, len(tags))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting filesystem removal parameters")
		}
		allParams[i] = result.Result
	}
	return allParams, nil
}

func filesystemFromParams(in params.Filesystem) (storage.Filesystem, error) {
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	var volumeTag names.VolumeTag
	if in.VolumeTag != "" {
		volumeTag, err = names.ParseVolumeTag(in.VolumeTag)
		if err != nil {
			return storage.Filesystem{}, errors.Trace(err)
		}
	}
	return storage.Filesystem{
		filesystemTag,
		volumeTag,
		storage.FilesystemInfo{
			in.Info.FilesystemId,
			in.Info.Size,
		},
	}, nil
}

func filesystemParamsFromParams(in params.FilesystemParams) (storage.FilesystemParams, error) {
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return storage.FilesystemParams{}, errors.Trace(err)
	}
	var volumeTag names.VolumeTag
	if in.VolumeTag != "" {
		volumeTag, err = names.ParseVolumeTag(in.VolumeTag)
		if err != nil {
			return storage.FilesystemParams{}, errors.Trace(err)
		}
	}
	providerType := storage.ProviderType(in.Provider)
	return storage.FilesystemParams{
		Tag:          filesystemTag,
		Volume:       volumeTag,
		Size:         in.Size,
		Provider:     providerType,
		Attributes:   in.Attributes,
		ResourceTags: in.Tags,
	}, nil
}

func filesystemAttachmentParamsFromParams(in params.FilesystemAttachmentParams) (storage.FilesystemAttachmentParams, error) {
	hostTag, err := names.ParseTag(in.MachineTag)
	if err != nil {
		return storage.FilesystemAttachmentParams{}, errors.Trace(err)
	}
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return storage.FilesystemAttachmentParams{}, errors.Trace(err)
	}
	return storage.FilesystemAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider:   storage.ProviderType(in.Provider),
			Machine:    hostTag,
			InstanceId: instance.Id(in.InstanceId),
			ReadOnly:   in.ReadOnly,
		},
		Filesystem:   filesystemTag,
		FilesystemId: in.FilesystemId,
		Path:         in.MountPoint,
	}, nil
}
