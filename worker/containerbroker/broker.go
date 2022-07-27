// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/worker/containerbroker State
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/machine_mock.go github.com/juju/juju/api/agent/provisioner MachineProvisioner

// Config describes the dependencies of a Tracker.
//
// It's arguable that it should be called TrackerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	APICaller     base.APICaller
	AgentConfig   agent.Config
	MachineLock   machinelock.Lock
	NewBrokerFunc func(broker.Config) (environs.InstanceBroker, error)
	NewStateFunc  func(base.APICaller) State
}

// State represents the interaction for the apiserver
type State interface {
	broker.APICalls
	Machines(...names.MachineTag) ([]provisioner.MachineResult, error)
	ContainerManagerConfig(params.ContainerManagerConfigParams) (params.ContainerManagerConfig, error)
}

// Validate returns an error if the config cannot be used to start a Tracker.
func (config Config) Validate() error {
	if config.APICaller == nil {
		return errors.NotValidf("nil APICaller")
	}
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.MachineLock == nil {
		return errors.NotValidf("nil MachineLock")
	}
	if config.NewBrokerFunc == nil {
		return errors.NotValidf("nil NewBrokerFunc")
	}
	if config.NewStateFunc == nil {
		return errors.NotValidf("nil NewStateFunc")
	}
	return nil
}

// NewWorkerTracker defines a function that is covariant in the return type
// so that the manifold can use the covariance of the function as an alias.
func NewWorkerTracker(config Config) (worker.Worker, error) {
	return NewTracker(config)
}

// Tracker loads a broker, makes it available to clients, and updates
// the broker in response to config changes until it is killed.
type Tracker struct {
	config   Config
	catacomb catacomb.Catacomb
	broker   environs.InstanceBroker
}

// NewTracker returns a new Tracker, or an error if anything goes wrong.
// If a tracker is returned, its Broker() method is immediately usable.
//
// The caller is responsible for Kill()ing the returned Tracker and Wait()ing
// for any errors it might return.
func NewTracker(config Config) (*Tracker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	machineTag := config.AgentConfig.Tag().(names.MachineTag)
	provisioner := config.NewStateFunc(config.APICaller)
	result, err := provisioner.Machines(machineTag)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load machine %s from state", machineTag)
	}
	if len(result) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result))
	}
	if errors.IsNotFound(result[0].Err) || (result[0].Err == nil && result[0].Machine.Life() == life.Dead) {
		return nil, dependency.ErrUninstall
	}
	machine := result[0].Machine
	instanceContainers, determined, err := machine.SupportedContainers()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving supported container types")
	}
	if !determined {
		return nil, errors.Errorf("no container types determined")
	}
	if len(instanceContainers) == 0 {
		return nil, dependency.ErrUninstall
	}
	// we only work on LXD, so check for that.
	var found bool
	for _, containerType := range instanceContainers {
		if containerType == instance.LXD {
			found = true
			break
		}
	}
	if !found {
		return nil, dependency.ErrUninstall
	}

	// We guarded against non-LXD types, so lets talk about specific container
	// types to prevent confusion.
	containerType := instance.LXD
	managerConfigResult, err := provisioner.ContainerManagerConfig(
		params.ContainerManagerConfigParams{Type: containerType},
	)
	if err != nil {
		return nil, errors.Annotate(err, "generating container manager config")
	}
	managerConfig := container.ManagerConfig(managerConfigResult.ManagerConfig)
	managerConfigWithZones, err := broker.ConfigureAvailabilityZone(managerConfig, machine)
	if err != nil {
		return nil, errors.Annotate(err, "configuring availability zones")
	}

	broker, err := config.NewBrokerFunc(broker.Config{
		Name:          "instance-broker",
		ContainerType: containerType,
		ManagerConfig: managerConfigWithZones,
		APICaller:     provisioner,
		AgentConfig:   config.AgentConfig,
		MachineTag:    machineTag,
		MachineLock:   config.MachineLock,
		GetNetConfig:  common.GetObservedNetworkConfig,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot create instance broker")
	}
	t := &Tracker{
		config: config,
		broker: broker,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Broker returns the encapsulated Broker. It will continue to be updated in
// the background for as long as the Tracker continues to run.
func (t *Tracker) Broker() environs.InstanceBroker {
	return t.broker
}

func (t *Tracker) loop() error {
	for {
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (t *Tracker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Tracker) Wait() error {
	return t.catacomb.Wait()
}
