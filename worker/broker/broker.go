// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package broker

import (
	"github.com/juju/errors"
	names "gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
)

// Config describes the dependencies of a Tracker.
//
// It's arguable that it should be called TrackerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	APICaller     base.APICaller
	AgentConfig   agent.Config
	MachineLock   machinelock.Lock
	NewBrokerFunc func(broker.Config) (environs.InstanceBroker, error)
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
	return nil
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
	provisioner := provisioner.NewState(config.APICaller)
	result, err := provisioner.Machines(machineTag)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load machine %s from state", machineTag)
	}
	if len(result) != 1 {
		return nil, errors.Annotatef(err, "expected 1 result, got %d", len(result))
	}
	if errors.IsNotFound(result[0].Err) || (result[0].Err == nil && result[0].Machine.Life() == params.Dead) {
		return nil, worker.ErrTerminateAgent
	}
	machine := result[0].Machine
	// Expose SupportedContainers <- ([]instance.ContainerType, bool)
	// if anything but LXD worker.ErrTerminateAgent
	instanceContainers, determined, err := machine.SupportedContainers()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving supported container types")
	}
	if len(instanceContainers) == 0 && !determined {
		return nil, errors.Annotate(err, "no container types deterimined")
	}
	// we only work on LXD, so check for that.
	for _, containerType := range instanceContainers {
		if containerType != instance.LXD {
			return nil, worker.ErrTerminateAgent
		}
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
