// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/agent/storageprovisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

// MachineManifoldConfig defines a storage provisioner's configuration and dependencies.
type MachineManifoldConfig struct {
	AgentName     string
	APICallerName string
	Clock         clock.Clock
	Logger        logger.Logger
}

func (config MachineManifoldConfig) newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	if config.Clock == nil {
		return nil, errors.NotValidf("missing Clock")
	}
	if config.Logger == nil {
		return nil, errors.NotValidf("missing Logger")
	}
	cfg := a.CurrentConfig()
	api, err := storageprovisioner.NewClient(apiCaller)
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
		Registry: storage.StaticProviderRegistry{
			Providers: map[storage.ProviderType]storage.Provider{
				provider.LoopProviderType:   provider.NewLoopProvider(provider.LogAndExec),
				provider.RootfsProviderType: provider.NewRootfsProvider(provider.LogAndExec),
				provider.TmpfsProviderType:  provider.NewTmpfsProvider(provider.LogAndExec),
			},
		},
		Machines: api,
		Status:   api,
		Clock:    config.Clock,
		Logger:   config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// MachineManifold returns a dependency.Manifold that runs a storage provisioner.
func MachineManifold(config MachineManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentAPIManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	return engine.AgentAPIManifold(typedConfig, config.newWorker)
}
