// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	tomb            tomb.Tomb
	st              *state.State
	environ         environs.Environ
	environWatcher  *state.EnvironConfigWatcher
	machinesWatcher *state.LifecycleWatcher
	machineds       map[string]*machineData
	unitsChange     chan *unitsChange
	unitds          map[string]*unitData
	portsChange     chan *portsChange
	serviceds       map[string]*serviceData
	exposedChange   chan *exposedChange
	globalMode      bool
	globalPortRef   map[instance.Port]int
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) *Firewaller {
	fw := &Firewaller{
		st:              st,
		environWatcher:  st.WatchEnvironConfig(),
		machinesWatcher: st.WatchEnvironMachines(),
		machineds:       make(map[string]*machineData),
		unitsChange:     make(chan *unitsChange),
		unitds:          make(map[string]*unitData),
		portsChange:     make(chan *portsChange),
		serviceds:       make(map[string]*serviceData),
		exposedChange:   make(chan *exposedChange),
	}
	go func() {
		defer fw.tomb.Done()
		fw.tomb.Kill(fw.loop())
	}()
	return fw
}

func (fw *Firewaller) loop() error {
	defer fw.stopWatchers()

	var err error
	var reconciled bool

	fw.environ, err = worker.WaitForEnviron(fw.environWatcher, fw.tomb.Dying())
	if err != nil {
		return err
	}
	if fw.environ.Config().FirewallMode() == config.FwGlobal {
		fw.globalMode = true
		fw.globalPortRef = make(map[instance.Port]int)
	}
	for {
		select {
		case <-fw.tomb.Dying():
			return tomb.ErrDying
		case change, ok := <-fw.environWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.environWatcher)
			}
			if err := fw.environ.SetConfig(change); err != nil {
				log.Errorf("worker/firewaller: loaded invalid environment configuration: %v", err)
			}
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.machinesWatcher)
			}
			for _, id := range change {
				fw.machineLifeChanged(id)
			}
			if !reconciled {
				reconciled = true
				var err error
				if fw.globalMode {
					err = fw.reconcileGlobal()
				} else {
					err = fw.reconcileInstances()
				}
				if err != nil {
					return err
				}
			}
		case change := <-fw.unitsChange:
			if err := fw.unitsChanged(change); err != nil {
				return err
			}
		case change := <-fw.portsChange:
			change.unitd.ports = change.ports
			if err := fw.flushUnits([]*unitData{change.unitd}); err != nil {
				return fmt.Errorf("cannot change firewall ports: %v", err)
			}
		case change := <-fw.exposedChange:
			change.serviced.exposed = change.exposed
			unitds := []*unitData{}
			for _, unitd := range change.serviced.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(unitds); err != nil {
				return fmt.Errorf("cannot change firewall ports: %v", err)
			}
		}
	}
	panic("not reached")
}

// stop a watcher with logging of a possible error.
func stop(what string, stopper watcher.Stopper) {
	if err := stopper.Stop(); err != nil {
		log.Errorf("worker/firewaller: error stopping %s: %v", what, err)
	}
}

// startMachine creates a new data value for tracking details of the
// machine and starts watching the machine for units added or removed.
func (fw *Firewaller) startMachine(id string) error {
	machined := &machineData{
		fw:     fw,
		id:     id,
		unitds: make(map[string]*unitData),
		ports:  make([]instance.Port, 0),
	}
	m, err := machined.machine()
	if errors.IsNotFoundError(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("worker/firewaller: cannot watch machine units: %v", err)
	}
	unitw := m.WatchUnits()
	select {
	case <-fw.tomb.Dying():
		stop("units watcher", unitw)
		return tomb.ErrDying
	case change, ok := <-unitw.Changes():
		if !ok {
			stop("units watcher", unitw)
			return watcher.MustErr(unitw)
		}
		fw.machineds[id] = machined
		err = fw.unitsChanged(&unitsChange{machined, change})
		if err != nil {
			stop("units watcher", unitw)
			return fmt.Errorf("worker/firewaller: cannot respond to units changes for machine %q: %v", id, err)
		}
	}
	go machined.watchLoop(unitw)
	return nil
}

// startUnit creates a new data value for tracking details of the
// unit and starts watching the unit for port changes. The provided
// machineId must be the id for the machine the unit was last observed
// to be assigned to.
func (fw *Firewaller) startUnit(unit *state.Unit, machineId string) error {
	service, err := unit.Service()
	if err != nil {
		return err
	}
	serviceName := service.Name()
	unitName := unit.Name()
	unitd := &unitData{
		fw:    fw,
		unit:  unit,
		ports: unit.OpenedPorts(),
	}
	fw.unitds[unitName] = unitd

	unitd.machined = fw.machineds[machineId]
	unitd.machined.unitds[unitName] = unitd
	if fw.serviceds[serviceName] == nil {
		err := fw.startService(service)
		if err != nil {
			delete(fw.unitds, unitName)
			return err
		}
	}
	unitd.serviced = fw.serviceds[serviceName]
	unitd.serviced.unitds[unitName] = unitd

	ports := make([]instance.Port, len(unitd.ports))
	copy(ports, unitd.ports)

	go unitd.watchLoop(ports)
	return nil
}

