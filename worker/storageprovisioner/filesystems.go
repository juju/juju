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
	logger.Debugf("filesystems alive: %v, dying: %v, dead: %v", alive, dying, dead)
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
	filesystemResults, err := ctx.filesystemAccessor.Filesystems(filesystemTags)
	if err != nil {
		return errors.Annotatef(err, "getting filesystem information")
	}

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

// processDyingFilesystems processes the FilesystemResults for Dying filesystems,
// removing them from provisioning-pending as necessary, and storing the current
// filesystem info for provisioned filesystems so that attachments may be destroyed.
func processDyingFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	for _, tag := range tags {
		delete(ctx.pendingFilesystems, tag)
	}
	for i, result := range filesystemResults {
		tag := tags[i]
		if result.Error == nil {
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			ctx.filesystems[tag] = filesystem
		} else if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(result.Error, "getting information for filesystem %s", tag.Id())
		}
	}
	return nil
}

// processDeadFilesystems processes the FilesystemResults for Dead filesystems,
// deprovisioning filesystems and removing from state as necessary.
func processDeadFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	for _, tag := range tags {
		delete(ctx.pendingFilesystems, tag)
	}
	var destroy []names.FilesystemTag
	var remove []names.Tag
	for i, result := range filesystemResults {
		tag := tags[i]
		if result.Error == nil {
			logger.Debugf("filesystem %s is provisioned, queuing for deprovisioning", tag.Id())
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			ctx.filesystems[tag] = filesystem
			destroy = append(destroy, tag)
			continue
		}
		if params.IsCodeNotProvisioned(result.Error) {
			logger.Debugf("filesystem %s is not provisioned, queuing for removal", tag.Id())
			remove = append(remove, tag)
			continue
		}
		return errors.Annotatef(result.Error, "getting filesystem information for filesystem %s", tag.Id())
	}
	if len(destroy)+len(remove) == 0 {
		return nil
	}
	if len(destroy) > 0 {
		errorResults, err := destroyFilesystems(ctx, destroy)
		if err != nil {
			return errors.Annotate(err, "destroying filesystems")
		}
		for i, tag := range destroy {
			if err := errorResults[i]; err != nil {
				return errors.Annotatef(err, "destroying %s", names.ReadableString(tag))
			}
			remove = append(remove, tag)
		}
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
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		delete(ctx.pendingFilesystemAttachments, id)
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
		for i, params := range attachmentParams {
			ctx.pendingDyingFilesystemAttachments[detach[i]] = params
		}
	}
	if len(remove) > 0 {
		if err := removeAttachments(ctx, remove); err != nil {
			return errors.Annotate(err, "removing attachments from state")
		}
	}
	return nil
}

