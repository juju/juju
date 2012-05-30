package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/state"

	// register providers
	_ "launchpad.net/juju/go/environs/dummy"
	_ "launchpad.net/juju/go/environs/ec2"
)

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	Conf    AgentConf
	environ environs.Environ // the provider this agent operates against.
	State   *state.State

	providerIdToInstance  map[string]environs.Instance
	machineIdToProviderId map[int]string
}

func NewProvisioningAgent() *ProvisioningAgent {
	return &ProvisioningAgent{
		providerIdToInstance:  make(map[string]environs.Instance),
		machineIdToProviderId: make(map[int]string),
	}
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
	var err error
	a.State, err = state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}

	// step 1. wait for a valid environment
	configWatcher := a.State.WatchEnvironConfig()
	for {
		log.Printf("provisioning: waiting for valid environment")
		config, ok := <-configWatcher.Changes()
		if !ok {
			return fmt.Errorf("environment watcher has shutdown: %v", configWatcher.Stop())
		}
		var err error
		a.environ, err = environs.NewEnviron(config.Map())
		if err == nil {
			break
		}
		log.Printf("provisioning: unable to create environment from supplied configuration: %v", err)
	}
	log.Printf("provisioning: valid environment configured")

	// step 2. listen for changes to the environment or the machine topology and action both.
	machinesWatcher := a.State.WatchMachines()
	for {
		select {
		case changes, ok := <-configWatcher.Changes():
			if !ok {
				return fmt.Errorf("environment watcher has shutdown: %v", configWatcher.Stop())
			}
			config, err := environs.NewConfig(changes.Map())
			if err != nil {
				log.Printf("provisioning: new configuration received, but was not valid: %v", err)
				continue
			}
			a.environ.SetConfig(config)
			log.Printf("provisioning: new configuartion applied")
		case changes, ok := <-machinesWatcher.Changes():
			if !ok {
				return fmt.Errorf("machines watcher has shutdown: %v", configWatcher.Stop())
			}
			for _, added := range changes.Added {
				if err := a.addMachine(added); err != nil {
					// TODO(dfc) implement retry logic
					return err
				}
				log.Printf("provisioning: machine %d added", added.Id())
			}
			for _, deleted := range changes.Deleted {
				if err := a.deleteMachine(deleted); err != nil {
					// TODO(dfc) implement retry logic
					return err
				}
				log.Printf("provisioning: machine %d deleted", deleted.Id())
			}
		}
	}
	if err = configWatcher.Stop(); err == nil {
		err = machinesWatcher.Stop()
	}
	return err
}

func (a *ProvisioningAgent) addMachine(m *state.Machine) error {
	id, err := m.InstanceId()
	if err != nil {
		return err
	}
	if id != "" {
		return fmt.Errorf("machine-%010d already reports a provider id %q, skipping", m.Id(), id)
	}

	// TODO(dfc) the state.Info passed to environ.StartInstance remains contentious
	// however as the PA only knows one state.Info, and that info is used by MAs and 
	// UAs to locate the ZK for this environment, it is logical to use the same 
	// state.Info as the PA. 
	inst, err := a.environ.StartInstance(m.Id(), &a.Conf.StateInfo)
	if err != nil {
		return err
	}

	// assign the provider id to the macine
	if err := m.SetInstanceId(inst.Id()); err != nil {
		return fmt.Errorf("unable to store provider id: %v", err)
	}

	// populate the local caches
	a.machineIdToProviderId[m.Id()] = inst.Id()
	a.providerIdToInstance[inst.Id()] = inst
	return nil
}

func (a *ProvisioningAgent) deleteMachine(m *state.Machine) error {
	insts, err := a.InstancesForMachines(m)
	if err != nil {
		return fmt.Errorf("machine-%010d has no refrence to a provider id, skipping", m.Id())
	}
	return a.environ.StopInstances(insts)
}

// InstanceForMachine returns the environs.Instance that represents this machines' running
// instance.
func (a *ProvisioningAgent) InstanceForMachine(m *state.Machine) (environs.Instance, error) {
	id, ok := a.machineIdToProviderId[m.Id()]
	if !ok {
		// not cached locally, ask the provider.
		var err error
		id, err = m.InstanceId()
		if err != nil {
			return nil, err
		}
		if id == "" {
			// nobody knows about this machine, give up.
			return nil, fmt.Errorf("instance not found")
		}
		a.machineIdToProviderId[m.Id()] = id
	}
	inst, ok := a.providerIdToInstance[id]
	if !ok {
		// not cached locally, ask the provider
		var err error
		inst, err = a.findInstance(id)
		if err != nil {
			// the provider doesn't know about this instance, give up.
			return nil, err
		}
		return nil, nil
	}
	return inst, nil
}

// InstancesForMachines returns a list of environs.Instance that represent the list of machines running
// in the provider.
func (a *ProvisioningAgent) InstancesForMachines(machines ...*state.Machine) ([]environs.Instance, error) {
	var insts []environs.Instance
	for _, m := range machines {
		inst, err := a.InstanceForMachine(m)
		if err != nil {
			return nil, err
		}
		insts = append(insts, inst)
	}
	return insts, nil
}

func (a *ProvisioningAgent) findInstance(id string) (environs.Instance, error) {
	insts, err := a.environ.Instances([]string{id})
	if err != nil {
		return nil, err
	}
	if len(insts) < 1 {
		return nil, fmt.Errorf("instance not found")
	}
	return insts[0], nil
}
