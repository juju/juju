// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
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
	// TODO(axw) wait for volumes to have no attachments first.
	// We can then have the removal of the last attachment trigger
	// the volume's Life being transitioned to Dead, or watch the
	// attachments until they're all gone. We need to watch
	// attachments *anyway*, so we can probably integrate the two
	// things.
	logger.Debugf("volumes alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if err := ensureDead(ctx, dying); err != nil {
		return errors.Annotate(err, "ensuring volumes dead")
	}
	// Once the entities are Dead, they can be removed from state
	// after the corresponding cloud storage resources are removed.
	dead = append(dead, dying...)
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
	volumeResults, err := ctx.volumeAccessor.Volumes(volumeTags)
	if err != nil {
		return errors.Annotatef(err, "getting volume information")
	}

	// Deprovision "dead" volumes, and then remove from state.
	if err := processDeadVolumes(ctx, dead, volumeResults[len(alive):]); err != nil {
		return errors.Annotate(err, "deprovisioning volumes")
	}

	// Provision "alive" volumes.
	if err := processAliveVolumes(ctx, alive, volumeResults[:len(alive)]); err != nil {
		return errors.Annotate(err, "provisioning volumes")
	}

	return nil
}

// volumeAttachmentsChanged is called when the lifecycle states of the volume
// attachments with the provided IDs have been seen to have changed.
func volumeAttachmentsChanged(ctx *context, ids []params.MachineStorageId) error {
	alive, dying, dead, err := attachmentLife(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("volume attachments alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(dead) != 0 {
		// We should not see dead volume attachments;
		// attachments go directly from Dying to removed.
		logger.Debugf("unexpected dead volume attachments: %v", dead)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get volume information for alive and dying volume attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	volumeAttachmentResults, err := ctx.volumeAccessor.VolumeAttachments(ids)
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

// processDeadVolumes processes the VolumeResults for Dead volumes,
// deprovisioning volumes and removing from state as necessary.
func processDeadVolumes(ctx *context, tags []names.Tag, volumeResults []params.VolumeResult) error {
	volumes := make([]params.Volume, len(volumeResults))
	for i, result := range volumeResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "getting volume information for volume %q", tags[i].Id())
		}
		volumes[i] = result.Result
	}
	if len(volumes) == 0 {
		return nil
	}
	errorResults, err := destroyVolumes(volumes)
	if err != nil {
		return errors.Annotate(err, "destroying volumes")
	}
	destroyed := make([]names.Tag, 0, len(tags))
	for i, tag := range tags {
		if err := errorResults[i]; err != nil {
			logger.Errorf("destroying %s: %v", names.ReadableString(tag), err)
			continue
		}
		destroyed = append(destroyed, tag)
	}
	if err := removeEntities(ctx, destroyed); err != nil {
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
		delete(ctx.pendingVolumeAttachments, id)
	}
	volumeAttachments := make([]params.VolumeAttachment, len(volumeAttachmentResults))
	for i, result := range volumeAttachmentResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "getting information for volume attachment %v", ids[i])
		}
		volumeAttachments[i] = result.Result
	}
	if len(volumeAttachments) == 0 {
		return nil
	}
	errorResults, err := detachVolumes(volumeAttachments)
	if err != nil {
		return errors.Annotate(err, "detaching volumes")
	}
	detached := make([]params.MachineStorageId, 0, len(ids))
	for i, id := range ids {
		if err := errorResults[i]; err != nil {
			logger.Errorf("detaching %v from %v: %v", ids[i].AttachmentTag, ids[i].MachineTag, err)
			continue
		}
		detached = append(detached, id)
	}
	if err := removeAttachments(ctx, detached); err != nil {
		return errors.Annotate(err, "removing attachments from state")
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
			ctx.volumes[volumeTag] = volume
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
	paramsResults, err := ctx.volumeAccessor.VolumeParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting volume params")
	}
	volumeParams := make([]storage.VolumeParams, 0, len(paramsResults))
	for _, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting volume parameters")
		}
		params, err := volumeParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting volume parameters")
		}
		volumeParams = append(volumeParams, params)
	}
	volumes, volumeAttachments, err := createVolumes(
		ctx.environConfig, ctx.storageDir, volumeParams,
	)
	if err != nil {
		return errors.Annotate(err, "creating volumes")
	}
	if len(volumes) > 0 {
		// TODO(axw) we need to be able to list volumes in the provider,
		// by environment, so that we can "harvest" them if they're
		// unknown. This will take care of killing volumes that we fail
		// to record in state.
		errorResults, err := ctx.volumeAccessor.SetVolumeInfo(
			volumesFromStorage(volumes),
		)
		if err != nil {
			return errors.Annotate(err, "publishing volumes to state")
		}
		for i, result := range errorResults {
			if result.Error != nil {
				return errors.Annotatef(
					err, "publishing volume %s to state",
					volumes[i].Tag.Id(),
				)
			}
		}
		for _, v := range volumes {
			ctx.volumes[v.Tag] = v
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
	//
	// TODO(axw) record locally which volumes have been attached this
	// session, and issue a reattach each time we restart. We should
	// limit this to machine-scoped volumes to start with.
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range volumeAttachmentResults {
		if result.Error == nil {
			// Volume attachment is already provisioned: skip.
			logger.Debugf(
				"%s is already attached to %s, nothing to do",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
			volumeAttachment, err := volumeAttachmentFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting volume attachment info")
			}
			ctx.volumeAttachments[ids[i]] = volumeAttachment
			delete(ctx.pendingVolumeAttachments, ids[i])
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
	paramsResults, err := ctx.volumeAccessor.VolumeAttachmentParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting volume params")
	}
	for i, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting volume attachment parameters")
		}
		params, err := volumeAttachmentParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting volume attachment parameters")
		}
		ctx.pendingVolumeAttachments[pending[i]] = params
	}
	return nil
}

