// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	apiwatcher "launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/state/watcher"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/worker"
)

type ProvisionerType string

var (
	logger = loggo.GetLogger("juju.provisioner")

	// ENVIRON provisioners create machines from the environment
	ENVIRON ProvisionerType = "environ"
	// LXC provisioners create lxc containers on their parent machine
	LXC ProvisionerType = "lxc"
	// KVM provisioners create kvm containers on their parent machine
	KVM ProvisionerType = "kvm"
)

// Provisioner represents a running provisioning worker.
type Provisioner struct {
	pt          ProvisionerType
	st          *apiprovisioner.State
	machine     *apiprovisioner.Machine
	environ     environs.Environ
	agentConfig agent.Config
	tomb        tomb.Tomb

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
func NewProvisioner(pt ProvisionerType, st *apiprovisioner.State, agentConfig agent.Config) *Provisioner {
	p := &Provisioner{
		pt:          pt,
		st:          st,
		agentConfig: agentConfig,
	}
	logger.Tracef("Starting %s provisioner for %q", p.pt, p.agentConfig.Tag())
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}

func (p *Provisioner) loop() error {
	// Only wait for the environment if we are an environmental provisioner.
	var environConfigChanges <-chan struct{}
	var environWatcher apiwatcher.NotifyWatcher
	if p.pt == ENVIRON {
		environWatcher, err := p.st.WatchForEnvironConfigChanges()
		if err != nil {
			return err
		}
		environConfigChanges = environWatcher.Changes()
		defer watcher.Stop(environWatcher, &p.tomb)

		p.environ, err = worker.WaitForEnviron(environWatcher, p.st, p.tomb.Dying())
		if err != nil {
			return err
		}
	}

	auth, err := NewAPIAuthenticator(p.st)
	if err != nil {
		return err
	}

	// Start a new worker for the environment or container provisioner,
	// it depends on the provisioner type passed in NewProvisioner.

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
	task := NewProvisionerTask(
		p.agentConfig.Tag(),
		p.st,
		machineWatcher,
		instanceBroker,
		auth)
	defer watcher.Stop(task, &p.tomb)

	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case <-task.Dying():
			err := task.Err()
			logger.Errorf("%s provisioner died: %v", p.pt, err)
			return err
		case _, ok := <-environConfigChanges:
			if !ok {
				return watcher.MustErr(environWatcher)
			}
			config, err := p.st.EnvironConfig()
			if err != nil {
				logger.Errorf("cannot load environment configuration: %v", err)
				return err
			}
			if err := p.setConfig(config); err != nil {
				logger.Errorf("loaded invalid environment configuration: %v", err)
			}
		}
	}
}

func (p *Provisioner) getMachine() (*apiprovisioner.Machine, error) {
	if p.machine == nil {
		var err error
		if p.machine, err = p.st.Machine(p.agentConfig.Tag()); err != nil {
			logger.Errorf("%s is not in state", p.agentConfig.Tag())
			return nil, err
		}
	}
	return p.machine, nil
}

func (p *Provisioner) getWatcher() (Watcher, error) {
	switch p.pt {
	case ENVIRON:
		return p.st.WatchEnvironMachines()
	}
	var ctype instance.ContainerType
	switch p.pt {
	case LXC:
		ctype = instance.LXC
	case KVM:
		ctype = instance.KVM
	default:
		return nil, fmt.Errorf("unknown provisioner type")
	}
	machine, err := p.getMachine()
	if err != nil {
		return nil, err
	}
	return machine.WatchContainers(ctype)

}

func (p *Provisioner) getBroker() (environs.InstanceBroker, error) {
	switch p.pt {
	case ENVIRON:
		return p.environ, nil
	}
	tools, err := p.getAgentTools()
	if err != nil {
		logger.Errorf("cannot get tools from machine for %s broker", p.pt)
		return nil, err
	}
	switch p.pt {
	case LXC:
		return NewLxcBroker(p.st, tools, p.agentConfig), nil
	case KVM:
		return nil, fmt.Errorf("kvm not implemented yet")
	}
	return nil, fmt.Errorf("unknown provisioner type")
}

func (p *Provisioner) getAgentTools() (*coretools.Tools, error) {
	return p.st.Tools(p.agentConfig.Tag())
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

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}
