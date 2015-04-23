// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

// filesystemsChanged is called when the lifecycle states of the filesystems
// with the provided IDs have been seen to have changed.
func filesystemsChanged(ctx *context, changes []string) error {
	tags := make([]names.Tag, len(changes))
	for i, change := range changes {
		tags[i] = names.NewFilesystemTag(change)
	}
	alive, dying, dead, err := storageEntityLife(ctx, tags)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(axw) wait for filesystems to have no attachments first.
	// We can then have the removal of the last attachment trigger
	// the filesystem's Life being transitioned to Dead, or watch
	// the attachments until they're all gone. We need to watch
	// attachments *anyway*, so we can probably integrate the two
	// things.
	logger.Debugf("filesystems alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if err := ensureDead(ctx, dying); err != nil {
		return errors.Annotate(err, "ensuring filesystems dead")
	}
	// Once the entities are Dead, they can be removed from state
	// after the corresponding cloud storage resources are removed.
	dead = append(dead, dying...)
	if len(alive)+len(dead) == 0 {
		return nil
	}

	// Get filesystem information for alive and dead filesystems, so
	// we can provision/deprovision.
	filesystemTags := make([]names.FilesystemTag, 0, len(alive)+len(dead))
	for _, tag := range alive {
		filesystemTags = append(filesystemTags, tag.(names.FilesystemTag))
	}
	for _, tag := range dead {
		filesystemTags = append(filesystemTags, tag.(names.FilesystemTag))
	}
	filesystemResults, err := ctx.filesystemAccessor.Filesystems(filesystemTags)
	if err != nil {
		return errors.Annotatef(err, "getting filesystem information")
	}

	// Deprovision "dead" filesystems, and then remove from state.
	if err := processDeadFilesystems(ctx, dead, filesystemResults[len(alive):]); err != nil {
		return errors.Annotate(err, "deprovisioning filesystems")
	}

	// Provision "alive" filesystems.
	if err := processAliveFilesystems(ctx, alive, filesystemResults[:len(alive)]); err != nil {
		return errors.Annotate(err, "provisioning filesystems")
	}

	return nil
}

// filesystemAttachmentsChanged is called when the lifecycle states of the filesystem
// attachments with the provided IDs have been seen to have changed.
func filesystemAttachmentsChanged(ctx *context, ids []params.MachineStorageId) error {
	alive, dying, dead, err := attachmentLife(ctx, ids)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("filesystem attachment alive: %v, dying: %v, dead: %v", alive, dying, dead)
	if len(dead) != 0 {
		// We should not see dead filesystem attachments;
		// attachments go directly from Dying to removed.
		logger.Debugf("unexpected dead filesystem attachments: %v", dead)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get filesystem information for alive and dying filesystem attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	filesystemAttachmentResults, err := ctx.filesystemAccessor.FilesystemAttachments(ids)
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

// processDeadFilesystems processes the FilesystemResults for Dead filesystems,
// deprovisioning filesystems and removing from state as necessary.
func processDeadFilesystems(ctx *context, tags []names.Tag, filesystemResults []params.FilesystemResult) error {
	for _, tag := range tags {
		delete(ctx.pendingFilesystems, tag.(names.FilesystemTag))
	}
	filesystems := make([]params.Filesystem, len(filesystemResults))
	for i, result := range filesystemResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "getting filesystem information for filesystem %q", tags[i].Id())
		}
		filesystems[i] = result.Result
	}
	if len(filesystems) == 0 {
		return nil
	}
	errorResults, err := destroyFilesystems(filesystems)
	if err != nil {
		return errors.Annotate(err, "destroying filesystems")
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
	filesystemAttachments := make([]params.FilesystemAttachment, len(filesystemAttachmentResults))
	for i, result := range filesystemAttachmentResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "getting information for filesystem attachment %v", ids[i])
		}
		filesystemAttachments[i] = result.Result
	}
	if len(filesystemAttachments) == 0 {
		return nil
	}
	errorResults, err := detachFilesystems(filesystemAttachments)
	if err != nil {
		return errors.Annotate(err, "detaching filesystems")
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

// processAliveFilesystems processes the FilesystemResults for Alive filesystems,
// provisioning filesystems and setting the info in state as necessary.
func processAliveFilesystems(ctx *context, tags []names.Tag, filesystemResults []params.FilesystemResult) error {
	// Filter out the already-provisioned filesystems.
	pending := make([]names.FilesystemTag, 0, len(tags))
	for i, result := range filesystemResults {
		filesystemTag := tags[i].(names.FilesystemTag)
		if result.Error == nil {
			// Filesystem is already provisioned: skip.
			logger.Debugf("filesystem %q is already provisioned, nothing to do", filesystemTag.Id())
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			ctx.filesystems[filesystemTag] = filesystem
			if filesystem.Volume != (names.VolumeTag{}) {
				// Ensure that volume-backed filesystems' block
				// devices are present even after creating the
				// filesystem, so that attachments can be made.
				maybeAddPendingVolumeBlockDevice(ctx, filesystem.Volume)
			}
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting filesystem information for filesystem %q", filesystemTag.Id(),
			)
		}
		// The filesystem has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, filesystemTag)
	}
	if len(pending) == 0 {
		return nil
	}
	paramsResults, err := ctx.filesystemAccessor.FilesystemParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	for i, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting filesystem parameters")
		}
		params, err := filesystemParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting filesystem parameters")
		}
		ctx.pendingFilesystems[pending[i]] = params
		if params.Volume != (names.VolumeTag{}) {
			// The filesystem is volume-backed: we must watch for
			// the corresponding block device. This will trigger a
			// one-time (for the volume) forced update of block
			// devices. If the block device is not immediately
			// available, then we rely on the watcher. The forced
			// update is necessary in case the block device was
			// added to state already, and we didn't observe it.
			maybeAddPendingVolumeBlockDevice(ctx, params.Volume)
		}
	}
	return nil
}

