// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/plans"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/rpc/params"
)

// volumesChanged is called when the lifecycle states of the volumes
// with the provided IDs have been seen to have changed.
func volumesChanged(ctx context.Context, deps *dependencies, changes []string) error {
	tags := make([]names.Tag, len(changes))
	for i, change := range changes {
		tags[i] = names.NewVolumeTag(change)
	}
	alive, dying, dead, err := storageEntityLife(ctx, deps, tags)
	if err != nil {
		return errors.Trace(err)
	}
	deps.config.Logger.Debugf(ctx, "volumes alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if err := processDyingVolumes(ctx, deps, dying); err != nil {
		return errors.Annotate(err, "processing dying volumes")
	}
	if len(alive)+len(dead) == 0 {
		return nil
	}

	// Get volume information for alive and dead volumes, so
	// we can provision/deprovision.
	volumeTags := make([]names.VolumeTag, 0, len(alive)+len(dead))
	for _, tag := range alive {
		volumeTags = append(volumeTags, tag.(names.VolumeTag))
	}
	for _, tag := range dead {
		volumeTags = append(volumeTags, tag.(names.VolumeTag))
	}
	volumeResults, err := deps.config.Volumes.Volumes(ctx, volumeTags)
	if err != nil {
		return errors.Annotatef(err, "getting volume information")
	}
	if err := processDeadVolumes(ctx, deps, volumeTags[len(alive):], volumeResults[len(alive):]); err != nil {
		return errors.Annotate(err, "deprovisioning volumes")
	}
	if err := processAliveVolumes(ctx, deps, alive, volumeResults[:len(alive)]); err != nil {
		return errors.Annotate(err, "provisioning volumes")
	}
	return nil
}

func sortVolumeAttachmentPlans(
	ctx context.Context,
	deps *dependencies, ids []params.MachineStorageId) (alive, dying, dead []params.VolumeAttachmentPlanResult, err error) {
	plans, err := deps.config.Volumes.VolumeAttachmentPlans(ctx, ids)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	deps.config.Logger.Debugf(ctx, "Found plans: %v", plans)
	for _, plan := range plans {
		switch plan.Result.Life {
		case life.Alive:
			alive = append(alive, plan)
		case life.Dying:
			dying = append(dying, plan)
		case life.Dead:
			dead = append(dead, plan)
		}
	}
	return
}

func volumeAttachmentPlansChanged(
	ctx context.Context,
	deps *dependencies, watcherIds []watcher.MachineStorageID) error {
	deps.config.Logger.Debugf(ctx, "Got machine storage ids: %v", watcherIds)
	ids := copyMachineStorageIds(watcherIds)
	alive, dying, dead, err := sortVolumeAttachmentPlans(ctx, deps, ids)
	if err != nil {
		return errors.Trace(err)
	}
	deps.config.Logger.Debugf(ctx, "volume attachment plans alive: %v, dying: %v, dead: %v", alive, dying, dead)

	if err := processAliveVolumePlans(ctx, deps, alive); err != nil {
		return err
	}

	if err := processDyingVolumePlans(ctx, deps, dying); err != nil {
		return err
	}
	return nil
}

