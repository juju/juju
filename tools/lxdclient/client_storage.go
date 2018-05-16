// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

type rawStorageClient interface {
	GetStoragePool(name string) (pool *api.StoragePool, ETag string, err error)
	CreateStoragePool(pool api.StoragePoolsPost) (err error)
	GetStoragePools() (pools []api.StoragePool, err error)
	CreateStoragePoolVolume(pool string, volume api.StorageVolumesPost) (err error)
	DeleteStoragePoolVolume(pool string, volType string, name string) (err error)
	GetStoragePoolVolume(pool string, volType string, name string) (volume *api.StorageVolume, ETag string, err error)
	UpdateStoragePoolVolume(pool string, volType string, name string, volume api.StorageVolumePut, ETag string) (err error)
	GetStoragePoolVolumes(pool string) (volumes []api.StorageVolume, err error)
}

type storageClient struct {
	raw       rawStorageClient
	supported bool
}

// StorageSupported reports whether or not storage is supported by the LXD remote.
func (c *storageClient) StorageSupported() bool {
	return c.supported
}

// Volume creates a volume in a storage pool.
func (c *storageClient) Volume(pool, volumeName string) (api.StorageVolume, error) {
	if !c.supported {
		return api.StorageVolume{}, errors.NotSupportedf("storage API on this remote")
	}
	volume, _, err := c.raw.GetStoragePoolVolume(pool, "custom", volumeName)
	if err != nil {
		if IsLXDNotFound(err) {
			return api.StorageVolume{}, errors.NotFoundf("volume %q in pool %q", volumeName, pool)
		}
		return api.StorageVolume{}, errors.Trace(err)
	}
	return *volume, nil
}

// VolumeCreate creates a volume in a storage pool.
func (c *storageClient) VolumeCreate(pool, volume string, config map[string]string) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}
	req := api.StorageVolumesPost{
		Name:             volume,
		Type:             "custom",
		StorageVolumePut: api.StorageVolumePut{Config: config},
	}

	return errors.Trace(c.raw.CreateStoragePoolVolume(pool, req))
}

// VolumeUpdate updates a volume in a storage pool.
func (c *storageClient) VolumeUpdate(pool, volume string, update api.StorageVolume) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}

	return errors.Trace(c.raw.UpdateStoragePoolVolume(pool, "custom", volume, update.Writable(), ""))
}

// VolumeDelete deletes a volume from a storage pool.
func (c *storageClient) VolumeDelete(pool, volume string) error {
	if !c.supported {
		return errors.NotSupportedf("storage API on this remote")
	}
	if err := c.raw.DeleteStoragePoolVolume(pool, "custom", volume); err != nil {
		if IsLXDNotFound(err) {
			return errors.NotFoundf("volume %q in pool %q", volume, pool)
		}
		return errors.Trace(err)
	}
	return nil
}

// VolumeList lists volumes in a storage pool, excluding any non-custom type
// volumes.
func (c *storageClient) VolumeList(pool string) ([]api.StorageVolume, error) {
	if !c.supported {
		return nil, errors.NotSupportedf("storage API on this remote")
	}
	all, err := c.raw.GetStoragePoolVolumes(pool)
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
	pool, _, err := c.raw.GetStoragePool(name)
	if err != nil {
		if IsLXDNotFound(err) {
			return api.StoragePool{}, errors.NotFoundf("storage pool %q", name)
		}
		return api.StoragePool{}, errors.Annotatef(err, "getting storage pool %q", name)
	}
	return *pool, nil
}

// StoragePools returns all of the LXD storage pools.
func (c *storageClient) StoragePools() ([]api.StoragePool, error) {
	if !c.supported {
		return nil, errors.NotSupportedf("storage API on this remote")
	}
	pools, err := c.raw.GetStoragePools()
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
	req := api.StoragePoolsPost{
		Name:           name,
		Driver:         driver,
		StoragePoolPut: api.StoragePoolPut{Config: attrs},
	}
	return errors.Annotatef(c.raw.CreateStoragePool(req), "creating storage pool %q", name)
}
