// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

// createVolumes creates volumes with the specified parameters.
func createVolumes(ctx *context, ops map[names.VolumeTag]*createVolumeOp) error {
	volumeParams := make([]storage.VolumeParams, 0, len(ops))
	for _, op := range ops {
		volumeParams = append(volumeParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeParamsBySource(
		ctx.environConfig, ctx.storageDir, volumeParams,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var volumes []storage.Volume
	var volumeAttachments []storage.VolumeAttachment
	var statuses []params.EntityStatusArgs
	for sourceName, volumeParams := range paramsBySource {
		logger.Debugf("creating volumes: %v", volumeParams)
		volumeSource := volumeSources[sourceName]
		results, err := volumeSource.CreateVolumes(volumeParams)
		if err != nil {
			return errors.Annotatef(err, "creating volumes from source %q", sourceName)
		}
		for i, result := range results {
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    volumeParams[i].Tag.String(),
				Status: params.StatusAttaching,
			})
			status := &statuses[len(statuses)-1]
			if result.Error != nil {
				// Reschedule the volume creation.
				reschedule = append(reschedule, ops[volumeParams[i].Tag])

				// Note: we keep the status as "pending" to indicate
				// that we will retry. When we distinguish between
				// transient and permanent errors, we will set the
				// status to "error" for permanent errors.
				status.Status = params.StatusPending
				status.Info = result.Error.Error()
				logger.Debugf(
					"failed to create %s: %v",
					names.ReadableString(volumeParams[i].Tag),
					result.Error,
				)
				continue
			}
			volumes = append(volumes, *result.Volume)
			if result.VolumeAttachment != nil {
				status.Status = params.StatusAttached
				volumeAttachments = append(volumeAttachments, *result.VolumeAttachment)
			}
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if len(volumes) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list volumes in the provider,
	// by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing volumes that we fail
	// to record in state.
	errorResults, err := ctx.volumeAccessor.SetVolumeInfo(volumesFromStorage(volumes))
	if err != nil {
		return errors.Annotate(err, "publishing volumes to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			logger.Errorf(
				"publishing volume %s to state: %v",
				volumes[i].Tag.Id(),
				result.Error,
			)
		}
	}
	for _, v := range volumes {
		updateVolume(ctx, v)
	}
	// Note: the storage provisioner that creates a volume is also
	// responsible for creating the volume attachment. It is therefore
	// safe to set the volume attachment info after the volume info,
	// without leading to the possibility of concurrent, duplicate
	// attachments.
	err = setVolumeAttachmentInfo(ctx, volumeAttachments)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// attachVolumes creates volume attachments with the specified parameters.
func attachVolumes(ctx *context, ops map[params.MachineStorageId]*attachVolumeOp) error {
	volumeAttachmentParams := make([]storage.VolumeAttachmentParams, 0, len(ops))
	for _, op := range ops {
		volumeAttachmentParams = append(volumeAttachmentParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeAttachmentParamsBySource(
		ctx.environConfig, ctx.storageDir, volumeAttachmentParams,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var volumeAttachments []storage.VolumeAttachment
	var statuses []params.EntityStatusArgs
	for sourceName, volumeAttachmentParams := range paramsBySource {
		logger.Debugf("attaching volumes: %+v", volumeAttachmentParams)
		volumeSource := volumeSources[sourceName]
		results, err := volumeSource.AttachVolumes(volumeAttachmentParams)
		if err != nil {
			return errors.Annotatef(err, "attaching volumes from source %q", sourceName)
		}
		for i, result := range results {
			p := volumeAttachmentParams[i]
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    p.Volume.String(),
				Status: params.StatusAttached,
			})
			status := &statuses[len(statuses)-1]
			if result.Error != nil {
				// Reschedule the volume attachment.
				id := params.MachineStorageId{
					MachineTag:    p.Machine.String(),
					AttachmentTag: p.Volume.String(),
				}
				reschedule = append(reschedule, ops[id])

				// Note: we keep the status as "attaching" to
				// indicate that we will retry. When we distinguish
				// between transient and permanent errors, we will
				// set the status to "error" for permanent errors.
				status.Status = params.StatusAttaching
				status.Info = result.Error.Error()
				logger.Debugf(
					"failed to attach %s to %s: %v",
					names.ReadableString(p.Volume),
					names.ReadableString(p.Machine),
					result.Error,
				)
				continue
			}
			volumeAttachments = append(volumeAttachments, *result.VolumeAttachment)
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if err := setVolumeAttachmentInfo(ctx, volumeAttachments); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// destroyVolumes destroys volumes with the specified parameters.
func destroyVolumes(ctx *context, ops map[names.VolumeTag]*destroyVolumeOp) error {
	tags := make([]names.VolumeTag, 0, len(ops))
	for tag := range ops {
		tags = append(tags, tag)
	}
	volumeParams, err := volumeParams(ctx, tags)
	if err != nil {
		return errors.Trace(err)
	}
	paramsBySource, volumeSources, err := volumeParamsBySource(
		ctx.environConfig, ctx.storageDir, volumeParams,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var remove []names.Tag
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	for sourceName, volumeParams := range paramsBySource {
		logger.Debugf("destroying volumes from %q: %v", sourceName, volumeParams)
		volumeSource := volumeSources[sourceName]
		volumeIds := make([]string, len(volumeParams))
		for i, volumeParams := range volumeParams {
			volume, ok := ctx.volumes[volumeParams.Tag]
			if !ok {
				return errors.NotFoundf("volume %s", volumeParams.Tag.Id())
			}
			volumeIds[i] = volume.VolumeId
		}
		errs, err := volumeSource.DestroyVolumes(volumeIds)
		if err != nil {
			return errors.Trace(err)
		}
		for i, err := range errs {
			tag := volumeParams[i].Tag
			if err == nil {
				remove = append(remove, tag)
				continue
			}
			// Failed to destroy volume; reschedule and update status.
			reschedule = append(reschedule, ops[tag])
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    tag.String(),
				Status: params.StatusDestroying,
				Info:   err.Error(),
			})
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if err := removeEntities(ctx, remove); err != nil {
		return errors.Annotate(err, "removing volumes from state")
	}
	return nil
}

// detachVolumes destroys volume attachments with the specified parameters.
func detachVolumes(ctx *context, ops map[params.MachineStorageId]*detachVolumeOp) error {
	volumeAttachmentParams := make([]storage.VolumeAttachmentParams, 0, len(ops))
	for _, op := range ops {
		volumeAttachmentParams = append(volumeAttachmentParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeAttachmentParamsBySource(
		ctx.environConfig, ctx.storageDir, volumeAttachmentParams,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	var remove []params.MachineStorageId
	for sourceName, volumeAttachmentParams := range paramsBySource {
		logger.Debugf("detaching volumes: %+v", volumeAttachmentParams)
		volumeSource := volumeSources[sourceName]
		errs, err := volumeSource.DetachVolumes(volumeAttachmentParams)
		if err != nil {
			return errors.Annotatef(err, "detaching volumes from source %q", sourceName)
		}
		for i, err := range errs {
			p := volumeAttachmentParams[i]
			statuses = append(statuses, params.EntityStatusArgs{
				Tag: p.Volume.String(),
				// TODO(axw) when we support multiple
				// attachment, we'll have to check if
				// there are any other attachments
				// before saying the status "detached".
				Status: params.StatusDetached,
			})
			id := params.MachineStorageId{
				MachineTag:    p.Machine.String(),
				AttachmentTag: p.Volume.String(),
			}
			status := &statuses[len(statuses)-1]
			if err != nil {
				reschedule = append(reschedule, ops[id])
				status.Status = params.StatusDetaching
				status.Info = err.Error()
				logger.Debugf(
					"failed to detach %s from %s: %v",
					names.ReadableString(p.Volume),
					names.ReadableString(p.Machine),
					err,
				)
				continue
			}
			remove = append(remove, id)
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if err := removeAttachments(ctx, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
	}
	for _, id := range remove {
		delete(ctx.volumeAttachments, id)
	}
	return nil
}

// volumeParamsBySource separates the volume parameters by volume source.
func volumeParamsBySource(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.VolumeParams,
) (map[string][]storage.VolumeParams, map[string]storage.VolumeSource, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.
	volumeSources := make(map[string]storage.VolumeSource)
	for _, params := range params {
		sourceName := string(params.Provider)
		if _, ok := volumeSources[sourceName]; ok {
			continue
		}
		volumeSource, err := volumeSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
		)
		if errors.Cause(err) == errNonDynamic {
			volumeSource = nil
		} else if err != nil {
			return nil, nil, errors.Annotate(err, "getting volume source")
		}
		volumeSources[sourceName] = volumeSource
	}
	paramsBySource := make(map[string][]storage.VolumeParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		volumeSource := volumeSources[sourceName]
		if volumeSource == nil {
			// Ignore nil volume sources; this means that the
			// volume should be created by the machine-provisioner.
			continue
		}
		err := volumeSource.ValidateVolumeParams(params)
		switch errors.Cause(err) {
		case nil:
			paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
		default:
			return nil, nil, errors.Annotatef(err, "invalid parameters for volume %s", params.Tag.Id())
		}
	}
	return paramsBySource, volumeSources, nil
}

// volumeAttachmentParamsBySource separates the volume attachment parameters by volume source.
func volumeAttachmentParamsBySource(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.VolumeAttachmentParams,
) (map[string][]storage.VolumeAttachmentParams, map[string]storage.VolumeSource, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.
	volumeSources := make(map[string]storage.VolumeSource)
	paramsBySource := make(map[string][]storage.VolumeAttachmentParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
		if _, ok := volumeSources[sourceName]; ok {
			continue
		}
		volumeSource, err := volumeSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
		)
		if err != nil {
			return nil, nil, errors.Annotate(err, "getting volume source")
		}
		volumeSources[sourceName] = volumeSource
	}
	return paramsBySource, volumeSources, nil
}

func setVolumeAttachmentInfo(ctx *context, volumeAttachments []storage.VolumeAttachment) error {
	if len(volumeAttachments) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list volume attachments in the
	// provider, by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing volumes that we fail to
	// record in state.
	errorResults, err := ctx.volumeAccessor.SetVolumeAttachmentInfo(
		volumeAttachmentsFromStorage(volumeAttachments),
	)
	if err != nil {
		return errors.Annotate(err, "publishing volumes to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				result.Error, "publishing attachment of %s to %s to state",
				names.ReadableString(volumeAttachments[i].Volume),
				names.ReadableString(volumeAttachments[i].Machine),
			)
		}
		// Record the volume attachment in the context.
		id := params.MachineStorageId{
			MachineTag:    volumeAttachments[i].Machine.String(),
			AttachmentTag: volumeAttachments[i].Volume.String(),
		}
		ctx.volumeAttachments[id] = volumeAttachments[i]
		removePendingVolumeAttachment(ctx, id)
	}
	return nil
}

type createVolumeOp struct {
	exponentialBackoff
	args storage.VolumeParams
}

func (op *createVolumeOp) key() interface{} {
	return op.args.Tag
}

type destroyVolumeOp struct {
	exponentialBackoff
	tag names.VolumeTag
}

func (op *destroyVolumeOp) key() interface{} {
	return op.tag
}

type attachVolumeOp struct {
	exponentialBackoff
	args storage.VolumeAttachmentParams
}

func (op *attachVolumeOp) key() interface{} {
	return params.MachineStorageId{
		MachineTag:    op.args.Machine.String(),
		AttachmentTag: op.args.Volume.String(),
	}
}

type detachVolumeOp struct {
	exponentialBackoff
	args storage.VolumeAttachmentParams
}

func (op *detachVolumeOp) key() interface{} {
	return params.MachineStorageId{
		MachineTag:    op.args.Machine.String(),
		AttachmentTag: op.args.Volume.String(),
	}
}
