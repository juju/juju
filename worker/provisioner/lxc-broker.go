// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/golxc"
	"launchpad.net/juju-core/constraints"
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
		golxc:   factory,
		manager: lxc.NewContainerManager(factory, "juju"),
		config:  config,
		tools:   tools,
	}
}

type lxcBroker struct {
	golxc   golxc.ContainerFactory
	manager lxc.ContainerManager
	config  *config.Config
	tools   *state.Tools
}

func (broker *lxcBroker) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (instance.Instance, error) {
	lxcLogger.Infof("starting lxc container for machineId: %s", machineId)

	inst, err := broker.manager.StartContainer(machineId, series, machineNonce, broker.tools, broker.config, info, apiInfo)
	if err != nil {
		lxcLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	return inst, nil
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances(instances []instance.Instance) error {
	// TODO: potentially parallelise.
	for _, instance := range instances {
		lxcLogger.Infof("stopping lxc container for instance: %s", instance.Id())
		if err := broker.manager.StopContainer(instance); err != nil {
			lxcLogger.Errorf("container did not stop: %v", err)
			return err
		}
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxcBroker) AllInstances() (result []instance.Instance, err error) {
	// TODO(thumper): work on some prefix to avoid getting *all* containers.
	return broker.manager.ListContainers()
}
