// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

// PoolInfo is used to register default pool information.
type PoolInfo struct {
	Name   string
	Type   storage.ProviderType
	Config map[string]interface{}
}

var defaultPools []PoolInfo

// RegisterDefaultStoragePools registers pool information to be saved to
// state when AddDefaultStoragePools is called.
func RegisterDefaultStoragePools(pools []PoolInfo) {
	defaultPools = append(defaultPools, pools...)
}

type poolConfig interface {
	DataDir() string
}

// AddDefaultStoragePools is run at bootstrap and on upgrade to ensure that
// out of the box storage pools are created.
func AddDefaultStoragePools(settings SettingsManager, config poolConfig) error {
	pm := NewPoolManager(settings)

	for _, poolInfo := range defaultPools {
		if err := addDefaultPool(pm, poolInfo.Name, poolInfo.Type, poolInfo.Config); err != nil {
			return err
		}
	}

	// Register the default loop pool.
	cfg := map[string]interface{}{}
	return addDefaultPool(pm, "loop", provider.LoopProviderType, cfg)
}

func addDefaultPool(pm PoolManager, name string, providerType storage.ProviderType, attrs map[string]interface{}) error {
	_, err := pm.Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotatef(err, "loading default pool %q", name)
	}
	if err != nil {
		// We got a not found error, so default pool doesn't exist.
		if _, err := pm.Create(name, providerType, attrs); err != nil {
			return errors.Annotatef(err, "creating default pool %q", name)
		}
	}
	return nil
}
