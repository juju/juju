// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	stdcontext "context"
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/agent/storageprovisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/worker/common"
)

// MachineManifoldConfig defines a storage provisioner's configuration and dependencies.
type MachineManifoldConfig struct {
	AgentName                    string
	APICallerName                string
	Clock                        clock.Clock
	Logger                       logger.Logger
	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
}

func (config MachineManifoldConfig) newWorker(_ stdcontext.Context, a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
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

	credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageDir := filepath.Join(cfg.DataDir(), "storage")
	w, err := NewStorageProvisioner(Config{
		Scope:                tag,
		StorageDir:           storageDir,
		Volumes:              api,
		Filesystems:          api,
		Life:                 api,
		Registry:             provider.CommonStorageProviders(),
		Machines:             api,
		Status:               api,
		Clock:                config.Clock,
		Logger:               config.Logger,
		CloudCallContextFunc: common.NewCloudCallContextFunc(credentialAPI),
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