func processAliveVolumePlans(
	ctx context.Context,
	deps *dependencies, volumePlans []params.VolumeAttachmentPlanResult) error {
	volumeAttachmentPlans := make([]params.VolumeAttachmentPlan, len(volumePlans))
	volumeTags := make([]names.VolumeTag, len(volumePlans))
	for i, val := range volumePlans {
		volumeAttachmentPlans[i] = val.Result
		tag, err := names.ParseVolumeTag(val.Result.VolumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		volumeTags[i] = tag
	}

	for idx, val := range volumeAttachmentPlans {
		volPlan, err := plans.PlanByType(val.PlanInfo.DeviceType)
		if err != nil {
			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			continue
		}
		if blockDeviceInfo, err := volPlan.AttachVolume(val.PlanInfo.DeviceAttributes); err != nil {
			return errors.Trace(err)
		} else {
			volumeAttachmentPlans[idx].BlockDevice = blockDeviceToParams(blockDeviceInfo)
		}
	}

	results, err := deps.config.Volumes.SetVolumeAttachmentPlanBlockInfo(ctx, volumeAttachmentPlans)
	if err != nil {
		return errors.Trace(err)
	}
	for _, result := range results {
		if result.Error != nil {
			return errors.Errorf("failed to publish block info to state: %s", result.Error)
		}
	}
	_, err = refreshVolumeBlockDevices(ctx, deps, volumeTags)
	return err
}

func blockDeviceToParams(in blockdevice.BlockDevice) params.BlockDevice {
	return params.BlockDevice{
		DeviceName:     in.DeviceName,
		DeviceLinks:    in.DeviceLinks,
		Label:          in.Label,
		UUID:           in.UUID,
		HardwareId:     in.HardwareId,
		WWN:            in.WWN,
		BusAddress:     in.BusAddress,
		Size:           in.SizeMiB,
		FilesystemType: in.FilesystemType,
		InUse:          in.InUse,
		MountPoint:     in.MountPoint,
		SerialId:       in.SerialId,
	}
}

func processDyingVolumePlans(
	ctx context.Context,
	deps *dependencies, volumePlans []params.VolumeAttachmentPlanResult) error {
	ids := volumePlansToMachineIds(volumePlans)
	for _, val := range volumePlans {
		volPlan, err := plans.PlanByType(val.Result.PlanInfo.DeviceType)
		if err != nil {
			if !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			continue
		}
		if err := volPlan.DetachVolume(val.Result.PlanInfo.DeviceAttributes); err != nil {
			return errors.Trace(err)
		}
		if wrench.IsActive("storageprovisioner", "DetachVolume") {
			return errors.New("wrench active")
		}
	}
	results, err := deps.config.Volumes.RemoveVolumeAttachmentPlan(ctx, ids)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.Error != nil {
			return errors.Annotate(result.Error, "removing volume plan")
		}
	}
	return nil
}

func volumePlansToMachineIds(plans []params.VolumeAttachmentPlanResult) []params.MachineStorageId {
	storageIds := make([]params.MachineStorageId, len(plans))
	for i, plan := range plans {
		storageIds[i] = params.MachineStorageId{
			MachineTag:    plan.Result.MachineTag,
			AttachmentTag: plan.Result.VolumeTag,
		}
	}
	return storageIds
}

