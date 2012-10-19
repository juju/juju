package provisioner

import "launchpad.net/juju-core/state"

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

// WaitConfigReload waits until the Provisioner has completed a 
// configuration reload.
func (p *Provisioner) WaitConfigReload() {
	<-p.reload
}
