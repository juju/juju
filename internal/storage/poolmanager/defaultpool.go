// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/storage"
)

// AddDefaultStoragePools adds the default storage pools for the given
// provider to the given pool manager. This is called whenever a new
// model is created.
func AddDefaultStoragePools(p storage.Provider, pm PoolManager) error {
	for _, pool := range p.DefaultPools() {
		if err := addDefaultPool(pm, pool); err != nil {
			return err
		}
	}
	return nil
}

func addDefaultPool(pm PoolManager, pool *storage.Config) error {
	_, err := pm.Get(pool.Name())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotatef(err, "loading default pool %q", pool.Name())
	}
	if err != nil {
		// We got a not found error, so default pool doesn't exist.
		if _, err := pm.Create(pool.Name(), pool.Provider(), pool.Attrs()); err != nil {
			return errors.Annotatef(err, "creating default pool %q", pool.Name())
		}
	}
	return nil
}
