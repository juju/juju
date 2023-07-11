// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
)

func (s *Server) StorageSupported() bool {
	return s.storageAPISupport
}

func (s *Server) CreatePool(name, driver string, cfg map[string]string) error {
	req := api.StoragePoolsPost{
		Name:           name,
		Driver:         driver,
		StoragePoolPut: api.StoragePoolPut{Config: cfg},
	}
	return errors.Annotatef(s.CreateStoragePool(req), "creating storage pool %q", name)
}

func (s *Server) CreateVolume(pool, name string, cfg map[string]string) error {
	req := api.StorageVolumesPost{
		Name:             name,
		Type:             "custom",
		StorageVolumePut: api.StorageVolumePut{Config: cfg},
	}
	return errors.Annotatef(s.CreateStoragePoolVolume(pool, req), "creating storage pool volume %q", name)
}

// EnsureDefaultStorage ensures that the input profile is configured with a
// disk device, creating a new storage pool and a device if required.
func (s *Server) EnsureDefaultStorage(profile *api.Profile, eTag string) error {
	// If there is already a "/" device, we have nothing to do.
	for _, dev := range profile.Devices {
		if dev["path"] == "/" {
			return nil
		}
	}

	// If there is a "default" pool, use it.
	// Otherwise if there are other pools available, choose the first.
	pools, err := s.GetStoragePoolNames()
	if err != nil {
		return errors.Trace(err)
	}
	poolName := ""
	for _, p := range pools {
		if p == "default" {
			poolName = p
		}
	}
	if poolName == "" && len(pools) > 0 {
		poolName = pools[0]
	}

	// We need to create a new storage pool.
	if poolName == "" {
		poolName = "default"
		req := api.StoragePoolsPost{
			Name:   poolName,
			Driver: "dir",
		}
		err := s.CreateStoragePool(req)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Create a new disk device in the input profile.
	if profile.Devices == nil {
		profile.Devices = map[string]device{}
	}
	profile.Devices["root"] = map[string]string{
		"type": "disk",
		"path": "/",
		"pool": poolName,
	}

	if err := s.UpdateProfile(profile.Name, profile.Writable(), eTag); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("created new disk device \"root\" in profile %q", profile.Name)
	return nil
}
