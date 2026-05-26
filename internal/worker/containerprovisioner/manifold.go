// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	internalerors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ErrNotContainerHost is returned by machineSupportsContainers when the
// machine should not run a container provisioner. Call sites that manage the
// manifold lifecycle must map this to dependency.ErrUninstall.
const ErrNotContainerHost = internalerors.ConstError("not a container host")

// GetContainerWatcherFunc is a function that returns a watcher for
// the containers on a machine.
type GetContainerWatcherFunc func(context.Context) (watcher.StringsWatcher, error)

// Manifold creates a manifold that runs a
// container provisioner.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: config.start,
	}
}

// ManifoldConfig defines a container provisioner's dependencies,
// including how to initialise the container system.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        logger.Logger
	MachineLock   machinelock.Lock
	ContainerType instance.ContainerType
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
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
	if cfg.ContainerType == "" {
		return errors.NotValidf("missing Container Type")
	}
	return nil
}

func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
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
	if errors.Is(err, ErrNotContainerHost) || errors.Is(err, errors.NotFound) {
		return nil, dependency.ErrUninstall
	}
	if err != nil {
		return nil, err
	}

	cs := NewContainerSetup(ContainerSetupParams{
		Logger:        cfg.Logger,
		ContainerType: cfg.ContainerType,
		MachineZone:   machine,
		MTag:          mTag,
		Provisioner:   pr,
		Config:        agentConfig,
		MachineLock:   cfg.MachineLock,
		GetNetConfig:  network.GetObservedNetworkConfig,
	})

	getContainerWatcherFunc := func(ctx context.Context) (watcher.StringsWatcher, error) {
		return machine.WatchContainers(ctx, cfg.ContainerType)
	}

	return NewContainerSetupAndProvisioner(ctx, cs, getContainerWatcherFunc)
}

type ContainerMachine interface {
	AvailabilityZone(context.Context) (string, error)
	Life() life.Value
	SupportedContainers(context.Context) ([]instance.ContainerType, bool, error)
	WatchContainers(_ context.Context, ctype instance.ContainerType) (watcher.StringsWatcher, error)
}

type ContainerMachineGetter interface {
	Machines(ctx context.Context, tags ...names.MachineTag) ([]ContainerMachineResult, error)
}

type ContainerMachineResult struct {
	Machine ContainerMachine
	Err     *params.Error
}

type containerShim struct {
	api *apiprovisioner.Client
}

func (s *containerShim) Machines(ctx context.Context, tags ...names.MachineTag) ([]ContainerMachineResult, error) {
	result, err := s.api.Machines(ctx, tags...)
	if err != nil {
		return nil, errors.Annotatef(err, "loading machines %v from state", tags)
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

func (cfg ManifoldConfig) machineSupportsContainers(ctx context.Context, pr ContainerMachineGetter, mTag names.MachineTag) (ContainerMachine, error) {
	// Container machines cannot provision further containers.
	if names.IsContainerMachine(mTag.Id()) {
		cfg.Logger.Infof(ctx, "uninstalling container provisioner: machine %q is itself a container", mTag)
		return nil, internalerors.Errorf("machine %q is itself a container", mTag).Add(ErrNotContainerHost)
	}

	result, err := pr.Machines(ctx, mTag)
	if err != nil {
		return nil, errors.Annotatef(err, "loading machine %s from state", mTag)
	} else if len(result) == 0 {
		return nil, errors.Errorf("loading machine %s from state: no results", mTag)
	}
	if result[0].Err != nil {
		return nil, errors.Annotatef(result[0].Err, "loading machine %s from state", mTag)
	}
	if result[0].Machine.Life() == life.Dead {
		return nil, internalerors.Errorf("machine %s is dead", mTag).Add(ErrNotContainerHost)
	}
	machine := result[0].Machine
	types, known, err := machine.SupportedContainers(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving supported container types")
	}
	if !known {
		return nil, errors.NotYetAvailablef("container types not yet available")
	}
	if len(types) == 0 {
		cfg.Logger.Infof(ctx, "uninstalling no supported containers on %q", mTag)
		return nil, internalerors.Errorf("no supported container types on %s", mTag).Add(ErrNotContainerHost)
	}

	cfg.Logger.Debugf(ctx, "%s supported containers types set as %q", mTag, types)

	typeSet := set.NewStrings()
	for _, v := range types {
		typeSet.Add(string(v))
	}
	if !typeSet.Contains(string(cfg.ContainerType)) {
		cfg.Logger.Infof(ctx, "%s does not support %s container", mTag, string(cfg.ContainerType))
		return nil, internalerors.Errorf("%s does not support %s containers", mTag, cfg.ContainerType).Add(ErrNotContainerHost)
	}
	return machine, nil
}