func maybeAddPendingVolumeBlockDevice(ctx *context, v names.VolumeTag) {
	if _, ok := ctx.volumeBlockDevices[v]; !ok {
		ctx.pendingVolumeBlockDevices.Add(v)
	}
}

// processPendingFilesystems creates as many of the pending filesystems
// as possible, first ensuring that their prerequisites have been met.
func processPendingFilesystems(ctx *context) error {
	if len(ctx.pendingFilesystems) == 0 {
		return nil
	}
	ready := make([]storage.FilesystemParams, 0, len(ctx.pendingFilesystems))
	for tag, filesystemParams := range ctx.pendingFilesystems {
		if filesystemParams.Volume != (names.VolumeTag{}) {
			// The filesystem is backed by a volume; ensure that
			// the volume is attached by virtue of there being a
			// matching block device on the machine.
			if _, ok := ctx.volumeBlockDevices[filesystemParams.Volume]; !ok {
				logger.Debugf(
					"filesystem %v backing-volume %v is not attached yet",
					filesystemParams.Tag.Id(),
					filesystemParams.Volume.Id(),
				)
				continue
			}
		}
		ready = append(ready, filesystemParams)
		delete(ctx.pendingFilesystems, tag)
	}
	if len(ready) == 0 {
		return nil
	}
	filesystems, err := createFilesystems(ctx, ready)
	if err != nil {
		return errors.Annotate(err, "creating filesystems")
	}
	if err := setFilesystemInfo(ctx, filesystems); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func setFilesystemInfo(ctx *context, filesystems []storage.Filesystem) error {
	if len(filesystems) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list filesystems in the provider,
	// by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing filesystems that we fail
	// to record in state.
	errorResults, err := ctx.filesystemAccessor.SetFilesystemInfo(
		filesystemsFromStorage(filesystems),
	)
	if err != nil {
		return errors.Annotate(err, "publishing filesystems to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				err, "publishing filesystem %s to state",
				filesystems[i].Tag.Id(),
			)
		}
		ctx.filesystems[filesystems[i].Tag] = filesystems[i]
	}
	return nil
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
	//
	// TODO(axw) record locally which filesystems have been attached this
	// session, and issue a reattach each time we restart. We should
	// limit this to machine-scoped filesystems to start with.
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range filesystemAttachmentResults {
		switch {
		case result.Error != nil && params.IsCodeNotProvisioned(result.Error):
			// The filesystem has not yet been attached, so
			// record its tag to enquire about parameters below.
			pending = append(pending, ids[i])
		case result.Error == nil:
			// Filesystem is already attached: skip.
			logger.Debugf(
				"%s is already attached to %s, nothing to do",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
			filesystemAttachment, err := filesystemAttachmentFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem attachment info")
			}
			ctx.filesystemAttachments[ids[i]] = filesystemAttachment
			delete(ctx.pendingFilesystemAttachments, ids[i])
		case result.Error != nil:
			return errors.Annotatef(
				result.Error, "getting information for attachment %v", ids[i],
			)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	paramsResults, err := ctx.filesystemAccessor.FilesystemAttachmentParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	for i, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting filesystem attachment parameters")
		}
		params, err := filesystemAttachmentParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting filesystem attachment parameters")
		}
		ctx.pendingFilesystemAttachments[pending[i]] = params
	}
	return nil
}

