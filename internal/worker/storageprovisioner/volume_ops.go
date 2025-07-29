// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/rpc/params"
)

// createVolumes creates volumes with the specified parameters.
func createVolumes(ctx context.Context, deps *dependencies, ops map[names.VolumeTag]*createVolumeOp) error {
	volumeParams := make([]storage.VolumeParams, 0, len(ops))
	for _, op := range ops {
		volumeParams = append(volumeParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeParamsBySource(
		deps.config.StorageDir, volumeParams, deps.config.Registry,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var volumes []storage.Volume
	var volumeAttachments []storage.VolumeAttachment
	var statuses []params.EntityStatusArgs
	for sourceName, volumeParams := range paramsBySource {
		deps.config.Logger.Debugf(ctx, "creating volumes: %v", volumeParams)
		volumeSource := volumeSources[sourceName]
		validVolumeParams, validationErrors := validateVolumeParams(volumeSource, volumeParams)
		for i, err := range validationErrors {
			if err == nil {
				continue
			}
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    volumeParams[i].Tag.String(),
				Status: status.Error.String(),
				Info:   err.Error(),
			})
			deps.config.Logger.Debugf(ctx,
				"failed to validate parameters for %s: %v",
				names.ReadableString(volumeParams[i].Tag), err,
			)
		}
		volumeParams = validVolumeParams
		if len(volumeParams) == 0 {
			continue
		}
		results, err := volumeSource.CreateVolumes(ctx, volumeParams)
		if err != nil {
			return errors.Annotatef(err, "creating volumes from source %q", sourceName)
		}
		for i, result := range results {
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    volumeParams[i].Tag.String(),
				Status: status.Attaching.String(),
			})
			entityStatus := &statuses[len(statuses)-1]
			if result.Error != nil {
				// Reschedule the volume creation.
				reschedule = append(reschedule, ops[volumeParams[i].Tag])

				// Note: we keep the status as "pending" to indicate
				// that we will retry. When we distinguish between
				// transient and permanent errors, we will set the
				// status to "error" for permanent errors.
				entityStatus.Status = status.Pending.String()
				entityStatus.Info = result.Error.Error()
				deps.config.Logger.Debugf(ctx,
					"failed to create %s: %v",
					names.ReadableString(volumeParams[i].Tag),
					result.Error,
				)
				continue
			}
			volumes = append(volumes, *result.Volume)
			if result.VolumeAttachment != nil {
				entityStatus.Status = status.Attached.String()
				volumeAttachments = append(volumeAttachments, *result.VolumeAttachment)
			}
		}
	}
	scheduleOperations(deps, reschedule...)
	setStatus(ctx, deps, statuses)
	if len(volumes) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list volumes in the provider,
	// by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing volumes that we fail
	// to record in state.
	errorResults, err := deps.config.Volumes.SetVolumeInfo(ctx, volumesFromStorage(volumes))
	if err != nil {
		return errors.Annotate(err, "publishing volumes to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			deps.config.Logger.Errorf(ctx,
				"publishing volume %s to state: %v",
				volumes[i].Tag.Id(),
				result.Error,
			)
		}
	}
	for _, v := range volumes {
		updateVolume(deps, v)
	}
	// Note: the storage provisioner that creates a volume is also
	// responsible for creating the volume attachment. It is therefore
	// safe to set the volume attachment info after the volume info,
	// without leading to the possibility of concurrent, duplicate
	// attachments.
	err = setVolumeAttachmentInfo(ctx, deps, volumeAttachments)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// attachVolumes creates volume attachments with the specified parameters.
