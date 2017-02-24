// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

type rawStorageClient interface {
	StoragePoolVolumeTypeCreate(pool string, volume string, volumeType string, config map[string]string) error
	StoragePoolVolumeTypeDelete(pool string, volume string, volumeType string) error
	StoragePoolVolumesList(pool string) ([]api.StorageVolume, error)
}

type storageClient struct {
	raw       rawStorageClient
	supported bool
}

// StorageSupported reports whether or not storage is supported by the LXD remote.
func (c *storageClient) StorageSupported() bool {
	return c.supported
}

// VolumeCreate creates a volume in a storage pool.
func (c *storageClient) VolumeCreate(pool, volume string, config map[string]string) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}
	return c.raw.StoragePoolVolumeTypeCreate(pool, volume, "custom", config)
}

// VolumeDelete deletes a volume from a storage pool.
func (c *storageClient) VolumeDelete(pool, volume string) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}
	return c.raw.StoragePoolVolumeTypeDelete(pool, volume, "custom")
}

// VolumeList lists volumes in a storage pool, excluding any non-custom type
// volumes.
func (c *storageClient) VolumeList(pool string) ([]api.StorageVolume, error) {
	if !c.supported {
		return nil, errors.NotSupportedf("storage API on this remote")
	}
	all, err := c.raw.StoragePoolVolumesList(pool)
	if err != nil {
		return nil, errors.Trace(err)
	}
	custom := make([]api.StorageVolume, 0, len(all))
	for _, v := range all {
		if v.Type == "custom" {
			custom = append(custom, v)
		}
	}
	return custom, nil
}
