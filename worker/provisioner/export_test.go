// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs/config"
)

func (o *configObserver) SetObserver(observer chan<- *config.Config) {
	o.Lock()
	o.observer = observer
	o.Unlock()
}
