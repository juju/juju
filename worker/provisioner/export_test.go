// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/watcher"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	ep := p.(*environProvisioner)
	ep.Lock()
	ep.observer = observer
	ep.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}
