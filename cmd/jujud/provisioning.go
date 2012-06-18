package main

import (
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

	// machine.Id => environs.Instance
	instances map[int]environs.Instance
	// instance.Id() => *state.Machine
	machines map[string]*state.Machine
}

// NewProvisioner returns a Provisioner.
func NewProvisioner(info *state.Info) (*Provisioner, error) {
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	p := &Provisioner{
		st:        st,
		info:      info,
		instances: make(map[int]environs.Instance),
		machines:  make(map[string]*state.Machine),
	}
	go p.loop()
	return p, nil
}

func (p *Provisioner) loop() {
	defer p.tomb.Done()
	defer p.st.Close()
	p.environWatcher = p.st.WatchEnvironConfig()
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
			// TODO(dfc) fire process machines periodically to shut down unknown
			// instances.
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
	// step 1. find which of the added machines have not
	// yet been allocated a started instance.
	notstarted, err := p.findNotStarted(changes.Added)
	if err != nil {
		return err
	}

	// step 2. start an instance for any machines we found.
	if _, err := p.startMachines(notstarted); err != nil {
		return err
	}

	// step 3. stop all machines that were removed from the state.
	stopping, err := p.instancesForMachines(changes.Deleted)
	if err != nil {
		return err
	}

	// step 4. find instances which are running but have no machine 
	// associated with them.
	unknown, err := p.findUnknownInstances()

	// although calling StopInstance with an empty slice should produce no change in the 
	// provider, environs like dummy do not consider this a noop.
	if len(stopping) > 0 {
		return p.environ.StopInstances(append(stopping, unknown...))
	}
	return nil
}

// findUnknownInstances finds instances which are not associated with supplied list of machines.
func (p *Provisioner) findUnknownInstances() ([]environs.Instance, error) {
	all, err := p.environ.AllInstances()
	if err != nil {
		return nil, err
	}
	instances := make(map[string]environs.Instance)
	for _, i := range all {
		instances[i.Id()] = i
	}
	machines, err := p.st.AllMachines()
	if err != nil { return nil, err }
	for _, m := range machines {
		id, err := m.InstanceId()
		if err != nil { return nil, err }
		if _, ok := instances[id]; ok { delete(instances, id) }
	}
	var unknown []environs.Instance
	for _, i := range instances {
		unknown = append(unknown, i)
	}
	return unknown, nil
}

// findNotStarted finds machines without an InstanceId set, these are defined as not started.
func (p *Provisioner) findNotStarted(machines []*state.Machine) ([]*state.Machine, error) {
	var notstarted []*state.Machine
	for _, m := range machines {
		id, err := m.InstanceId()
		if err != nil {
			return nil, err
		}
		if id == "" {
			notstarted = append(notstarted, m)
		} else {
			log.Printf("machine %s already started as instance %q", m, id)
		}
	}
	return notstarted, nil
}

func (p *Provisioner) startMachines(machines []*state.Machine) ([]*state.Machine, error) {
	var started []*state.Machine
	for _, m := range machines {
		if err := p.startMachine(m); err != nil {
			return nil, err
		}
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
		log.Printf("provisioner can't start machine %s: %v", m, err)
		return err
	}

	// assign the instance id to the machine
	if err := m.SetInstanceId(inst.Id()); err != nil {
		return err
	}

	// populate the local cache
	p.instances[m.Id()] = inst
	p.machines[inst.Id()] = m
	log.Printf("provisioner started machine %s as instance %s", m, inst.Id())
	return nil
}

func (p *Provisioner) stopInstances(instances []environs.Instance) error {
	// although calling StopInstance with an empty slice should produce no change in the 
	// provider, environs like dummy do not consider this a noop.
	if len(instances) == 0 {
		return nil
	}
	if err := p.environ.StopInstances(instances); err != nil {
		return err
	}

	// cleanup cache
	for _, i := range instances {
		if m, ok := p.machines[i.Id()]; ok {
			delete(p.machines, i.Id())
			delete(p.instances, m.Id())
		}
	}
	return nil
}

// instanceForMachine returns the environs.Instance that represents this machine's instance.
func (p *Provisioner) instanceForMachine(m *state.Machine) (environs.Instance, error) {
	inst, ok := p.instances[m.Id()]
	if !ok {
		// not cached locally, ask the environ.
		id, err := m.InstanceId()
		if err != nil {
			return nil, err
		}
		if id == "" {
			// TODO(dfc) InstanceId should return an error if the id isn't set.
			return nil, fmt.Errorf("machine %s not found", m)
		}
		// TODO(dfc) this should be batched, or the cache preloaded at startup to
		// avoid N calls to the envirion.
		insts, err := p.environ.Instances([]string{id})
		if err != nil {
			// the provider doesn't know about this instance, give up.
			return nil, err
		}
		inst = insts[0]
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
