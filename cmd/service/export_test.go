package service

import "launchpad.net/juju-core/state"

// exported so we can manuall close the Provisioniers underlying
// state connection.
func (p *Provisioner) CloseState() error {
	return p.st.Close()
}

// exported so we can discover all machines visible to the 
// Provisioners state connection.
func (p *Provisioner) AllMachines() ([]*state.Machine, error) {
	return p.st.AllMachines()
}