// processPendingVolumeAttachments creates as many of the pending volume
// attachments as possible, first ensuring that their prerequisites have
// been met.
func processPendingVolumeAttachments(ctx *context) error {
	if len(ctx.pendingVolumeAttachments) == 0 {
		return nil
	}
	ready := make([]storage.VolumeAttachmentParams, 0, len(ctx.pendingVolumeAttachments))
	for id, params := range ctx.pendingVolumeAttachments {
		volume, ok := ctx.volumes[params.Volume]
		if !ok {
			// volume hasn't been provisioned yet
			logger.Debugf("volume %v has not been provisioned yet", params.Volume.Id())
			continue
		}
		// TODO(axw) watch machines in storageprovisioner
		if params.InstanceId == "" {
			logger.Debugf("machine %v has not been provisioned yet", params.Machine.Id())
			continue
		}
		params.VolumeId = volume.VolumeId
		ready = append(ready, params)
		delete(ctx.pendingVolumeAttachments, id)
	}
	if len(ready) == 0 {
		return nil
	}
	volumeAttachments, err := createVolumeAttachments(ctx.environConfig, ctx.storageDir, ready)
	if err != nil {
		return errors.Annotate(err, "creating volume attachments")
	}
	if err := setVolumeAttachmentInfo(ctx, volumeAttachments); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// createVolumes creates volumes with the specified parameters.
func createVolumes(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.VolumeParams,
) ([]storage.Volume, []storage.VolumeAttachment, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.

	// Create volume sources.
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

	// Validate and gather volume parameters.
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
		case storage.ErrVolumeNeedsInstance:
			// TODO(axw) defer creation of volume until instance
			// is created. This requires that we watch machines.
			//
			// For now, rely on the worker bouncing to retry.
			return nil, nil, err
		default:
			// TODO(axw) we should set an error status for params.Tag here.
			logger.Errorf("ignoring invalid volume parameters: %v", err)
		}
	}

	var allVolumes []storage.Volume
	var allVolumeAttachments []storage.VolumeAttachment
	for sourceName, params := range paramsBySource {
		volumeSource := volumeSources[sourceName]
		volumes, volumeAttachments, err := volumeSource.CreateVolumes(params)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "creating volumes from source %q", sourceName)
		}
		allVolumes = append(allVolumes, volumes...)
		allVolumeAttachments = append(allVolumeAttachments, volumeAttachments...)
	}
	return allVolumes, allVolumeAttachments, nil
}

