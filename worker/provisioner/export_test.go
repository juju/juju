// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

// exported so we can manually close the Provisioners underlying
// state connection.
func (p *Provisioner) CloseState() error {
	return p.st.Close()
}

// exported so we can discover all machines visible to the
// Provisioners state connection.
func (p *Provisioner) AllMachines() ([]*state.Machine, error) {
	return p.st.AllMachines()
}

func (o *configObserver) SetObserver(observer chan<- *config.Config) {
	o.Lock()
	o.observer = observer
	o.Unlock()
}
