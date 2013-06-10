// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"sync"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/loggo"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.provisioner")

// Provisioner represents a running provisioning worker.
type Provisioner struct {
	st        *state.State
	machineId string // Which machine runs the provisioner.
	environ   environs.Environ
	tomb      tomb.Tomb

	configObserver
}

type configObserver struct {
	sync.Mutex
	observer chan<- *config.Config
}

// nofity notifies the observer of a configuration change.
func (o *configObserver) notify(cfg *config.Config) {
	o.Lock()
	if o.observer != nil {
		o.observer <- cfg
	}
	o.Unlock()
}

// NewProvisioner returns a new Provisioner. When new machines
// are added to the state, it allocates instances from the environment
// and allocates them to the new machines.
func NewProvisioner(st *state.State, machineId string) *Provisioner {
	p := &Provisioner{
		st:        st,
		machineId: machineId,
	}
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}

func (p *Provisioner) loop() error {
	environWatcher := p.st.WatchEnvironConfig()
	defer watcher.Stop(environWatcher, &p.tomb)

	var err error
	p.environ, err = worker.WaitForEnviron(environWatcher, p.tomb.Dying())
	if err != nil {
		return err
	}

	// Get a new StateInfo from the environment: the one used to
	// launch the agent may refer to localhost, which will be
	// unhelpful when attempting to run an agent on a new machine.
	stateInfo, apiInfo, err := p.environ.StateInfo()
	if err != nil {
		return err
	}

	// Start a new worker for the environment provider.

	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	machinesWatcher := p.st.WatchMachines()
	environmentBroker := newEnvironBroker(p.environ)
	environmentProvisioner := newProvisionerTask(
		p.machineId,
		p.st,
		machinesWatcher,
		environmentBroker,
		stateInfo,
		apiInfo)
	defer environmentProvisioner.Stop()

	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case cfg, ok := <-environWatcher.Changes():
			if !ok {
				return watcher.MustErr(environWatcher)
			}
			if err := p.setConfig(cfg); err != nil {
				logger.Error("loaded invalid environment configuration: %v", err)
			}
		case <-environmentProvisioner.Dying():
			err := environmentProvisioner.Err()
			logger.Error("environment provisioner died: %v", err)
			return err
		}
	}
	panic("not reached")
}

// setConfig updates the environment configuration and notifies
// the config observer.
func (p *Provisioner) setConfig(config *config.Config) error {
	if err := p.environ.SetConfig(config); err != nil {
		return err
	}
	p.configObserver.notify(config)
	return nil
}

// Err returns the reason why the Provisioner has stopped or tomb.ErrStillAlive
// when it is still alive.
func (p *Provisioner) Err() (reason error) {
	return p.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (p *Provisioner) Kill() {
	p.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (p *Provisioner) Wait() error {
	return p.tomb.Wait()
}

func (p *Provisioner) String() string {
	return "provisioning worker"
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}