// processAliveFilesystems processes the FilesystemResults for Alive filesystems,
// provisioning filesystems and setting the info in state as necessary.
func processAliveFilesystems(ctx *context, tags []names.FilesystemTag, filesystemResults []params.FilesystemResult) error {
	// Filter out the already-provisioned filesystems.
	pending := make([]names.FilesystemTag, 0, len(tags))
	for i, result := range filesystemResults {
		tag := tags[i]
		if result.Error == nil {
			// Filesystem is already provisioned: skip.
			logger.Debugf("filesystem %q is already provisioned, nothing to do", tag.Id())
			filesystem, err := filesystemFromParams(result.Result)
			if err != nil {
				return errors.Annotate(err, "getting filesystem info")
			}
			ctx.filesystems[tag] = filesystem
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
	paramsResults, err := ctx.filesystemAccessor.FilesystemParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	for i, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(result.Error, "getting filesystem parameters")
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
		logger.Tracef("no pending filesystems")
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
				result.Error, "publishing filesystem %s to state",
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
	pending := make([]params.MachineStorageId, 0, len(ids))
	for i, result := range filesystemAttachmentResults {
		if result.Error == nil {
			delete(ctx.pendingFilesystemAttachments, ids[i])
			// Filesystem attachment is already provisioned: if we
			// didn't (re)attach in this session, then we must do
			// so now.
			action := "nothing to do"
			if _, ok := ctx.filesystemAttachments[ids[i]]; !ok {
				// Not yet (re)attached in this session.
				pending = append(pending, ids[i])
				action = "will reattach"
			}
			logger.Debugf(
				"%s is already attached to %s, %s",
				ids[i].AttachmentTag, ids[i].MachineTag, action,
			)
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
		if params.InstanceId == "" {
			watchMachine(ctx, params.Machine)
		}
		ctx.pendingFilesystemAttachments[pending[i]] = params
	}
	return nil
}

// filesystemAttachmentParams obtains the specified attachments' parameters.
func filesystemAttachmentParams(
	ctx *context, ids []params.MachineStorageId,
) ([]storage.FilesystemAttachmentParams, error) {
	paramsResults, err := ctx.filesystemAccessor.FilesystemAttachmentParams(ids)
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

func processPendingFilesystemAttachments(ctx *context) error {
	if len(ctx.pendingFilesystemAttachments) == 0 {
		logger.Tracef("no pending filesystem attachments")
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

func processPendingDyingFilesystemAttachments(ctx *context) error {
	if len(ctx.pendingDyingFilesystemAttachments) == 0 {
		logger.Tracef("no pending, dying filesystem attachments")
		return nil
	}
	var detach []storage.FilesystemAttachmentParams
	var remove []params.MachineStorageId
	for id, params := range ctx.pendingDyingFilesystemAttachments {
		if _, ok := ctx.filesystems[params.Filesystem]; !ok {
			// Wait until the filesystem info is known.
			continue
		}
		delete(ctx.pendingDyingFilesystemAttachments, id)
		detach = append(detach, params)
		remove = append(remove, id)
	}
	if len(detach) == 0 {
		return nil
	}
	if err := detachFilesystems(ctx, detach); err != nil {
		return errors.Annotate(err, "detaching filesystems")
	}
	if err := removeAttachments(ctx, remove); err != nil {
		return errors.Annotate(err, "removing attachments from state")
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
		logger.Debugf("creating filesystems: %v", params)
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
	paramsBySource, filesystemSources, err := filesystemAttachmentParamsBySource(ctx, params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var allFilesystemAttachments []storage.FilesystemAttachment
	for sourceName, params := range paramsBySource {
		logger.Debugf("attaching filesystems: %v", params)
		filesystemSource := filesystemSources[sourceName]
		filesystemAttachments, err := filesystemSource.AttachFilesystems(params)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching filesystems from source %q", sourceName)
		}
		allFilesystemAttachments = append(allFilesystemAttachments, filesystemAttachments...)
	}
	return allFilesystemAttachments, nil
}

func destroyFilesystems(ctx *context, tags []names.FilesystemTag) ([]error, error) {
	// TODO(axw) add storage.FilesystemSource.DestroyFilesystems
	return make([]error, len(tags)), nil
}

func detachFilesystems(ctx *context, attachments []storage.FilesystemAttachmentParams) error {
	paramsBySource, filesystemSources, err := filesystemAttachmentParamsBySource(ctx, attachments)
	if err != nil {
		return errors.Trace(err)
	}
	for sourceName, params := range paramsBySource {
		logger.Debugf("detaching filesystems: %v", params)
		filesystemSource := filesystemSources[sourceName]
		if err := filesystemSource.DetachFilesystems(params); err != nil {
			return errors.Annotatef(err, "detaching filesystems from source %q", sourceName)
		}
	}
	return nil
}

func filesystemAttachmentParamsBySource(
	ctx *context, params []storage.FilesystemAttachmentParams,
) (map[string][]storage.FilesystemAttachmentParams, map[string]storage.FilesystemSource, error) {
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
			return nil, nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
	}
	return paramsBySource, filesystemSources, nil
}

func filesystemsFromStorage(in []storage.Filesystem) []params.Filesystem {
	out := make([]params.Filesystem, len(in))
	for i, f := range in {
		paramsFilesystem := params.Filesystem{
			f.Tag.String(),
			"",
			params.FilesystemInfo{
				f.FilesystemId,
				f.Size,
			},
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
			params.FilesystemAttachmentInfo{
				f.Path,
				f.ReadOnly,
			},
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
		filesystemTag,
		volumeTag,
		in.Size,
		providerType,
		in.Attributes,
		in.Tags,
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
			ReadOnly:   in.ReadOnly,
		},
		Filesystem:   filesystemTag,
		FilesystemId: in.FilesystemId,
		Path:         in.MountPoint,
	}, nil
}
