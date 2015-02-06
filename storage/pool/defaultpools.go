// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pool

import (
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

// RegisterDefaultPools ensures that the required out of the box storage pools
// are registered.
func RegisterDefaultPools(pm PoolManager, agentConfig agent.Config) error {
	if err := registerRootFsLoop(pm, agentConfig); err != nil {
		return err
	}
	// TODO - register other pools
	return nil
}

const RootFsLoopPoolName = "loop"

func registerRootFsLoop(pm PoolManager, agentConfig agent.Config) error {
	rootfsPoolConfig := map[string]interface{}{
		provider.LoopDataDir: filepath.Join(agentConfig.DataDir(), "storage", "block", "loop"),
	}
	return RegisterPool(pm, RootFsLoopPoolName, provider.LoopProviderType, false, rootfsPoolConfig)
}

// RegisterPool creates a named pool for the provider type with the specified config.
func RegisterPool(pm PoolManager, name string, providerType storage.ProviderType, replace bool, cfg map[string]interface{}) error {
	if _, err := pm.Get(name); err == nil {
		// Already registered.
		if !replace {
			return nil
		}
		if err := pm.Delete(name); err != nil {
			return errors.Annotatef(err, "deleting existing pool %q", name)
		}
	}
	_, err := pm.Create(name, providerType, cfg)
	return err
}