// volumeAttachmentsChanged is called when the lifecycle states of the volume
// attachments with the provided IDs have been seen to have changed.
func volumeAttachmentsChanged(
	ctx context.Context,
	deps *dependencies, watcherIds []watcher.MachineStorageID) error {
	ids := copyMachineStorageIds(watcherIds)
	alive, dying, dead, gone, err := attachmentLife(ctx, deps, ids)
	if err != nil {
		return errors.Trace(err)
	}
	deps.config.Logger.Debugf(ctx, "volume attachments alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(dead) != 0 {
		// We should not see dead volume attachments;
		// attachments go directly from Dying to removed.
		deps.config.Logger.Warningf(ctx, "unexpected dead volume attachments: %v", dead)
	}
	// Clean up any attachments which have been removed.
	for _, id := range gone {
		delete(deps.volumeAttachments, id)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get volume information for alive and dying volume attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	volumeAttachmentResults, err := deps.config.Volumes.VolumeAttachments(ctx, ids)
	if err != nil {
		return errors.Annotatef(err, "getting volume attachment information")
	}

	// Deprovision Dying volume attachments.
	dyingVolumeAttachmentResults := volumeAttachmentResults[len(alive):]
	if err := processDyingVolumeAttachments(ctx, deps, dying, dyingVolumeAttachmentResults); err != nil {
		return errors.Annotate(err, "deprovisioning volume attachments")
	}

	// Provision Alive volume attachments.
	aliveVolumeAttachmentResults := volumeAttachmentResults[:len(alive)]
	if err := processAliveVolumeAttachments(ctx, deps, alive, aliveVolumeAttachmentResults); err != nil {
		return errors.Annotate(err, "provisioning volumes")
	}

	return nil
}

// processDyingVolumes processes the VolumeResults for Dying volumes,
// removing them from provisioning-pending as necessary.
func processDyingVolumes(ctx context.Context, deps *dependencies, tags []names.Tag) error {
	deps.config.Logger.Infof(ctx, "processing dying volumes: %v", tags)
	if deps.isApplicationKind() {
		// only care dead for application.
		return nil
	}
	for _, tag := range tags {
		removePendingVolume(deps, tag.(names.VolumeTag))
	}
	return nil
}

// updateVolume updates the context with the given volume info.
func updateVolume(deps *dependencies, info storage.Volume) {
	deps.volumes[info.Tag] = info
	for id, params := range deps.incompleteVolumeAttachmentParams {
		if params.VolumeId == "" && id.AttachmentTag == info.Tag.String() {
			params.VolumeId = info.VolumeId
			updatePendingVolumeAttachment(deps, id, params)
		}
	}
}

// updatePendingVolume adds the given volume params to either the incomplete
// set or the schedule. If the params are incomplete due to a missing instance
// ID, updatePendingVolume will request that the machine be watched so its
// instance ID can be learned.
func updatePendingVolume(deps *dependencies, params storage.VolumeParams) {
	if params.Attachment == nil {
		// NOTE(axw) this would only happen if the model is
		// in an incoherent state; we should never have an
		// alive, unprovisioned, and unattached volume.
		deps.config.Logger.Warningf(context.TODO(),
			"%s is in an incoherent state, ignoring",
			names.ReadableString(params.Tag),
		)
		return
	}
	if params.Attachment.InstanceId == "" {
		watchMachine(deps, params.Attachment.Machine.(names.MachineTag))
		deps.incompleteVolumeParams[params.Tag] = params
	} else {
		delete(deps.incompleteVolumeParams, params.Tag)
		scheduleOperations(deps, &createVolumeOp{args: params})
	}
}

// removePendingVolume removes the specified pending volume from the
// incomplete set and/or the schedule if it exists there.
func removePendingVolume(deps *dependencies, tag names.VolumeTag) {
	delete(deps.incompleteVolumeParams, tag)
	deps.schedule.Remove(tag)
}

// updatePendingVolumeAttachment adds the given volume attachment params to
// either the incomplete set or the schedule. If the params are incomplete
// due to a missing instance ID, updatePendingVolumeAttachment will request
// that the machine be watched so its instance ID can be learned.
func updatePendingVolumeAttachment(
	deps *dependencies,
	id params.MachineStorageId,
	params storage.VolumeAttachmentParams,
) {
	if params.InstanceId == "" {
		watchMachine(deps, params.Machine.(names.MachineTag))
	} else if params.VolumeId != "" {
		delete(deps.incompleteVolumeAttachmentParams, id)
		scheduleOperations(deps, &attachVolumeOp{args: params})
		return
	}
	deps.incompleteVolumeAttachmentParams[id] = params
}

// removePendingVolumeAttachment removes the specified pending volume
// attachment from the incomplete set and/or the schedule if it exists
// there.
func removePendingVolumeAttachment(deps *dependencies, id params.MachineStorageId) {
	delete(deps.incompleteVolumeAttachmentParams, id)
	deps.schedule.Remove(id)
}

// processDeadVolumes processes the VolumeResults for Dead volumes,
// deprovisioning volumes and removing from state as necessary.
func processDeadVolumes(
	ctx context.Context,
	deps *dependencies, tags []names.VolumeTag, volumeResults []params.VolumeResult) error {
	deps.config.Logger.Infof(ctx, "processing dead volumes: %v", tags)
	for _, tag := range tags {
		removePendingVolume(deps, tag)
	}
	var destroy []names.VolumeTag
	var remove []names.Tag
	for i, result := range volumeResults {
		tag := tags[i]
		if result.Error == nil {
			deps.config.Logger.Debugf(ctx, "volume %s is provisioned, queuing for deprovisioning", tag.Id())
			volume, err := volumeFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting volume info")
			}
			updateVolume(deps, volume)
			destroy = append(destroy, tag)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			deps.config.Logger.Debugf(ctx, "volume %s is not provisioned, queuing for removal", tag.Id())
			remove = append(remove, tag)
			continue
		}
		return errors.Annotatef(result.Error, "getting volume information for volume %s", tag.Id())
	}
	if len(destroy) > 0 {
		ops := make([]scheduleOp, len(destroy))
		for i, tag := range destroy {
			ops[i] = &removeVolumeOp{tag: tag}
		}
		scheduleOperations(deps, ops...)
	}
	if err := removeEntities(ctx, deps, remove); err != nil {
		return errors.Annotate(err, "removing volumes from state")
	}
	return nil
}

