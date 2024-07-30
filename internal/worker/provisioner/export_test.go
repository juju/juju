// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	if ep, ok := p.(*environProvisioner); ok {
		configObserver = &ep.configObserver
		configObserver.catacomb = &ep.catacomb
	} else {
		cp := p.(*containerProvisioner)
		configObserver = &cp.configObserver
		configObserver.catacomb = &cp.catacomb
	}
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher(context.Background())
}

var (
	GetContainerInitialiser = &getContainerInitialiser
)

func MachineSupportsContainers(
	cfg ContainerManifoldConfig, pr ContainerMachineGetter, mTag names.MachineTag,
) (ContainerMachine, error) {
	return cfg.machineSupportsContainers(context.Background(), pr, mTag)
}
