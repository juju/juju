// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// storageEntityLife queries the lifecycle state of each specified
// storage entity (volume or filesystem), and then partitions the
// tags by them.
func storageEntityLife(ctx context.Context, deps *dependencies, tags []names.Tag) (alive, dying, dead []names.Tag, _ error) {
	lifeResults, err := deps.config.Life.Life(ctx, tags)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "getting storage entity life")
	}
	for i, result := range lifeResults {
		value := result.Life
		if result.Error != nil {
			if !params.IsCodeNotFound(result.Error) {
				return nil, nil, nil, errors.Annotatef(
					result.Error, "getting life of %s",
					names.ReadableString(tags[i]),
				)
			}
			value = life.Dead
		}
		switch value {
		case life.Alive:
			alive = append(alive, tags[i])
		case life.Dying:
			dying = append(dying, tags[i])
		case life.Dead:
			dead = append(dead, tags[i])
		}
	}
	return alive, dying, dead, nil
}

// attachmentLife queries the lifecycle state of each specified
// attachment, and then partitions the IDs by them.
func attachmentLife(ctx context.Context, deps *dependencies, ids []params.MachineStorageId) (
	alive, dying, dead, gone []params.MachineStorageId, _ error,
) {
	lifeResults, err := deps.config.Life.AttachmentLife(ctx, ids)
	if err != nil {
		return nil, nil, nil, nil, errors.Annotate(err, "getting machine attachment life")
	}
	for i, result := range lifeResults {
		value := result.Life
		if result.Error != nil {
			if !params.IsCodeNotFound(result.Error) {
				return nil, nil, nil, nil, errors.Annotatef(
					result.Error, "getting life of %s attached to %s",
					ids[i].AttachmentTag, ids[i].MachineTag,
				)
			}
			gone = append(gone, ids[i])
			continue
		}
		switch value {
		case life.Alive:
			alive = append(alive, ids[i])
		case life.Dying:
			dying = append(dying, ids[i])
		case life.Dead:
			dead = append(dead, ids[i])
		}
	}
	return alive, dying, dead, gone, nil
}

// removeEntities removes each specified Dead entity from state.
func removeEntities(ctx context.Context, deps *dependencies, tags []names.Tag) error {
	if len(tags) == 0 {
		return nil
	}
	deps.config.Logger.Debugf(context.TODO(), "removing entities: %v", tags)
	errorResults, err := deps.config.Life.Remove(ctx, tags)
	if err != nil {
		return errors.Annotate(err, "removing storage entities")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "removing %s from state", names.ReadableString(tags[i]))
		}
	}
	return nil
}

// removeAttachments removes each specified attachment from state.
func removeAttachments(ctx context.Context, deps *dependencies, ids []params.MachineStorageId) error {
	if len(ids) == 0 {
		return nil
	}
	errorResults, err := deps.config.Life.RemoveAttachments(ctx, ids)
	if err != nil {
		return errors.Annotate(err, "removing attachments")
	}
	for i, result := range errorResults {
		if result.Error != nil && !params.IsCodeNotFound(result.Error) {
			// ignore not found error.
			return errors.Annotatef(
				result.Error, "removing attachment of %s to %s from state",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
		}
	}
	return nil
}

// setStatus sets the given entity statuses, if any. If setting
// the status fails the error is logged but otherwise ignored.
func setStatus(ctx context.Context, deps *dependencies, statuses []params.EntityStatusArgs) {
	if len(statuses) > 0 {
		if err := deps.config.Status.SetStatus(ctx, statuses); err != nil {
			deps.config.Logger.Errorf(context.TODO(), "failed to set status: %v", err)
		}
	}
}

var errNonDynamic = errors.New("non-dynamic storage provider")

// volumeSource returns a volume source given a name, provider type,
// environment config and storage directory.
//
// TODO(axw) move this to the main storageprovisioner, and have
// it watch for changes to storage source configurations, updating
// a map in-between calls to the volume/filesystem/attachment
// event handlers.
func volumeSource(
	baseStorageDir string,
	sourceName string,
	providerType storage.ProviderType,
	registry storage.ProviderRegistry,
) (storage.VolumeSource, error) {
	provider, sourceConfig, err := sourceParams(baseStorageDir, sourceName, providerType, registry)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q params", sourceName)
	}
	if !provider.Dynamic() {
		return nil, errNonDynamic
	}
	source, err := provider.VolumeSource(sourceConfig)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q", sourceName)
	}
	return source, nil
}

// filesystemSource returns a filesystem source given a name, provider type,
// environment config and storage directory.
//
// TODO(axw) move this to the main storageprovisioner, and have
// it watch for changes to storage source configurations, updating
// a map in-between calls to the volume/filesystem/attachment
// event handlers.
func filesystemSource(
	baseStorageDir string,
	sourceName string,
	providerType storage.ProviderType,
	registry storage.ProviderRegistry,
) (storage.FilesystemSource, error) {
	provider, sourceConfig, err := sourceParams(baseStorageDir, sourceName, providerType, registry)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q params", sourceName)
	}
	source, err := provider.FilesystemSource(sourceConfig)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q", sourceName)
	}
	return source, nil
}

func sourceParams(
	baseStorageDir string,
	sourceName string,
	providerType storage.ProviderType,
	registry storage.ProviderRegistry,
) (storage.Provider, *storage.Config, error) {
	provider, err := registry.StorageProvider(providerType)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting provider")
	}
	attrs := make(map[string]interface{})
	if baseStorageDir != "" {
		storageDir := filepath.Join(baseStorageDir, sourceName)
		attrs[storage.ConfigStorageDir] = storageDir
	}
	sourceConfig, err := storage.NewConfig(sourceName, providerType, attrs)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting config")
	}
	return provider, sourceConfig, nil
}

func copyMachineStorageIds(src []watcher.MachineStorageID) []params.MachineStorageId {
	dst := make([]params.MachineStorageId, len(src))
	for i, msid := range src {
		dst[i] = params.MachineStorageId{
			MachineTag:    msid.MachineTag,
			AttachmentTag: msid.AttachmentTag,
		}
	}
	return dst
}
