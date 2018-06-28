// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
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
