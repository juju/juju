// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"path/filepath"

	"github.com/juju/errors"

	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

var defaultEBSPools = map[string]map[string]interface{}{
	// TODO(wallyworld) - remove "ebs" pool which has no params when we support
	// specifying pool type for pool name
	"ebs":     map[string]interface{}{},
	"ebs-ssd": map[string]interface{}{"volume-type": "gp2"},
}

type poolConfig interface {
	DataDir() string
}

func AddDefaultStoragePools(settings SettingsManager, config poolConfig) error {
	pm := NewPoolManager(settings)

	for name, attrs := range defaultEBSPools {
		if err := addDefaultPool(pm, name, ec2storage.EBSProviderType, attrs); err != nil {
			return err
		}
	}

	// Register the default loop pool.
	// TODO(wallyworld) - remove loop data dir as we don't want to hard code it here
	cfg := map[string]interface{}{
		provider.LoopDataDir: filepath.Join(config.DataDir(), "storage", "block", "loop"),
	}
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
