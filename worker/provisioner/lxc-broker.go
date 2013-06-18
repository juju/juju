// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/golxc"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/loggo"
)

var lxcLogger = loggo.GetLogger("juju.provisioner.lxc")

func NewLxcBroker(factory golxc.ContainerFactory, config *config.Config, tools *state.Tools) Broker {
	return &lxcBroker{
		factory: lxc.NewFactory(factory),
		config:  config,
		tools:   tools,
	}
}

type lxcBroker struct {
	factory lxc.ContainerFactory
	config  *config.Config
	tools   *state.Tools
}

func (broker *lxcBroker) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (instance.Instance, error) {
	lxcLogger.Infof("starting lxc container for machineId: %s", machineId)

	lxcContainer, err := broker.factory.NewContainer(machineId)
	if err != nil {
		lxcLogger.Errorf("failed to create container: %v", err)
		return nil, err
	}
	err = lxcContainer.Create(series, machineNonce, broker.tools, broker.config, info, apiInfo)
	if err != nil {
		lxcLogger.Errorf("failed to create container: %v", err)
		return nil, err
	}
	if err := lxcContainer.Start(); err != nil {
		lxcLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	// check to make sure it started.
	return lxcContainer, nil
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances(instances []environs.Instance) error {
	// TODO: potentially parallelise.
	for _, instance := range instances {
		lxcLogger.Infof("stopping lxc container for instance: %s", instance.Id())
		lxcContainer, ok := instance.(container.Container)
		if !ok {
			lxcLogger.Warningf("instance is not a container - shouldn't happen")
			continue
		}
		if err := lxcContainer.Stop(); err != nil {
			lxcLogger.Errorf("container did not stop: %v", err)
			return err
		}
		if err := lxcContainer.Destroy(); err != nil {
			lxcLogger.Errorf("container did not get destroyed: %v", err)
			return err
		}
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxcBroker) AllInstances() (result []environs.Instance, err error) {
	// TODO(thumper): work on some prefix to avoid getting *all* containers.
	containers, err := golxc.List()
	if err != nil {
		lxcLogger.Errorf("failed getting all instances: %v", err)
		return
	}
	for _, container := range containers {
		if container.IsRunning() {
			lxcContainer, err := broker.factory.NewFromExisting(container)
			if err != nil {
				return nil, err
			}
			result = append(result, lxcContainer.Instance())
		}
	}
	return
}