func processPendingFilesystemAttachments(ctx *context) error {
	if len(ctx.pendingFilesystemAttachments) == 0 {
		return nil
	}
	ready := make([]storage.FilesystemAttachmentParams, 0, len(ctx.pendingFilesystemAttachments))
	for id, params := range ctx.pendingFilesystemAttachments {
		filesystem, ok := ctx.filesystems[params.Filesystem]
		if !ok {
			logger.Debugf("filesystem %v has not been provisioned yet", params.Filesystem.Id())
			continue
		}
		if filesystem.Volume != (names.VolumeTag{}) {
			// The filesystem is volume-backed: if the filesystem
			// was created in another session, then the block device
			// may not have been seen yet. We must wait for the block
			// device watcher to trigger.
			if _, ok := ctx.volumeBlockDevices[filesystem.Volume]; !ok {
				logger.Debugf(
					"filesystem %v backing-volume %v is not attached yet",
					filesystem.Tag.Id(),
					filesystem.Volume.Id(),
				)
				continue
			}
		}
		// TODO(axw) watch machines in storageprovisioner
		if params.InstanceId == "" {
			logger.Debugf("machine %v has not been provisioned yet", params.Machine.Id())
			continue
		}
		if params.Path == "" {
			params.Path = filepath.Join(ctx.storageDir, params.Filesystem.Id())
		}
		params.FilesystemId = filesystem.FilesystemId
		ready = append(ready, params)
		delete(ctx.pendingFilesystemAttachments, id)
	}
	if len(ready) == 0 {
		return nil
	}
	filesystemAttachments, err := createFilesystemAttachments(ctx, ready)
	if err != nil {
		return errors.Annotate(err, "creating filesystem attachments")
	}
	if err := setFilesystemAttachmentInfo(ctx, filesystemAttachments); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func setFilesystemAttachmentInfo(ctx *context, filesystemAttachments []storage.FilesystemAttachment) error {
	if len(filesystemAttachments) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list filesystem attachments in the
	// provider, by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing filesystems that we fail to
	// record in state.
	errorResults, err := ctx.filesystemAccessor.SetFilesystemAttachmentInfo(
		filesystemAttachmentsFromStorage(filesystemAttachments),
	)
	if err != nil {
		return errors.Annotate(err, "publishing filesystems to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				result.Error, "publishing attachment of %s to %s to state",
				names.ReadableString(filesystemAttachments[i].Filesystem),
				names.ReadableString(filesystemAttachments[i].Machine),
			)
		}
		// Record the filesystem attachment in the context.
		ctx.filesystemAttachments[params.MachineStorageId{
			MachineTag:    filesystemAttachments[i].Machine.String(),
			AttachmentTag: filesystemAttachments[i].Filesystem.String(),
		}] = filesystemAttachments[i]
	}
	return nil
}

