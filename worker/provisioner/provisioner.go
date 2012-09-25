package provisioner

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// Provisioner represents a running provisioning worker.
type Provisioner struct {
	st      *state.State
	info    *state.Info
	environ environs.Environ
	tomb    tomb.Tomb

	// machine.Id => environs.Instance
	instances map[int]environs.Instance
	// instance.Id => machine id
	machines map[string]int
}

// NewProvisioner returns a new Provisioner. When new machines
// are added to the state, it allocates instances from the environment
// and allocates them to the new machines.
func NewProvisioner(st *state.State) *Provisioner {
	p := &Provisioner{
		st:        st,
		instances: make(map[int]environs.Instance),
		machines:  make(map[string]int),
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
	if p.info, err = p.environ.StateInfo(); err != nil {
		return err
	}

	// Call processMachines to stop any unknown instances before watching machines.
	if err := p.processMachines(nil, nil); err != nil {
		return err
	}

	// Start responding to changes in machines, and to any further updates
	// to the environment config.
	machinesWatcher := p.st.WatchMachines()
	defer watcher.Stop(machinesWatcher, &p.tomb)
	for {
		select {
		case <-p.tomb.Dying():
			return tomb.ErrDying
		case cfg, ok := <-environWatcher.Changes():
			if !ok {
				return watcher.MustErr(environWatcher)
			}
			if err := p.environ.SetConfig(cfg); err != nil {
				log.Printf("provisioner loaded invalid environment configuration: %v", err)
			}
		case machines, ok := <-machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(machinesWatcher)
			}
			// TODO(dfc) fire process machines periodically to shut down unknown
			// instances.
			if err := p.processMachines(machines.Alive, machines.Dead); err != nil {
				return err
			}
		}
	}
	panic("not reached")
}

// Dying returns a channel that signals a Provisioners exit.
func (p *Provisioner) Dying() <-chan struct{} {
	return p.tomb.Dying()
}

// Err returns the reason why the Provisioner has stopped or tomb.ErrStillAlive
// when it is still alive.
func (p *Provisioner) Err() (reason error) {
	return p.tomb.Err()
}

// Wait waits for the Provisioner to exit.
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

func (p *Provisioner) processMachines(alive, dead []int) error {
	// step 1. find which of the added machines have not
	// yet been allocated a started instance.
	notstarted, err := p.findNotStarted(alive)
	if err != nil {
		return err
	}

	// step 2. start an instance for any machines we found.
	if err := p.startMachines(notstarted); err != nil {
		return err
	}

	// step 3. stop all machines that were removed from the state.
	stopping, err := p.instancesForMachines(dead)
	if err != nil {
		return err
	}

	// step 4. find instances which are running but have no machine 
	// associated with them.
	unknown, err := p.findUnknownInstances()
	if err != nil {
		return err
	}

	return p.stopInstances(append(stopping, unknown...))
}

// findUnknownInstances finds instances which are not associated with a machine.
func (p *Provisioner) findUnknownInstances() ([]environs.Instance, error) {
	all, err := p.environ.AllInstances()
	if err != nil {
		return nil, err
	}
	instances := make(map[string]environs.Instance)
	for _, i := range all {
		instances[i.Id()] = i
	}
	// TODO(dfc) this is very inefficient, p.machines cache may help.
	machines, err := p.st.AllMachines()
	if err != nil {
		return nil, err
	}
	for _, m := range machines {
		if m.Life() == state.Dead {
			continue
		}
		instId, err := m.InstanceId()
		if state.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		delete(instances, instId)
	}
	var unknown []environs.Instance
	for _, i := range instances {
		unknown = append(unknown, i)
	}
	return unknown, nil
}

// findNotStarted finds machines without an InstanceId set, these are defined as not started.
func (p *Provisioner) findNotStarted(alive []int) ([]*state.Machine, error) {
	var notstarted []*state.Machine
	// TODO(niemeyer): ms, err := st.Machines(alive)
	for _, id := range alive {
		m, err := p.st.Machine(id)
		if state.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		inst, err := m.InstanceId()
		if state.IsNotFound(err) {
			notstarted = append(notstarted, m)
			continue
		}
		if err != nil && !state.IsNotFound(err) {
			return nil, err
		}
		log.Printf("machine %s already started as instance %q", m, inst)
	}
	return notstarted, nil
}

func (p *Provisioner) startMachines(machines []*state.Machine) error {
	for _, m := range machines {
		if err := p.startMachine(m); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) startMachine(m *state.Machine) error {
	// TODO(dfc) the state.Info passed to environ.StartInstance remains contentious
	// however as the PA only knows one state.Info, and that info is used by MAs and 
	// UAs to locate the ZK for this environment, it is logical to use the same 
	// state.Info as the PA. 
	inst, err := p.environ.StartInstance(m.Id(), p.info, nil)
	if err != nil {
		log.Printf("provisioner cannot start machine %s: %v", m, err)
		return err
	}

	// assign the instance id to the machine
	if err := m.SetInstanceId(inst.Id()); err != nil {
		return err
	}

	// populate the local cache
	p.instances[m.Id()] = inst
	p.machines[inst.Id()] = m.Id()
	log.Printf("provisioner started machine %s as instance %s", m, inst.Id())
	return nil
}

func (p *Provisioner) stopInstances(instances []environs.Instance) error {
	// Although calling StopInstance with an empty slice should produce no change in the 
	// provider, environs like dummy do not consider this a noop.
	if len(instances) == 0 {
		return nil
	}
	if err := p.environ.StopInstances(instances); err != nil {
		return err
	}

	// cleanup cache
	for _, i := range instances {
		if id, ok := p.machines[i.Id()]; ok {
			delete(p.machines, i.Id())
			delete(p.instances, id)
		}
	}
	return nil
}

// instanceForMachine returns the environs.Instance that represents this machine's instance.
func (p *Provisioner) instanceForMachine(id int) (environs.Instance, error) {
	inst, ok := p.instances[id]
	if ok {
		return inst, nil
	}
	m, err := p.st.Machine(id)
	if err != nil {
		return nil, err
	}
	instId, err := m.InstanceId()
	if err != nil {
		return nil, err
	}
	// TODO(dfc): Ask for all instances at once.
	insts, err := p.environ.Instances([]string{instId})
	if err != nil {
		return nil, err
	}
	inst = insts[0]
	return inst, nil
}

// instancesForMachines returns a list of environs.Instance that represent
// the list of machines running in the provider. Missing machines are
// omitted from the list.
func (p *Provisioner) instancesForMachines(ids []int) ([]environs.Instance, error) {
	var insts []environs.Instance
	for _, id := range ids {
		inst, err := p.instanceForMachine(id)
		if state.IsNotFound(err) || err == environs.ErrNoInstances {
			continue
		}
		if err != nil {
			return nil, err
		}
		insts = append(insts, inst)
	}
	return insts, nil
}