// processDyingVolumeAttachments processes the VolumeAttachmentResults for
// Dying volume attachments, detaching volumes and updating state as necessary.
func processDyingVolumeAttachments(
	ctx context.Context,
	deps *dependencies,
	ids []params.MachineStorageId,
	volumeAttachmentResults []params.VolumeAttachmentResult,
) error {
	deps.config.Logger.Infof(ctx, "processing dying volume attachments: %v", ids)
	for _, id := range ids {
		removePendingVolumeAttachment(deps, id)
	}
	detach := make([]params.MachineStorageId, 0, len(ids))
	remove := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range volumeAttachmentResults {
		id := ids[i]
		if result.Error == nil {
			detach = append(detach, id)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			remove = append(remove, id)
			continue
		}
		return errors.Annotatef(result.Error, "getting information for volume attachment %v", id)
	}
	if len(detach) > 0 {
		attachmentParams, err := volumeAttachmentParams(ctx, deps, detach)
		if err != nil {
			return errors.Trace(err)
		}
		ops := make([]scheduleOp, len(attachmentParams))
		for i, p := range attachmentParams {
			ops[i] = &detachVolumeOp{args: p}
		}
		scheduleOperations(deps, ops...)
	}
	if err := removeAttachments(ctx, deps, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
	}
	for _, id := range remove {
		delete(deps.volumeAttachments, id)
	}
	return nil
}

// processAliveVolumes processes the VolumeResults for Alive volumes,
// provisioning volumes and setting the info in state as necessary.
func processAliveVolumes(
	ctx context.Context,
	deps *dependencies, tags []names.Tag, volumeResults []params.VolumeResult) error {
	deps.config.Logger.Infof(ctx, "processing alive volumes: %v", tags)
	if deps.isApplicationKind() {
		// only care dead for application kind.
		return nil
	}

	// Filter out the already-provisioned volumes.
	pending := make([]names.VolumeTag, 0, len(tags))
	for i, result := range volumeResults {
		volumeTag := tags[i].(names.VolumeTag)
		if result.Error == nil {
			// Volume is already provisioned: skip.
			deps.config.Logger.Debugf(ctx, "volume %q is already provisioned, nothing to do", tags[i].Id())
			volume, err := volumeFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting volume info")
			}
			updateVolume(deps, volume)
			removePendingVolume(deps, volumeTag)
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting volume information for volume %q", tags[i].Id(),
			)
		}
		// The volume has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, volumeTag)
	}
	if len(pending) == 0 {
		return nil
	}
	volumeParams, err := volumeParams(ctx, deps, pending)
	if err != nil {
		return errors.Annotate(err, "getting volume params")
	}
	for _, params := range volumeParams {
		if params.Attachment != nil && params.Attachment.Machine.Kind() != names.MachineTagKind {
			deps.config.Logger.Debugf(ctx, "not queuing volume for non-machine %v", params.Attachment.Machine)
			continue
		}
		updatePendingVolume(deps, params)
	}
	return nil
}

