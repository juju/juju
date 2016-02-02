// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

// createFilesystems creates filesystems with the specified parameters.
func createFilesystems(ctx *context, ops map[names.FilesystemTag]*createFilesystemOp) error {
	filesystemParams := make([]storage.FilesystemParams, 0, len(ops))
	for _, op := range ops {
		filesystemParams = append(filesystemParams, op.args)
	}
	paramsBySource, filesystemSources, err := filesystemParamsBySource(
		ctx.modelConfig, ctx.config.StorageDir,
		filesystemParams, ctx.managedFilesystemSource,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var filesystems []storage.Filesystem
	var statuses []params.EntityStatusArgs
	for sourceName, filesystemParams := range paramsBySource {
		logger.Debugf("creating filesystems: %v", filesystemParams)
		filesystemSource := filesystemSources[sourceName]
		validFilesystemParams, validationErrors := validateFilesystemParams(
			filesystemSource, filesystemParams,
		)
		for i, err := range validationErrors {
			if err == nil {
				continue
			}
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    filesystemParams[i].Tag.String(),
				Status: params.StatusError,
				Info:   err.Error(),
			})
			logger.Debugf(
				"failed to validate parameters for %s: %v",
				names.ReadableString(filesystemParams[i].Tag), err,
			)
		}
		filesystemParams = validFilesystemParams
		if len(filesystemParams) == 0 {
			continue
		}
		results, err := filesystemSource.CreateFilesystems(filesystemParams)
		if err != nil {
			return errors.Annotatef(err, "creating filesystems from source %q", sourceName)
		}
		for i, result := range results {
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    filesystemParams[i].Tag.String(),
				Status: params.StatusAttaching,
			})
			status := &statuses[len(statuses)-1]
			if result.Error != nil {
				// Reschedule the filesystem creation.
				reschedule = append(reschedule, ops[filesystemParams[i].Tag])

				// Note: we keep the status as "pending" to indicate
				// that we will retry. When we distinguish between
				// transient and permanent errors, we will set the
				// status to "error" for permanent errors.
				status.Status = params.StatusPending
				status.Info = result.Error.Error()
				logger.Debugf(
					"failed to create %s: %v",
					names.ReadableString(filesystemParams[i].Tag),
					result.Error,
				)
				continue
			}
			filesystems = append(filesystems, *result.Filesystem)
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if len(filesystems) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list filesystems in the provider,
	// by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing filesystems that we fail
	// to record in state.
	errorResults, err := ctx.config.Filesystems.SetFilesystemInfo(filesystemsFromStorage(filesystems))
	if err != nil {
		return errors.Annotate(err, "publishing filesystems to state")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			logger.Errorf(
				"publishing filesystem %s to state: %v",
				filesystems[i].Tag.Id(),
				result.Error,
			)
		}
	}
	for _, v := range filesystems {
		updateFilesystem(ctx, v)
	}
	return nil
}

// attachFilesystems creates filesystem attachments with the specified parameters.
func attachFilesystems(ctx *context, ops map[params.MachineStorageId]*attachFilesystemOp) error {
	filesystemAttachmentParams := make([]storage.FilesystemAttachmentParams, 0, len(ops))
	for _, op := range ops {
		args := op.args
		if args.Path == "" {
			args.Path = filepath.Join(ctx.config.StorageDir, args.Filesystem.Id())
		}
		filesystemAttachmentParams = append(filesystemAttachmentParams, args)
	}
	paramsBySource, filesystemSources, err := filesystemAttachmentParamsBySource(
		ctx.modelConfig,
		ctx.config.StorageDir,
		filesystemAttachmentParams,
		ctx.filesystems,
		ctx.managedFilesystemSource,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var filesystemAttachments []storage.FilesystemAttachment
	var statuses []params.EntityStatusArgs
	for sourceName, filesystemAttachmentParams := range paramsBySource {
		logger.Debugf("attaching filesystems: %+v", filesystemAttachmentParams)
		filesystemSource := filesystemSources[sourceName]
		results, err := filesystemSource.AttachFilesystems(filesystemAttachmentParams)
		if err != nil {
			return errors.Annotatef(err, "attaching filesystems from source %q", sourceName)
		}
		for i, result := range results {
			p := filesystemAttachmentParams[i]
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    p.Filesystem.String(),
				Status: params.StatusAttached,
			})
			status := &statuses[len(statuses)-1]
			if result.Error != nil {
				// Reschedule the filesystem attachment.
				id := params.MachineStorageId{
					MachineTag:    p.Machine.String(),
					AttachmentTag: p.Filesystem.String(),
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
					names.ReadableString(p.Filesystem),
					names.ReadableString(p.Machine),
					result.Error,
				)
				continue
			}
			filesystemAttachments = append(filesystemAttachments, *result.FilesystemAttachment)
		}
	}
	scheduleOperations(ctx, reschedule...)
	setStatus(ctx, statuses)
	if err := setFilesystemAttachmentInfo(ctx, filesystemAttachments); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// destroyFilesystems destroys filesystems with the specified parameters.