func attachVolumes(ctx context.Context, deps *dependencies, ops map[params.MachineStorageId]*attachVolumeOp) error {
	volumeAttachmentParams := make([]storage.VolumeAttachmentParams, 0, len(ops))
	for _, op := range ops {
		volumeAttachmentParams = append(volumeAttachmentParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeAttachmentParamsBySource(
		deps.config.StorageDir, volumeAttachmentParams, deps.config.Registry,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var volumeAttachments []storage.VolumeAttachment
	var statuses []params.EntityStatusArgs
	for sourceName, volumeAttachmentParams := range paramsBySource {
		deps.config.Logger.Debugf(ctx, "attaching volumes: %+v", volumeAttachmentParams)
		volumeSource := volumeSources[sourceName]
		if volumeSource == nil {
			// The storage provider does not support dynamic
			// storage, there's nothing for the provisioner
			// to do here.
			continue
		}
		results, err := volumeSource.AttachVolumes(ctx, volumeAttachmentParams)
		if err != nil {
			return errors.Annotatef(err, "attaching volumes from source %q", sourceName)
		}

		for i, result := range results {
			p := volumeAttachmentParams[i]
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    p.Volume.String(),
				Status: status.Attached.String(),
			})
			entityStatus := &statuses[len(statuses)-1]
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
				entityStatus.Status = status.Attaching.String()
				entityStatus.Info = result.Error.Error()
				deps.config.Logger.Warningf(ctx,
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
	scheduleOperations(deps, reschedule...)
	setStatus(ctx, deps, statuses)
	if err := createVolumeAttachmentPlans(ctx, deps, volumeAttachments); err != nil {
		return errors.Trace(err)
	}
	if err := setVolumeAttachmentInfo(ctx, deps, volumeAttachments); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// createVolumeAttachmentPlans creates a volume info plan in state, which notifies the machine
// agent of the target instance that something has been attached to it.
func createVolumeAttachmentPlans(ctx context.Context, deps *dependencies, volumeAttachments []storage.VolumeAttachment) error {
	// NOTE(gsamfira): should we merge this with setVolumeInfo?
	if len(volumeAttachments) == 0 {
		return nil
	}

	volumeAttachmentPlans := make([]params.VolumeAttachmentPlan, len(volumeAttachments))
	for i, val := range volumeAttachments {
		volumeAttachmentPlans[i] = volumeAttachmentPlanFromAttachment(val)
	}

	errorResults, err := deps.config.Volumes.CreateVolumeAttachmentPlans(ctx, volumeAttachmentPlans)
	if err != nil {
		return errors.Annotatef(err, "creating volume plans")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				result.Error, "creating volume plan of %s to %s to state",
				names.ReadableString(volumeAttachments[i].Volume),
				names.ReadableString(volumeAttachments[i].Machine),
			)
		}
		// Record the volume attachment in the context.
		id := params.MachineStorageId{
			MachineTag:    volumeAttachmentPlans[i].MachineTag,
			AttachmentTag: volumeAttachmentPlans[i].VolumeTag,
		}
		deps.volumeAttachments[id] = volumeAttachments[i]
		// removePendingVolumeAttachment(deps, id)
	}
	return nil
}

func volumeAttachmentPlanFromAttachment(attachment storage.VolumeAttachment) params.VolumeAttachmentPlan {
	var planInfo params.VolumeAttachmentPlanInfo
	if attachment.PlanInfo != nil {
		planInfo.DeviceAttributes = attachment.PlanInfo.DeviceAttributes
		planInfo.DeviceType = attachment.PlanInfo.DeviceType
	} else {
		planInfo.DeviceType = storage.DeviceTypeLocal
	}
	return params.VolumeAttachmentPlan{
		VolumeTag:  attachment.Volume.String(),
		MachineTag: attachment.Machine.String(),
		Life:       life.Alive,
		PlanInfo:   planInfo,
	}
}

// removeVolumes destroys or releases volumes with the specified parameters.
func removeVolumes(ctx context.Context, deps *dependencies, ops map[names.VolumeTag]*removeVolumeOp) error {
	tags := make([]names.VolumeTag, 0, len(ops))
	for tag := range ops {
		tags = append(tags, tag)
	}
	removeVolumeParams, err := removeVolumeParams(ctx, deps, tags)
	if err != nil {
		return errors.Trace(err)
	}
	volumeParams := make([]storage.VolumeParams, len(tags))
	removeVolumeParamsByTag := make(map[names.VolumeTag]params.RemoveVolumeParams)
	for i, args := range removeVolumeParams {
		removeVolumeParamsByTag[tags[i]] = args
		volumeParams[i] = storage.VolumeParams{
			Tag:      tags[i],
			Provider: storage.ProviderType(args.Provider),
		}
	}
	paramsBySource, volumeSources, err := volumeParamsBySource(
		deps.config.StorageDir, volumeParams, deps.config.Registry,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var remove []names.Tag
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	removeVolumes := func(tags []names.VolumeTag, ids []string, f func(context.Context, []string) ([]error, error)) error {
		if len(ids) == 0 {
			return nil
		}
		errs, err := f(ctx, ids)
		if err != nil {
			return errors.Trace(err)
		}
		for i, err := range errs {
			tag := tags[i]
			if wrench.IsActive("storageprovisioner", "RemoveVolume") {
				err = errors.New("wrench active")
			}
			if err == nil {
				remove = append(remove, tag)
				continue
			}
			// Failed to destroy or release volume; reschedule and update status.
			reschedule = append(reschedule, ops[tag])
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    tag.String(),
				Status: status.Error.String(),
				Info:   errors.Annotate(err, "destroying volume").Error(),
			})
		}
		return nil
	}
	for sourceName, volumeParams := range paramsBySource {
		deps.config.Logger.Debugf(ctx, "removing volumes from %q: %v", sourceName, volumeParams)
		volumeSource := volumeSources[sourceName]
		removeTags := make([]names.VolumeTag, len(volumeParams))
		removeParams := make([]params.RemoveVolumeParams, len(volumeParams))
		for i, args := range volumeParams {
			removeTags[i] = args.Tag
			removeParams[i] = removeVolumeParamsByTag[args.Tag]
		}
		destroyTags, destroyIds, releaseTags, releaseIds := partitionRemoveVolumeParams(removeTags, removeParams)
		if err := removeVolumes(destroyTags, destroyIds, volumeSource.DestroyVolumes); err != nil {
			return errors.Trace(err)
		}
		if err := removeVolumes(releaseTags, releaseIds, volumeSource.ReleaseVolumes); err != nil {
			return errors.Trace(err)
		}
	}
	scheduleOperations(deps, reschedule...)
	setStatus(ctx, deps, statuses)
	if err := removeEntities(ctx, deps, remove); err != nil {
		return errors.Annotate(err, "removing volumes from state")
	}
	return nil
}

func partitionRemoveVolumeParams(removeTags []names.VolumeTag, removeParams []params.RemoveVolumeParams) (
	destroyTags []names.VolumeTag, destroyIds []string,
	releaseTags []names.VolumeTag, releaseIds []string,
) {
	destroyTags = make([]names.VolumeTag, 0, len(removeParams))
	destroyIds = make([]string, 0, len(removeParams))
	releaseTags = make([]names.VolumeTag, 0, len(removeParams))
	releaseIds = make([]string, 0, len(removeParams))
	for i, args := range removeParams {
		tag := removeTags[i]
		if args.Destroy {
			destroyTags = append(destroyTags, tag)
			destroyIds = append(destroyIds, args.ProviderId)
		} else {
			releaseTags = append(releaseTags, tag)
			releaseIds = append(releaseIds, args.ProviderId)
		}
	}
	return
}

// detachVolumes destroys volume attachments with the specified parameters.
func detachVolumes(ctx context.Context, deps *dependencies, ops map[params.MachineStorageId]*detachVolumeOp) error {
	volumeAttachmentParams := make([]storage.VolumeAttachmentParams, 0, len(ops))
	for _, op := range ops {
		volumeAttachmentParams = append(volumeAttachmentParams, op.args)
	}
	paramsBySource, volumeSources, err := volumeAttachmentParamsBySource(
		deps.config.StorageDir, volumeAttachmentParams, deps.config.Registry,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	var remove []params.MachineStorageId
	for sourceName, volumeAttachmentParams := range paramsBySource {
		deps.config.Logger.Debugf(ctx, "detaching volumes: %+v", volumeAttachmentParams)
		volumeSource := volumeSources[sourceName]
		if volumeSource == nil {
			// The storage provider does not support dynamic
			// storage, there's nothing for the provisioner
			// to do here.
			continue
		}
		errs, err := volumeSource.DetachVolumes(ctx, volumeAttachmentParams)
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
				Status: status.Detached.String(),
			})
			id := params.MachineStorageId{
				MachineTag:    p.Machine.String(),
				AttachmentTag: p.Volume.String(),
			}
			entityStatus := &statuses[len(statuses)-1]
			if wrench.IsActive("storageprovisioner", "DetachVolume") {
				err = errors.New("wrench active")
			}
			if err != nil {
				reschedule = append(reschedule, ops[id])
				entityStatus.Status = status.Detaching.String()
				entityStatus.Info = err.Error()
				deps.config.Logger.Debugf(ctx,
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
	scheduleOperations(deps, reschedule...)
	setStatus(ctx, deps, statuses)
	if err := removeAttachments(ctx, deps, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
	}
	for _, id := range remove {
		delete(deps.volumeAttachments, id)
	}
	return nil
}

// volumeParamsBySource separates the volume parameters by volume source.
func volumeParamsBySource(
	baseStorageDir string,
	params []storage.VolumeParams,
	registry storage.ProviderRegistry,
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
			baseStorageDir, sourceName, params.Provider, registry,
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
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
	}
	return paramsBySource, volumeSources, nil
}

// validateVolumeParams validates a collection of volume parameters.
func validateVolumeParams(
	volumeSource storage.VolumeSource, volumeParams []storage.VolumeParams,
) ([]storage.VolumeParams, []error) {
	valid := make([]storage.VolumeParams, 0, len(volumeParams))
	results := make([]error, len(volumeParams))
	for i, params := range volumeParams {
		err := volumeSource.ValidateVolumeParams(params)
		if err == nil {
			valid = append(valid, params)
		}
		results[i] = err
	}
	return valid, results
}

// volumeAttachmentParamsBySource separates the volume attachment parameters by volume source.
func volumeAttachmentParamsBySource(
	baseStorageDir string,
	params []storage.VolumeAttachmentParams,
	registry storage.ProviderRegistry,
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
			baseStorageDir, sourceName, params.Provider, registry,
		)
		if errors.Cause(err) == errNonDynamic {
			volumeSource = nil
		} else if err != nil {
			return nil, nil, errors.Annotate(err, "getting volume source")
		}
		volumeSources[sourceName] = volumeSource
	}
	return paramsBySource, volumeSources, nil
}

func setVolumeAttachmentInfo(ctx context.Context, deps *dependencies, volumeAttachments []storage.VolumeAttachment) error {
	if len(volumeAttachments) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list volume attachments in the
	// provider, by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing volumes that we fail to
	// record in state.
	errorResults, err := deps.config.Volumes.SetVolumeAttachmentInfo(
		ctx,
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
		deps.volumeAttachments[id] = volumeAttachments[i]
		removePendingVolumeAttachment(deps, id)
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

type removeVolumeOp struct {
	exponentialBackoff
	tag names.VolumeTag
}

func (op *removeVolumeOp) key() interface{} {
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
