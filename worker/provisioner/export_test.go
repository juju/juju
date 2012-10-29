package provisioner

import "launchpad.net/juju-core/state"
import "launchpad.net/juju-core/environs"

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

// NewProvisionerWithReloadChan is similar to NewProvisioner and allows
// the caller to provide a channel to receive when the Provisioner actions
// a configuration change.
func NewProvisionerWithReloadChan(st *state.State, reload chan<- bool) *Provisioner {
	p := &Provisioner{
		st:        st,
		instances: make(map[int]environs.Instance),
		machines:  make(map[string]int),
		reload:    reload,
	}
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}