func destroyFilesystems(ctx *context, ops map[names.FilesystemTag]*destroyFilesystemOp) error {
	tags := make([]names.FilesystemTag, 0, len(ops))
	for tag := range ops {
		tags = append(tags, tag)
	}
	filesystemParams, err := filesystemParams(ctx, tags)
	if err != nil {
		return errors.Trace(err)
	}
	paramsBySource, filesystemSources, err := filesystemParamsBySource(
		ctx.modelConfig, ctx.config.StorageDir,
		filesystemParams, ctx.managedFilesystemSource,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var remove []names.Tag
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	for sourceName, filesystemParams := range paramsBySource {
		logger.Debugf("destroying filesystems from %q: %v", sourceName, filesystemParams)
		filesystemSource := filesystemSources[sourceName]
		validFilesystemParams, validationErrors := validateFilesystemParams(filesystemSource, filesystemParams)
		for i, err := range validationErrors {
			if err == nil {
				continue
			}
			statuses = append(statuses, params.EntityStatusArgs{
				Tag:    filesystemParams[i].Tag.String(),
				Status: params.StatusError,
				Info:   err.Error(),
			})
			logger.Debugf(
				"failed to validate parameters for %s: %v",
				names.ReadableString(filesystemParams[i].Tag), err,
			)
		}
		filesystemParams = validFilesystemParams
		if len(filesystemParams) == 0 {
			continue
		}
		filesystemIds := make([]string, len(filesystemParams))
		for i, filesystemParams := range filesystemParams {
			filesystem, ok := ctx.filesystems[filesystemParams.Tag]
			if !ok {
				return errors.NotFoundf("filesystem %s", filesystemParams.Tag.Id())
			}
			filesystemIds[i] = filesystem.FilesystemId
		}
		errs, err := filesystemSource.DestroyFilesystems(filesystemIds)
		if err != nil {
			return errors.Trace(err)
		}
		for i, err := range errs {
			tag := filesystemParams[i].Tag
			if err == nil {
				remove = append(remove, tag)
				continue
			}
			// Failed to destroy filesystem; reschedule and update status.
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
		return errors.Annotate(err, "removing filesystems from state")
	}
	return nil
}

// detachFilesystems destroys filesystem attachments with the specified parameters.
func detachFilesystems(ctx *context, ops map[params.MachineStorageId]*detachFilesystemOp) error {
	filesystemAttachmentParams := make([]storage.FilesystemAttachmentParams, 0, len(ops))
	for _, op := range ops {
		filesystemAttachmentParams = append(filesystemAttachmentParams, op.args)
	}
	paramsBySource, filesystemSources, err := filesystemAttachmentParamsBySource(
		ctx.modelConfig, ctx.config.StorageDir,
		filesystemAttachmentParams,
		ctx.filesystems,
		ctx.managedFilesystemSource,
	)
	if err != nil {
		return errors.Trace(err)
	}
	var reschedule []scheduleOp
	var statuses []params.EntityStatusArgs
	var remove []params.MachineStorageId
	for sourceName, filesystemAttachmentParams := range paramsBySource {
		logger.Debugf("detaching filesystems: %+v", filesystemAttachmentParams)
		filesystemSource := filesystemSources[sourceName]
		errs, err := filesystemSource.DetachFilesystems(filesystemAttachmentParams)
		if err != nil {
			return errors.Annotatef(err, "detaching filesystems from source %q", sourceName)
		}
		for i, err := range errs {
			p := filesystemAttachmentParams[i]
			statuses = append(statuses, params.EntityStatusArgs{
				Tag: p.Filesystem.String(),
				// TODO(axw) when we support multiple
				// attachment, we'll have to check if
				// there are any other attachments
				// before saying the status "detached".
				Status: params.StatusDetached,
			})
			id := params.MachineStorageId{
				MachineTag:    p.Machine.String(),
				AttachmentTag: p.Filesystem.String(),
			}
			status := &statuses[len(statuses)-1]
			if err != nil {
				reschedule = append(reschedule, ops[id])
				status.Status = params.StatusDetaching
				status.Info = err.Error()
				logger.Debugf(
					"failed to detach %s from %s: %v",
					names.ReadableString(p.Filesystem),
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
		delete(ctx.filesystemAttachments, id)
	}
	return nil
}

// filesystemParamsBySource separates the filesystem parameters by filesystem source.
func filesystemParamsBySource(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.FilesystemParams,
	managedFilesystemSource storage.FilesystemSource,
) (map[string][]storage.FilesystemParams, map[string]storage.FilesystemSource, error) {
	// TODO(axw) later we may have multiple instantiations (sources)
	// for a storage provider, e.g. multiple Ceph installations. For
	// now we assume a single source for each provider type, with no
	// configuration.
	filesystemSources := make(map[string]storage.FilesystemSource)
	for _, params := range params {
		sourceName := string(params.Provider)
		if _, ok := filesystemSources[sourceName]; ok {
			continue
		}
		if params.Volume != (names.VolumeTag{}) {
			filesystemSources[sourceName] = managedFilesystemSource
			continue
		}
		filesystemSource, err := filesystemSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
		)
		if errors.Cause(err) == errNonDynamic {
			filesystemSource = nil
		} else if err != nil {
			return nil, nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
	}
	paramsBySource := make(map[string][]storage.FilesystemParams)
	for _, params := range params {
		sourceName := string(params.Provider)
		filesystemSource := filesystemSources[sourceName]
		if filesystemSource == nil {
			// Ignore nil filesystem sources; this means that the
			// filesystem should be created by the machine-provisioner.
			continue
		}
		paramsBySource[sourceName] = append(paramsBySource[sourceName], params)
	}
	return paramsBySource, filesystemSources, nil
}

// validateFilesystemParams validates a collection of filesystem parameters.
func validateFilesystemParams(
	filesystemSource storage.FilesystemSource,
	filesystemParams []storage.FilesystemParams,
) ([]storage.FilesystemParams, []error) {
	valid := make([]storage.FilesystemParams, 0, len(filesystemParams))
	results := make([]error, len(filesystemParams))
	for i, params := range filesystemParams {
		err := filesystemSource.ValidateFilesystemParams(params)
		if err == nil {
			valid = append(valid, params)
		}
		results[i] = err
	}
	return valid, results
}

// filesystemAttachmentParamsBySource separates the filesystem attachment parameters by filesystem source.
func filesystemAttachmentParamsBySource(
	environConfig *config.Config,
	baseStorageDir string,
	params []storage.FilesystemAttachmentParams,
	filesystems map[names.FilesystemTag]storage.Filesystem,
	managedFilesystemSource storage.FilesystemSource,
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
		filesystem := filesystems[params.Filesystem]
		if filesystem.Volume != (names.VolumeTag{}) {
			filesystemSources[sourceName] = managedFilesystemSource
			continue
		}
		filesystemSource, err := filesystemSource(
			environConfig, baseStorageDir, sourceName, params.Provider,
		)
		if err != nil {
			return nil, nil, errors.Annotate(err, "getting filesystem source")
		}
		filesystemSources[sourceName] = filesystemSource
	}
	return paramsBySource, filesystemSources, nil
}

func setFilesystemAttachmentInfo(ctx *context, filesystemAttachments []storage.FilesystemAttachment) error {
	if len(filesystemAttachments) == 0 {
		return nil
	}
	// TODO(axw) we need to be able to list filesystem attachments in the
	// provider, by environment, so that we can "harvest" them if they're
	// unknown. This will take care of killing filesystems that we fail to
	// record in state.
	errorResults, err := ctx.config.Filesystems.SetFilesystemAttachmentInfo(
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
		id := params.MachineStorageId{
			MachineTag:    filesystemAttachments[i].Machine.String(),
			AttachmentTag: filesystemAttachments[i].Filesystem.String(),
		}
		ctx.filesystemAttachments[id] = filesystemAttachments[i]
		removePendingFilesystemAttachment(ctx, id)
	}
	return nil
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

type createFilesystemOp struct {
	exponentialBackoff
	args storage.FilesystemParams
}

func (op *createFilesystemOp) key() interface{} {
	return op.args.Tag
}

type destroyFilesystemOp struct {
	exponentialBackoff
	tag names.FilesystemTag
}

func (op *destroyFilesystemOp) key() interface{} {
	return op.tag
}

type attachFilesystemOp struct {
	exponentialBackoff
	args storage.FilesystemAttachmentParams
}

func (op *attachFilesystemOp) key() interface{} {
	return params.MachineStorageId{
		MachineTag:    op.args.Machine.String(),
		AttachmentTag: op.args.Filesystem.String(),
	}
}

type detachFilesystemOp struct {
	exponentialBackoff
	args storage.FilesystemAttachmentParams
}

func (op *detachFilesystemOp) key() interface{} {
	return params.MachineStorageId{
		MachineTag:    op.args.Machine.String(),
		AttachmentTag: op.args.Filesystem.String(),
	}
}
