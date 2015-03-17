// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
)

func volumesChanged(ctx *context, changes []string) error {
	tags := make([]names.Tag, len(changes))
	for i, change := range changes {
		tags[i] = names.NewVolumeTag(change)
	}

	lifeResults, err := ctx.life.Life(tags)
	if err != nil {
		return errors.Annotate(err, "getting volume lifecycle")
	}
	var alive, dying, dead []names.Tag
	for i, result := range lifeResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "failed to get life for volume %q", tags[i].Id())
		}
		switch result.Life {
		case params.Alive:
			alive = append(alive, tags[i])
		case params.Dying:
			dying = append(dying, tags[i])
		case params.Dead:
			dead = append(dead, tags[i])
		default:
			return errors.Errorf("invalid life cycle %v", result.Life)
		}
	}

	// If we can, advance "dying" volumes to "dead".
	if len(dying) > 0 {
		// TODO(axw) wait for volumes to have no attachments first.
		// We'll either have to retry periodically, or watch the
		// volume attachments until they're all gone. We need to
		// watch volume attachments *anyway*, so we can probably
		// integrate the two things.
		errorResults, err := ctx.life.EnsureDead(dying)
		if err != nil {
			return errors.Annotate(err, "ensuring volumes dead")
		}
		for i, result := range errorResults {
			if result.Error != nil {
				return errors.Annotatef(result.Error, "failed to ensure volume %q dead", dying[i].Id())
			}
			dead = append(dead, dying[i])
		}
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
	volumeResults, err := ctx.volumes.Volumes(volumeTags)
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
	destroyed := make([]names.Tag, 0, len(volumes))
	for i, err := range errorResults {
		if err != nil {
			logger.Errorf("destroying volume %q: %v", volumes[i].VolumeTag, err)
			continue
		}
		destroyed = append(destroyed, tags[i])
	}
	if len(destroyed) > 0 {
		errorResults, err := ctx.life.Remove(destroyed)
		if err != nil {
			return errors.Annotate(err, "removing volumes from state")
		}
		for i, result := range errorResults {
			if result.Error != nil {
				logger.Errorf("removing volume %q from state: %v", destroyed[i].Id(), result.Error)
			}
		}
	}
	return nil
}

func processAliveVolumes(ctx *context, tags []names.Tag, volumeResults []params.VolumeResult) error {
	// Filter out the already-provisioned volumes.
	pending := make([]names.VolumeTag, 0, len(tags))
	for i, result := range volumeResults {
		if result.Error == nil {
			// Volume is already provisioned: skip.
			logger.Debugf("volume %q is already provisioned, nothing to do", tags[i].Id())
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting volume information for volume %q", tags[i].Id(),
			)
		}
		// The volume has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, tags[i].(names.VolumeTag))
	}
	if len(pending) == 0 {
		return nil
	}
	paramsResults, err := ctx.volumes.VolumeParams(pending)
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
		errorResults, err := ctx.volumes.SetVolumeInfo(volumes)
		if err != nil {
			return errors.Annotate(err, "publishing volumes to state")
		}
		if err := errorResults.Combine(); err != nil {
			return errors.Annotate(err, "publishing volumes to state")
		}
		// TODO(axw) record volume attachment info in state.
		_ = volumeAttachments
	}
	return nil
}

func volumeParamsFromParams(in params.VolumeParams) (storage.VolumeParams, error) {
	volumeTag, err := names.ParseVolumeTag(in.VolumeTag)
	if err != nil {
		return storage.VolumeParams{}, errors.Trace(err)
	}
	var attachment *storage.VolumeAttachmentParams
	if in.MachineTag != "" {
		machineTag, err := names.ParseMachineTag(in.MachineTag)
		if err != nil {
			return storage.VolumeParams{}, errors.Trace(err)
		}
		attachment = &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine: machineTag,
				// TODO(axw) we need to pass the instance ID over the API too.
				InstanceId: instance.Id(""),
			},
			Volume: volumeTag,
		}
	}
	return storage.VolumeParams{
		volumeTag,
		in.Size,
		storage.ProviderType(in.Provider),
		in.Attributes,
		attachment,
	}, nil
}

// createVolumes creates volumes with the specified parameters.
func createVolumes(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.VolumeParams,
) ([]params.Volume, []params.VolumeAttachment, error) {
	paramsByProvider := make(map[storage.ProviderType][]storage.VolumeParams)
	for _, params := range params {
		paramsByProvider[params.Provider] = append(paramsByProvider[params.Provider], params)
	}
	// TODO(axw) move this to the main storageprovisioner, and have it
	// watch for changes to storage source configurations, updating
	// a map in-between calls to the volume/filesystem/attachment
	// event handlers.
	volumeSources := make(map[string]storage.VolumeSource)
	for providerType := range paramsByProvider {
		provider, err := registry.StorageProvider(providerType)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting storage provider %q", providerType)
		}
		// TODO(axw) once we have storage source configuration separate
		// from pools, we need to pass it in here.
		sourceName := string(providerType)
		attrs := make(map[string]interface{})
		if baseStorageDir != "" {
			storageDir := filepath.Join(baseStorageDir, sourceName)
			attrs[storage.ConfigStorageDir] = storageDir
		}
		sourceConfig, err := storage.NewConfig(sourceName, providerType, attrs)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting storage source %q config", sourceName)
		}
		source, err := provider.VolumeSource(environConfig, sourceConfig)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "getting storage source %q", sourceName)
		}
		volumeSources[sourceName] = source
	}
	var allVolumes []storage.Volume
	var allVolumeAttachments []storage.VolumeAttachment
	for providerType, params := range paramsByProvider {
		// TODO(axw) we should be returning source source names in the
		// storage params, rather than provider types.
		sourceName := string(providerType)
		volumeSource := volumeSources[sourceName]
		volumes, volumeAttachments, err := volumeSource.CreateVolumes(params)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "creating volumes from source %q", sourceName)
		}
		allVolumes = append(allVolumes, volumes...)
		allVolumeAttachments = append(allVolumeAttachments, volumeAttachments...)
	}
	// TODO(axw) translate volumes/attachments to params
	return volumesFromStorage(allVolumes), volumeAttachmentsFromStorage(allVolumeAttachments), nil
}

func destroyVolumes(volumes []params.Volume) ([]error, error) {
	panic("not implemented")
}

func volumesFromStorage(in []storage.Volume) []params.Volume {
	out := make([]params.Volume, len(in))
	for i, v := range in {
		out[i] = params.Volume{
			v.Tag.String(),
			v.VolumeId,
			v.Serial,
			v.Size,
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
