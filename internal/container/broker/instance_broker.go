// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/factory"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/rpc/params"
)

// NewBrokerFunc returns a Instance Broker.
type NewBrokerFunc func(Config) (environs.InstanceBroker, error)

var (
	systemNetplanDirectory = "/etc/netplan"
	activateBridgesTimeout = 5 * time.Minute
)

// NetConfigFunc returns a slice of NetworkConfig from a source config.
type NetConfigFunc func(corenetwork.ConfigSource) (corenetwork.InterfaceInfos, error)

// Config describes the resources used by the instance broker.
type Config struct {
	Name          string
	ContainerType instance.ContainerType
	ManagerConfig container.ManagerConfig
	APICaller     APICalls
	AgentConfig   agent.Config
	MachineTag    names.MachineTag
	MachineLock   machinelock.Lock
	GetNetConfig  NetConfigFunc
}

// Validate validates the instance broker configuration.
func (c Config) Validate() error {
	if c.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if string(c.ContainerType) == "" {
		return errors.NotValidf("empty ContainerType")
	}
	if c.APICaller == nil {
		return errors.NotValidf("nil APICaller")
	}
	if c.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if c.MachineTag.Id() == "" {
		return errors.NotValidf("empty MachineTag")
	}
	if c.MachineLock == nil {
		return errors.NotValidf("nil MachineLock")
	}
	if c.GetNetConfig == nil {
		return errors.NotValidf("nil GetNetConfig")
	}
	return nil
}

// ContainerBrokerFunc is used to align the constructors of the various brokers
// so that we can create them with the same arguments.
type ContainerBrokerFunc func(PrepareHostFunc, APICalls, container.Manager, agent.Config) (environs.InstanceBroker, error)

// New creates a new InstanceBroker from the Config
func New(config Config) (environs.InstanceBroker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	manager, err := factory.NewContainerManager(config.ContainerType, config.ManagerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var newBroker ContainerBrokerFunc
	switch config.ContainerType {
	case instance.LXD:
		newBroker = NewLXDBroker
	default:
		return nil, errors.NotValidf("ContainerType %s", config.ContainerType)
	}

	broker, err := newBroker(prepareHost(config), config.APICaller, manager, config.AgentConfig)
	if err != nil {
		logger.Errorf(context.TODO(), "failed to create new %s broker", config.ContainerType)
		return nil, errors.Trace(err)
	}

	return broker, nil
}

func prepareHost(config Config) PrepareHostFunc {
	return func(ctx context.Context, containerTag names.MachineTag, logger corelogger.Logger, abort <-chan struct{}) error {
		bridger, err := network.DefaultNetplanBridger(activateBridgesTimeout, systemNetplanDirectory)
		if err != nil {
			return errors.Trace(err)
		}

		preparer := NewHostPreparer(HostPreparerParams{
			API:                config.APICaller,
			ObserveNetworkFunc: observeNetwork(config),
			AcquireLockFunc:    acquireLock(config),
			Bridger:            bridger,
			AbortChan:          abort,
			MachineTag:         config.MachineTag,
			Logger:             logger,
		})
		return errors.Trace(preparer.Prepare(ctx, containerTag))
	}
}

// acquireLock tries to grab the machine lock (initLockName), and either
// returns it in a locked state, or returns an error.
func acquireLock(config Config) func(string, <-chan struct{}) (func(), error) {
	return func(comment string, abort <-chan struct{}) (func(), error) {
		spec := machinelock.Spec{
			Cancel:  abort,
			Worker:  config.Name,
			Comment: comment,
		}
		return config.MachineLock.Acquire(spec)
	}
}

func observeNetwork(config Config) func() ([]params.NetworkConfig, error) {
	return func() ([]params.NetworkConfig, error) {
		interfaceInfos, err := config.GetNetConfig(corenetwork.DefaultConfigSource())
		if err != nil {
			return nil, err
		}
		return params.NetworkConfigFromInterfaceInfo(interfaceInfos), nil
	}
}

type AvailabilityZoner interface {
	AvailabilityZone(ctx context.Context) (string, error)
}

// ConfigureAvailabilityZone reads the availability zone from the machine and
// adds the resulting information to the the manager config.
func ConfigureAvailabilityZone(ctx context.Context, managerConfig container.ManagerConfig, machineZone AvailabilityZoner) (container.ManagerConfig, error) {
	availabilityZone, err := machineZone.AvailabilityZone(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	managerConfig[container.ConfigAvailabilityZone] = availabilityZone

	return managerConfig, nil
}
