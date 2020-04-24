// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package broker

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
)

// NewBrokerFunc returns a Instance Broker.
type NewBrokerFunc func(Config) (environs.InstanceBroker, error)

var (
	systemNetworkInterfacesFile = "/etc/network/interfaces"
	systemSbinIfup              = "/sbin/ifup"
	systemNetplanDirectory      = "/etc/netplan"
	activateBridgesTimeout      = 5 * time.Minute
)

// NetConfigFunc returns a slice of NetworkConfig from a source config.
type NetConfigFunc func(common.NetworkConfigSource) ([]params.NetworkConfig, error)

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
	case instance.KVM:
		newBroker = NewKVMBroker
	case instance.LXD:
		newBroker = NewLXDBroker
	default:
		return nil, errors.NotValidf("ContainerType %s", config.ContainerType)
	}

	broker, err := newBroker(prepareHost(config), config.APICaller, manager, config.AgentConfig)
	if err != nil {
		logger.Errorf("failed to create new %s broker", config.ContainerType)
		return nil, errors.Trace(err)
	}

	return broker, nil
}

func prepareHost(config Config) PrepareHostFunc {
	return func(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error {
		preparer := NewHostPreparer(HostPreparerParams{
			API:                config.APICaller,
			ObserveNetworkFunc: observeNetwork(config),
			AcquireLockFunc:    acquireLock(config),
			CreateBridger:      defaultBridger,
			AbortChan:          abort,
			MachineTag:         config.MachineTag,
			Logger:             log,
		})
		return preparer.Prepare(containerTag)
	}
}

func defaultBridger() (network.Bridger, error) {
	if _, err := os.Stat(systemSbinIfup); err == nil {
		return network.DefaultEtcNetworkInterfacesBridger(activateBridgesTimeout, systemNetworkInterfacesFile)
	} else {
		return network.DefaultNetplanBridger(activateBridgesTimeout, systemNetplanDirectory)
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
		return config.GetNetConfig(common.DefaultNetworkConfigSource())
	}
}

type AvailabilityZoner interface {
	AvailabilityZone() (string, error)
}

// ConfigureAvailabilityZone reads the availability zone from the machine and
// adds the resulting information to the the manager config.
func ConfigureAvailabilityZone(managerConfig container.ManagerConfig, machineZone AvailabilityZoner) (container.ManagerConfig, error) {
	availabilityZone, err := machineZone.AvailabilityZone()
	if err != nil {
		return nil, errors.Trace(err)
	}
	managerConfig[container.ConfigAvailabilityZone] = availabilityZone

	return managerConfig, nil
}
