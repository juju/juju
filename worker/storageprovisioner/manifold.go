// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/storageprovisioner"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines a storage provisioner's configuration and dependencies.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	Clock clock.Clock
}

// Manifold returns a dependency.Manifold that runs a storage provisioner.
func Manifold(config ManifoldConfig) dependency.Manifold {
	newWorker := func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		if config.Clock == nil {
			return nil, dependency.ErrMissing
		}

		cfg := a.CurrentConfig()
		api, err := storageprovisioner.NewState(apiCaller, cfg.Tag())
		if err != nil {
			return nil, errors.Trace(err)
		}

		tag, ok := cfg.Tag().(names.MachineTag)
		if !ok {
			return nil, errors.Errorf("this manifold may only be used inside a machine agent")
		}

		storageDir := filepath.Join(cfg.DataDir(), "storage")
		w, err := NewStorageProvisioner(Config{
			Scope:       tag,
			StorageDir:  storageDir,
			Volumes:     api,
			Filesystems: api,
			Life:        api,
			Environ:     api,
			Machines:    api,
			Status:      api,
			Clock:       config.Clock,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return w, nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