// startService creates a new data value for tracking details of the
// service and starts watching the service for exposure changes.
func (fw *Firewaller) startService(service *state.Service) error {
	serviced := &serviceData{
		fw:      fw,
		service: service,
		exposed: service.IsExposed(),
		unitds:  make(map[string]*unitData),
	}
	fw.serviceds[service.Name()] = serviced
	go serviced.watchLoop(serviced.exposed)
	return nil
}

// reconcileGlobal compares the initially started watcher for machines,
// units and services with the opened and closed ports globally and
// opens and closes the appropriate ports for the whole environment.
func (fw *Firewaller) reconcileGlobal() error {
	initialPorts, err := fw.environ.Ports()
	if err != nil {
		return err
	}
	collector := make(map[instance.Port]bool)
	for _, unitd := range fw.unitds {
		if unitd.serviced.exposed {
			for _, port := range unitd.ports {
				collector[port] = true
			}
		}
	}
	wantedPorts := []instance.Port{}
	for port := range collector {
		wantedPorts = append(wantedPorts, port)
	}
	// Check which ports to open or to close.
	toOpen := diff(wantedPorts, initialPorts)
	toClose := diff(initialPorts, wantedPorts)
	if len(toOpen) > 0 {
		log.Infof("worker/firewaller: opening global ports %v", toOpen)
		if err := fw.environ.OpenPorts(toOpen); err != nil {
			return err
		}
		state.SortPorts(toOpen)
	}
	if len(toClose) > 0 {
		log.Infof("worker/firewaller: closing global ports %v", toClose)
		if err := fw.environ.ClosePorts(toClose); err != nil {
			return err
		}
		state.SortPorts(toClose)
	}
	return nil
}

