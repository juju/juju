// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs/config"
)

func SetObserver(p *Provisioner, observer chan<- *config.Config) {
	p.Lock()
	p.observer = observer
	p.Unlock()
}
