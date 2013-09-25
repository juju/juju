// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"
	"sync"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/state/watcher"
	coretools "launchpad.net/juju-core/tools"
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
	pt          ProvisionerType
	st          *apiprovisioner.State
	machineTag  string // Which machine runs the provisioner.
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
		machineTag:  agentConfig.Tag(),
		agentConfig: agentConfig,
	}
	logger.Tracef("Starting %s provisioner for %q", p.pt, p.machineTag)
	go func() {
		defer p.tomb.Done()
		p.tomb.Kill(p.loop())
	}()
	return p
}

func (p *Provisioner) loop() error {
	environWatcher, err := p.st.WatchForEnvironConfigChanges()
	if err != nil {
		return err
	}
	environConfigChanges := environWatcher.Changes()
	defer func() {
		if environWatcher != nil {
			watcher.Stop(environWatcher, &p.tomb)
		}
	}()

	p.environ, err = worker.WaitForEnviron(environWatcher, p.st, p.tomb.Dying())
	if err != nil {
		return err
	}

	if p.pt != ENVIRON {
		// Only the environment provisioner cares about
		// changes to the environment configuration.
		watcher.Stop(environWatcher, &p.tomb)
		environWatcher = nil
		environConfigChanges = nil
	}

	auth, err := NewAgentConfigAuthenticator(p.agentConfig)
	if err != nil {
		return err
	}

	// Start a new worker for the environment or lxc provisioner,
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
		p.machineTag,
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
		if p.machine, err = p.st.Machine(p.machineTag); err != nil {
			logger.Errorf("%s is not in state", p.machineTag)
			return nil, err
		}
	}
	return p.machine, nil
}

func (p *Provisioner) getWatcher() (Watcher, error) {
	switch p.pt {
	case ENVIRON:
		return p.st.WatchEnvironMachines()
	case LXC:
		machine, err := p.getMachine()
		if err != nil {
			return nil, err
		}
		return machine.WatchContainers(instance.LXC)
	}
	return nil, fmt.Errorf("unknown provisioner type")
}

func (p *Provisioner) getBroker() (environs.InstanceBroker, error) {
	switch p.pt {
	case ENVIRON:
		return p.environ, nil
	case LXC:
		config := p.environ.Config()
		tools, err := p.getAgentTools()
		if err != nil {
			logger.Errorf("cannot get tools from machine for lxc broker")
			return nil, err
		}
		return NewLxcBroker(config, tools, p.agentConfig), nil
	}
	return nil, fmt.Errorf("unknown provisioner type")
}

func (p *Provisioner) getAgentTools() (*coretools.Tools, error) {
	dataDir := p.agentConfig.DataDir()
	tools, err := agenttools.ReadTools(dataDir, version.Current)
	if err != nil {
		logger.Errorf("cannot read agent tools from %q", dataDir)
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

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}