// createVolumeAttachments creates volume attachments with the specified parameters.
func createVolumeAttachments(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.VolumeAttachmentParams,
) ([]storage.VolumeAttachment, error) {
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
			return nil, errors.Annotate(err, "getting volume source")
		}
		volumeSources[sourceName] = volumeSource
	}
	var allVolumeAttachments []storage.VolumeAttachment
	for sourceName, params := range paramsBySource {
		volumeSource := volumeSources[sourceName]
		volumeAttachments, err := volumeSource.AttachVolumes(params)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching volumes from source %q", sourceName)
		}
		allVolumeAttachments = append(allVolumeAttachments, volumeAttachments...)
	}
	return allVolumeAttachments, nil
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
		ctx.volumeAttachments[params.MachineStorageId{
			MachineTag:    volumeAttachments[i].Machine.String(),
			AttachmentTag: volumeAttachments[i].Volume.String(),
		}] = volumeAttachments[i]
	}
	return nil
}

func destroyVolumes(volumes []params.Volume) ([]error, error) {
	panic("not implemented")
}

func detachVolumes(attachments []params.VolumeAttachment) ([]error, error) {
	panic("not implemented")
}

func volumesFromStorage(in []storage.Volume) []params.Volume {
	out := make([]params.Volume, len(in))
	for i, v := range in {
		out[i] = params.Volume{
			v.Tag.String(),
			v.VolumeId,
			v.HardwareId,
			v.Size,
			v.Persistent,
		}
	}
	return out
}

func volumeAttachmentsFromStorage(in []storage.VolumeAttachment) []params.VolumeAttachment {
	out := make([]params.VolumeAttachment, len(in))
	for i, v := range in {
		out[i] = params.VolumeAttachment{
			v.Volume.String(),
			v.Machine.String(),
			v.DeviceName,
			v.ReadOnly,
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
		in.VolumeId,
		in.HardwareId,
		in.Size,
		in.Persistent,
	}, nil
}

func volumeAttachmentFromParams(in params.VolumeAttachment) (storage.VolumeAttachment, error) {
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return storage.VolumeAttachment{}, errors.Trace(err)
	}
	machineTag, err := names.ParseMachineTag(in.MachineTag)
	if err != nil {
		return storage.VolumeAttachment{}, errors.Trace(err)
	}
	return storage.VolumeAttachment{
		volumeTag,
		machineTag,
		in.DeviceName,
		in.ReadOnly,
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
		machineTag, err := names.ParseMachineTag(in.Attachment.MachineTag)
		if err != nil {
			return storage.VolumeParams{}, errors.Annotate(
				err, "parsing attachment machine tag",
			)
		}
		attachment = &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider:   providerType,
				Machine:    machineTag,
				InstanceId: instance.Id(in.Attachment.InstanceId),
			},
			Volume: volumeTag,
		}
	}
	return storage.VolumeParams{
		volumeTag,
		in.Size,
		providerType,
		in.Attributes,
		attachment,
	}, nil
}

func volumeAttachmentParamsFromParams(in params.VolumeAttachmentParams) (storage.VolumeAttachmentParams, error) {
	machineTag, err := names.ParseMachineTag(in.MachineTag)
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
			Machine:    machineTag,
			InstanceId: instance.Id(in.InstanceId),
		},
		Volume: volumeTag,
	}, nil
}
