// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	stderrors "errors"
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
	"sync"
)

// Provisioner represents a running provisioning worker.
type Provisioner struct {
	st        *state.State
	machineId string // Which machine runs the provisioner.
	stateInfo *state.Info
	apiInfo   *api.Info
	environ   environs.Environ
	tomb      tomb.Tomb

	// machine.Id => environs.Instance
	instances map[string]environs.Instance
	// instance.Id => machine id
	machines map[state.InstanceId]string

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
		instances: make(map[string]environs.Instance),
		machines:  make(map[state.InstanceId]string),
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
	if p.stateInfo, p.apiInfo, err = p.environ.StateInfo(); err != nil {
		return err
	}

	// Call processMachines to stop any unknown instances before watching machines.
	if err := p.processMachines(nil); err != nil {
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
			if err := p.setConfig(cfg); err != nil {
				log.Errorf("worker/provisioner: loaded invalid environment configuration: %v", err)
			}
		case ids, ok := <-machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(machinesWatcher)
			}
			// TODO(dfc; lp:1042717) fire process machines periodically to shut down unknown
			// instances.
			if err := p.processMachines(ids); err != nil {
				return err
			}
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

func (p *Provisioner) processMachines(ids []string) error {
	// Find machines without an instance id or that are dead
	pending, dead, err := p.pendingOrDead(ids)
	if err != nil {
		return err
	}

	// Find running instances that have no machines associated
	unknown, err := p.findUnknownInstances()
	if err != nil {
		return err
	}

	// Stop all machines that are dead
	stopping, err := p.instancesForMachines(dead)
	if err != nil {
		return err
	}

	// It's important that we stop unknown instances before starting
	// pending ones, because if we start an instance and then fail to
	// set its InstanceId on the machine we don't want to start a new
	// instance for the same machine ID.
	if err := p.stopInstances(append(stopping, unknown...)); err != nil {
		return err
	}

	// Start an instance for the pending ones
	return p.startMachines(pending)
}

// findUnknownInstances finds instances which are not associated with a machine.
func (p *Provisioner) findUnknownInstances() ([]environs.Instance, error) {
	all, err := p.environ.AllInstances()
	if err != nil {
		return nil, err
	}
	instances := make(map[state.InstanceId]environs.Instance)
	for _, i := range all {
		instances[i.Id()] = i
	}
	// TODO(dfc) this is very inefficient.
	machines, err := p.st.AllMachines()
	if err != nil {
		return nil, err
	}
	for _, m := range machines {
		if instId, ok := m.InstanceId(); ok {
			delete(instances, instId)
		}
	}
	var unknown []environs.Instance
	for _, i := range instances {
		unknown = append(unknown, i)
	}
	return unknown, nil
}

// pendingOrDead looks up machines with ids and returns those that do not
// have an instance id assigned yet, and also those that are dead.
func (p *Provisioner) pendingOrDead(ids []string) (pending, dead []*state.Machine, err error) {
	// TODO(niemeyer): ms, err := st.Machines(alive)
	for _, id := range ids {
		m, err := p.st.Machine(id)
		if errors.IsNotFoundError(err) {
			log.Infof("worker/provisioner: machine %q not found in state", m)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		// For now, we ignore containers since we don't have a means to create them yet.
		if m.ContainerType() != "" {
			log.Infof("worker/provisioner: machine %q is a container which is not yet supported", m)
			continue
		}
		switch m.Life() {
		case state.Dying:
			if _, ok := m.InstanceId(); ok {
				continue
			}
			log.Infof("worker/provisioner: killing dying, unprovisioned machine %q", m)
			if err := m.EnsureDead(); err != nil {
				return nil, nil, err
			}
			fallthrough
		case state.Dead:
			dead = append(dead, m)
			log.Infof("worker/provisioner: removing dead machine %q", m)
			if err := m.Remove(); err != nil {
				return nil, nil, err
			}
			continue
		}
		if instId, hasInstId := m.InstanceId(); !hasInstId {
			status, _, err := m.Status()
			if err != nil {
				log.Infof("worker/provisioner: cannot get machine %q status: %v", m, err)
				continue
			}
			if status == params.StatusPending {
				pending = append(pending, m)
				log.Infof("worker/provisioner: found machine %q pending provisioning", m)
				continue
			}
		} else {
			log.Infof("worker/provisioner: machine %v already started as instance %q", m, instId)
		}
	}
	return
}

func (p *Provisioner) startMachines(machines []*state.Machine) error {
	for _, m := range machines {
		if err := p.startMachine(m); err != nil {
			return fmt.Errorf("cannot start machine %v: %v", m, err)
		}
	}
	return nil
}

func (p *Provisioner) startMachine(m *state.Machine) error {
	// TODO(dfc) the state.Info passed to environ.StartInstance remains contentious
	// however as the PA only knows one state.Info, and that info is used by MAs and
	// UAs to locate the state for this environment, it is logical to use the same
	// state.Info as the PA.
	stateInfo, apiInfo, err := p.setupAuthentication(m)
	if err != nil {
		return err
	}
	cons, err := m.Constraints()
	if err != nil {
		return err
	}
	// Generate a unique nonce for the new instance.
	uuid, err := utils.NewUUID()
	if err != nil {
		return err
	}
	// Generated nonce has the format: "machine-#:UUID". The first
	// part is a badge, specifying the tag of the machine the provisioner
	// is running on, while the second part is a random UUID.
	nonce := fmt.Sprintf("%s:%s", state.MachineTag(p.machineId), uuid.String())
	inst, err := p.environ.StartInstance(m.Id(), nonce, m.Series(), cons, stateInfo, apiInfo)
	if err != nil {
		// Set the state to error, so the machine will be skipped next
		// time until the error is resolved, but don't return an
		// error; just keep going with the other machines.
		log.Errorf("worker/provisioner: cannot start instance for machine %q: %v", m, err)
		if err1 := m.SetStatus(params.StatusError, err.Error()); err1 != nil {
			// Something is wrong with this machine, better report it back.
			log.Errorf("worker/provisioner: cannot set error status for machine %q: %v", m, err1)
			return err1
		}
		return nil
	}
	if err := m.SetProvisioned(inst.Id(), nonce); err != nil {
		// The machine is started, but we can't record the mapping in
		// state. It'll keep running while we fail out and restart,
		// but will then be detected by findUnknownInstances and
		// killed again.
		//
		// TODO(dimitern) Stop the instance right away here.
		//
		// Multiple instantiations of a given machine (with the same
		// machine ID) cannot coexist, because findUnknownInstances is
		// called before startMachines. However, if the first machine
		// had started to do work before being replaced, we may
		// encounter surprising problems.
		return err
	}
	// populate the local cache
	p.instances[m.Id()] = inst
	p.machines[inst.Id()] = m.Id()
	log.Noticef("worker/provisioner: started machine %s as instance %s", m, inst.Id())
	return nil
}

func (p *Provisioner) setupAuthentication(m *state.Machine) (*state.Info, *api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make password for machine %v: %v", m, err)
	}
	if err := m.SetMongoPassword(password); err != nil {
		return nil, nil, fmt.Errorf("cannot set password for machine %v: %v", m, err)
	}
	stateInfo := *p.stateInfo
	stateInfo.Tag = m.Tag()
	stateInfo.Password = password
	apiInfo := *p.apiInfo
	apiInfo.Tag = m.Tag()
	apiInfo.Password = password
	return &stateInfo, &apiInfo, nil
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

var errNotProvisioned = stderrors.New("machine has no instance id set")

// instanceForMachine returns the environs.Instance that represents this machine's instance.
func (p *Provisioner) instanceForMachine(m *state.Machine) (environs.Instance, error) {
	inst, ok := p.instances[m.Id()]
	if ok {
		return inst, nil
	}
	instId, ok := m.InstanceId()
	if !ok {
		return nil, errNotProvisioned
	}
	// TODO(dfc): Ask for all instances at once.
	insts, err := p.environ.Instances([]state.InstanceId{instId})
	if err != nil {
		return nil, err
	}
	inst = insts[0]
	return inst, nil
}

// instancesForMachines returns a list of environs.Instance that represent
// the list of machines running in the provider. Missing machines are
// omitted from the list.
func (p *Provisioner) instancesForMachines(ms []*state.Machine) ([]environs.Instance, error) {
	var insts []environs.Instance
	for _, m := range ms {
		switch inst, err := p.instanceForMachine(m); err {
		case nil:
			insts = append(insts, inst)
		case errNotProvisioned, environs.ErrNoInstances:
		default:
			return nil, err
		}
	}
	return insts, nil
}
