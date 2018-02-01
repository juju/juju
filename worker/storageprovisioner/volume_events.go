// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/plans"
)

// volumesChanged is called when the lifecycle states of the volumes
// with the provided IDs have been seen to have changed.
func volumesChanged(ctx *context, changes []string) error {
	tags := make([]names.Tag, len(changes))
	for i, change := range changes {
		tags[i] = names.NewVolumeTag(change)
	}
	alive, dying, dead, err := storageEntityLife(ctx, tags)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("volumes alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if err := processDyingVolumes(ctx, dying); err != nil {
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
	volumeResults, err := ctx.config.Volumes.Volumes(volumeTags)
	if err != nil {
		return errors.Annotatef(err, "getting volume information")
	}
	if err := processDeadVolumes(ctx, volumeTags[len(alive):], volumeResults[len(alive):]); err != nil {
		return errors.Annotate(err, "deprovisioning volumes")
	}
	if err := processAliveVolumes(ctx, alive, volumeResults[:len(alive)]); err != nil {
		return errors.Annotate(err, "provisioning volumes")
	}
	return nil
}

func sortVolumeAttachmentPlans(ctx *context, ids []params.MachineStorageId) (
	alive, dying, dead []params.VolumeAttachmentPlanResult, err error) {
	plans, err := ctx.config.Volumes.VolumeAttachmentPlans(ids)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}
	logger.Debugf("Found plans: %v", plans)
	for _, plan := range plans {
		switch plan.Result.Life {
		case params.Alive:
			alive = append(alive, plan)
		case params.Dying:
			dying = append(dying, plan)
		case params.Dead:
			dead = append(dead, plan)
		}
	}
	return
}

func volumeAttachmentPlansChanged(ctx *context, watcherIds []watcher.MachineStorageId) error {
	logger.Debugf("Got machine storage ids: %v", watcherIds)
	ids := copyMachineStorageIds(watcherIds)
	alive, dying, dead, err := sortVolumeAttachmentPlans(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("volume attachment plans alive: %v, dying: %v, dead: %v", alive, dying, dead)

	if err := processAliveVolumePlans(ctx, alive); err != nil {
		return err
	}

	if err := processDyingVolumePlans(ctx, dying); err != nil {
		return err
	}
	return nil
}

func processAliveVolumePlans(ctx *context, volumePlans []params.VolumeAttachmentPlanResult) error {
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
			if !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			continue
		}
		if blockDeviceInfo, err := volPlan.AttachVolume(val.PlanInfo.DeviceAttributes); err != nil {
			return errors.Trace(err)
		} else {
			volumeAttachmentPlans[idx].BlockDevice = blockDeviceInfo
		}
	}

	results, err := ctx.config.Volumes.SetVolumeAttachmentPlanBlockInfo(volumeAttachmentPlans)
	if err != nil {
		return errors.Trace(err)
	}
	for _, result := range results {
		if result.Error != nil {
			return errors.Errorf("failed to publish block info to state: %s", result.Error)
		}
	}
	return refreshVolumeBlockDevices(ctx, volumeTags)
}