// processAliveVolumeAttachments processes the VolumeAttachmentResults
// for Alive volume attachments, attaching volumes and setting the info
// in state as necessary.
func processAliveVolumeAttachments(
	ctx context.Context,
	deps *dependencies,
	ids []params.MachineStorageId,
	volumeAttachmentResults []params.VolumeAttachmentResult,
) error {
	deps.config.Logger.Infof(ctx, "processing alive volume attachments: %v", ids)
	// Filter out the already-attached.
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range volumeAttachmentResults {
		if result.Error == nil {
			// Volume attachment is already provisioned: if we
			// didn't (re)attach in this session, then we must
			// do so now.
			action := "nothing to do"
			if _, ok := deps.volumeAttachments[ids[i]]; !ok {
				// Not yet (re)attached in this session.
				pending = append(pending, ids[i])
				action = "will reattach"
			}
			deps.config.Logger.Debugf(ctx,
				"%s is already attached to %s, %s",
				ids[i].AttachmentTag, ids[i].MachineTag, action,
			)
			removePendingVolumeAttachment(deps, ids[i])
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting information for attachment %v", ids[i],
			)
		}
		// The volume has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, ids[i])
	}
	if len(pending) == 0 {
		return nil
	}
	params, err := volumeAttachmentParams(ctx, deps, pending)
	if err != nil {
		return errors.Trace(err)
	}
	for i, params := range params {
		if params.Machine.Kind() != names.MachineTagKind {
			deps.config.Logger.Debugf(ctx, "not queuing volume attachment for non-machine %v", params.Machine)
			continue
		}
		if volume, ok := deps.volumes[params.Volume]; ok {
			params.VolumeId = volume.VolumeId
		}
		updatePendingVolumeAttachment(deps, pending[i], params)
	}
	return nil
}

// volumeAttachmentParams obtains the specified attachments' parameters.
func volumeAttachmentParams(
	ctx context.Context,
	deps *dependencies, ids []params.MachineStorageId,
) ([]storage.VolumeAttachmentParams, error) {
	paramsResults, err := deps.config.Volumes.VolumeAttachmentParams(ctx, ids)
	if err != nil {
		return nil, errors.Annotate(err, "getting volume attachment params")
	}
	attachmentParams := make([]storage.VolumeAttachmentParams, len(ids))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting volume attachment parameters")
		}
		params, err := volumeAttachmentParamsFromParams(result.Result)
		if err != nil {
			return nil, errors.Annotate(err, "getting volume attachment parameters")
		}
		attachmentParams[i] = params
	}
	return attachmentParams, nil
}

// volumeParams obtains the specified volumes' parameters.
func volumeParams(
	ctx context.Context,
	deps *dependencies, tags []names.VolumeTag) ([]storage.VolumeParams, error) {
	paramsResults, err := deps.config.Volumes.VolumeParams(ctx, tags)
	if err != nil {
		return nil, errors.Annotate(err, "getting volume params")
	}
	allParams := make([]storage.VolumeParams, len(tags))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting volume parameters")
		}
		params, err := volumeParamsFromParams(result.Result)
		if err != nil {
			return nil, errors.Annotate(err, "getting volume parameters")
		}
		allParams[i] = params
	}
	return allParams, nil
}

// removeVolumeParams obtains the specified volumes' destruction parameters.
func removeVolumeParams(
	ctx context.Context,
	deps *dependencies, tags []names.VolumeTag) ([]params.RemoveVolumeParams, error) {
	paramsResults, err := deps.config.Volumes.RemoveVolumeParams(ctx, tags)
	if err != nil {
		return nil, errors.Annotate(err, "getting volume params")
	}
	allParams := make([]params.RemoveVolumeParams, len(tags))
	for i, result := range paramsResults {
		if result.Error != nil {
			return nil, errors.Annotate(result.Error, "getting volume removal parameters")
		}
		allParams[i] = result.Result
	}
	return allParams, nil
}

