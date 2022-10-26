// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	workercommon "github.com/juju/juju/worker/common"
)

// ContainerProvisioningManifold creates a manifold that runs a
// container provisioner. See the ContainerManifoldConfig type
// for discussion about how this can/should evolve.
func ContainerProvisioningManifold(config ContainerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: config.start,
	}
}

// ContainerManifoldConfig defines a container provisioner's dependencies,
// including how to setup the container system.
type ContainerManifoldConfig struct {
	AgentName                    string
	APICallerName                string
	Logger                       Logger
	MachineLock                  machinelock.Lock
	NewCredentialValidatorFacade func(base.APICaller) (workercommon.CredentialAPI, error)
	ContainerType                instance.ContainerType
}

// Validate is called by start to check for bad configuration.
func (cfg ContainerManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.MachineLock == nil {
		return errors.NotValidf("missing MachineLock")
	}
	if cfg.NewCredentialValidatorFacade == nil {
		return errors.NotValidf("missing NewCredentialValidatorFacade")
	}
	if cfg.ContainerType == "" {
		return errors.NotValidf("missing Container Type")
	}
	return nil
}

func (cfg ContainerManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	cfg.Logger.Debugf("starting")

	var a agent.Agent
	if err := context.Get(cfg.AgentName, &a); err != nil {
		return nil, errors.Trace(err)
	}

	agentConfig := a.CurrentConfig()
	tag := agentConfig.Tag()
	mTag, ok := tag.(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("%q machine tag", a)
	}

	var apiCaller base.APICaller
	if err := context.Get(cfg.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	pr := apiprovisioner.NewState(apiCaller)
	result, err := pr.Machines(mTag)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load machine %s from state", tag)
	}
	if len(result) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result))
	}
	if errors.IsNotFound(result[0].Err) || (result[0].Err == nil && result[0].Machine.Life() == life.Dead) {
		return nil, dependency.ErrUninstall
	}

	m := result[0].Machine
	types, known, err := m.SupportedContainers()
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving supported container types")
	}
	if !known {
		return nil, errors.Errorf("no container types determined")
	}
	if len(types) == 0 {
		cfg.Logger.Infof("uninstalling no supported containers on %q", mTag)
		return nil, dependency.ErrUninstall
	}

	cfg.Logger.Debugf("%s supported containers types set as %q", mTag, types)

	typeSet := set.NewStrings()
	for _, v := range types {
		cfg.Logger.Debugf("adding %q", v)
		typeSet.Add(string(v))
	}
	if !typeSet.Contains(string(cfg.ContainerType)) {
		cfg.Logger.Infof("%s does not support %s containers", mTag, string(cfg.ContainerType))
		return nil, dependency.ErrUninstall
	}

	credentialAPI, err := workercommon.NewCredentialInvalidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get credential invalidator facade")
	}

	cs := NewContainerSetup(ContainerSetupParams{
		Logger:        cfg.Logger,
		ContainerType: cfg.ContainerType,
		Machine:       m,
		MTag:          mTag,
		Provisioner:   pr,
		Config:        agentConfig,
		MachineLock:   cfg.MachineLock,
		CredentialAPI: credentialAPI,
		GetContainerWatcherFunc: func() (watcher.StringsWatcher, error) {
			return m.WatchContainers(cfg.ContainerType)
		},
	})

	return NewContainerWorker(cs)
}
