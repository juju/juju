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
	"github.com/juju/juju/storage/provider/registry"
)

// storageEntityLife queries the lifecycle state of each specified
// storage entity (volume or filesystem), and then partitions the
// tags by them.
func storageEntityLife(ctx *context, tags []names.Tag) (alive, dying, dead []names.Tag, _ error) {
	lifeResults, err := ctx.life.Life(tags)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "getting storage entity life")
	}
	for i, result := range lifeResults {
		if result.Error != nil {
			return nil, nil, nil, errors.Annotatef(
				result.Error, "getting life of %s",
				names.ReadableString(tags[i]),
			)
		}
		switch result.Life {
		case params.Alive:
			alive = append(alive, tags[i])
		case params.Dying:
			dying = append(dying, tags[i])
		case params.Dead:
			dead = append(dead, tags[i])
		}
	}
	return alive, dying, dead, nil
}

// attachmentLife queries the lifecycle state of each specified
// attachment, and then partitions the IDs by them.
func attachmentLife(ctx *context, ids []params.MachineStorageId) (
	alive, dying, dead []params.MachineStorageId, _ error,
) {
	lifeResults, err := ctx.life.AttachmentLife(ids)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "getting machine attachment life")
	}
	for i, result := range lifeResults {
		if result.Error != nil {
			return nil, nil, nil, errors.Annotatef(
				result.Error, "getting life of %s attached to %s",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
		}
		switch result.Life {
		case params.Alive:
			alive = append(alive, ids[i])
		case params.Dying:
			dying = append(dying, ids[i])
		case params.Dead:
			dead = append(dead, ids[i])
		}
	}
	return alive, dying, dead, nil
}

// ensureDead ensures that each specified entity immediately becomes Dead
// if it is not already Dead or removed.
func ensureDead(ctx *context, tags []names.Tag) error {
	errorResults, err := ctx.life.EnsureDead(tags)
	if err != nil {
		return errors.Annotate(err, "ensuring storage entities dead")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(result.Error, "ensuring %s dead", names.ReadableString(tags[i]))
		}
	}
	return nil
}

// removeEntities removes each specified Dead entity from state.
func removeEntities(ctx *context, tags []names.Tag) error {
	errorResults, err := ctx.life.Remove(tags)
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
func removeAttachments(ctx *context, ids []params.MachineStorageId) error {
	errorResults, err := ctx.life.RemoveAttachments(ids)
	if err != nil {
		return errors.Annotate(err, "removing attachments")
	}
	for i, result := range errorResults {
		if result.Error != nil {
			return errors.Annotatef(
				result.Error, "removing attachment of %s to %s from state",
				ids[i].AttachmentTag, ids[i].MachineTag,
			)
		}
	}
	return nil
}

// volumeSource returns a volume source given a name, provider type,
// environment config and storage directory.
//
// TODO(axw) move this to the main storageprovisioner, and have
// it watch for changes to storage source configurations, updating
// a map in-between calls to the volume/filesystem/attachment
// event handlers.
func volumeSource(
	environConfig *config.Config,
	baseStorageDir string,
	sourceName string,
	providerType storage.ProviderType,
) (storage.VolumeSource, error) {
	provider, sourceConfig, err := sourceParams(providerType, sourceName, baseStorageDir)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q params", sourceName)
	}
	source, err := provider.VolumeSource(environConfig, sourceConfig)
	if err != nil {
		return nil, errors.Annotatef(err, "getting storage source %q", sourceName)
	}
	return source, nil
}

func sourceParams(providerType storage.ProviderType, sourceName, baseStorageDir string) (storage.Provider, *storage.Config, error) {
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
