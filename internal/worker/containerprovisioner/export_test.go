// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/environs/config"
)

var (
	GetContainerInitialiser = &getContainerInitialiser
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	cp := p.(*containerProvisioner)
	configObserver = &cp.configObserver
	configObserver.catacomb = &cp.catacomb
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}

func MachineSupportsContainers(
	c *tc.C, cfg ManifoldConfig, pr ContainerMachineGetter, mTag names.MachineTag,
) (ContainerMachine, error) {
	return cfg.machineSupportsContainers(c.Context(), pr, mTag)
}