func processDyingVolumePlans(ctx *context, volumePlans []params.VolumeAttachmentPlanResult) error {
	ids := volumePlansToMachineIds(volumePlans)
	for _, val := range volumePlans {
		volPlan, err := plans.PlanByType(val.Result.PlanInfo.DeviceType)
		if err != nil {
			if !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			continue
		}
		if err := volPlan.DetachVolume(val.Result.PlanInfo.DeviceAttributes); err != nil {
			return errors.Trace(err)
		}
	}
	results, err := ctx.config.Volumes.RemoveVolumeAttachmentPlan(ids)
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
func volumeAttachmentsChanged(ctx *context, watcherIds []watcher.MachineStorageId) error {
	ids := copyMachineStorageIds(watcherIds)
	alive, dying, dead, err := attachmentLife(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("volume attachments alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(dead) != 0 {
		// We should not see dead volume attachments;
		// attachments go directly from Dying to removed.
		logger.Warningf("unexpected dead volume attachments: %v", dead)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get volume information for alive and dying volume attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	volumeAttachmentResults, err := ctx.config.Volumes.VolumeAttachments(ids)
	if err != nil {
		return errors.Annotatef(err, "getting volume attachment information")
	}

	// Deprovision Dying volume attachments.
	dyingVolumeAttachmentResults := volumeAttachmentResults[len(alive):]
	if err := processDyingVolumeAttachments(ctx, dying, dyingVolumeAttachmentResults); err != nil {
		return errors.Annotate(err, "deprovisioning volume attachments")
	}

	// Provision Alive volume attachments.
	aliveVolumeAttachmentResults := volumeAttachmentResults[:len(alive)]
	if err := processAliveVolumeAttachments(ctx, alive, aliveVolumeAttachmentResults); err != nil {
		return errors.Annotate(err, "provisioning volumes")
	}

	return nil
}

// processDyingVolumes processes the VolumeResults for Dying volumes,
// removing them from provisioning-pending as necessary.
func processDyingVolumes(ctx *context, tags []names.Tag) error {
	for _, tag := range tags {
		removePendingVolume(ctx, tag.(names.VolumeTag))
	}
	return nil
}

// updateVolume updates the context with the given volume info.
func updateVolume(ctx *context, info storage.Volume) {
	ctx.volumes[info.Tag] = info
	for id, params := range ctx.incompleteVolumeAttachmentParams {
		if params.VolumeId == "" && id.AttachmentTag == info.Tag.String() {
			params.VolumeId = info.VolumeId
			updatePendingVolumeAttachment(ctx, id, params)
		}
	}
}

// updatePendingVolume adds the given volume params to either the incomplete
// set or the schedule. If the params are incomplete due to a missing instance
// ID, updatePendingVolume will request that the machine be watched so its
// instance ID can be learned.
func updatePendingVolume(ctx *context, params storage.VolumeParams) {
	if params.Attachment == nil {
		// NOTE(axw) this would only happen if the model is
		// in an incoherent state; we should never have an
		// alive, unprovisioned, and unattached volume.
		logger.Warningf(
			"%s is in an incoherent state, ignoring",
			names.ReadableString(params.Tag),
		)
		return
	}
	if params.Attachment.InstanceId == "" {
		watchMachine(ctx, params.Attachment.Machine.(names.MachineTag))
		ctx.incompleteVolumeParams[params.Tag] = params
	} else {
		delete(ctx.incompleteVolumeParams, params.Tag)
		scheduleOperations(ctx, &createVolumeOp{args: params})
	}
}

// removePendingVolume removes the specified pending volume from the
// incomplete set and/or the schedule if it exists there.
func removePendingVolume(ctx *context, tag names.VolumeTag) {
	delete(ctx.incompleteVolumeParams, tag)
	ctx.schedule.Remove(tag)
}

// updatePendingVolumeAttachment adds the given volume attachment params to
// either the incomplete set or the schedule. If the params are incomplete
// due to a missing instance ID, updatePendingVolumeAttachment will request
// that the machine be watched so its instance ID can be learned.
func updatePendingVolumeAttachment(
	ctx *context,
	id params.MachineStorageId,
	params storage.VolumeAttachmentParams,
) {
	if params.InstanceId == "" {
		watchMachine(ctx, params.Machine.(names.MachineTag))
	} else if params.VolumeId != "" {
		delete(ctx.incompleteVolumeAttachmentParams, id)
		scheduleOperations(ctx, &attachVolumeOp{args: params})
		return
	}
	ctx.incompleteVolumeAttachmentParams[id] = params
}

// removePendingVolumeAttachment removes the specified pending volume
// attachment from the incomplete set and/or the schedule if it exists
// there.
func removePendingVolumeAttachment(ctx *context, id params.MachineStorageId) {
	delete(ctx.incompleteVolumeAttachmentParams, id)
	ctx.schedule.Remove(id)
}

// processDeadVolumes processes the VolumeResults for Dead volumes,
// deprovisioning volumes and removing from state as necessary.
func processDeadVolumes(ctx *context, tags []names.VolumeTag, volumeResults []params.VolumeResult) error {
	for _, tag := range tags {
		removePendingVolume(ctx, tag)
	}
	var destroy []names.VolumeTag
	var remove []names.Tag
	for i, result := range volumeResults {
		tag := tags[i]
		if result.Error == nil {
			logger.Debugf("volume %s is provisioned, queuing for deprovisioning", tag.Id())
			volume, err := volumeFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting volume info")
			}
			updateVolume(ctx, volume)
			destroy = append(destroy, tag)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			logger.Debugf("volume %s is not provisioned, queuing for removal", tag.Id())
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
		scheduleOperations(ctx, ops...)
	}
	if err := removeEntities(ctx, remove); err != nil {
		return errors.Annotate(err, "removing volumes from state")
	}
	return nil
}

// processDyingVolumeAttachments processes the VolumeAttachmentResults for
// Dying volume attachments, detaching volumes and updating state as necessary.
func processDyingVolumeAttachments(
	ctx *context,
	ids []params.MachineStorageId,
	volumeAttachmentResults []params.VolumeAttachmentResult,
) error {
	for _, id := range ids {
		removePendingVolumeAttachment(ctx, id)
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
		attachmentParams, err := volumeAttachmentParams(ctx, detach)
		if err != nil {
			return errors.Trace(err)
		}
		ops := make([]scheduleOp, len(attachmentParams))
		for i, p := range attachmentParams {
			ops[i] = &detachVolumeOp{args: p}
		}
		scheduleOperations(ctx, ops...)
	}
	if err := removeAttachments(ctx, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
	}
	for _, id := range remove {
		delete(ctx.volumeAttachments, id)
	}
	return nil
}

// processAliveVolumes processes the VolumeResults for Alive volumes,
// provisioning volumes and setting the info in state as necessary.
func processAliveVolumes(ctx *context, tags []names.Tag, volumeResults []params.VolumeResult) error {
	// Filter out the already-provisioned volumes.
	pending := make([]names.VolumeTag, 0, len(tags))
	for i, result := range volumeResults {
		volumeTag := tags[i].(names.VolumeTag)
		if result.Error == nil {
			// Volume is already provisioned: skip.
			logger.Debugf("volume %q is already provisioned, nothing to do", tags[i].Id())
			volume, err := volumeFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting volume info")
			}
			updateVolume(ctx, volume)
			removePendingVolume(ctx, volumeTag)
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
	volumeParams, err := volumeParams(ctx, pending)
	if err != nil {
		return errors.Annotate(err, "getting volume params")
	}
	for _, params := range volumeParams {
		if params.Attachment != nil && params.Attachment.Machine.Kind() != names.MachineTagKind {
			logger.Debugf("not queuing volume for non-machine %v", params.Attachment.Machine)
			continue
		}
		updatePendingVolume(ctx, params)
	}
	return nil
}

// processAliveVolumeAttachments processes the VolumeAttachmentResults
// for Alive volume attachments, attaching volumes and setting the info
// in state as necessary.
func processAliveVolumeAttachments(
	ctx *context,
	ids []params.MachineStorageId,
	volumeAttachmentResults []params.VolumeAttachmentResult,
) error {
	// Filter out the already-attached.
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range volumeAttachmentResults {
		if result.Error == nil {
			// Volume attachment is already provisioned: if we
			// didn't (re)attach in this session, then we must
			// do so now.
			action := "nothing to do"
			if _, ok := ctx.volumeAttachments[ids[i]]; !ok {
				// Not yet (re)attached in this session.
				pending = append(pending, ids[i])
				action = "will reattach"
			}
			logger.Debugf(
				"%s is already attached to %s, %s",
				ids[i].AttachmentTag, ids[i].MachineTag, action,
			)
			removePendingVolumeAttachment(ctx, ids[i])
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
	params, err := volumeAttachmentParams(ctx, pending)
	if err != nil {
		return errors.Trace(err)
	}
	for i, params := range params {
		if params.Machine.Kind() != names.MachineTagKind {
			logger.Debugf("not queuing volume attachment for non-machine %v", params.Machine)
			continue
		}
		if volume, ok := ctx.volumes[params.Volume]; ok {
			params.VolumeId = volume.VolumeId
		}
		updatePendingVolumeAttachment(ctx, pending[i], params)
	}
	return nil
}

// volumeAttachmentParams obtains the specified attachments' parameters.
func volumeAttachmentParams(
	ctx *context, ids []params.MachineStorageId,
) ([]storage.VolumeAttachmentParams, error) {
	paramsResults, err := ctx.config.Volumes.VolumeAttachmentParams(ids)
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
func volumeParams(ctx *context, tags []names.VolumeTag) ([]storage.VolumeParams, error) {
	paramsResults, err := ctx.config.Volumes.VolumeParams(tags)
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
func removeVolumeParams(ctx *context, tags []names.VolumeTag) ([]params.RemoveVolumeParams, error) {
	paramsResults, err := ctx.config.Volumes.RemoveVolumeParams(tags)
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
			v.Tag.String(),
			params.VolumeInfo{
				v.VolumeId,
				v.HardwareId,
				v.WWN,
				"", // pool
				v.Size,
				v.Persistent,
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
			v.Volume.String(),
			v.Machine.String(),
			params.VolumeAttachmentInfo{
				v.DeviceName,
				v.DeviceLink,
				v.BusAddress,
				v.ReadOnly,
				planInfo,
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
		volumeTag,
		storage.VolumeInfo{
			in.Info.VolumeId,
			in.Info.HardwareId,
			in.Info.WWN,
			in.Info.Size,
			in.Info.Persistent,
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
		volumeTag,
		in.Size,
		providerType,
		in.Attributes,
		in.Tags,
		attachment,
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
		VolumeId: in.VolumeId,
	}, nil
}
