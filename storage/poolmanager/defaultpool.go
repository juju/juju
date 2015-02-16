// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package poolmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

var defaultPools []*storage.Config

// RegisterDefaultStoragePools registers pool information to be saved to
// state when AddDefaultStoragePools is called.
func RegisterDefaultStoragePools(pools []*storage.Config) {
	defaultPools = append(defaultPools, pools...)
}

// AddDefaultStoragePools is run at bootstrap and on upgrade to ensure that
// out of the box storage pools are created.
func AddDefaultStoragePools(settings SettingsManager) error {
	pm := New(settings)
	for _, pool := range defaultPools {
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
