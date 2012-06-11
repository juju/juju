package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/juju/cmd"
	"launchpad.net/juju-core/juju/environs"
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/tomb"

	// register providers
	_ "launchpad.net/juju-core/juju/environs/dummy"
	_ "launchpad.net/juju-core/juju/environs/ec2"
)

var errInstanceNotFound = errors.New("instance not found")

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
	// TODO(dfc) place the logic in a loop with a suitable delay
	p, err := NewProvisioner(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	return p.Wait()
}

type Provisioner struct {
	st      *state.State
	info    *state.Info
	environ environs.Environ
	tomb    tomb.Tomb

	environWatcher  *state.ConfigWatcher
	machinesWatcher *state.MachinesWatcher

	providerIdToInstance  map[string]environs.Instance
	machineIdToProviderId map[int]string
}

// NewProvisioner returns a Provisioner.
func NewProvisioner(info *state.Info) (*Provisioner, error) {
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	p := &Provisioner{
		st:                    st,
		info:                  info,
		providerIdToInstance:  make(map[string]environs.Instance),
		machineIdToProviderId: make(map[int]string),
	}
	go p.loop()
	return p, nil
}

func (p *Provisioner) loop() {
	defer p.tomb.Done()
	defer func() {
		err := p.st.Close()
		if err != nil {
			p.tomb.Kill(err)
		}
	}()
	p.environWatcher = p.st.WatchEnvironConfig()
	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case config, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			var err error
			p.environ, err = environs.NewEnviron(config.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			log.Printf("provisioner loaded new environment configuration")
			p.innerLoop()
		}
	}
}

func (p *Provisioner) innerLoop() {
	p.machinesWatcher = p.st.WatchMachines()

	// TODO(dfc) we need a method like state.IsConnected() here to exit cleanly if
	// there is a connection problem.
	for {
		select {
		case <-p.tomb.Dying():
			return
		case change, ok := <-p.environWatcher.Changes():
			if !ok {
				err := p.environWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			config, err := environs.NewConfig(change.Map())
			if err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
				continue
			}
			p.environ.SetConfig(config)
			log.Printf("provisioner loaded new environment configuration")
		case machines, ok := <-p.machinesWatcher.Changes():
			if !ok {
				err := p.machinesWatcher.Stop()
				if err != nil {
					p.tomb.Kill(err)
				}
				return
			}
			if err := p.processMachines(machines); err != nil {
				p.tomb.Kill(err)
			}
		}
	}
}

// Wait waits for the Provisioner to exit.
func (p *Provisioner) Wait() error {
	return p.tomb.Wait()
}

// Stop stops the Provisioner and returns any error encountered while
// provisioning.
func (p *Provisioner) Stop() error {
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

func (p *Provisioner) processMachines(changes *state.MachinesChange) error {
	// step 1. filter machines with provider ids and without
	var notrunning []*state.Machine
	for _, m := range changes.Added {
		id, err := m.InstanceId()
		if err != nil {
			return err
		}
		if id == "" {
			notrunning = append(notrunning, m)
		} else {
			log.Printf("machine %s already running as instance %q", m, id)
		}
	}

	// step 2. start all the notrunning machines
	if _, err := p.startMachines(notrunning); err != nil {
		return err
	}

	// step 3. stop all unknown machines and the machines that were removed
	// from the state
	stopping, err := p.instancesForMachines(changes.Deleted)
	if err != nil {
		return err
	}

	// although calling StopInstance with an empty slice should produce no change in the 
	// provider, environs like dummy do not consider this a noop.
	if len(stopping) > 0 {
		return p.environ.StopInstances(stopping)
	}
	return nil
}

func (p *Provisioner) startMachines(machines []*state.Machine) ([]*state.Machine, error) {
	var started []*state.Machine
	for _, m := range machines {
		if err := p.startMachine(m); err != nil {
			return nil, err
		}
		log.Printf("starting machine %v", m)
		started = append(started, m)
	}
	return started, nil
}

func (p *Provisioner) startMachine(m *state.Machine) error {
	// TODO(dfc) the state.Info passed to environ.StartInstance remains contentious
	// however as the PA only knows one state.Info, and that info is used by MAs and 
	// UAs to locate the ZK for this environment, it is logical to use the same 
	// state.Info as the PA. 
	inst, err := p.environ.StartInstance(m.Id(), p.info)
	if err != nil {
		return err
	}

	// assign the provider id to the macine
	if err := m.SetInstanceId(inst.Id()); err != nil {
		return fmt.Errorf("unable to store provider id for machine %v: %v", m, err)
	}

	// populate the local caches
	p.machineIdToProviderId[m.Id()] = inst.Id()
	p.providerIdToInstance[inst.Id()] = inst
	return nil
}

// instanceForMachine returns the environs.Instance that represents this machines' running
// instance.
func (p *Provisioner) instanceForMachine(m *state.Machine) (environs.Instance, error) {
	id, ok := p.machineIdToProviderId[m.Id()]
	if !ok {
		// not cached locally, ask the environ.
		var err error
		id, err = m.InstanceId()
		if err != nil {
			return nil, err
		}
		if id == "" {
			// nobody knows about this machine, give up.
			return nil, errInstanceNotFound
		}
		p.machineIdToProviderId[m.Id()] = id
	}
	inst, ok := p.providerIdToInstance[id]
	if !ok {
		// not cached locally, ask the provider
		var err error
		inst, err = p.findInstance(id)
		if err != nil {
			// the provider doesn't know about this instance, give up.
			return nil, err
		}
		return nil, nil
	}
	return inst, nil
}

// instancesForMachines returns a list of environs.Instance that represent the list of machines running
// in the provider.
func (p *Provisioner) instancesForMachines(machines []*state.Machine) ([]environs.Instance, error) {
	var insts []environs.Instance
	for _, m := range machines {
		inst, err := p.instanceForMachine(m)
		if err != nil {
			return nil, err
		}
		insts = append(insts, inst)
	}
	return insts, nil
}

func (p *Provisioner) findInstance(id string) (environs.Instance, error) {
	insts, err := p.environ.Instances([]string{id})
	if err != nil {
		return nil, err
	}
	if len(insts) < 1 {
		return nil, errInstanceNotFound
	}
	return insts[0], nil
}
