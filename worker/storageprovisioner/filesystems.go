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
	// TODO(axw) wait for filesystems/filesystems to have no
	// attachments first. We can then have the removal of the
	// last attachment trigger the filesystem/filesystem's Life
	// being transitioned to Dead.
	// or watch the attachments until they're all gone. We need
	// to watch attachments *anyway*, so we can probably integrate
	// the two things.
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
	filesystemResults, err := ctx.filesystems.Filesystems(filesystemTags)
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
	if len(dead) != 0 {
		// We should not see dead filesystem attachments;
		// attachments go directly from Dying to removed.
		logger.Debugf("unexpected dead filesystem attachments: %f", dead)
	}
	if len(alive)+len(dying) == 0 {
		return nil
	}

	// Get filesystem information for alive and dying filesystem attachments, so
	// we can attach/detach.
	ids = append(alive, dying...)
	filesystemAttachmentResults, err := ctx.filesystems.FilesystemAttachments(ids)
	if err != nil {
		return errors.Annotatef(err, "getting filesystem attachment information")
	}

	// Deprovision Dying filesystem attachments.
	dyingFilesystemAttachmentResults := filesystemAttachmentResults[len(alive):]
	if err := processDyingFilesystemAttachments(ctx, dying, dyingFilesystemAttachmentResults); err != nil {
		return errors.Annotate(err, "deprovisioning filesystem attachments")
	}

	// Provision Alive filesystem attachments.
	aliveFilesystemAttachmentResults := filesystemAttachmentResults[:len(alive)]
	if err := processAliveFilesystemAttachments(ctx, alive, aliveFilesystemAttachmentResults); err != nil {
		return errors.Annotate(err, "provisioning filesystems")
	}

	return nil
}

// processDeadFilesystems processes the FilesystemResults for Dead filesystems,
// deprovisioning filesystems and removing from state as necessary.
func processDeadFilesystems(ctx *context, tags []names.Tag, filesystemResults []params.FilesystemResult) error {
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
			logger.Errorf("destroying %s: %f", names.ReadableString(tag), err)
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
			return errors.Annotatef(result.Error, "getting information for filesystem attachment %f", ids[i])
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
			logger.Errorf("detaching %f from %f: %f", ids[i].AttachmentTag, ids[i].MachineTag, err)
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
		if result.Error == nil {
			// Filesystem is already provisioned: skip.
			logger.Debugf("filesystem %q is already provisioned, nothing to do", tags[i].Id())
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting filesystem information for filesystem %q", tags[i].Id(),
			)
		}
		// The filesystem has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, tags[i].(names.FilesystemTag))
	}
	if len(pending) == 0 {
		return nil
	}
	paramsResults, err := ctx.filesystems.FilesystemParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	filesystemParams := make([]storage.FilesystemParams, 0, len(paramsResults))
	for _, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting filesystem parameters")
		}
		params, err := filesystemParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting filesystem parameters")
		}
		filesystemParams = append(filesystemParams, params)
	}
	filesystems, err := createFilesystems(ctx.environConfig, ctx.storageDir, filesystemParams)
	if err != nil {
		return errors.Annotate(err, "creating filesystems")
	}
	if len(filesystems) > 0 {
		// TODO(axw) we need to be able to list filesystems in the provider,
		// by environment, so that we can "harvest" them if they're
		// unknown. This will take care of killing filesystems that we fail
		// to record in state.
		errorResults, err := ctx.filesystems.SetFilesystemInfo(filesystems)
		if err != nil {
			return errors.Annotate(err, "publishing filesystems to state")
		}
		for i, result := range errorResults {
			if result.Error != nil {
				return errors.Annotatef(
					err, "publishing %s to state",
					filesystems[i].FilesystemTag,
				)
			}
		}
	}
	return nil
}

