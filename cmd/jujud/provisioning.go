package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/state"
	"launchpad.net/tomb"

	// register providers
	_ "launchpad.net/juju/go/environs/dummy"
	_ "launchpad.net/juju/go/environs/ec2"
)

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	Conf AgentConf
}

// Info returns usage information for the command.
func (a *ProvisioningAgent) Info() *cmd.Info {
	return &cmd.Info{"provisioning", "", "run a juju provisioning agent", ""}
}

// Init initializes the command for running.
func (a *ProvisioningAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return a.Conf.checkArgs(f.Args())
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	p := NewProvisioner(st)
	return p.tomb.Wait()
}

type Provisioner struct {
	st      *state.State
	environ environs.Environ
	tomb    tomb.Tomb

	environWatcher  *state.ConfigWatcher
	machinesWatcher *state.MachinesWatcher
}

// environChanges returns a channel that will receive the new *ConfigNode 
// when a change is detected. 
func (p *Provisioner) environChanges() <-chan *state.ConfigNode {
	if p.environWatcher == nil {
		p.environWatcher = p.st.WatchEnvironConfig()
	}
	return p.environWatcher.Changes()
}

// stopEnvironWatcher stops and invalidates the current environWatcher.
func (p *Provisioner) stopEnvironWatcher() (err error) {
	if p.environWatcher != nil {
		if err = p.environWatcher.Stop(); err != nil {
			log.Printf("provisioning: environWatcher reported error on Stop: %v", err)
		}
	}
	p.environWatcher = nil
	return
}

// changes returns a channel that will receive the new *ConfigNode when a
// change is detected. 
func (p *Provisioner) machinesChanges() <-chan *state.MachinesChange {
	if p.machinesWatcher == nil {
		p.machinesWatcher = p.st.WatchMachines()
	}
	return p.machinesWatcher.Changes()
}

// stopMachinesWatcher stops and invalidates the current machinesWatcher..
func (p *Provisioner) stopMachinesWatcher() (err error) {
	if p.machinesWatcher != nil {
		if err = p.machinesWatcher.Stop(); err != nil {
			log.Printf("provisioning: machinesWatcehr reported error on Stop: %v", err)
		}
	}
	p.machinesWatcher = nil
	return
}

// NewProvisioner returns a Provisioner.
func NewProvisioner(st *state.State) *Provisioner {
	p := &Provisioner{
		st: st,
	}
	go p.loop()
	return p
}

func (p *Provisioner) loop() {
	defer p.tomb.Done()
	// TODO(dfc) we need a method like state.IsValid() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case config, ok := <-p.environChanges():
			if !ok {
				p.stopEnvironWatcher()
				continue
			}
			var err error
			p.environ, err = environs.NewEnviron(config.Map())
			if err != nil {
				log.Printf("provisioner: unable to create environment from supplied configuration: %v", err)
				continue
			}
			log.Printf("provisioner: valid environment configured")
			p.innerLoop()
		}
	}
}

func (p *Provisioner) innerLoop() {
	// TODO(dfc) we need a method like state.IsValid() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case change, ok := <-p.environChanges():
			if !ok {
				p.stopEnvironWatcher()
				continue
			}
			config, err := environs.NewConfig(change.Map())
			if err != nil {
				log.Printf("provisioner: new configuration received, but was not valid: %v", err)
				continue
			}
			p.environ.SetConfig(config)
			log.Printf("provisioner: new environment configuartion applied")
		case machines, ok := <-p.machinesChanges():
			if !ok {
				p.stopMachinesWatcher()
				continue
			}
			p.processMachines(machines)
		}
	}
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	err1 := p.stopEnvironWatcher()
	err2 := p.stopMachinesWatcher()
	err3 := p.tomb.Wait()
	for _, err := range []error{err3, err2, err1} {
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) processMachines(changes *state.MachinesChange) {}