// createFilesystems creates filesystems with the specified parameters.
func createFilesystems(ctx *context, params []storage.FilesystemParams) ([]storage.Filesystem, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.

	// Create filesystem sources.
	filesystemSources := make(map[string]storage.FilesystemSource)
	for _, params := range params {
		sourceName := string(params.Provider)
		if _, ok := filesystemSources[sourceName]; ok {
			continue
		}
		if params.Volume != (names.VolumeTag{}) {
			filesystemSources[sourceName] = ctx.managedFilesystemSource
			continue
		}
		filesystemSource, err := filesystemSource(
			ctx.environConfig, ctx.storageDir, sourceName, params.Provider,
		)
		if err != nil {
			return nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
	}

	// Validate and gather filesystem parameters.
	paramsBySource := make(map[string][]storage.FilesystemParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		filesystemSource := filesystemSources[sourceName]
		err := filesystemSource.ValidateFilesystemParams(params)
		if err != nil {
			// TODO(axw) we should set an error status for params.Tag
			// here, and we should retry periodically.
			logger.Errorf("ignoring invalid filesystem: %v", err)
			continue
		}
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
	}

	var allFilesystems []storage.Filesystem
	for sourceName, params := range paramsBySource {
		filesystemSource := filesystemSources[sourceName]
		filesystems, err := filesystemSource.CreateFilesystems(params)
		if err != nil {
			return nil, errors.Annotatef(err, "creating filesystems from source %q", sourceName)
		}
		allFilesystems = append(allFilesystems, filesystems...)
	}
	return allFilesystems, nil
}

// createFilesystemAttachments creates filesystem attachments with the specified parameters.
func createFilesystemAttachments(
	ctx *context,
	params []storage.FilesystemAttachmentParams,
) ([]storage.FilesystemAttachment, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.
	filesystemSources := make(map[string]storage.FilesystemSource)
	paramsBySource := make(map[string][]storage.FilesystemAttachmentParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
		if _, ok := filesystemSources[sourceName]; ok {
			continue
		}
		filesystem := ctx.filesystems[params.Filesystem]
		if filesystem.Volume != (names.VolumeTag{}) {
			filesystemSources[sourceName] = ctx.managedFilesystemSource
			continue
		}
		filesystemSource, err := filesystemSource(
			ctx.environConfig, ctx.storageDir, sourceName, params.Provider,
		)
		if err != nil {
			return nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
	}
	var allFilesystemAttachments []storage.FilesystemAttachment
	for sourceName, params := range paramsBySource {
		filesystemSource := filesystemSources[sourceName]
		filesystemAttachments, err := filesystemSource.AttachFilesystems(params)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching filesystems from source %q", sourceName)
		}
		allFilesystemAttachments = append(allFilesystemAttachments, filesystemAttachments...)
	}
	return allFilesystemAttachments, nil
}

func destroyFilesystems(filesystems []params.Filesystem) ([]error, error) {
	panic("not implemented")
}

func detachFilesystems(attachments []params.FilesystemAttachment) ([]error, error) {
	panic("not implemented")
}

func filesystemsFromStorage(in []storage.Filesystem) []params.Filesystem {
	out := make([]params.Filesystem, len(in))
	for i, f := range in {
		paramsFilesystem := params.Filesystem{
			f.Tag.String(),
			"",
			f.FilesystemId,
			f.Size,
		}
		if f.Volume != (names.VolumeTag{}) {
			paramsFilesystem.VolumeTag = f.Volume.String()
		}
		out[i] = paramsFilesystem
	}
	return out
}

func filesystemAttachmentsFromStorage(in []storage.FilesystemAttachment) []params.FilesystemAttachment {
	out := make([]params.FilesystemAttachment, len(in))
	for i, f := range in {
		out[i] = params.FilesystemAttachment{
			f.Filesystem.String(),
			f.Machine.String(),
			f.Path,
		}
	}
	return out
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
		in.FilesystemId,
		in.Size,
	}, nil
}

func filesystemAttachmentFromParams(in params.FilesystemAttachment) (storage.FilesystemAttachment, error) {
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	machineTag, err := names.ParseMachineTag(in.MachineTag)
	if err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	return storage.FilesystemAttachment{
		filesystemTag,
		machineTag,
		in.MountPoint,
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
		filesystemTag,
		volumeTag,
		in.Size,
		providerType,
		in.Attributes,
	}, nil
}

func filesystemAttachmentParamsFromParams(in params.FilesystemAttachmentParams) (storage.FilesystemAttachmentParams, error) {
	machineTag, err := names.ParseMachineTag(in.MachineTag)
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
			Machine:    machineTag,
			InstanceId: instance.Id(in.InstanceId),
		},
		Filesystem: filesystemTag,
		Path:       in.MountPoint,
	}, nil
}
