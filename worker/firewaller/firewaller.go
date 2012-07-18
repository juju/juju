package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller manages the opening and closing of ports.
type Firewaller struct {
	st       *state.State
	info     *state.Info
	environ  environs.Environ
	tomb     tomb.Tomb
	machines map[int]*machineTracker
	units    map[string]*unitTracker
	services map[string]*serviceTracker
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(environ environs.Environ) (*Firewaller, error) {
	info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	fw := &Firewaller{
		st:       st,
		environ:  environ,
		machines: make(map[int]*machineTracker),
		units:    make(map[string]*unitTracker),
		services: make(map[string]*serviceTracker),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.finish()
	// Set up channels and watchers.
	machineUnitsChanges := make(chan *machineUnitsChange)
	defer close(machineUnitsChanges)
	unitPortsChanges := make(chan *unitPortsChange)
	defer close(unitPortsChanges)
	machinesWatcher := fw.st.WatchMachines()
	defer watcher.Stop(machinesWatcher, &fw.tomb)
	// Receive and handle changes.
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-machinesWatcher.Changes():
			if !ok {
				return
			}
			for _, removedMachine := range change.Removed {
				mt, ok := fw.machines[removedMachine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				if err := mt.stop(); err != nil {
					log.Printf("can't stop tracker of machine %d: %v", mt.id, err)
					continue
				}
				delete(fw.machines, removedMachine.Id())
				log.Debugf("removed machine %v", removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				mt := newMachineTracker(addedMachine, fw, machineUnitsChanges)
				fw.machines[addedMachine.Id()] = mt
				log.Debugf("added machine %v", mt.id)
			}
		case change, ok := <-machineUnitsChanges:
			if !ok {
				panic("aggregation of machine units changes failed")
			}
			if change.change == nil {
				// TODO(mue) Can we live with a dying machine units watcher?
				log.Printf("watching machine %d raised an error", change.machine.id)
				delete(fw.machines, change.machine.id)
				continue
			}
			for _, removedUnit := range change.change.Removed {
				ut, ok := fw.units[removedUnit.Name()]
				if !ok {
					panic("trying to remove unit that wasn't added")
				}
				delete(fw.units, removedUnit.Name())
				if err := ut.stop(); err != nil {
					log.Debugf("can't stop tracker of unit %q: %v", ut.name, err)
				}
				log.Debugf("removed unit %v", removedUnit.Name())
			}
			for _, addedUnit := range change.change.Added {
				ut := newUnitTracker(addedUnit, fw, unitPortsChanges)
				fw.units[addedUnit.Name()] = ut
				if fw.services[addedUnit.ServiceName()] == nil {
					// TODO(mue) Add service watcher.
				}
				log.Debugf("added unit %v", ut.name)
			}
		case <-unitPortsChanges:
			// TODO(mue) Handle changes of ports.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	for _, m := range fw.machines {
		fw.tomb.Kill(m.stop())
	}
	fw.st.Close()
	fw.tomb.Done()
}

// Wait waits for the Firewaller to exit.
func (fw *Firewaller) Wait() error {
	return fw.tomb.Wait()
}

// Stop stops the Firewaller and returns any error encountered while stopping.
func (fw *Firewaller) Stop() error {
	fw.tomb.Kill(nil)
	return fw.tomb.Wait()
}

// machineUnitsChange contains the changed units for one specific machine. 
type machineUnitsChange struct {
	machine *machineTracker
	change  *state.MachineUnitsChange
}

// machineTracker keeps track of the unit changes of a machine.
type machineTracker struct {
	firewaller *Firewaller
	changes    chan<- *machineUnitsChange
	tomb       tomb.Tomb
	id         int
	watcher    *state.MachineUnitsWatcher
	ports      map[state.Port]*unitTracker
}

// newMachineTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newMachineTracker(mst *state.Machine, fw *Firewaller, changes chan<- *machineUnitsChange) *machineTracker {
	mt := &machineTracker{
		firewaller: fw,
		changes:    changes,
		id:         mst.Id(),
		watcher:    mst.WatchUnits(),
		ports:      make(map[state.Port]*unitTracker),
	}
	go mt.loop()
	return mt
}

// loop is the backend watching for machine units changes.
func (mt *machineTracker) loop() {
	defer mt.tomb.Done()
	defer mt.watcher.Stop()
	for {
		select {
		case <-mt.firewaller.tomb.Dying():
			return
		case <-mt.tomb.Dying():
			return
		case change, ok := <-mt.watcher.Changes():
			// Send change or nil in case of an error.
			select {
			case mt.changes <- &machineUnitsChange{mt, change}:
			case <-mt.firewaller.tomb.Dying():
				return
			case <-mt.tomb.Dying():
				return
			}
			// Had been an error, so end the loop.
			if !ok {
				mt.firewaller.tomb.Kill(watcher.MustErr(mt.watcher))
				return
			}
		}
	}
}

// stop stops the machine tracker.
func (mt *machineTracker) stop() error {
	mt.tomb.Kill(nil)
	return mt.tomb.Wait()
}

// unitPortsChange contains the changed ports for one specific unit. 
type unitPortsChange struct {
	unit   *unitTracker
	change []state.Port
}

// unitTracker keeps track of the port changes of a unit.
type unitTracker struct {
	firewaller *Firewaller
	changes    chan<- *unitPortsChange
	tomb       tomb.Tomb
	name       string
	watcher    *state.PortsWatcher
	service    *serviceTracker
	ports      []state.Port
}

// newUnitTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newUnitTracker(ust *state.Unit, fw *Firewaller, changes chan<- *unitPortsChange) *unitTracker {
	ut := &unitTracker{
		firewaller: fw,
		changes:    changes,
		name:       ust.Name(),
		watcher:    ust.WatchPorts(),
		ports:      make([]state.Port, 0),
	}
	go ut.loop()
	return ut
}

func (ut *unitTracker) loop() {
	defer ut.tomb.Done()
	defer ut.watcher.Stop()
	for {
		select {
		case <-ut.firewaller.tomb.Dying():
			return
		case change, ok := <-ut.watcher.Changes():
			// Send change or nil in case of an error.
			select {
			case ut.changes <- &unitPortsChange{ut, change}:
			case <-ut.firewaller.tomb.Dying():
				return
			case <-ut.tomb.Dying():
				return
			}
			// The watcher terminated prematurely, so end the loop.
			if !ok {
				ut.firewaller.tomb.Kill(watcher.MustErr(ut.watcher))
				return
			}
		}
	}
}

// stop stops the unit tracker.
func (ut *unitTracker) stop() error {
	ut.tomb.Kill(nil)
	return ut.tomb.Wait()
}

// serviceTracker  keeps track of the changes of a service.
type serviceTracker struct {
	name    string
	exposed bool
}