func volumesFromStorage(in []storage.Volume) []params.Volume {
	out := make([]params.Volume, len(in))
	for i, v := range in {
		out[i] = params.Volume{
			VolumeTag: v.Tag.String(),
			Info: params.VolumeInfo{
				ProviderId: v.VolumeId,
				HardwareId: v.HardwareId,
				WWN:        v.WWN,
				Pool:       "", // pool
				SizeMiB:    v.Size,
				Persistent: v.Persistent,
			},
		}
	}
	return out
}

func volumeAttachmentsFromStorage(in []storage.VolumeAttachment) []params.VolumeAttachment {
	out := make([]params.VolumeAttachment, len(in))
	for i, v := range in {
		planInfo := &params.VolumeAttachmentPlanInfo{}
		if v.PlanInfo != nil {
			planInfo.DeviceType = v.PlanInfo.DeviceType
			planInfo.DeviceAttributes = v.PlanInfo.DeviceAttributes
		} else {
			planInfo = nil
		}
		out[i] = params.VolumeAttachment{
			VolumeTag:  v.Volume.String(),
			MachineTag: v.Machine.String(),
			Info: params.VolumeAttachmentInfo{
				DeviceName: v.DeviceName,
				DeviceLink: v.DeviceLink,
				BusAddress: v.BusAddress,
				ReadOnly:   v.ReadOnly,
				PlanInfo:   planInfo,
			},
		}
	}
	return out
}

func volumeFromParams(in params.Volume) (storage.Volume, error) {
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return storage.Volume{}, errors.Trace(err)
	}
	return storage.Volume{
		Tag: volumeTag,
		VolumeInfo: storage.VolumeInfo{
			VolumeId:   in.Info.ProviderId,
			HardwareId: in.Info.HardwareId,
			WWN:        in.Info.WWN,
			Size:       in.Info.SizeMiB,
			Persistent: in.Info.Persistent,
		},
	}, nil
}

func volumeParamsFromParams(in params.VolumeParams) (storage.VolumeParams, error) {
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return storage.VolumeParams{}, errors.Trace(err)
	}
	providerType := storage.ProviderType(in.Provider)

	var attachment *storage.VolumeAttachmentParams
	if in.Attachment != nil {
		if in.Attachment.Provider != in.Provider {
			return storage.VolumeParams{}, errors.Errorf(
				"storage provider mismatch: volume (%q), attachment (%q)",
				in.Provider, in.Attachment.Provider,
			)
		}
		if in.Attachment.VolumeTag != in.VolumeTag {
			return storage.VolumeParams{}, errors.Errorf(
				"volume tag mismatch: volume (%q), attachment (%q)",
				in.VolumeTag, in.Attachment.VolumeTag,
			)
		}
		hostTag, err := names.ParseTag(in.Attachment.MachineTag)
		if err != nil {
			return storage.VolumeParams{}, errors.Annotate(
				err, "parsing attachment machine tag",
			)
		}
		attachment = &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   providerType,
				Machine:    hostTag,
				InstanceId: instance.Id(in.Attachment.InstanceId),
				ReadOnly:   in.Attachment.ReadOnly,
			},
			Volume: volumeTag,
		}
	}
	return storage.VolumeParams{
		Tag:          volumeTag,
		Size:         in.Size,
		Provider:     providerType,
		Attributes:   in.Attributes,
		ResourceTags: in.Tags,
		Attachment:   attachment,
	}, nil
}

func volumeAttachmentParamsFromParams(in params.VolumeAttachmentParams) (storage.VolumeAttachmentParams, error) {
	hostTag, err := names.ParseTag(in.MachineTag)
	if err != nil {
		return storage.VolumeAttachmentParams{}, errors.Trace(err)
	}
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return storage.VolumeAttachmentParams{}, errors.Trace(err)
	}
	return storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider:   storage.ProviderType(in.Provider),
			Machine:    hostTag,
			InstanceId: instance.Id(in.InstanceId),
			ReadOnly:   in.ReadOnly,
		},
		Volume:   volumeTag,
		VolumeId: in.ProviderId,
	}, nil
}
