// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineprovisioner

import (
	"github.com/juju/juju/environs/config"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	ep := p.(*environProvisioner)
	configObserver = &ep.configObserver
	configObserver.catacomb = &ep.catacomb
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}
