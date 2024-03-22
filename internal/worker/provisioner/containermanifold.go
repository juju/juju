// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	workercommon "github.com/juju/juju/internal/worker/common"
)

type GetContainerWatcherFunc func() (watcher.StringsWatcher, error)

// ContainerProvisioningManifold creates a manifold that runs a
// container provisioner.
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
// including how to initialise the container system.
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

func (cfg ContainerManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var a agent.Agent
	if err := getter.Get(cfg.AgentName, &a); err != nil {
		return nil, errors.Trace(err)
	}

	// Check current config, has the machine-setup worker run
	// to confirm supported container types.
	agentConfig := a.CurrentConfig()
	tag := agentConfig.Tag()
	mTag, ok := tag.(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("%q machine tag", a)
	}

	var apiCaller base.APICaller
	if err := getter.Get(cfg.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	pr := apiprovisioner.NewClient(apiCaller)

	machine, err := cfg.machineSupportsContainers(ctx, &containerShim{api: pr}, mTag)
	if err != nil {
		return nil, err
	}

	credentialAPI, err := workercommon.NewCredentialInvalidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get credential invalidator facade")
	}

	cs := NewContainerSetup(ContainerSetupParams{
		Logger:        cfg.Logger,
		ContainerType: cfg.ContainerType,
		MachineZone:   machine,
		MTag:          mTag,
		Provisioner:   pr,
		Config:        agentConfig,
		MachineLock:   cfg.MachineLock,
		CredentialAPI: credentialAPI,
		GetNetConfig:  network.GetObservedNetworkConfig,
	})

	getContainerWatcherFunc := func() (watcher.StringsWatcher, error) {
		return machine.WatchContainers(cfg.ContainerType)
	}

	return NewContainerSetupAndProvisioner(cs, getContainerWatcherFunc)
}

type ContainerMachine interface {
	AvailabilityZone() (string, error)
	Life() life.Value
	SupportedContainers() ([]instance.ContainerType, bool, error)
	WatchContainers(ctype instance.ContainerType) (watcher.StringsWatcher, error)
}

type ContainerMachineGetter interface {
	Machines(ctx context.Context, tags ...names.MachineTag) ([]ContainerMachineResult, error)
}

type ContainerMachineResult struct {
	Machine ContainerMachine
	Err     error
}

type containerShim struct {
	api *apiprovisioner.Client
}

func (s *containerShim) Machines(ctx context.Context, tags ...names.MachineTag) ([]ContainerMachineResult, error) {
	result, err := s.api.Machines(ctx, tags...)
	if err != nil {
		return nil, err
	}
	newResult := make([]ContainerMachineResult, len(result))
	for i, v := range result {
		newResult[i] = ContainerMachineResult{
			Machine: v.Machine,
			Err:     v.Err,
		}
	}
	return newResult, nil
}

func (cfg ContainerManifoldConfig) machineSupportsContainers(ctx context.Context, pr ContainerMachineGetter, mTag names.MachineTag) (ContainerMachine, error) {
	result, err := pr.Machines(ctx, mTag)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load machine %s from state", mTag)
	}
	if errors.Is(err, errors.NotFound) || (result[0].Err == nil && result[0].Machine.Life() == life.Dead) {
		return nil, dependency.ErrUninstall
	}
	machine := result[0].Machine
	types, known, err := machine.SupportedContainers()
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving supported container types")
	}
	if !known {
		return nil, errors.NotYetAvailablef("container types not yet available")
	}
	if len(types) == 0 {
		cfg.Logger.Infof("uninstalling no supported containers on %q", mTag)
		return nil, dependency.ErrUninstall
	}

	cfg.Logger.Debugf("%s supported containers types set as %q", mTag, types)

	typeSet := set.NewStrings()
	for _, v := range types {
		typeSet.Add(string(v))
	}
	if !typeSet.Contains(string(cfg.ContainerType)) {
		cfg.Logger.Infof("%s does not support %s container", mTag, string(cfg.ContainerType))
		return nil, dependency.ErrUninstall
	}
	return machine, nil
}