// reconcileInstances compares the initially started watcher for machines,
// units and services with the opened and closed ports of the instances and
// opens and closes the appropriate ports for each instance.
func (fw *Firewaller) reconcileInstances() error {
	for _, machined := range fw.machineds {
		m, err := machined.machine()
		if errors.IsNotFoundError(err) {
			if err := fw.forgetMachine(machined); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		instanceId, err := m.InstanceId()
		if err != nil {
			return err
		}
		instances, err := fw.environ.Instances([]instance.Id{instanceId})
		if err == environs.ErrNoInstances {
			return nil
		} else if err != nil {
			return err
		}
		initialPorts, err := instances[0].Ports(machined.id)
		if err != nil {
			return err
		}
		// Check which ports to open or to close.
		toOpen := diff(machined.ports, initialPorts)
		toClose := diff(initialPorts, machined.ports)
		if len(toOpen) > 0 {
			log.Infof("worker/firewaller: opening instance ports %v for machine %s",
				toOpen, machined.id)
			if err := instances[0].OpenPorts(machined.id, toOpen); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
			state.SortPorts(toOpen)
		}
		if len(toClose) > 0 {
			log.Infof("worker/firewaller: closing instance ports %v for machine %s",
				toClose, machined.id)
			if err := instances[0].ClosePorts(machined.id, toClose); err != nil {
				// TODO(mue) Add local retry logic.
				return err
			}
			state.SortPorts(toClose)
		}
	}
	return nil
}

// unitsChanged responds to changes to the assigned units.
func (fw *Firewaller) unitsChanged(change *unitsChange) error {
	changed := []*unitData{}
	for _, name := range change.units {
		unit, err := fw.st.Unit(name)
		if err != nil && !errors.IsNotFoundError(err) {
			return err
		}
		var machineId string
		if unit != nil {
			machineId, err = unit.AssignedMachineId()
			if errors.IsNotFoundError(err) {
				continue
			} else if err != nil && !state.IsNotAssigned(err) {
				return err
			}
		}
		if unitd, known := fw.unitds[name]; known {
			knownMachineId := fw.unitds[name].machined.id
			if unit == nil || unit.Life() == state.Dead || machineId != knownMachineId {
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
				log.Debugf("worker/firewaller: stopped watching unit %s", name)
			}
		} else if unit != nil && unit.Life() != state.Dead && fw.machineds[machineId] != nil {
			err = fw.startUnit(unit, machineId)
			if err != nil {
				return err
			}
			changed = append(changed, fw.unitds[name])
			log.Debugf("worker/firewaller: started watching unit %s", name)
		}
	}
	if err := fw.flushUnits(changed); err != nil {
		return fmt.Errorf("cannot change firewall ports: %v", err)
	}
	return nil
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(unitds []*unitData) error {
	machineds := map[string]*machineData{}
	for _, unitd := range unitds {
		machineds[unitd.machined.id] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMachine(machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMachine opens and closes ports for the passed machine.
func (fw *Firewaller) flushMachine(machined *machineData) error {
	// Gather ports to open and close.
	ports := map[instance.Port]bool{}
	for _, unitd := range machined.unitds {
		if unitd.serviced.exposed {
			for _, port := range unitd.ports {
				ports[port] = true
			}
		}
	}
	want := []instance.Port{}
	for port := range ports {
		want = append(want, port)
	}
	toOpen := diff(want, machined.ports)
	toClose := diff(machined.ports, want)
	machined.ports = want
	if fw.globalMode {
		return fw.flushGlobalPorts(toOpen, toClose)
	}
	return fw.flushInstancePorts(machined, toOpen, toClose)
}

// flushGlobalPorts opens and closes global ports in the environment.
// It keeps a reference count for ports so that only 0-to-1 and 1-to-0 events
// modify the environment.
func (fw *Firewaller) flushGlobalPorts(rawOpen, rawClose []instance.Port) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose []instance.Port
	for _, port := range rawOpen {
		if fw.globalPortRef[port] == 0 {
			toOpen = append(toOpen, port)
		}
		fw.globalPortRef[port]++
	}
	for _, port := range rawClose {
		fw.globalPortRef[port]--
		if fw.globalPortRef[port] == 0 {
			toClose = append(toClose, port)
			delete(fw.globalPortRef, port)
		}
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fw.environ.OpenPorts(toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toOpen)
		log.Infof("worker/firewaller: opened ports %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environ.ClosePorts(toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Infof("worker/firewaller: closed ports %v in environment", toClose)
	}
	return nil
}

// flushGlobalPorts opens and closes ports global on the machine.
func (fw *Firewaller) flushInstancePorts(machined *machineData, toOpen, toClose []instance.Port) error {
	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
	m, err := machined.machine()
	if errors.IsNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	instanceId, err := m.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]instance.Id{instanceId})
	if err != nil {
		return err
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := instances[0].OpenPorts(machined.id, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toOpen)
		log.Infof("worker/firewaller: opened ports %v on machine %s", toOpen, machined.id)
	}
	if len(toClose) > 0 {
		if err := instances[0].ClosePorts(machined.id, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Infof("worker/firewaller: closed ports %v on machine %s", toClose, machined.id)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *Firewaller) machineLifeChanged(id string) error {
	m, err := fw.st.Machine(id)
	found := !errors.IsNotFoundError(err)
	if found && err != nil {
		return err
	}
	dead := !found || m.Life() == state.Dead
	machined, known := fw.machineds[id]
	if known && dead {
		return fw.forgetMachine(machined)
	}
	if !known && !dead {
		err = fw.startMachine(id)
		if err != nil {
			return err
		}
		log.Debugf("worker/firewaller: started watching machine %s", id)
	}
	return nil
}

// forgetMachine cleans the machine data after the machine is removed.
func (fw *Firewaller) forgetMachine(machined *machineData) error {
	for _, unitd := range machined.unitds {
		fw.forgetUnit(unitd)
	}
	if err := fw.flushMachine(machined); err != nil {
		return err
	}
	delete(fw.machineds, machined.id)
	if err := machined.Stop(); err != nil {
		return err
	}
	log.Debugf("worker/firewaller: stopped watching machine %s", machined.id)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *Firewaller) forgetUnit(unitd *unitData) {
	name := unitd.unit.Name()
	serviced := unitd.serviced
	machined := unitd.machined
	if err := unitd.Stop(); err != nil {
		log.Errorf("worker/firewaller: unit watcher %q returned error when stopping: %v", name, err)
	}
	// Clean up after stopping.
	delete(fw.unitds, name)
	delete(machined.unitds, name)
	delete(serviced.unitds, name)
	if len(serviced.unitds) == 0 {
		// Stop service data after all units are removed.
		if err := serviced.Stop(); err != nil {
			log.Errorf("worker/firewaller: service watcher %q returned error when stopping: %v", serviced.service, err)
		}
		delete(fw.serviceds, serviced.service.Name())
	}
}

// stopWatchers stops all the firewaller's watchers.
func (fw *Firewaller) stopWatchers() {
	watcher.Stop(fw.environWatcher, &fw.tomb)
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitds {
		watcher.Stop(unitd, &fw.tomb)
	}
	for _, serviced := range fw.serviceds {
		watcher.Stop(serviced, &fw.tomb)
	}
	for _, machined := range fw.machineds {
		watcher.Stop(machined, &fw.tomb)
	}
}

func (fw *Firewaller) String() string {
	return "firewaller"
}

// Err returns the reason why the firewaller has stopped or tomb.ErrStillAlive
// when it is still alive.
func (fw *Firewaller) Err() (reason error) {
	return fw.tomb.Err()
}

// Kill implements worker.Worker.Kill.
func (fw *Firewaller) Kill() {
	fw.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (fw *Firewaller) Wait() error {
	return fw.tomb.Wait()
}

// Stop stops the Firewaller and returns any error encountered while stopping.
func (fw *Firewaller) Stop() error {
	fw.tomb.Kill(nil)
	return fw.tomb.Wait()
}

// unitsChange contains the changed units for one specific machine.
type unitsChange struct {
	machined *machineData
	units    []string
}

// machineData holds machine details and watches units added or removed.
type machineData struct {
	tomb   tomb.Tomb
	fw     *Firewaller
	id     string
	unitds map[string]*unitData
	ports  []instance.Port
}

func (md *machineData) machine() (*state.Machine, error) {
	return md.fw.st.Machine(md.id)
}

// watchLoop watches the machine for units added or removed.
func (md *machineData) watchLoop(unitw *state.MachineUnitsWatcher) {
	defer md.tomb.Done()
	defer watcher.Stop(unitw, &md.tomb)
	for {
		select {
		case <-md.tomb.Dying():
			return
		case change, ok := <-unitw.Changes():
			if !ok {
				_, err := md.machine()
				if !errors.IsNotFoundError(err) {
					md.fw.tomb.Kill(watcher.MustErr(unitw))
				}
				return
			}
			select {
			case md.fw.unitsChange <- &unitsChange{md, change}:
			case <-md.tomb.Dying():
				return
			}
		}
	}
}

// stopWatch stops the machine watching.
func (md *machineData) Stop() error {
	md.tomb.Kill(nil)
	return md.tomb.Wait()
}

// portsChange contains the changed ports for one specific unit.
type portsChange struct {
	unitd *unitData
	ports []instance.Port
}

// unitData holds unit details and watches port changes.
type unitData struct {
	tomb     tomb.Tomb
	fw       *Firewaller
	unit     *state.Unit
	serviced *serviceData
	machined *machineData
	ports    []instance.Port
}

// watchLoop watches the unit for port changes.
func (ud *unitData) watchLoop(latestPorts []instance.Port) {
	defer ud.tomb.Done()
	w := ud.unit.Watch()
	defer watcher.Stop(w, &ud.tomb)
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				ud.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := ud.unit.Refresh(); err != nil {
				if !errors.IsNotFoundError(err) {
					ud.fw.tomb.Kill(err)
				}
				return
			}
			change := ud.unit.OpenedPorts()
			if samePorts(change, latestPorts) {
				continue
			}
			latestPorts = append(latestPorts[:0], change...)
			select {
			case ud.fw.portsChange <- &portsChange{ud, change}:
			case <-ud.tomb.Dying():
				return
			}
		}
	}
}

// samePorts returns whether old and new contain the same set of ports.
// Both old and new must be sorted.
func samePorts(old, new []instance.Port) bool {
	if len(old) != len(new) {
		return false
	}
	for i, p := range old {
		if new[i] != p {
			return false
		}
	}
	return true
}

// Stop stops the unit watching.
func (ud *unitData) Stop() error {
	ud.tomb.Kill(nil)
	return ud.tomb.Wait()
}

// exposedChange contains the changed exposed flag for one specific service.
type exposedChange struct {
	serviced *serviceData
	exposed  bool
}

// serviceData holds service details and watches exposure changes.
type serviceData struct {
	tomb    tomb.Tomb
	fw      *Firewaller
	service *state.Service
	exposed bool
	unitds  map[string]*unitData
}

// watchLoop watches the service's exposed flag for changes.
func (sd *serviceData) watchLoop(exposed bool) {
	defer sd.tomb.Done()
	w := sd.service.Watch()
	defer watcher.Stop(w, &sd.tomb)
	for {
		select {
		case <-sd.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				sd.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := sd.service.Refresh(); err != nil {
				if !errors.IsNotFoundError(err) {
					sd.fw.tomb.Kill(err)
				}
				return
			}
			change := sd.service.IsExposed()
			if change == exposed {
				continue
			}
			exposed = change
			select {
			case sd.fw.exposedChange <- &exposedChange{sd, change}:
			case <-sd.tomb.Dying():
				return
			}
		}
	}
}

// Stop stops the service watching.
func (sd *serviceData) Stop() error {
	sd.tomb.Kill(nil)
	return sd.tomb.Wait()
}

// diff returns all the ports that exist in A but not B.
func diff(A, B []instance.Port) (missing []instance.Port) {
next:
	for _, a := range A {
		for _, b := range B {
			if a == b {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}