// processAliveFilesystems processes the FilesystemAttachmentResults for Alive
// filesystem attachments, attaching filesystems and setting the info in state
// as necessary.
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
		if result.Error == nil {
			// Filesystem attachment is already provisioned: skip.
			logger.Debugf(
				"%s is already attached to %s, nothing to do",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
			continue
		}
		if !params.IsCodeNotProvisioned(result.Error) {
			return errors.Annotatef(
				result.Error, "getting information for attachment %f", ids[i],
			)
		}
		// The filesystem has not yet been provisioned, so record its tag
		// to enquire about parameters below.
		pending = append(pending, ids[i])
	}
	if len(pending) == 0 {
		return nil
	}
	paramsResults, err := ctx.filesystems.FilesystemAttachmentParams(pending)
	if err != nil {
		return errors.Annotate(err, "getting filesystem params")
	}
	filesystemAttachmentParams := make([]storage.FilesystemAttachmentParams, 0, len(paramsResults))
	for _, result := range paramsResults {
		if result.Error != nil {
			return errors.Annotate(err, "getting filesystem attachment parameters")
		}
		params, err := filesystemAttachmentParamsFromParams(result.Result)
		if err != nil {
			return errors.Annotate(err, "getting filesystem attachment parameters")
		}
		if params.FilesystemId == "" || params.InstanceId == "" {
			// Don't attempt to attach to filesystems that haven't yet
			// been provisioned.
			//
			// TODO(axw) we should store a set of pending attachments
			// in the context, so that if when the filesystem is created
			// the attachment isn't created with it, we can then try
			// to attach.
			continue
		}
		if params.Path == "" {
			params.Path = filepath.Join(ctx.storageDir, params.Filesystem.Id())
		}
		filesystemAttachmentParams = append(filesystemAttachmentParams, params)
	}
	filesystemAttachments, err := createFilesystemAttachments(
		ctx.environConfig, ctx.storageDir, filesystemAttachmentParams,
	)
	if err != nil {
		return errors.Annotate(err, "creating filesystem attachments")
	}
	if err := setFilesystemAttachmentInfo(ctx, filesystemAttachments); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func setFilesystemAttachmentInfo(ctx *context, filesystemAttachments []params.FilesystemAttachment) error {
	if len(filesystemAttachments) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list filesystem attachments in the
	// provider, by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing filesystems that we fail to
	// record in state.
	errorResults, err := ctx.filesystems.SetFilesystemAttachmentInfo(filesystemAttachments)
	if err != nil {
		return errors.Annotate(err, "publishing filesystems to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				result.Error, "publishing attachment of %s to %s to state",
				filesystemAttachments[i].FilesystemTag,
				filesystemAttachments[i].MachineTag,
			)
		}
	}
	return nil
}

// createFilesystems creates filesystems with the specified parameters.
func createFilesystems(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.FilesystemParams,
) ([]params.Filesystem, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.
	filesystemSources := make(map[string]storage.FilesystemSource)
	paramsBySource := make(map[string][]storage.FilesystemParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
		if _, ok := filesystemSources[sourceName]; ok {
			continue
		}
		filesystemSource, err := filesystemSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
		)
		if err != nil {
			return nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
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
	return filesystemsFromStorage(allFilesystems), nil
}

// createFilesystemAttachments creates filesystem attachments with the specified parameters.
func createFilesystemAttachments(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.FilesystemAttachmentParams,
) ([]params.FilesystemAttachment, error) {
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
		filesystemSource, err := filesystemSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
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
	return filesystemAttachmentsFromStorage(allFilesystemAttachments), nil
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
		out[i] = params.Filesystem{
			f.Tag.String(),
			f.FilesystemId,
			f.Size,
		}
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

func filesystemParamsFromParams(in params.FilesystemParams) (storage.FilesystemParams, error) {
	filesystemTag, err := names.ParseFilesystemTag(in.FilesystemTag)
	if err != nil {
		return storage.FilesystemParams{}, errors.Trace(err)
	}
	providerType := storage.ProviderType(in.Provider)
	return storage.FilesystemParams{
		filesystemTag,
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
		Filesystem:   filesystemTag,
		FilesystemId: in.FilesystemId,
		Path:         in.MountPoint,
	}, nil
}
