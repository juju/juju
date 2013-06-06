// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

type Watcher interface {
	Changes() <-chan []string
}

func newProvisionerTask(
	machineId string,
	watcher Watcher,
	broker Broker,
	stateInfo *state.Info,
	apiInfo *api.Info,
) worker.Worker {
	task = &provisionerTask{
		machineId: machineId,
		watcher:   watcher,
		broker:    broker,
		stateInfo: stateInfo,
		apiInfo:   apiInfo,
	}
	go func() {
		defer task.tomb.Done()
		task.tomb.Kill(task.loop())
	}()
	return task
}

type provisionerTask struct {
	machineId      string
	machineWatcher Watcher
	broker         Broker
	tomb           tomb.Tomb
	stateInfo      *state.Info
	apiInfo        *api.Info

	// instance id -> instance
	instances map[state.InstanceId]environs.Instance
	// machine id -> machine
	machines map[string]*state.Machine
}

// Kill implements worker.Worker.Kill.
func (task *provisionerTask) Kill() {
	task.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (task *provisionerTask) Wait() error {
	return task.tomb.Wait()
}

func (task *provisionerTask) loop() error {
	logger.Info("Starting up provisioner task %s", task.machineId)
	defer watcher.Stop(task.machineWatcher, &task.tomb)

	// Call processMachines to stop any unknown instances before watching machines.
	if err := task.processMachines(nil); err != nil {
		logger.Error("Error clearing unknown instances before watching new changes: %v", err)
		return err
	}

	for {
		select {
		case <-task.tomb.Dying():
			logger.Info("Shutting down provisioner task %s", task.machineId)
			return tomb.ErrDying
		case ids, ok := <-this.machineWatcher.Changes():
			if !ok {
				return watcher.MustErr(this.machineWatcher)
			}
			// TODO(dfc; lp:1042717) fire process machines periodically to shut down unknown
			// instances.
			if err := task.processMachines(ids); err != nil {
				return err
			}
		}
	}
	panic("not reached")
}

func (task *provisionerTask) processMachines(ids []string) error {

	// Populate the tasks maps of current instances and machines.
	err := task.populateMachineMaps()
	if err != nil {
		return err
	}

	// Find machines without an instance id or that are dead
	pending, dead, err := task.pendingOrDead(ids)
	if err != nil {
		return err
	}

	// Find running instances that have no machines associated
	unknown, err := task.findUnknownInstances()
	if err != nil {
		return err
	}

	// Stop all machines that are dead
	stopping, err := task.instancesForMachines(dead)
	if err != nil {
		return err
	}

	// It's important that we stop unknown instances before starting
	// pending ones, because if we start an instance and then fail to
	// set its InstanceId on the machine we don't want to start a new
	// instance for the same machine ID.
	if err := task.stopInstances(append(stopping, unknown...)); err != nil {
		return err
	}

	// Start an instance for the pending ones
	return task.startMachines(pending)
}

func (task *provisionerTask) populateMachineMaps() error {
	task.instances = make(map[state.InstanceId]environs.Instance)
	task.machines = make(map[string]*state.Machine)

	instances, err := task.broker.AllInstances()
	if err != nil {
		logger.Error("failed to get all instances from broker: %v", err)
		return nil, err
	}
	for _, i := range instances {
		task.instances[i.Id()] = i
	}

	machines, err := task.broker.AllMachines()
	if err != nil {
		logger.Error("failed to get all machines from broker: %v", err)
		return nil, err
	}
	for _, m := range machines {
		task.machines[m.Id()] = m
	}
}

// pendingOrDead looks up machines with ids and retuns those that do not
// have an instance id assigned yet, and also those that are dead.
func (task *provisionerTask) pendingOrDead(ids []string) (pending, dead []*state.Machine, err error) {
	// TODO(niemeyer): ms, err := st.Machines(alive)
	for _, id := range ids {
		machine, found := task.machines[id]
		if !found {
			logger.Info("machine %q not found", id)
			continue
		}
		switch machine.Life() {
		case state.Dying:
			if _, ok := machine.InstanceId(); ok {
				continue
			}
			logger.Info("killing dying, unprovisioned machine %q", machine)
			if err := machine.EnsureDead(); err != nil {
				logger.Error("failed to ensure machine dead %q: %v", machine, err)
				return nil, nil, err
			}
			fallthrough
		case state.Dead:
			dead = append(dead, machine)
			logger.Info("removing dead machine %q", machine)
			if err := machine.Remove(); err != nil {
				logger.Error("failed to remove dead machine %q", machine)
				return nil, nil, err
			}
			continue
		}
		if instId, hasInstId := machine.InstanceId(); !hasInstId {
			status, _, err := machine.Status()
			if err != nil {
				logger.Info("cannot get machine %q status: %v", machine, err)
				continue
			}
			if status == params.StatusPending {
				pending = append(pending, machine)
				logger.Info("found machine %q pending provisioning", machine)
				continue
			}
		} else {
			logger.Info("machine %v already started as instance %q", machine, instId)
		}
	}
	return
}

// findUnknownInstances finds instances which are not associated with a machine.
func (task *provisionerTask) findUnknownInstances() ([]environs.Instance, error) {
	// Make a copy of the instances we know about.
	instances = make(map[state.InstanceId]environs.Instance)
	for k, v := range task.instances {
		instances[k] = v
	}

	for _, m := range task.machines {
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

// instancesForMachines returns a list of environs.Instance that represent
// the list of machines running in the provider. Missing machines are
// omitted from the list.
func (task *provisionerTask) instancesForMachines(machines []*state.Machine) []environs.Instance {
	var instances []environs.Instance
	for _, machine := range machines {
		instId, ok := machine.InstanceId()
		if ok {
			instance, found := task.instances[instId]
			// If the instance is not found, it means that the underlying
			// instance is already dead, and we don't need to stop it.
			if found {
				instances := append(instances, instance)
			}
		}
	}
	return instances, nil
}

func (task *provisionerTask) stopInstances(instances []environs.Instance) error {
	// Although calling StopInstance with an empty slice should produce no change in the
	// provider, environs like dummy do not consider this a noop.
	if len(instances) == 0 {
		return nil
	}
	if err := task.broker.StopInstances(instances); err != nil {
		logger.Error("broker failed to stop instances: %v", err)
		return err
	}

	return nil
}

func (task *provisionerTask) startMachines(machines []*state.Machine) error {
	for _, m := range machines {
		if err := task.startMachine(m); err != nil {
			return fmt.Errorf("cannot start machine %v: %v", m, err)
		}
	}
	return nil
}

func (task *provisionerTask) startMachine(machine *state.Machine) error {
	// TODO(dfc) the state.Info passed to environ.StartInstance remains contentious
	// however as the PA only knows one state.Info, and that info is used by MAs and
	// UAs to locate the state for this environment, it is logical to use the same
	// state.Info as the PA.
	stateInfo, apiInfo, err := task.setupAuthentication(machine)
	if err != nil {
		logger.error("failed to setup authentication: %v", err)
		return err
	}
	cons, err := machine.Constraints()
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
	nonce := fmt.Sprintf("%s:%s", state.MachineTag(task.machineId), uuid.String())
	inst, err := task.broker.StartInstance(machine.Id(), nonce, m.Series(), cons, stateInfo, apiInfo)
	if err != nil {
		// Set the state to error, so the machine will be skipped next
		// time until the error is resolved, but don't return an
		// error; just keep going with the other machines.
		logger.Error("cannot start instance for machine %q: %v", machine, err)
		if err1 := machine.SetStatus(params.StatusError, err.Error()); err1 != nil {
			// Something is wrong with this machine, better report it back.
			logger.Error("cannot set error status for machine %q: %v", machine, err1)
			return err1
		}
		return nil
	}
	if err := machine.SetProvisioned(inst.Id(), nonce); err != nil {
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
	logger.Info("started machine %s as instance %s", machine, inst.Id())
	return nil
}

func (task *provisionerTask) setupAuthentication(machine *state.Machine) (*state.Info, *api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetMongoPassword(password); err != nil {
		return nil, nil, fmt.Errorf("cannot set password for machine %v: %v", machine, err)
	}
	stateInfo := *task.stateInfo
	stateInfo.Tag = machine.Tag()
	stateInfo.Password = password
	apiInfo := *task.apiInfo
	apiInfo.Tag = machine.Tag()
	apiInfo.Password = password
	return &stateInfo, &apiInfo, nil
}
