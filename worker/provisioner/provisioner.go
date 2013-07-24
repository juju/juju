// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
)

type ProvisionerType string

var (
	logger = loggo.GetLogger("juju.provisioner")

	// ENVIRON provisioners create machines from the environment
	ENVIRON ProvisionerType = "environ"
	// LXC provisioners create lxc containers on their parent machine
	LXC ProvisionerType = "lxc"
)

// Provisioner represents a running provisioning worker.
type Provisioner struct {
	pt        ProvisionerType
	st        *state.State
	machineId string // Which machine runs the provisioner.
	dataDir   string
	machine   *state.Machine
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
func NewProvisioner(pt ProvisionerType, st *state.State, machineId, dataDir string) *Provisioner {
	p := &Provisioner{
		pt:        pt,
		st:        st,
		machineId: machineId,
		dataDir:   dataDir,
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

	auth, err := NewSimpleAuthenticator(p.environ)
	if err != nil {
		return err
	}

	// Start a new worker for the environment provider.

	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	instanceBroker, err := p.getBroker()
	if err != nil {
		return err
	}
	machineWatcher, err := p.getWatcher()
	if err != nil {
		return err
	}
	environmentProvisioner := NewProvisionerTask(
		p.machineId,
		p.st,
		machineWatcher,
		instanceBroker,
		auth)
	defer watcher.Stop(environmentProvisioner, &p.tomb)

	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-environmentProvisioner.Dying():
			err := environmentProvisioner.Err()
			logger.Errorf("environment provisioner died: %v", err)
			return err
		case cfg, ok := <-environWatcher.Changes():
			if !ok {
				return watcher.MustErr(environWatcher)
			}
			if err := p.setConfig(cfg); err != nil {
				logger.Errorf("loaded invalid environment configuration: %v", err)
			}
		}
	}
	panic("not reached")
}

func (p *Provisioner) getMachine() (*state.Machine, error) {
	if p.machine == nil {
		var err error
		if p.machine, err = p.st.Machine(p.machineId); err != nil {
			logger.Errorf("machine %s is not in state", p.machineId)
			return nil, err
		}
	}
	return p.machine, nil
}

func (p *Provisioner) getWatcher() (Watcher, error) {
	switch p.pt {
	case ENVIRON:
		return p.st.WatchEnvironMachines(), nil
	case LXC:
		machine, err := p.getMachine()
		if err != nil {
			return nil, err
		}
		return machine.WatchContainers(instance.LXC), nil
	}
	return nil, fmt.Errorf("unknown provisioner type")
}

func (p *Provisioner) getBroker() (Broker, error) {
	switch p.pt {
	case ENVIRON:
		return newEnvironBroker(p.environ), nil
	case LXC:
		config := p.environ.Config()
		tools, err := p.getAgentTools()
		if err != nil {
			logger.Errorf("cannot get tools from machine for lxc broker")
			return nil, err
		}
		return NewLxcBroker(config, tools), nil
	}
	return nil, fmt.Errorf("unknown provisioner type")
}

func (p *Provisioner) getAgentTools() (*tools.Tools, error) {
	tools, err := tools.ReadTools(p.dataDir, version.Current)
	if err != nil {
		logger.Errorf("cannot read agent tools from %q", p.dataDir)
		return nil, err
	}
	return tools, nil
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
	return fmt.Sprintf("%s provisioning worker for machine %s", string(p.pt), p.machineId)
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}
