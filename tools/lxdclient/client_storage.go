// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared/api"
)

type rawStorageClient interface {
	StoragePoolCreate(name string, driver string, config map[string]string) error
	StoragePoolGet(name string) (api.StoragePool, error)
	ListStoragePools() ([]api.StoragePool, error)

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
	if err := c.raw.StoragePoolVolumeTypeDelete(pool, volume, "custom"); err != nil {
		if err == lxd.LXDErrors[http.StatusNotFound] {
			return errors.NotFoundf("volume %q in pool %q", volume, pool)
		}
		return err
	}
	return nil
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

// StoragePool returns the LXD storage pool with the given name.
func (c *storageClient) StoragePool(name string) (api.StoragePool, error) {
	if !c.supported {
		return api.StoragePool{}, errors.NotSupportedf("storage API on this remote")
	}
	pool, err := c.raw.StoragePoolGet(name)
	if err != nil {
		if err == lxd.LXDErrors[http.StatusNotFound] {
			return api.StoragePool{}, errors.NotFoundf("storage pool %q", name)
		}
		return api.StoragePool{}, errors.Annotatef(err, "getting storage pool %q", name)
	}
	return pool, nil
}

// StoragePools returns all of the LXD storage pools.
func (c *storageClient) StoragePools() ([]api.StoragePool, error) {
	if !c.supported {
		return nil, errors.NotSupportedf("storage API on this remote")
	}
	pools, err := c.raw.ListStoragePools()
	if err != nil {
		return nil, errors.Annotate(err, "listing storage pools")
	}
	return pools, nil
}

// CreateStoragePool creates a LXD storage pool with the given name, driver,
// and configuration attributes.
func (c *storageClient) CreateStoragePool(name, driver string, attrs map[string]string) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}
	err := c.raw.StoragePoolCreate(name, driver, attrs)
	return errors.Annotatef(err, "creating storage pool %q", name)
}
